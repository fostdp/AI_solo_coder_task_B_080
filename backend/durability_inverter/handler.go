package durability_inverter

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"aqueduct-monitor/models"
)

type InvertRequest struct {
	SegmentID          string  `json:"segment_id" binding:"required"`
	ObservedWeathering float64 `json:"observed_weathering_mm" binding:"required,gte=0"`
	ObservedStrength   float64 `json:"observed_strength_mpa" binding:"required,gt=0"`
	AgeYears           float64 `json:"age_years"`
	ObservedPH         float64 `json:"observed_ph"`
	SaveResult         bool    `json:"save_result"`
}

type InverterHandler struct {
	service *InverterService
}

func NewInverterHandler(service *InverterService) *InverterHandler {
	return &InverterHandler{service: service}
}

func (h *InverterHandler) RegisterRoutes(r *gin.RouterGroup) {
	inversionGroup := r.Group("/inversion")
	{
		inversionGroup.POST("/invert", h.InvertConcrete)
		inversionGroup.GET("/formulas", h.GetFormulas)
		inversionGroup.GET("/aqueducts/:aqueduct_id", h.GetAqueductInversionResults)
	}
}

func (h *InverterHandler) InvertConcrete(c *gin.Context) {
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
	result, err := h.service.InvertConcreteProperties(ctx, segID,
		req.ObservedWeathering, req.ObservedStrength, age, ph)
	if err != nil {
		log.Printf("Inversion ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	if req.SaveResult {
		_ = h.service.SaveResult(ctx, result)
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (h *InverterHandler) GetFormulas(c *gin.Context) {
	ctx := context.Background()
	list, err := h.service.GetAllFormulas(ctx)
	if err != nil || len(list) == 0 {
		c.JSON(http.StatusOK, models.SuccessResponse{Status: "generated", Data: BuildDefaultFormulas()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (h *InverterHandler) GetAqueductInversionResults(c *gin.Context) {
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
	list, err := h.service.GetResultsByAqueduct(ctx, id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}
