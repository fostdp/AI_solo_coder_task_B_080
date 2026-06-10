package pipeline

import (
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/models"
)

type SensorReadingMsg struct {
	SegmentID   uuid.UUID `json:"segment_id"`
	AqueductID  uuid.UUID `json:"aqueduct_id"`
	SensorID    string    `json:"sensor_id"`
	SensorType   string    `json:"sensor_type"`
	Value        float64   `json:"value"`
	Timestamp    time.Time `json:"timestamp"`
	DTUID       string    `json:"dtu_id,omitempty"`
	RSSI        float64   `json:"rssi,omitempty"`
	Stored       bool      `json:"-"`
}

type EvalRequest struct {
	RequestID   string        `json:"request_id"`
	SegmentID   uuid.UUID   `json:"segment_id"`
	AqueductID  uuid.UUID   `json:"aqueduct_id"`
	TriggerType string      `json:"trigger_type"`
	Timestamp   time.Time   `json:"timestamp"`
	SensorData  []SensorReadingMsg `json:"sensor_data,omitempty"`
	Immediate  bool        `json:"immediate"`
}

type EvalResult struct {
	RequestID       string              `json:"request_id"`
	SegmentID       uuid.UUID           `json:"segment_id"`
	AqueductID      uuid.UUID           `json:"aqueduct_id"`
	SafetyLevel     string              `json:"safety_level"`
	ResidualRatio   float64             `json:"residual_ratio"`
	MaxStress       float64             `json:"max_stress"`
	MaxDisplacement float64             `json:"max_displacement"`
	WeatheringRate  float64             `json:"weathering_rate"`
	WeatheringAccel  bool                `json:"weathering_acceleration"`
	SettlementMM  float64             `json:"settlement_mm"`
	Alerts          []models.Alert       `json:"alerts"`
	NeedsRepair   bool                `json:"needs_repair"`
	DamageType      string              `json:"damage_type,omitempty"`
	DamageSeverity  float64             `json:"damage_severity"`
	FEAConverged   bool                `json:"fea_converged"`
	FEAIterations int                 `json:"fea_iterations"`
	FEAModelFallback bool              `json:"fea_model_fallback"`
	ProcessedAt     time.Time           `json:"processed_at"`
	Error           string              `json:"error,omitempty"`
}

type RepairRequest struct {
	RequestID      string    `json:"request_id"`
	SegmentID      uuid.UUID `json:"segment_id"`
	AqueductID     uuid.UUID `json:"aqueduct_id"`
	DamageType     string    `json:"damage_type"`
	DamageSeverity float64   `json:"damage_severity"`
	SafetyLevel    string    `json:"safety_level"`
	TriggerType    string    `json:"trigger_type"`
	Timestamp      time.Time `json:"timestamp"`
}

type RepairResult struct {
	RequestID       string                        `json:"request_id"`
	SegmentID       uuid.UUID                     `json:"segment_id"`
	AqueductID      uuid.UUID                     `json:"aqueduct_id"`
	Recommendation *models.RepairRecommendation `json:"recommendation"`
	Sensitivity    interface{}                 `json:"sensitivity,omitempty"`
	ProcessedAt      time.Time                   `json:"processed_at"`
	Error            string                      `json:"error,omitempty"`
}

type AlertMsg struct {
	ID          string    `json:"id"`
	Alert       *models.Alert `json:"alert"`
	Source      string    `json:"source"`
	Timestamp   time.Time `json:"timestamp"`
	Priority    int       `json:"priority"`
}

type PipelineStats struct {
	DTUReceived       int64 `json:"dtu_received"`
	EvalProcessed      int64 `json:"eval_processed"`
	EvalErrors        int64 `json:"eval_errors"`
	RepairProcessed  int64 `json:"repair_processed"`
	AlertsPublished  int64 `json:"alerts_published"`
	AlertsBuffered   int64 `json:"alerts_buffered"`
	QueueSizeDTU        int   `json:"queue_size_dtu"`
	QueueSizeEval     int   `json:"queue_size_eval"`
	QueueSizeRepair   int   `json:"queue_size_repair"`
	QueueSizeAlert    int   `json:"queue_size_alert"`
}
