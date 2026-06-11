package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================
// Feature 1: 古罗马混凝土耐久性反演
// ============================================

type RomanConcreteFormula struct {
	ID                   uuid.UUID              `json:"id" db:"id"`
	FormulaName          string                 `json:"formula_name" db:"formula_name"`
	LimeRatio            float64                `json:"lime_ratio" db:"lime_ratio"`
	PozzolanaRatio       float64                `json:"pozzolana_ratio" db:"pozzolana_ratio"`
	AggregateRatio       float64                `json:"aggregate_ratio" db:"aggregate_ratio"`
	WaterRatio           float64                `json:"water_ratio" db:"water_ratio"`
	AggregateType        string                 `json:"aggregate_type,omitempty" db:"aggregate_type"`
	AdditiveType         string                 `json:"additive_type,omitempty" db:"additive_type"`
	OriginalFyMPa        float64                `json:"original_fy_mpa" db:"original_fy_mpa"`
	OriginalFmMPa        float64                `json:"original_fm_mpa" db:"original_fm_mpa"`
	OriginalEmGPa        float64                `json:"original_em_gpa" db:"original_em_gpa"`
	Porosity             float64                `json:"porosity" db:"porosity"`
	PoreSizeDistribution map[string]interface{} `json:"pore_size_distribution,omitempty" db:"pore_size_distribution"`
	DurabilityIndex      float64                `json:"durability_index" db:"durability_index"`
	EraDescription       string                 `json:"era_description,omitempty" db:"era_description"`
	ArchaeologicalSources string                `json:"archaeological_sources,omitempty" db:"archaeological_sources"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
}

type ConcreteInversionResult struct {
	ID                        uuid.UUID              `json:"id" db:"id"`
	AqueductID                uuid.UUID              `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentID                 *uuid.UUID             `json:"segment_id,omitempty" db:"segment_id"`
	AnalysisTime              time.Time              `json:"analysis_time" db:"analysis_time"`
	ObservedWeatheringDepth   float64                `json:"observed_weathering_depth" db:"observed_weathering_depth"`
	ObservedStrength          float64                `json:"observed_strength" db:"observed_strength"`
	ObservedMortarPH          float64                `json:"observed_mortar_ph" db:"observed_mortar_ph"`
	AgeYears                  float64                `json:"age_years" db:"age_years"`
	BestMatchFormulaID        *uuid.UUID             `json:"best_match_formula_id,omitempty" db:"best_match_formula_id"`
	CandidateFormulas         map[string]interface{} `json:"candidate_formulas,omitempty" db:"candidate_formulas"`
	InversionConfidence       float64                `json:"inversion_confidence" db:"inversion_confidence"`
	InferredOriginalFy        float64                `json:"inferred_original_fy" db:"inferred_original_fy"`
	InferredDurabilityMechanism map[string]interface{} `json:"inferred_durability_mechanism,omitempty" db:"inferred_durability_mechanism"`
	LeachingRate              float64                `json:"leaching_rate" db:"leaching_rate"`
	CarbonationDepth          float64                `json:"carbonation_depth" db:"carbonation_depth"`
	ModernReferenceFormula    map[string]interface{} `json:"modern_reference_formula,omitempty" db:"modern_reference_formula"`
	Notes                     string                 `json:"notes,omitempty" db:"notes"`
	CreatedAt                 time.Time              `json:"created_at" db:"created_at"`
	BestMatchFormula          *RomanConcreteFormula  `json:"best_match_formula,omitempty" db:"-"`
}

// ============================================
// Feature 2: 地震易损性评估
// ============================================

type HistoricalEarthquake struct {
	ID                uuid.UUID              `json:"id" db:"id"`
	EventName         string                 `json:"event_name" db:"event_name"`
	EventDate         string                 `json:"event_date" db:"event_date"`
	Magnitude         float64                `json:"magnitude" db:"magnitude"`
	EpicenterLat      float64                `json:"epicenter_lat" db:"epicenter_lat"`
	EpicenterLng      float64                `json:"epicenter_lng" db:"epicenter_lng"`
	DepthKm           float64                `json:"depth_km" db:"depth_km"`
	IntensityMSK      float64                `json:"intensity_msk" db:"intensity_msk"`
	Region            string                 `json:"region,omitempty" db:"region"`
	AffectedAqueducts map[string]interface{} `json:"affected_aqueducts,omitempty" db:"affected_aqueducts"`
	HistoricalSources string                 `json:"historical_sources,omitempty" db:"historical_sources"`
	DamageDescription string                 `json:"damage_description,omitempty" db:"damage_description"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
}

type SeismicVulnerability struct {
	ID                  uuid.UUID              `json:"id" db:"id"`
	AqueductID          uuid.UUID              `json:"aqueduct_id" db:"aqueduct_id"`
	SegmentID           *uuid.UUID             `json:"segment_id,omitempty" db:"segment_id"`
	AnalysisTime        time.Time              `json:"analysis_time" db:"analysis_time"`
	DamageState         string                 `json:"damage_state" db:"damage_state"`
	Magnitude           float64                `json:"magnitude" db:"magnitude"`
	PGAG                float64                `json:"pga_g" db:"pga_g"`
	Probability         float64                `json:"probability" db:"probability"`
	FragilityCurveParams map[string]interface{} `json:"fragility_curve_params,omitempty" db:"fragility_curve_params"`
	CapacitySpectrum    map[string]interface{} `json:"capacity_spectrum,omitempty" db:"capacity_spectrum"`
	DemandSpectrum      map[string]interface{} `json:"demand_spectrum,omitempty" db:"demand_spectrum"`
	ExpectedRepairCost  float64                `json:"expected_repair_cost" db:"expected_repair_cost"`
	ExpectedDowntimeDays int                   `json:"expected_downtime_days" db:"expected_downtime_days"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
}

type AqueductSeismicRisk struct {
	ID                    uuid.UUID `json:"id" db:"id"`
	AqueductID            uuid.UUID `json:"aqueduct_id" db:"aqueduct_id"`
	Region                string    `json:"region,omitempty" db:"region"`
	PeakGroundAccel475Yr  float64   `json:"peak_ground_accel_475yr" db:"peak_ground_accel_475yr"`
	PeakGroundAccel2475Yr float64   `json:"peak_ground_accel_2475yr" db:"peak_ground_accel_2475yr"`
	OverallRiskLevel      string    `json:"overall_risk_level" db:"overall_risk_level"`
	SiteClass             string    `json:"site_class,omitempty" db:"site_class"`
	SoilAmplification     float64   `json:"soil_amplification" db:"soil_amplification"`
	PredominantPeriodSec  float64   `json:"predominant_period_sec" db:"predominant_period_sec"`
	VulnerableSegments    int       `json:"vulnerable_segments" db:"vulnerable_segments"`
	EstimatedTotalLoss    float64   `json:"estimated_total_loss" db:"estimated_total_loss"`
	AnalysisTime          time.Time `json:"analysis_time" db:"analysis_time"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	AqueductName          string    `json:"aqueduct_name,omitempty" db:"-"`
	AqueductLat           float64   `json:"aqueduct_lat,omitempty" db:"-"`
	AqueductLng           float64   `json:"aqueduct_lng,omitempty" db:"-"`
}

// ============================================
// Feature 3: 修复材料长期性能预测
// ============================================

type AcceleratedAgingData struct {
	ID                 uuid.UUID `json:"id" db:"id"`
	MaterialID         uuid.UUID `json:"material_id" db:"material_id"`
	TestType           string    `json:"test_type" db:"test_type"`
	TemperatureC       float64   `json:"temperature_c" db:"temperature_c"`
	HumidityPct        float64   `json:"humidity_pct" db:"humidity_pct"`
	Cycles             int       `json:"cycles" db:"cycles"`
	ExposureDays       int       `json:"exposure_days" db:"exposure_days"`
	StrengthRetention  float64   `json:"strength_retention" db:"strength_retention"`
	MassLossPct        float64   `json:"mass_loss_pct" db:"mass_loss_pct"`
	ElasticModulusLoss float64   `json:"elastic_modulus_loss" db:"elastic_modulus_loss"`
	CrackingIndex      float64   `json:"cracking_index,omitempty" db:"cracking_index"`
	PHChange           float64   `json:"ph_change,omitempty" db:"ph_change"`
	TestNotes          string    `json:"test_notes,omitempty" db:"test_notes"`
	TestStandard       string    `json:"test_standard,omitempty" db:"test_standard"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

type MaterialLifetimePrediction struct {
	ID                        uuid.UUID              `json:"id" db:"id"`
	MaterialID                uuid.UUID              `json:"material_id" db:"material_id"`
	PredictionTime            time.Time              `json:"prediction_time" db:"prediction_time"`
	Scenario                  string                 `json:"scenario" db:"scenario"`
	PredictionYears           int                    `json:"prediction_years" db:"prediction_years"`
	ArrheniusActivationEV     float64                `json:"arrhenius_activation_ev" db:"arrhenius_activation_ev"`
	TimeTempShiftFactor       map[string]interface{} `json:"time_temp_shift_factor,omitempty" db:"time_temp_shift_factor"`
	DegradationCurve          map[string]interface{} `json:"degradation_curve,omitempty" db:"degradation_curve"`
	StrengthAt50Yr            float64                `json:"strength_at_50yr" db:"strength_at_50yr"`
	StrengthAt100Yr           float64                `json:"strength_at_100yr" db:"strength_at_100yr"`
	EstimatedServiceLife      float64                `json:"estimated_service_life" db:"estimated_service_life"`
	ThresholdStrengthRatio    float64                `json:"threshold_strength_ratio" db:"threshold_strength_ratio"`
	ConfidenceIntervalLow     float64                `json:"confidence_interval_low" db:"confidence_interval_low"`
	ConfidenceIntervalHigh    float64                `json:"confidence_interval_high" db:"confidence_interval_high"`
	ModelAssumptions          string                 `json:"model_assumptions,omitempty" db:"model_assumptions"`
	CreatedAt                 time.Time              `json:"created_at" db:"created_at"`
	MaterialName              string                 `json:"material_name,omitempty" db:"-"`
	MaterialType              string                 `json:"material_type,omitempty" db:"-"`
	RepairValidityYears       int                    `json:"repair_validity_years,omitempty" db:"-"`
}

// ============================================
// Feature 4: 多水道对比与旅游规划
// ============================================

type AqueductTourismData struct {
	ID                       uuid.UUID `json:"id" db:"id"`
	AqueductID               uuid.UUID `json:"aqueduct_id" db:"aqueduct_id"`
	VisitorCountPerYear      int       `json:"visitor_count_per_year" db:"visitor_count_per_year"`
	TicketPriceEur           float64   `json:"ticket_price_eur" db:"ticket_price_eur"`
	AccessibilityScore       float64   `json:"accessibility_score" db:"accessibility_score"`
	VisibilityScore          float64   `json:"visibility_score" db:"visibility_score"`
	HistoricalSignificance   float64   `json:"historical_significance" db:"historical_significance"`
	PhotographicValue        float64   `json:"photographic_value" db:"photographic_value"`
	CurrentConditionScore    float64   `json:"current_condition_score" db:"current_condition_score"`
	ProximityToCityKm        float64   `json:"proximity_to_city_km" db:"proximity_to_city_km"`
	NearbyAmenitiesScore     float64   `json:"nearby_amenities_score" db:"nearby_amenities_score"`
	MaxDailyVisitors         int       `json:"max_daily_visitors" db:"max_daily_visitors"`
	GuidedTourAvailable      bool      `json:"guided_tour_available" db:"guided_tour_available"`
	WheelchairAccessible     bool      `json:"wheelchair_accessible" db:"wheelchair_accessible"`
	PublicTransportAccess    bool      `json:"public_transport_access" db:"public_transport_access"`
	PeakSeason               string    `json:"peak_season,omitempty" db:"peak_season"`
	TourismNotes             string    `json:"tourism_notes,omitempty" db:"tourism_notes"`
	LastUpdated              time.Time `json:"last_updated" db:"last_updated"`
	CreatedAt                time.Time `json:"created_at" db:"created_at"`
	AqueductName             string    `json:"aqueduct_name,omitempty" db:"-"`
	SafetyScore              float64   `json:"safety_score,omitempty" db:"-"`
	RepairCostEstimate       float64   `json:"repair_cost_estimate,omitempty" db:"-"`
	TourismCarryingCapacity  float64   `json:"tourism_carrying_capacity,omitempty" db:"-"`
	OverallPriorityScore     float64   `json:"overall_priority_score,omitempty" db:"-"`
	HeritageType             string    `json:"heritage_type,omitempty" db:"-"`
	HeritageValueScore       float64   `json:"heritage_value_score,omitempty" db:"-"`
	ExpertJudgmentScore      float64   `json:"expert_judgment_score,omitempty" db:"-"`
	HeritageBonus            float64   `json:"heritage_bonus,omitempty" db:"-"`
}

type AqueductComparison struct {
	ID                   uuid.UUID              `json:"id" db:"id"`
	ComparisonName       string                 `json:"comparison_name,omitempty" db:"comparison_name"`
	AqueductIDs          map[string]interface{} `json:"aqueduct_ids" db:"aqueduct_ids"`
	AnalysisTime         time.Time              `json:"analysis_time" db:"analysis_time"`
	StructuralMetrics    map[string]interface{} `json:"structural_metrics,omitempty" db:"structural_metrics"`
	CostMetrics          map[string]interface{} `json:"cost_metrics,omitempty" db:"cost_metrics"`
	TourismMetrics       map[string]interface{} `json:"tourism_metrics,omitempty" db:"tourism_metrics"`
	RadarChartData       map[string]interface{} `json:"radar_chart_data,omitempty" db:"radar_chart_data"`
	PriorityRanking      map[string]interface{} `json:"priority_ranking,omitempty" db:"priority_ranking"`
	OverallScore         map[string]interface{} `json:"overall_score,omitempty" db:"overall_score"`
	RecommendationSummary string                `json:"recommendation_summary,omitempty" db:"recommendation_summary"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
	AqueductsDetail      []AqueductTourismData  `json:"aqueducts_detail,omitempty" db:"-"`
}

// ============================================
// 退化曲线数据点
// ============================================

type DegradationPoint struct {
	Year              int     `json:"year"`
	StrengthRatio     float64 `json:"strength_ratio"`
	ConfidenceLow     float64 `json:"confidence_low,omitempty"`
	ConfidenceHigh    float64 `json:"confidence_high,omitempty"`
}

type InversionFormulaCandidate struct {
	FormulaID      uuid.UUID `json:"formula_id"`
	FormulaName    string    `json:"formula_name"`
	MatchScore     float64   `json:"match_score"`
	Ranking        int       `json:"ranking"`
	SimulatedDepth float64   `json:"simulated_depth"`
	ResidualError  float64   `json:"residual_error"`
}

type SeismicFragilityPoint struct {
	PGA         float64 `json:"pga_g"`
	Magnitude   float64 `json:"magnitude"`
	Slight      float64 `json:"slight_prob"`
	Moderate    float64 `json:"moderate_prob"`
	Extensive   float64 `json:"extensive_prob"`
	Complete    float64 `json:"complete_prob"`
}

type RadarAxis struct {
	Axis   string  `json:"axis"`
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Max    float64 `json:"max"`
}
