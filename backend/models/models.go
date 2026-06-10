package models

import (
	"time"

	"github.com/google/uuid"
)

type Aqueduct struct {
	ID               uuid.UUID              `json:"id" db:"id"`
	Name             string                 `json:"name" db:"name"`
	LatinName        string                 `json:"latin_name" db:"latin_name"`
	ConstructionYear int                    `json:"construction_year" db:"construction_year"`
	LengthKM         float64                `json:"length_km" db:"length_km"`
	HeightM          float64                `json:"height_m" db:"height_m"`
	StartLocation    string                 `json:"start_location" db:"start_location"`
	EndLocation      string                 `json:"end_location" db:"end_location"`
	Description      string                 `json:"description" db:"description"`
	GeoPath          map[string]interface{} `json:"geo_path" db:"geo_path"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
}

type StructureSegment struct {
	ID                  uuid.UUID              `json:"id" db:"id"`
	AqueductID          uuid.UUID              `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentType         string                 `json:"segment_type" db:"segment_type"`
	SegmentIndex        int                    `json:"segment_index" db:"segment_index"`
	PositionGeo         map[string]interface{} `json:"position_geo" db:"position_geo"`
	Position3D          map[string]interface{} `json:"position_3d" db:"position_3d"`
	DesignStrength      float64                `json:"design_strength" db:"design_strength"`
	OriginalMaterial    string                 `json:"original_material" db:"original_material"`
	DesignLoadCapacity  float64                `json:"design_load_capacity" db:"design_load_capacity"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
	WeatheringDepth     float64                `json:"weathering_depth,omitempty" db:"-"`
	WeatheringColor     string                 `json:"weathering_color,omitempty" db:"-"`
	CurrentStress       float64                `json:"current_stress,omitempty" db:"-"`
	SettlementMM        float64                `json:"settlement_mm,omitempty" db:"-"`
	ResidualCapacity    float64                `json:"residual_capacity,omitempty" db:"-"`
	CapacityRatio       float64                `json:"capacity_ratio,omitempty" db:"-"`
	SafetyLevel         string                 `json:"safety_level,omitempty" db:"-"`
}

type Sensor struct {
	ID                uuid.UUID `json:"id" db:"id"`
	SensorCode        string    `json:"sensor_code" db:"sensor_code"`
	SegmentID         uuid.UUID `json:"segment_id" db:"segment_id"`
	AqueductID        uuid.UUID `json:"aqueduct_id" db:"aqueduct_id"`
	SensorType        string    `json:"sensor_type" db:"sensor_type"`
	LocationDesc      string    `json:"location_description" db:"location_description"`
	InstalledDate     string    `json:"installed_date" db:"installed_date"`
	SamplingInterval  int       `json:"sampling_interval_sec" db:"sampling_interval_sec"`
	CalibrationDate   string    `json:"calibration_date" db:"calibration_date"`
	IsActive          bool      `json:"is_active" db:"is_active"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

type SensorData struct {
	SensorID   uuid.UUID `json:"sensor_id" db:"sensor_id"`
	AqueductID uuid.UUID `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentID  uuid.UUID `json:"segment_id" db:"segment_id"`
	SensorType string    `json:"sensor_type" db:"sensor_type"`
	Timestamp  time.Time `json:"timestamp" db:"timestamp"`
	Value      float64   `json:"value" db:"value"`
	Unit       string    `json:"unit" db:"unit"`
	Quality    int16     `json:"quality" db:"quality"`
	DtuID      string    `json:"dtu_id,omitempty" db:"dtu_id"`
	RSSI       float64   `json:"rssi,omitempty" db:"rssi"`
}

type SensorDataBatch struct {
	DtuID     string       `json:"dtu_id"`
	RSSI      float64      `json:"rssi"`
	Readings  []SensorData `json:"readings"`
	Timestamp time.Time    `json:"timestamp"`
}

type StructuralEvaluation struct {
	ID                    uuid.UUID              `json:"id" db:"id"`
	AqueductID            uuid.UUID              `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentID             uuid.UUID              `json:"segment_id" db:"segment_id"`
	EvaluationTime        time.Time              `json:"evaluation_time" db:"evaluation_time"`
	CurrentStress         float64                `json:"current_stress" db:"current_stress"`
	MaxStress             float64                `json:"max_stress" db:"max_stress"`
	WeatheringDepth       float64                `json:"weathering_depth" db:"weathering_depth"`
	SettlementMM          float64                `json:"settlement_mm" db:"settlement_mm"`
	ResidualStrength      float64                `json:"residual_strength" db:"residual_strength"`
	ResidualCapacityRatio float64                `json:"residual_capacity_ratio" db:"residual_capacity_ratio"`
	SafetyLevel           string                 `json:"safety_level" db:"safety_level"`
	FEAModelData          map[string]interface{} `json:"fea_model_data,omitempty" db:"fea_model_data"`
	Recommendations       string                 `json:"recommendations,omitempty" db:"recommendations"`
	CreatedAt             time.Time              `json:"created_at" db:"created_at"`
}

type Alert struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	AqueductID     uuid.UUID  `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentID      *uuid.UUID `json:"segment_id,omitempty" db:"segment_id"`
	SensorID       *uuid.UUID `json:"sensor_id,omitempty" db:"sensor_id"`
	AlertType      string     `json:"alert_type" db:"alert_type"`
	Severity       string     `json:"severity" db:"severity"`
	Title          string     `json:"title" db:"title"`
	Description    string     `json:"description,omitempty" db:"description"`
	ThresholdValue float64    `json:"threshold_value,omitempty" db:"threshold_value"`
	MeasuredValue  float64    `json:"measured_value,omitempty" db:"measured_value"`
	Unit           string     `json:"unit,omitempty" db:"unit"`
	MQTTPublished  bool       `json:"mqtt_published" db:"mqtt_published"`
	MQTTMessageID  string     `json:"mqtt_message_id,omitempty" db:"mqtt_message_id"`
	Acknowledged   bool       `json:"acknowledged" db:"acknowledged"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty" db:"acknowledged_by"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
	ResolutionNotes string    `json:"resolution_notes,omitempty" db:"resolution_notes"`
	Resolved       bool       `json:"resolved" db:"resolved"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	TriggeredAt    time.Time  `json:"triggered_at" db:"triggered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	AqueductName   string     `json:"aqueduct_name,omitempty" db:"-"`
}

type RepairMaterial struct {
	ID                  uuid.UUID              `json:"id" db:"id"`
	Name                string                 `json:"name" db:"name"`
	MaterialType        string                 `json:"material_type" db:"material_type"`
	Composition         map[string]interface{} `json:"composition,omitempty" db:"composition"`
	CompressiveStrength float64                `json:"compressive_strength" db:"compressive_strength"`
	TensileStrength     float64                `json:"tensile_strength" db:"tensile_strength"`
	ElasticModulus      float64                `json:"elastic_modulus" db:"elastic_modulus"`
	DurabilityRating    float64                `json:"durability_rating" db:"durability_rating"`
	CompatibilityRating float64                `json:"compatibility_rating" db:"compatibility_rating"`
	CostPerUnit         float64                `json:"cost_per_unit" db:"cost_per_unit"`
	Unit                string                 `json:"unit" db:"unit"`
	EaseOfApplication   float64                `json:"ease_of_application" db:"ease_of_application"`
	EnvironmentalImpact float64                `json:"environmental_impact" db:"environmental_impact"`
	AestheticMatch      float64                `json:"aesthetic_match" db:"aesthetic_match"`
	Description         string                 `json:"description,omitempty" db:"description"`
	IsActive            bool                   `json:"is_active" db:"is_active"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
	DecisionScore       float64                `json:"decision_score,omitempty" db:"-"`
	WeightedScores      map[string]float64     `json:"weighted_scores,omitempty" db:"-"`
}

type RepairRecommendation struct {
	ID                  uuid.UUID              `json:"id" db:"id"`
	AqueductID          uuid.UUID              `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentID           uuid.UUID              `json:"segment_id" db:"segment_id"`
	EvaluationID        *uuid.UUID             `json:"evaluation_id,omitempty" db:"evaluation_id"`
	RecommendationTime  time.Time              `json:"recommendation_time" db:"recommendation_time"`
	DamageType          string                 `json:"damage_type" db:"damage_type"`
	DamageSeverity      float64                `json:"damage_severity" db:"damage_severity"`
	RecommendedMaterials []RepairMaterial      `json:"recommended_materials" db:"-"`
	DecisionScores      map[string]interface{} `json:"decision_scores,omitempty" db:"decision_scores"`
	ExpectedCost        float64                `json:"expected_cost" db:"expected_cost"`
	ExpectedLifespan    int                    `json:"expected_lifespan_years" db:"expected_lifespan_years"`
	ConstructionNotes   string                 `json:"construction_notes,omitempty" db:"construction_notes"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
}

type TrendDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	AvgValue  float64   `json:"avg_value,omitempty"`
	MaxValue  float64   `json:"max_value,omitempty"`
	MinValue  float64   `json:"min_value,omitempty"`
}

type StatsSummary struct {
	TotalAqueducts       int     `json:"total_aqueducts"`
	TotalSegments        int     `json:"total_segments"`
	ActiveSensors        int     `json:"active_sensors"`
	SafeSegments         int     `json:"safe_segments"`
	WarningSegments      int     `json:"warning_segments"`
	DangerSegments       int     `json:"danger_segments"`
	CriticalSegments     int     `json:"critical_segments"`
	ActiveAlerts         int     `json:"active_alerts"`
	CriticalAlerts       int     `json:"critical_alerts"`
	AvgCapacityRatio     float64 `json:"avg_capacity_ratio"`
	DataPointsLast24h    int64   `json:"data_points_last_24h"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Count   int         `json:"count,omitempty"`
}
