package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/dtu_receiver"
	"aqueduct-monitor/evaluation"
	"aqueduct-monitor/metrics"
	"aqueduct-monitor/models"
	"aqueduct-monitor/mqtt"
	"aqueduct-monitor/pipeline"
	"aqueduct-monitor/recommendation"
	"aqueduct-monitor/repository"
)

type Handler struct {
	repo        *repository.Repository
	cfg         *config.Config
	evaluator   *evaluation.StructuralEvaluator
	recommender *recommendation.RepairRecommender
	mqttClient  *mqtt.AlertPublisher
	pipeline    *pipeline.Pipeline
	dtuRecv     *dtu_receiver.DTUReceiver
	metrics     *metrics.Metrics
}

func New(repo *repository.Repository, cfg *config.Config, evaluator *evaluation.StructuralEvaluator,
	recommender *recommendation.RepairRecommender, mqttClient *mqtt.AlertPublisher, pipe *pipeline.Pipeline, m *metrics.Metrics) *Handler {
	return &Handler{
		repo:        repo,
		cfg:         cfg,
		evaluator:   evaluator,
		recommender: recommender,
		mqttClient:  mqttClient,
		pipeline:    pipe,
		dtuRecv:     pipe.Receiver(),
		metrics:     m,
	}
}

type DTUSubmitRequest struct {
	DtuID     string                  `json:"dtu_id" binding:"required"`
	RSSI      float64                 `json:"rssi"`
	Timestamp string                  `json:"timestamp"`
	Readings  []DTUSensorReading      `json:"readings" binding:"required,min=1"`
}

type DTUSensorReading struct {
	SensorCode string  `json:"sensor_code" binding:"required"`
	Value      float64 `json:"value" binding:"required"`
	Unit       string  `json:"unit" binding:"required"`
	Timestamp  string  `json:"timestamp"`
}

func (h *Handler) SubmitSensorData(c *gin.Context) {
	var req DTUSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	batchTime := time.Now().UTC()
	if req.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			batchTime = t.UTC()
		}
	}

	ctx := context.Background()

	dtuReadings := make([]dtu_receiver.DTUSensorReading, 0, len(req.Readings))
	for _, r := range req.Readings {
		readingTime := batchTime
		if r.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
				readingTime = t.UTC()
			}
		}
		dtuReadings = append(dtuReadings, dtu_receiver.DTUSensorReading{
			SensorCode: r.SensorCode,
			Value:      r.Value,
			Timestamp:  readingTime,
		})
	}

	msgs, err := h.dtuRecv.SubmitReadings(ctx, req.DtuID, req.RSSI, dtuReadings)
	if err != nil {
		log.Printf("ERROR processing DTU readings: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "processing_error",
			Message: "Failed to process sensor data",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if len(msgs) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "no_valid_sensor_data",
			Message: "No valid sensor readings were accepted",
			Code:    http.StatusBadRequest,
		})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status:  "success",
		Message: "Sensor data accepted",
		Data: gin.H{
			"accepted_readings": len(msgs),
			"dtu_id":            req.DtuID,
			"processed_at":      time.Now().UTC().Format(time.RFC3339),
			"pipeline_queued":   true,
		},
	})
}

func (h *Handler) GetAqueducts(c *gin.Context) {
	ctx := context.Background()
	aqueducts, err := h.repo.GetAllAqueducts(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   aqueducts,
		Count:  len(aqueducts),
	})
}

func (h *Handler) GetAqueductDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid aqueduct id"})
		return
	}

	ctx := context.Background()
	aq, err := h.repo.GetAqueductByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "aqueduct not found"})
		return
	}

	segments, err := h.repo.GetAllSegmentsWithStatus(ctx, &id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	sensors, err := h.repo.GetSensorsByAqueduct(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	alerts, err := h.repo.GetActiveAlerts(ctx, &id, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data: gin.H{
			"aqueduct": aq,
			"segments": segments,
			"sensors":  sensors,
			"alerts":   alerts,
		},
	})
}

func (h *Handler) GetAllSegments(c *gin.Context) {
	ctx := context.Background()
	var aqueductID *uuid.UUID

	if aqIDStr := c.Query("aqueduct_id"); aqIDStr != "" {
		id, err := uuid.Parse(aqIDStr)
		if err == nil {
			aqueductID = &id
		}
	}

	segments, err := h.repo.GetAllSegmentsWithStatus(ctx, aqueductID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   segments,
		Count:  len(segments),
	})
}

func (h *Handler) GetSegmentDetail(c *gin.Context) {
	idStr := c.Param("id")
	segmentID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}

	ctx := context.Background()
	sensorVals, err := h.repo.GetSegmentLatestSensorValues(ctx, segmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	end := time.Now().UTC()
	start := end.AddDate(-1, 0, 0)

	trends := make(map[string]interface{})
	sensorTypes := []string{"stress", "weathering", "settlement"}

	for _, stype := range sensorTypes {
		var sensorID uuid.UUID
		query := `SELECT id FROM sensors WHERE segment_id = $1 AND sensor_type = $2 LIMIT 1`
		h.repo.GetPool().QueryRow(ctx, query, segmentID, stype).Scan(&sensorID)
		if sensorID == uuid.Nil {
			continue
		}

		data, err := h.repo.GetSensorDataTrend(ctx, sensorID, start, end, "day")
		if err == nil {
			trends[stype] = data
		}
	}

	weatheringRate, _ := h.repo.GetWeatheringRate(ctx, segmentID, 90)
	segmentsList, _ := h.repo.GetAllSegmentsWithStatus(ctx, nil)
	var current *models.StructureSegment
	for i := range segmentsList {
		if segmentsList[i].ID == segmentID {
			current = &segmentsList[i]
			break
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data: gin.H{
			"segment":          current,
			"latest_sensors":   sensorVals,
			"yearly_trends":    trends,
			"weathering_rate":  weatheringRate,
		},
	})
}

func (h *Handler) GetAlerts(c *gin.Context) {
	ctx := context.Background()
	var aqueductID *uuid.UUID

	if aqIDStr := c.Query("aqueduct_id"); aqIDStr != "" {
		id, err := uuid.Parse(aqIDStr)
		if err == nil {
			aqueductID = &id
		}
	}

	limit := 100
	alerts, err := h.repo.GetActiveAlerts(ctx, aqueductID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   alerts,
		Count:  len(alerts),
	})
}

func (h *Handler) GetStats(c *gin.Context) {
	ctx := context.Background()
	stats, err := h.repo.GetStatsSummary(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   stats,
	})
}

func (h *Handler) GetRepairMaterials(c *gin.Context) {
	ctx := context.Background()
	materials, err := h.repo.GetAllRepairMaterials(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   materials,
		Count:  len(materials),
	})
}

func (h *Handler) GetRepairRecommendation(c *gin.Context) {
	idStr := c.Param("segment_id")
	segmentID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid segment id"})
		return
	}

	ctx := context.Background()
	segments, _ := h.repo.GetAllSegmentsWithStatus(ctx, nil)
	var segment *models.StructureSegment
	for i := range segments {
		if segments[i].ID == segmentID {
			segment = &segments[i]
			break
		}
	}

	if segment == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "segment not found"})
		return
	}

	rec, err := h.recommender.RecommendForSegment(ctx, segment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.repo.InsertRepairRecommendation(ctx, rec); err != nil {
		log.Printf("Warning: Could not store recommendation: %v", err)
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   rec,
	})
}

func (h *Handler) RunFullEvaluation(c *gin.Context) {
	ctx := context.Background()
	segments, err := h.repo.GetAllSegmentsWithStatus(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	totalAlerts := 0
	for i := range segments {
		alerts, err := h.evaluator.EvaluateSegment(ctx, segments[i].ID)
		if err != nil {
			log.Printf("ERROR evaluating segment %s: %v", segments[i].ID, err)
			continue
		}
		for _, a := range alerts {
			h.mqttClient.PublishAlert(ctx, a)
			totalAlerts++
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status:  "success",
		Message: "Full evaluation completed",
		Data: gin.H{
			"segments_evaluated": len(segments),
			"alerts_generated":   totalAlerts,
		},
	})
}

func (h *Handler) GetSensorTrend(c *gin.Context) {
	idStr := c.Param("sensor_id")
	sensorID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid sensor id"})
		return
	}

	days := 365
	granularity := "day"
	if g := c.Query("granularity"); g != "" {
		granularity = g
	}
	if d := c.Query("days"); d != "" {
		if parsed, err := time.ParseDuration(d + "h"); err == nil {
			days = int(parsed.Hours() / 24)
		}
	}

	ctx := context.Background()
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -days)

	data, err := h.repo.GetSensorDataTrend(ctx, sensorID, start, end, granularity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse{
		Status: "success",
		Data:   data,
		Count:  len(data),
	})
}
