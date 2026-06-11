package material_predictor

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"aqueduct-monitor/models"
)

type PredictLifetimeRequest struct {
	MaterialID string `json:"material_id" binding:"required"`
	Scenario   string `json:"scenario"`
	SaveResult bool   `json:"save_result"`
}

type CalibrateRequest struct {
	MaterialType string                      `json:"material_type"`
	AgingData    []models.AcceleratedAgingData `json:"aging_data" binding:"required"`
}

type PredictorHandler struct {
	service *PredictorService
}

func NewPredictorHandler(service *PredictorService) *PredictorHandler {
	return &PredictorHandler{service: service}
}

func (h *PredictorHandler) RegisterRoutes(r *gin.RouterGroup) {
	predictorGroup := r.Group("/material-predictor")
	{
		predictorGroup.POST("/predict", h.PredictLifetime)
		predictorGroup.POST("/calibrate", h.Calibrate)
		predictorGroup.GET("/materials/:material_id", h.GetMaterialPredictions)
		predictorGroup.GET("/aqueducts/:aqueduct_id", h.GetAqueductPredictions)
	}
}

func (h *PredictorHandler) PredictLifetime(c *gin.Context) {
	var req PredictLifetimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	matID, err := uuid.Parse(req.MaterialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid material_id"})
		return
	}

	scenario := req.Scenario
	if scenario == "" {
		scenario = "TEMPERATE"
	}

	ctx := context.Background()
	result, err := h.service.PredictMaterialLifetime(ctx, matID, scenario)
	if err != nil {
		log.Printf("Lifetime prediction ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	if req.SaveResult {
		_ = h.service.SavePrediction(ctx, result)
	}

	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (h *PredictorHandler) Calibrate(c *gin.Context) {
	var req CalibrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	ctx := context.Background()
	activationEnergy, err := h.service.CalibrateActivationEnergy(ctx, req.MaterialType, req.AgingData)
	if err != nil {
		log.Printf("Calibration ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data: gin.H{
			"material_type":           req.MaterialType,
			"activation_energy_ev":    activationEnergy,
			"activation_energy_j_mol": activationEnergy * 96485.33,
			"calibration_status":      "calibrated",
		},
	})
}

func (h *PredictorHandler) GetMaterialPredictions(c *gin.Context) {
	idStr := c.Param("material_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid material id"})
		return
	}

	limit := 10
	if lStr := c.Query("limit"); lStr != "" {
		if n, e := strconv.Atoi(lStr); e == nil && n > 0 {
			limit = n
		}
	}

	ctx := context.Background()
	list, err := h.service.GetPredictionsBySegment(ctx, id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (h *PredictorHandler) GetAqueductPredictions(c *gin.Context) {
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
	list, err := h.service.GetPredictionsByAqueduct(ctx, id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}
