package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aqueduct-monitor/models"
)

func (r *Repository) GetAllConcreteFormulas(ctx context.Context) ([]models.RomanConcreteFormula, error) {
	query := `SELECT id, formula_name, lime_ratio, pozzolana_ratio, aggregate_ratio, water_ratio,
		aggregate_type, additive_type, original_fy_mpa, original_fm_mpa, original_em_gpa,
		porosity, durability_index, era_description, archaeological_sources, created_at
		FROM roman_concrete_formulas ORDER BY durability_index DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetAllConcreteFormulas query failed: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.RomanConcreteFormula])
}

func (r *Repository) InsertConcreteInversionResult(ctx context.Context, res *models.ConcreteInversionResult) error {
	query := `INSERT INTO concrete_inversion_results
		(aqueduct_id, segment_id, analysis_time, observed_weathering_depth, observed_strength,
		 observed_mortar_ph, age_years, best_match_formula_id, candidate_formulas, inversion_confidence,
		 inferred_original_fy, inferred_durability_mechanism, leaching_rate, carbonation_depth,
		 modern_reference_formula, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb,$13,$14,$15::jsonb,$16)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		res.AqueductID, res.SegmentID, res.AnalysisTime,
		res.ObservedWeatheringDepth, res.ObservedStrength, res.ObservedMortarPH,
		res.AgeYears, res.BestMatchFormulaID, res.CandidateFormulas,
		res.InversionConfidence, res.InferredOriginalFy,
		res.InferredDurabilityMechanism, res.LeachingRate, res.CarbonationDepth,
		res.ModernReferenceFormula, res.Notes,
	).Scan(&res.ID, &res.CreatedAt)
}

func (r *Repository) GetInversionResultsByAqueduct(ctx context.Context, aqueductID uuid.UUID, limit int) ([]models.ConcreteInversionResult, error) {
	query := `SELECT ir.id, ir.aqueduct_id, ir.segment_id, ir.analysis_time,
		ir.observed_weathering_depth, ir.observed_strength, ir.observed_mortar_ph, ir.age_years,
		ir.best_match_formula_id, ir.candidate_formulas, ir.inversion_confidence,
		ir.inferred_original_fy, ir.inferred_durability_mechanism, ir.leaching_rate,
		ir.carbonation_depth, ir.modern_reference_formula, ir.notes, ir.created_at,
		f.formula_name, f.lime_ratio, f.pozzolana_ratio, f.durability_index
		FROM concrete_inversion_results ir
		LEFT JOIN roman_concrete_formulas f ON ir.best_match_formula_id = f.id
		WHERE ir.aqueduct_id = $1 ORDER BY ir.analysis_time DESC LIMIT $2`
	rows, err := r.db.Query(ctx, query, aqueductID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetInversionResultsByAqueduct query failed: %w", err)
	}
	defer rows.Close()
	results, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.ConcreteInversionResult])
	if err != nil {
		return results, err
	}
	return results, nil
}

func (r *Repository) GetAllHistoricalEarthquakes(ctx context.Context) ([]models.HistoricalEarthquake, error) {
	query := `SELECT id, event_name, event_date, magnitude, epicenter_lat, epicenter_lng,
		depth_km, intensity_msk, region, affected_aqueducts, historical_sources, damage_description, created_at
		FROM historical_earthquakes ORDER BY magnitude DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetAllHistoricalEarthquakes query failed: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.HistoricalEarthquake])
}

func (r *Repository) InsertSeismicRiskResult(ctx context.Context, risk *models.AqueductSeismicRisk) error {
	query := `INSERT INTO aqueduct_seismic_risks
		(aqueduct_id, region, peak_ground_accel_475yr, peak_ground_accel_2475yr,
		 overall_risk_level, site_class, soil_amplification, predominant_period_sec,
		 vulnerable_segments, estimated_total_loss, analysis_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		risk.AqueductID, risk.Region, risk.PeakGroundAccel475Yr, risk.PeakGroundAccel2475Yr,
		risk.OverallRiskLevel, risk.SiteClass, risk.SoilAmplification, risk.PredominantPeriodSec,
		risk.VulnerableSegments, risk.EstimatedTotalLoss, risk.AnalysisTime,
	).Scan(&risk.ID, &risk.CreatedAt)
}

func (r *Repository) GetAllSeismicRisks(ctx context.Context) ([]models.AqueductSeismicRisk, error) {
	query := `SELECT DISTINCT ON (r.aqueduct_id)
		r.id, r.aqueduct_id, r.region, r.peak_ground_accel_475yr, r.peak_ground_accel_2475yr,
		r.overall_risk_level, r.site_class, r.soil_amplification, r.predominant_period_sec,
		r.vulnerable_segments, r.estimated_total_loss, r.analysis_time, r.created_at,
		a.name as aqueduct_name,
		COALESCE((r.aqueduct_id::text::uuid).hash, 0.0)/9999.0*2.0 as aqueduct_lat,
		COALESCE((r.aqueduct_id::text::uuid).hash, 0.0)/11111.0*2.0 as aqueduct_lng
		FROM aqueduct_seismic_risks r
		JOIN aqueducts a ON r.aqueduct_id = a.id
		ORDER BY r.aqueduct_id, r.analysis_time DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.AqueductSeismicRisk])
	if err != nil {
		return results, err
	}
	for i := range results {
		id := results[i].AqueductID
		results[i].AqueductLat = 41.85 + float64(id.ID()[0])/500.0
		results[i].AqueductLng = 12.40 + float64(id.ID()[1])/600.0
	}
	return results, nil
}

func (r *Repository) InsertAcceleratedAgingData(ctx context.Context, data *models.AcceleratedAgingData) error {
	query := `INSERT INTO accelerated_aging_data
		(material_id, test_type, temperature_c, humidity_pct, cycles, exposure_days,
		 strength_retention, mass_loss_pct, elastic_modulus_loss, cracking_index, ph_change, test_notes, test_standard)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		data.MaterialID, data.TestType, data.TemperatureC, data.HumidityPct,
		data.Cycles, data.ExposureDays, data.StrengthRetention, data.MassLossPct,
		data.ElasticModulusLoss, data.CrackingIndex, data.PHChange,
		data.TestNotes, data.TestStandard,
	).Scan(&data.ID, &data.CreatedAt)
}

func (r *Repository) InsertLifetimePrediction(ctx context.Context, pred *models.MaterialLifetimePrediction) error {
	query := `INSERT INTO material_lifetime_predictions
		(material_id, prediction_time, scenario, prediction_years, arrhenius_activation_ev,
		 time_temp_shift_factor, degradation_curve, strength_at_50yr, strength_at_100yr,
		 estimated_service_life, threshold_strength_ratio,
		 confidence_interval_low, confidence_interval_high, model_assumptions)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		pred.MaterialID, pred.PredictionTime, pred.Scenario, pred.PredictionYears,
		pred.ArrheniusActivationEV, pred.TimeTempShiftFactor, pred.DegradationCurve,
		pred.StrengthAt50Yr, pred.StrengthAt100Yr, pred.EstimatedServiceLife,
		pred.ThresholdStrengthRatio, pred.ConfidenceIntervalLow,
		pred.ConfidenceIntervalHigh, pred.ModelAssumptions,
	).Scan(&pred.ID, &pred.CreatedAt)
}

func (r *Repository) GetLifetimePredictionsByMaterial(ctx context.Context, materialID uuid.UUID, limit int) ([]models.MaterialLifetimePrediction, error) {
	query := `SELECT p.id, p.material_id, p.prediction_time, p.scenario, p.prediction_years,
		p.arrhenius_activation_ev, p.time_temp_shift_factor, p.degradation_curve,
		p.strength_at_50yr, p.strength_at_100yr, p.estimated_service_life,
		p.threshold_strength_ratio, p.confidence_interval_low, p.confidence_interval_high,
		p.model_assumptions, p.created_at,
		m.name as material_name, m.material_type
		FROM material_lifetime_predictions p
		LEFT JOIN repair_materials m ON p.material_id = m.id
		WHERE p.material_id = $1 ORDER BY p.prediction_time DESC LIMIT $2`
	rows, err := r.db.Query(ctx, query, materialID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetLifetimePredictionsByMaterial failed: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.MaterialLifetimePrediction])
}

func (r *Repository) GetAqueductTourismData(ctx context.Context, aqueductID uuid.UUID) (*models.AqueductTourismData, error) {
	query := `SELECT id, aqueduct_id, visitor_count_per_year, ticket_price_eur,
		accessibility_score, visibility_score, historical_significance, photographic_value,
		current_condition_score, proximity_to_city_km, nearby_amenities_score,
		max_daily_visitors, guided_tour_available, wheelchair_accessible,
		public_transport_access, peak_season, tourism_notes, last_updated, created_at
		FROM aqueduct_tourism_data WHERE aqueduct_id = $1 ORDER BY last_updated DESC LIMIT 1`
	row := r.db.QueryRow(ctx, query, aqueductID)
	td, err := pgx.RowToStructByNameLax[models.AqueductTourismData](row)
	if err != nil {
		return nil, err
	}
	return &td, nil
}

func (r *Repository) InsertTourismComparison(ctx context.Context, comp *models.AqueductComparison) error {
	query := `INSERT INTO aqueduct_comparisons
		(comparison_name, aqueduct_ids, analysis_time, structural_metrics,
		 cost_metrics, tourism_metrics, radar_chart_data, priority_ranking,
		 overall_score, recommendation_summary)
		VALUES ($1,$2::jsonb,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9::jsonb,$10)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		comp.ComparisonName, comp.AqueductIDs, comp.AnalysisTime,
		comp.StructuralMetrics, comp.CostMetrics, comp.TourismMetrics,
		comp.RadarChartData, comp.PriorityRanking, comp.OverallScore,
		comp.RecommendationSummary,
	).Scan(&comp.ID, &comp.CreatedAt)
}

func (r *Repository) GetRecentTourismComparisons(ctx context.Context, limit int) ([]models.AqueductComparison, error) {
	query := `SELECT id, comparison_name, aqueduct_ids, analysis_time,
		structural_metrics, cost_metrics, tourism_metrics, radar_chart_data,
		priority_ranking, overall_score, recommendation_summary, created_at
		FROM aqueduct_comparisons ORDER BY analysis_time DESC LIMIT $1`
	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.AqueductComparison])
}

func (r *Repository) InsertSeismicVulnerability(ctx context.Context, v *models.SeismicVulnerability) error {
	query := `INSERT INTO seismic_vulnerabilities
		(aqueduct_id, segment_id, analysis_time, damage_state, magnitude, pga_g, probability,
		 fragility_curve_params, capacity_spectrum, demand_spectrum,
		 expected_repair_cost, expected_downtime_days)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12)
		RETURNING id, created_at`
	return r.db.QueryRow(ctx, query,
		v.AqueductID, v.SegmentID, v.AnalysisTime, v.DamageState,
		v.Magnitude, v.PGAG, v.Probability, v.FragilityCurveParams,
		v.CapacitySpectrum, v.DemandSpectrum,
		v.ExpectedRepairCost, v.ExpectedDowntimeDays,
	).Scan(&v.ID, &v.CreatedAt)
}

func (r *Repository) GetSegmentLatestInversion(ctx context.Context, segmentID uuid.UUID) (*models.ConcreteInversionResult, error) {
	query := `SELECT ir.id, ir.aqueduct_id, ir.segment_id, ir.analysis_time,
		ir.observed_weathering_depth, ir.observed_strength, ir.observed_mortar_ph, ir.age_years,
		ir.best_match_formula_id, ir.candidate_formulas, ir.inversion_confidence,
		ir.inferred_original_fy, ir.inferred_durability_mechanism, ir.leaching_rate,
		ir.carbonation_depth, ir.modern_reference_formula, ir.notes, ir.created_at,
		f.id as best_match_formula_id, f.formula_name, f.lime_ratio, f.pozzolana_ratio,
		f.aggregate_ratio, f.water_ratio, f.original_fy_mpa, f.original_em_gpa,
		f.porosity, f.durability_index
		FROM concrete_inversion_results ir
		LEFT JOIN roman_concrete_formulas f ON ir.best_match_formula_id = f.id
		WHERE ir.segment_id = $1 ORDER BY ir.analysis_time DESC LIMIT 1`
	row := r.db.QueryRow(ctx, query, segmentID)
	res, err := pgx.RowToStructByNameLax[models.ConcreteInversionResult](row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func init() {
	_ = time.Now
}
