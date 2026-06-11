package aqueduct_comparator

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"aqueduct-monitor/models"
)

type CompareRequest struct {
	AqueductIDs []string               `json:"aqueduct_ids" binding:"required"`
	Weights     map[string]float64    `json:"weights,omitempty"`
	SaveResult  bool                   `json:"save_result"`
}

type TourismPlanRequest struct {
	Preferences map[string]interface{} `json:"preferences"`
}

type ComparatorHandler struct {
	service *ComparatorService
}

func NewComparatorHandler(service *ComparatorService) *ComparatorHandler {
	return &ComparatorHandler{service: service}
}

func (h *ComparatorHandler) RegisterRoutes(r *gin.RouterGroup) {
	comparatorGroup := r.Group("/aqueduct-comparator")
	{
		comparatorGroup.POST("/compare", h.Compare)
		comparatorGroup.POST("/tourism-plan", h.GetTourismPlan)
		comparatorGroup.GET("/tourism/:aqueduct_id", h.GetTourismData)
		comparatorGroup.GET("/comparisons", h.GetComparisons)
		comparatorGroup.GET("/radar/:aqueduct_ids", h.GetRadarData)
	}
}

func (h *ComparatorHandler) Compare(c *gin.Context) {
	var req CompareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	ids := make([]uuid.UUID, 0, len(req.AqueductIDs))
	for _, idStr := range req.AqueductIDs {
		if id, e := uuid.Parse(idStr); e == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) < 2 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "at least 2 aqueduct ids required",
		})
		return
	}

	ctx := context.Background()
	result, err := h.service.CompareAqueducts(ctx, ids)
	if err != nil {
		log.Printf("Comparison ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	if req.SaveResult {
		_ = h.service.SaveComparison(ctx, result)
	}

	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: result})
}

func (h *ComparatorHandler) GetTourismPlan(c *gin.Context) {
	var req TourismPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Preferences = map[string]interface{}{
			"heritage_priority": 0.6,
			"accessibility":     0.3,
			"crowd_level":       0.4,
		}
	}

	ctx := context.Background()
	results, err := h.service.RecommendTourismPlan(ctx, req.Preferences)
	if err != nil {
		log.Printf("Tourism plan ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: results, Count: len(results)})
}

func (h *ComparatorHandler) GetTourismData(c *gin.Context) {
	idStr := c.Param("aqueduct_id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid aqueduct id"})
		return
	}

	ctx := context.Background()
	data, err := h.service.GetTourismDataByAqueduct(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: data})
}

func (h *ComparatorHandler) GetComparisons(c *gin.Context) {
	limit := 10
	if lStr := c.Query("limit"); lStr != "" {
		if n, e := strconv.Atoi(lStr); e == nil && n > 0 {
			limit = n
		}
	}

	ctx := context.Background()
	list, err := h.service.GetComparisons(ctx, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: list, Count: len(list)})
}

func (h *ComparatorHandler) GetRadarData(c *gin.Context) {
	idsParam := c.Param("aqueduct_ids")

	ids := make([]uuid.UUID, 0)
	if idsParam != "" {
		parts := splitString(idsParam, ",")
		for _, p := range parts {
			if id, e := uuid.Parse(p); e == nil {
				ids = append(ids, id)
			}
		}
	}

	if len(ids) < 2 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "at least 2 aqueduct ids required, comma-separated",
		})
		return
	}

	ctx := context.Background()
	radarData, err := h.service.BuildRadarDataForAqueducts(ctx, ids)
	if err != nil {
		log.Printf("Radar data ERROR: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{Status: "success", Data: radarData, Count: len(radarData)})
}

func splitString(s, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
