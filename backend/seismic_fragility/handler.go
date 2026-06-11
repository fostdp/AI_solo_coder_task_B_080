package seismic_fragility

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"aqueduct-monitor/models"
)

type IDAJob struct {
	ID        string
	Status    string
	SegmentIDs []uuid.UUID
	Results   []models.SeismicVulnerability
	Error     string
}

type FragilityHandler struct {
	service    *FragilityService
	idaJobs    map[string]*IDAJob
	idaJobsMu  sync.RWMutex
}

func NewFragilityHandler(service *FragilityService) *FragilityHandler {
	return &FragilityHandler{
		service: service,
		idaJobs: make(map[string]*IDAJob),
	}
}

func (h *FragilityHandler) RegisterRoutes(r *gin.RouterGroup) {
	seismicGroup := r.Group("/seismic")
	{
		seismicGroup.GET("/earthquakes/historical", h.GetHistoricalEarthquakes)
		seismicGroup.GET("/risks", h.GetAllSeismicRisks)
		seismicGroup.POST("/analyze/:aqueduct_id", h.AnalyzeSeismicRisk)
		seismicGroup.POST("/ida/:segment_id", h.RunIDA)
		seismicGroup.POST("/ida/batch", h.RunBatchIDA)
		seismicGroup.GET("/ida/jobs/:job_id", h.GetIDAJobStatus)
		seismicGroup.GET("/fragility-curves/:segment_id", h.GetFragilityCurves)
		seismicGroup.GET("/vulnerability/:segment_id", h.GetSegmentVulnerability)
	}
}

func (h *FragilityHandler) GetHistoricalEarthquakes(c *gin.Context) {
	ctx := context.Background()
	list, err := h.service.GetAllHistoricalEarthquakes(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (h *FragilityHandler) GetAllSeismicRisks(c *gin.Context) {
	ctx := context.Background()
	list, err := h.service.GetAllSeismicRisks(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (h *FragilityHandler) AnalyzeSeismicRisk(c *gin.Context) {
	idStr := c.Param("aqueduct_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid aqueduct id"})
		return
	}
	ctx := context.Background()
	result, err := h.service.AnalyzeAqueductSeismicRisk(ctx, id)
	if err != nil {
		log.Printf("Seismic analysis ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	if save := c.Query("save"); save == "true" {
		_ = h.service.SaveRiskResult(ctx, result)
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (h *FragilityHandler) RunIDA(c *gin.Context) {
	idStr := c.Param("segment_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}
	ctx := context.Background()
	results, err := h.service.AnalyzeIncrementalDynamic(ctx, id)
	if err != nil {
		log.Printf("IDA ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: results, Count: len(results)})
}

func (h *FragilityHandler) RunBatchIDA(c *gin.Context) {
	var req struct {
		SegmentIDs []string `json:"segment_ids" binding:"required"`
		Async      bool     `json:"async"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}
	segmentIDs := make([]uuid.UUID, 0, len(req.SegmentIDs))
	for _, sid := range req.SegmentIDs {
		if id, e := uuid.Parse(sid); e == nil {
			segmentIDs = append(segmentIDs, id)
		}
	}
	if len(segmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no valid segment ids"})
		return
	}

	if req.Async {
		jobID := uuid.New().String()
		job := &IDAJob{
			ID:         jobID,
			Status:     "running",
			SegmentIDs: segmentIDs,
		}
		h.idaJobsMu.Lock()
		h.idaJobs[jobID] = job
		h.idaJobsMu.Unlock()

		go func() {
			ctx := context.Background()
			results, err := h.service.AnalyzeBatchIDA(ctx, segmentIDs)
			h.idaJobsMu.Lock()
			defer h.idaJobsMu.Unlock()
			if err != nil {
				job.Status = "failed"
				job.Error = err.Error()
			} else {
				job.Status = "completed"
				job.Results = results
			}
		}()

		c.JSON(http.StatusAccepted, gin.H{
			"job_id":  jobID,
			"status":  "running",
			"message": "IDA analysis started",
		})
		return
	}

	ctx := context.Background()
	results, err := h.service.AnalyzeBatchIDA(ctx, segmentIDs)
	if err != nil {
		log.Printf("Batch IDA ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: results, Count: len(results)})
}

func (h *FragilityHandler) GetIDAJobStatus(c *gin.Context) {
	jobID := c.Param("job_id")
	h.idaJobsMu.RLock()
	job, exists := h.idaJobs[jobID]
	h.idaJobsMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "job not found"})
		return
	}

	response := gin.H{
		"job_id": job.ID,
		"status": job.Status,
	}
	if job.Error != "" {
		response["error"] = job.Error
	}
	if job.Status == "completed" {
		response["count"] = len(job.Results)
		response["data"] = job.Results
	}

	c.JSON(http.StatusOK, response)
}

func (h *FragilityHandler) GetFragilityCurves(c *gin.Context) {
	idStr := c.Param("segment_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}
	ctx := context.Background()
	curves, err := h.service.GenerateFragilityCurves(ctx, id)
	if err != nil {
		log.Printf("Fragility curve ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: curves, Count: len(curves)})
}

func (h *FragilityHandler) GetSegmentVulnerability(c *gin.Context) {
	idStr := c.Param("segment_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}
	pgaStr := c.Query("pga")
	pga := 0.3
	if pgaStr != "" {
		if v, e := strconv.ParseFloat(pgaStr, 64); e == nil {
			pga = v
		}
	}

	ctx := context.Background()
	results, err := h.service.AnalyzeIncrementalDynamic(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	matched := make([]models.SeismicVulnerability, 0)
	for _, r := range results {
		if r.PeakGroundAccel >= pga {
			matched = append(matched, r)
			break
		}
	}
	if len(matched) == 0 && len(results) > 0 {
		matched = append(matched, results[len(results)-1])
	}

	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: matched, Count: len(matched)})
}
