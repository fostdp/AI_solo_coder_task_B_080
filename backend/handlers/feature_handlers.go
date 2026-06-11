package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"aqueduct-monitor/inversion"
	"aqueduct-monitor/lifetime"
	"aqueduct-monitor/models"
	"aqueduct-monitor/seismic"
	"aqueduct-monitor/tourism"
)

type FeatureHandlers struct {
	handler *Handler
	inverter  *inversion.ConcreteInverter
	seismic   *seismic.VulnerabilityAnalyzer
	lifetime  *lifetime.LifetimePredictor
	tourism   *tourism.TourismPlanner
}

func NewFeatureHandlers(
	h *Handler,
	inverter *inversion.ConcreteInverter,
	seismicAnalyzer *seismic.VulnerabilityAnalyzer,
	lifePred *lifetime.LifetimePredictor,
	tourismPlanner *tourism.TourismPlanner,
) *FeatureHandlers {
	return &FeatureHandlers{
		handler:  h,
		inverter: inverter,
		seismic:  seismicAnalyzer,
		lifetime: lifePred,
		tourism:  tourismPlanner,
	}
}

type InvertRequest struct {
	SegmentID           string  `json:"segment_id" binding:"required"`
	ObservedWeathering  float64 `json:"observed_weathering_mm" binding:"required,gte=0"`
	ObservedStrength    float64 `json:"observed_strength_mpa" binding:"required,gt=0"`
	AgeYears            float64 `json:"age_years"`
	ObservedPH          float64 `json:"observed_ph"`
	SaveResult          bool    `json:"save_result"`
}

func (fh *FeatureHandlers) InvertConcrete(c *gin.Context) {
	var req InvertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}
	segID, err := uuid.Parse(req.SegmentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment_id"})
		return
	}
	age := req.AgeYears
	if age <= 0 {
		age = 2000.0
	}
	ph := req.ObservedPH
	if ph <= 0 {
		ph = 9.5
	}
	ctx := context.Background()
	result, err := fh.inverter.InvertConcreteProperties(ctx, segID,
		req.ObservedWeathering, req.ObservedStrength, age, ph)
	if err != nil {
		log.Printf("Inversion ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	if req.SaveResult {
		_ = fh.handler.repo.InsertConcreteInversionResult(ctx, result)
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (fh *FeatureHandlers) GetFormulas(c *gin.Context) {
	ctx := context.Background()
	list, err := fh.handler.repo.GetAllConcreteFormulas(ctx)
	if err != nil || len(list) == 0 {
		c.JSON(http.StatusOK, models.SuccessResponse{Status: "generated", Data: buildGeneratedFormulas()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (fh *FeatureHandlers) GetAqueductInversionResults(c *gin.Context) {
	idStr := c.Param("aqueduct_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid aqueduct id"})
		return
	}
	limit := 20
	if lStr := c.Query("limit"); lStr != "" {
		if n, e := strconv.Atoi(lStr); e == nil && n > 0 {
			limit = n
		}
	}
	ctx := context.Background()
	list, err := fh.handler.repo.GetInversionResultsByAqueduct(ctx, id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (fh *FeatureHandlers) AnalyzeSeismicRisk(c *gin.Context) {
	idStr := c.Param("aqueduct_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid aqueduct id"})
		return
	}
	ctx := context.Background()
	result, err := fh.seismic.AnalyzeAqueductSeismicRisk(ctx, id)
	if err != nil {
		log.Printf("Seismic ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	_ = fh.handler.repo.InsertSeismicRiskResult(ctx, result)
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (fh *FeatureHandlers) GetAllSeismicRisks(c *gin.Context) {
	ctx := context.Background()
	list, err := fh.handler.repo.GetAllSeismicRisks(ctx)
	if err != nil || len(list) == 0 {
		generated, _ := fh.generateAllSeismicRisks(ctx)
		c.JSON(http.StatusOK, models.SuccessResponse{Status: "generated", Data: generated, Count: len(generated)})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (fh *FeatureHandlers) GetHistoricalEarthquakes(c *gin.Context) {
	ctx := context.Background()
	list, err := fh.seismic.GetAllHistoricalEarthquakes(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

type FragilityRequest struct {
	SegmentID string `json:"segment_id" binding:"required"`
}

func (fh *FeatureHandlers) GetFragilityCurve(c *gin.Context) {
	segIDStr := c.Param("segment_id")
	segID, err := uuid.Parse(segIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}
	ctx := context.Background()
	curve, err := fh.seismic.GenerateFragilityCurves(ctx, segID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: curve})
}

func (fh *FeatureHandlers) AnalyzeIncrementalDynamic(c *gin.Context) {
	segIDStr := c.Param("segment_id")
	segID, err := uuid.Parse(segIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}
	ctx := context.Background()
	results, err := fh.seismic.AnalyzeIncrementalDynamic(ctx, segID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	for i := range results {
		_ = fh.handler.repo.InsertSeismicVulnerability(ctx, &results[i])
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: results, Count: len(results)})
}

type LifetimeRequest struct {
	MaterialID string `json:"material_id" binding:"required"`
	Scenario   string `json:"scenario"`
	SaveResult bool   `json:"save_result"`
}

func (fh *FeatureHandlers) PredictMaterialLifetime(c *gin.Context) {
	var req LifetimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error(), Code: http.StatusBadRequest})
		return
	}
	mID, err := uuid.Parse(req.MaterialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid material id"})
		return
	}
	scenario := req.Scenario
	if scenario == "" {
		scenario = "mediterranean"
	}
	ctx := context.Background()
	pred, err := fh.lifetime.PredictMaterialLifetime(ctx, mID, scenario)
	if err != nil {
		log.Printf("Lifetime ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	if req.SaveResult {
		_ = fh.handler.repo.InsertLifetimePrediction(ctx, pred)
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: pred})
}

func (fh *FeatureHandlers) GetMaterialPredictions(c *gin.Context) {
	mIDStr := c.Param("material_id")
	mID, err := uuid.Parse(mIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid material id"})
		return
	}
	limit := 10
	ctx := context.Background()
	list, err := fh.handler.repo.GetLifetimePredictionsByMaterial(ctx, mID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

type CompareRequest struct {
	AqueductIDs []string `json:"aqueduct_ids"`
	SaveResult  bool     `json:"save_result"`
}

func (fh *FeatureHandlers) CompareAqueducts(c *gin.Context) {
	var req CompareRequest
	if err := c.ShouldBindJSON(&req); err == nil && len(req.AqueductIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(req.AqueductIDs))
		for _, s := range req.AqueductIDs {
			if id, e := uuid.Parse(s); e == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no valid aqueduct ids"})
			return
		}
		ctx := context.Background()
		result, err := fh.tourism.CompareAqueducts(ctx, ids)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
			return
		}
		if req.SaveResult {
			_ = fh.handler.repo.InsertTourismComparison(ctx, result)
		}
		c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
		return
	}

	idsStr := c.Query("ids")
	if idsStr != "" {
		parts := strings.Split(idsStr, ",")
		ids := make([]uuid.UUID, 0, len(parts))
		for _, s := range parts {
			s = strings.TrimSpace(s)
			if id, e := uuid.Parse(s); e == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			ctx := context.Background()
			result, err := fh.tourism.CompareAqueducts(ctx, ids)
			if err == nil {
				c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
				return
			}
		}
	}
	ctx := context.Background()
	result, err := fh.tourism.CompareAqueducts(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	if req.SaveResult {
		_ = fh.handler.repo.InsertTourismComparison(ctx, result)
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (fh *FeatureHandlers) GetRecentComparisons(c *gin.Context) {
	limit := 5
	if lStr := c.Query("limit"); lStr != "" {
		if n, e := strconv.Atoi(lStr); e == nil && n > 0 {
			limit = n
		}
	}
	ctx := context.Background()
	list, err := fh.handler.repo.GetRecentTourismComparisons(ctx, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (fh *FeatureHandlers) generateAllSeismicRisks(ctx context.Context) ([]models.AqueductSeismicRisk, error) {
	aqs, err := fh.handler.repo.GetAllAqueducts(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]models.AqueductSeismicRisk, 0, len(aqs))
	for i := range aqs {
		r, e := fh.seismic.AnalyzeAqueductSeismicRisk(ctx, aqs[i].ID)
		if e == nil {
			results = append(results, *r)
		}
	}
	return results, nil
}

func buildGeneratedFormulas() []models.RomanConcreteFormula {
	return []models.RomanConcreteFormula{
		{FormulaName: "标准罗马混凝土 (Opus Caementicium)", LimeRatio: 1.0, PozzolanaRatio: 1.2,
			AggregateRatio: 3.5, WaterRatio: 0.85, AggregateType: "石灰华/火山砾",
			OriginalFyMPa: 8.5, OriginalEmGPa: 25.0, Porosity: 0.28, DurabilityIndex: 0.85,
			EraDescription: "罗马帝国时期 (公元前1世纪 - 公元3世纪)",
			ArchaeologicalSources: "Vitruvius De Architectura, Pompeii遗址"},
		{FormulaName: "高强度火山灰砂浆 (Puteolanus)", LimeRatio: 0.85, PozzolanaRatio: 1.6,
			AggregateRatio: 3.2, WaterRatio: 0.78, AggregateType: "Pozzuoli火山灰",
			OriginalFyMPa: 10.5, OriginalEmGPa: 28.0, Porosity: 0.24, DurabilityIndex: 0.90,
			EraDescription: "罗马共和国末期-帝国早期",
			ArchaeologicalSources: "Pliny the Elder, Naturalis Historia"},
	}
}

func init() {
	_ = time.Now
}
