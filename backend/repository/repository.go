package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aqueduct-monitor/models"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetPool() *pgxpool.Pool {
	return r.db
}

func (r *Repository) GetAllAqueducts(ctx context.Context) ([]models.Aqueduct, error) {
	query := `SELECT id, name, latin_name, construction_year, length_km, height_m, 
		start_location, end_location, description, geo_path, created_at, updated_at 
		FROM aqueducts ORDER BY construction_year ASC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetAllAqueducts query failed: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.Aqueduct])
}

func (r *Repository) GetAqueductByID(ctx context.Context, id uuid.UUID) (*models.Aqueduct, error) {
	query := `SELECT id, name, latin_name, construction_year, length_km, height_m, 
		start_location, end_location, description, geo_path, created_at, updated_at 
		FROM aqueducts WHERE id = $1`

	row := r.db.QueryRow(ctx, query, id)
	aq, err := pgx.RowToStructByNameLax[models.Aqueduct](row)
	if err != nil {
		return nil, fmt.Errorf("GetAqueductByID failed: %w", err)
	}
	return &aq, nil
}

func (r *Repository) GetSegmentsByAqueduct(ctx context.Context, aqueductID uuid.UUID) ([]models.StructureSegment, error) {
	query := `SELECT id, aqueduct_id, segment_type, segment_index, position_geo, position_3d,
		design_strength, original_material, design_load_capacity, created_at
		FROM structure_segments WHERE aqueduct_id = $1 
		ORDER BY segment_type DESC, segment_index ASC`

	rows, err := r.db.Query(ctx, query, aqueductID)
	if err != nil {
		return nil, fmt.Errorf("GetSegmentsByAqueduct query failed: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.StructureSegment])
}

func (r *Repository) GetAllSegmentsWithStatus(ctx context.Context, aqueductID *uuid.UUID) ([]models.StructureSegment, error) {
	var rows pgx.Rows
	var err error

	if aqueductID != nil {
		query := `
			SELECT s.id, s.aqueduct_id, s.segment_type, s.segment_index, s.position_geo, 
				   s.position_3d, s.design_strength, s.original_material, s.design_load_capacity,
				   COALESCE(e.residual_capacity_ratio, 1.0) as capacity_ratio,
				   COALESCE(e.current_stress, 0) as current_stress,
				   COALESCE(e.weathering_depth, 0) as weathering_depth,
				   COALESCE(e.settlement_mm, 0) as settlement_mm,
				   COALESCE(e.safety_level, 'SAFE') as safety_level,
				   COALESCE(e.residual_strength, s.design_strength) as residual_capacity
			FROM structure_segments s
			LEFT JOIN LATERAL (
				SELECT residual_capacity_ratio, current_stress, weathering_depth, 
					   settlement_mm, safety_level, residual_strength
				FROM structural_evaluations
				WHERE segment_id = s.id
				ORDER BY evaluation_time DESC
				LIMIT 1
			) e ON true
			WHERE s.aqueduct_id = $1
			ORDER BY s.segment_type, s.segment_index
		`
		rows, err = r.db.Query(ctx, query, *aqueductID)
	} else {
		query := `
			SELECT s.id, s.aqueduct_id, s.segment_type, s.segment_index, s.position_geo, 
				   s.position_3d, s.design_strength, s.original_material, s.design_load_capacity,
				   COALESCE(e.residual_capacity_ratio, 1.0) as capacity_ratio,
				   COALESCE(e.current_stress, 0) as current_stress,
				   COALESCE(e.weathering_depth, 0) as weathering_depth,
				   COALESCE(e.settlement_mm, 0) as settlement_mm,
				   COALESCE(e.safety_level, 'SAFE') as safety_level,
				   COALESCE(e.residual_strength, s.design_strength) as residual_capacity
			FROM structure_segments s
			LEFT JOIN LATERAL (
				SELECT residual_capacity_ratio, current_stress, weathering_depth, 
					   settlement_mm, safety_level, residual_strength
				FROM structural_evaluations
				WHERE segment_id = s.id
				ORDER BY evaluation_time DESC
				LIMIT 1
			) e ON true
			ORDER BY s.aqueduct_id, s.segment_type, s.segment_index
		`
		rows, err = r.db.Query(ctx, query)
	}

	if err != nil {
		return nil, fmt.Errorf("GetAllSegmentsWithStatus query failed: %w", err)
	}
	defer rows.Close()

	segments, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.StructureSegment])
	if err != nil {
		return nil, err
	}

	for i := range segments {
		segments[i].WeatheringColor = getWeatheringColor(segments[i].WeatheringDepth)
	}

	return segments, nil
}

func (r *Repository) GetSegmentByIDWithStatus(ctx context.Context, segmentID uuid.UUID) (*models.StructureSegment, error) {
	query := `
		SELECT s.id, s.aqueduct_id, s.segment_type, s.segment_index, s.position_geo, 
			   s.position_3d, s.design_strength, s.original_material, s.design_load_capacity,
			   COALESCE(e.residual_capacity_ratio, 1.0) as capacity_ratio,
			   COALESCE(e.current_stress, 0) as current_stress,
			   COALESCE(e.weathering_depth, 0) as weathering_depth,
			   COALESCE(e.settlement_mm, 0) as settlement_mm,
			   COALESCE(e.safety_level, 'SAFE') as safety_level,
			   COALESCE(e.residual_strength, s.design_strength) as residual_capacity
		FROM structure_segments s
		LEFT JOIN LATERAL (
			SELECT residual_capacity_ratio, current_stress, weathering_depth, 
				   settlement_mm, safety_level, residual_strength
			FROM structural_evaluations
			WHERE segment_id = s.id
			ORDER BY evaluation_time DESC
			LIMIT 1
		) e ON true
		WHERE s.id = $1
	`

	row := r.db.QueryRow(ctx, query, segmentID)
	segment, err := pgx.RowToStructByNameLax[models.StructureSegment](row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetSegmentByIDWithStatus query failed: %w", err)
	}

	segment.WeatheringColor = getWeatheringColor(segment.WeatheringDepth)

	return &segment, nil
}

func (r *Repository) GetSensorByCode(ctx context.Context, code string) (*models.Sensor, error) {
	query := `SELECT id, sensor_code, segment_id, aqueduct_id, sensor_type, location_description,
		installed_date, sampling_interval_sec, calibration_date, is_active, created_at
		FROM sensors WHERE sensor_code = $1 AND is_active = true`

	row := r.db.QueryRow(ctx, query, code)
	s, err := pgx.RowToStructByNameLax[models.Sensor](row)
	if err != nil {
		return nil, fmt.Errorf("GetSensorByCode failed: %w", err)
	}
	return &s, nil
}

func (r *Repository) GetSensorsByAqueduct(ctx context.Context, aqueductID uuid.UUID) ([]models.Sensor, error) {
	query := `SELECT id, sensor_code, segment_id, aqueduct_id, sensor_type, location_description,
		installed_date, sampling_interval_sec, calibration_date, is_active, created_at
		FROM sensors WHERE aqueduct_id = $1 AND is_active = true ORDER BY sensor_type, sensor_code`

	rows, err := r.db.Query(ctx, query, aqueductID)
	if err != nil {
		return nil, fmt.Errorf("GetSensorsByAqueduct query failed: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.Sensor])
}

func (r *Repository) InsertSensorData(ctx context.Context, data []models.SensorData) error {
	if len(data) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, d := range data {
		batch.Queue(`INSERT INTO sensor_data (sensor_id, aqueduct_id, segment_id, sensor_type, timestamp, value, unit, quality, dtu_id, rssi)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (sensor_id, timestamp) DO UPDATE 
			SET value = EXCLUDED.value, quality = EXCLUDED.quality`,
			d.SensorID, d.AqueductID, d.SegmentID, d.SensorType, d.Timestamp,
			d.Value, d.Unit, d.Quality, d.DtuID, d.RSSI)
	}

	br := r.db.SendBatch(ctx, batch)
	defer br.Close()

	for range data {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("InsertSensorData batch exec failed: %w", err)
		}
	}
	return nil
}

func (r *Repository) GetSensorDataTrend(ctx context.Context, sensorID uuid.UUID, start, end time.Time, granularity string) ([]models.TrendDataPoint, error) {
	var bucketExpr string
	switch granularity {
	case "hour":
		bucketExpr = "time_bucket('1 hour', timestamp)"
	case "day":
		bucketExpr = "time_bucket('1 day', timestamp)"
	case "week":
		bucketExpr = "time_bucket('7 days', timestamp)"
	default:
		bucketExpr = "time_bucket('1 day', timestamp)"
	}

	query := fmt.Sprintf(`
		SELECT %s as bucket,
			   AVG(value) as avg_value,
			   MAX(value) as max_value,
			   MIN(value) as min_value,
			   AVG(value) as value
		FROM sensor_data
		WHERE sensor_id = $1 AND timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketExpr)

	rows, err := r.db.Query(ctx, query, sensorID, start, end)
	if err != nil {
		return nil, fmt.Errorf("GetSensorDataTrend query failed: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.TrendDataPoint])
}

func (r *Repository) GetSegmentLatestSensorValues(ctx context.Context, segmentID uuid.UUID) (map[string]float64, error) {
	query := `
		SELECT DISTINCT ON (sensor_type) sensor_type, value
		FROM sensor_data
		WHERE segment_id = $1
		ORDER BY sensor_type, timestamp DESC
	`
	rows, err := r.db.Query(ctx, query, segmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var stype string
		var val float64
		if err := rows.Scan(&stype, &val); err != nil {
			return nil, err
		}
		result[stype] = val
	}
	return result, rows.Err()
}

func (r *Repository) InsertEvaluation(ctx context.Context, eval *models.StructuralEvaluation) error {
	query := `INSERT INTO structural_evaluations 
		(aqueduct_id, segment_id, evaluation_time, current_stress, max_stress, weathering_depth,
		 settlement_mm, residual_strength, residual_capacity_ratio, safety_level, fea_model_data, recommendations)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12)
		RETURNING id`

	var id uuid.UUID
	err := r.db.QueryRow(ctx, query,
		eval.AqueductID, eval.SegmentID, eval.EvaluationTime,
		eval.CurrentStress, eval.MaxStress, eval.WeatheringDepth,
		eval.SettlementMM, eval.ResidualStrength, eval.ResidualCapacityRatio,
		eval.SafetyLevel, eval.FEAModelData, eval.Recommendations,
	).Scan(&id)

	if err == nil {
		eval.ID = id
	}
	return err
}

func (r *Repository) InsertAlert(ctx context.Context, alert *models.Alert) error {
	query := `INSERT INTO alerts 
		(aqueduct_id, segment_id, sensor_id, alert_type, severity, title, description,
		 threshold_value, measured_value, unit, triggered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, query,
		alert.AqueductID, alert.SegmentID, alert.SensorID,
		alert.AlertType, alert.Severity, alert.Title, alert.Description,
		alert.ThresholdValue, alert.MeasuredValue, alert.Unit, alert.TriggeredAt,
	).Scan(&alert.ID, &alert.CreatedAt)

	return err
}

func (r *Repository) UpdateAlertMQTT(ctx context.Context, alertID uuid.UUID, published bool, messageID string) error {
	query := `UPDATE alerts SET mqtt_published = $1, mqtt_message_id = $2 WHERE id = $3`
	_, err := r.db.Exec(ctx, query, published, messageID, alertID)
	return err
}

func (r *Repository) GetActiveAlerts(ctx context.Context, aqueductID *uuid.UUID, limit int) ([]models.Alert, error) {
	var rows pgx.Rows
	var err error

	if aqueductID != nil {
		query := `SELECT a.id, a.aqueduct_id, a.segment_id, a.sensor_id, a.alert_type, 
			a.severity, a.title, a.description, a.threshold_value, a.measured_value, a.unit,
			a.mqtt_published, a.mqtt_message_id, a.acknowledged, a.acknowledged_by, 
			a.acknowledged_at, a.resolution_notes, a.resolved, a.resolved_at, a.triggered_at, a.created_at,
			q.name as aqueduct_name
			FROM alerts a JOIN aqueducts q ON a.aqueduct_id = q.id
			WHERE a.resolved = false AND a.aqueduct_id = $1
			ORDER BY 
				CASE a.severity 
					WHEN 'EMERGENCY' THEN 1 
					WHEN 'CRITICAL' THEN 2 
					WHEN 'WARNING' THEN 3 
					ELSE 4 
				END,
				a.triggered_at DESC
			LIMIT $2`
		rows, err = r.db.Query(ctx, query, *aqueductID, limit)
	} else {
		query := `SELECT a.id, a.aqueduct_id, a.segment_id, a.sensor_id, a.alert_type, 
			a.severity, a.title, a.description, a.threshold_value, a.measured_value, a.unit,
			a.mqtt_published, a.mqtt_message_id, a.acknowledged, a.acknowledged_by, 
			a.acknowledged_at, a.resolution_notes, a.resolved, a.resolved_at, a.triggered_at, a.created_at,
			q.name as aqueduct_name
			FROM alerts a JOIN aqueducts q ON a.aqueduct_id = q.id
			WHERE a.resolved = false
			ORDER BY 
				CASE a.severity 
					WHEN 'EMERGENCY' THEN 1 
					WHEN 'CRITICAL' THEN 2 
					WHEN 'WARNING' THEN 3 
					ELSE 4 
				END,
				a.triggered_at DESC
			LIMIT $1`
		rows, err = r.db.Query(ctx, query, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("GetActiveAlerts query failed: %w", err)
	}
	defer rows.Close()

	alerts, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.Alert])
	if err != nil {
		return nil, err
	}
	return alerts, nil
}

func (r *Repository) GetAllRepairMaterials(ctx context.Context) ([]models.RepairMaterial, error) {
	query := `SELECT id, name, material_type, composition, compressive_strength, tensile_strength,
		elastic_modulus, durability_rating, compatibility_rating, cost_per_unit, unit,
		ease_of_application, environmental_impact, aesthetic_match, description, is_active, created_at
		FROM repair_materials WHERE is_active = true ORDER BY material_type, cost_per_unit`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetAllRepairMaterials query failed: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[models.RepairMaterial])
}

func (r *Repository) InsertRepairRecommendation(ctx context.Context, rec *models.RepairRecommendation) error {
	query := `INSERT INTO repair_recommendations
		(aqueduct_id, segment_id, evaluation_id, recommendation_time, damage_type, damage_severity,
		 expected_cost, expected_lifespan_years, construction_notes, decision_scores)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, query,
		rec.AqueductID, rec.SegmentID, rec.EvaluationID,
		rec.RecommendationTime, rec.DamageType, rec.DamageSeverity,
		rec.ExpectedCost, rec.ExpectedLifespan, rec.ConstructionNotes,
		rec.DecisionScores,
	).Scan(&rec.ID, &rec.CreatedAt)

	return err
}

func (r *Repository) GetStatsSummary(ctx context.Context) (*models.StatsSummary, error) {
	summary := &models.StatsSummary{}

	var query string
	query = `SELECT COUNT(*) FROM aqueducts`
	r.db.QueryRow(ctx, query).Scan(&summary.TotalAqueducts)

	query = `SELECT COUNT(*) FROM structure_segments`
	r.db.QueryRow(ctx, query).Scan(&summary.TotalSegments)

	query = `SELECT COUNT(*) FROM sensors WHERE is_active = true`
	r.db.QueryRow(ctx, query).Scan(&summary.ActiveSensors)

	query = `SELECT 
		COUNT(*) FILTER (WHERE latest.safety_level = 'SAFE') as safe,
		COUNT(*) FILTER (WHERE latest.safety_level = 'WARNING') as warning,
		COUNT(*) FILTER (WHERE latest.safety_level = 'DANGER') as danger,
		COUNT(*) FILTER (WHERE latest.safety_level = 'CRITICAL') as critical,
		AVG(CASE WHEN latest.residual_capacity_ratio IS NOT NULL THEN latest.residual_capacity_ratio ELSE 1.0 END) as avg_ratio
	FROM structure_segments s
	LEFT JOIN LATERAL (
		SELECT safety_level, residual_capacity_ratio
		FROM structural_evaluations WHERE segment_id = s.id
		ORDER BY evaluation_time DESC LIMIT 1
	) latest ON true`
	r.db.QueryRow(ctx, query).Scan(
		&summary.SafeSegments, &summary.WarningSegments,
		&summary.DangerSegments, &summary.CriticalSegments,
		&summary.AvgCapacityRatio,
	)

	query = `SELECT 
		COUNT(*) FILTER (WHERE resolved = false) as active,
		COUNT(*) FILTER (WHERE resolved = false AND severity IN ('CRITICAL','EMERGENCY')) as critical
	FROM alerts`
	r.db.QueryRow(ctx, query).Scan(&summary.ActiveAlerts, &summary.CriticalAlerts)

	query = `SELECT COUNT(*) FROM sensor_data WHERE timestamp > NOW() - INTERVAL '24 hours'`
	r.db.QueryRow(ctx, query).Scan(&summary.DataPointsLast24h)

	return summary, nil
}

func (r *Repository) GetWeatheringRate(ctx context.Context, segmentID uuid.UUID, days int) (float64, error) {
	query := `
		WITH recent AS (
			SELECT timestamp, value,
				   ROW_NUMBER() OVER (ORDER BY timestamp ASC) as rn_asc,
				   ROW_NUMBER() OVER (ORDER BY timestamp DESC) as rn_desc
			FROM sensor_data
			WHERE segment_id = $1 AND sensor_type = 'weathering'
			  AND timestamp > NOW() - ($2 || ' days')::INTERVAL
		)
		SELECT 
			(COALESCE((SELECT value FROM recent WHERE rn_desc = 1), 0) - 
			 COALESCE((SELECT value FROM recent WHERE rn_asc = 1), 0)) / 
			NULLIF(EXTRACT(DAY FROM (
				(SELECT timestamp FROM recent WHERE rn_desc = 1) - 
				(SELECT timestamp FROM recent WHERE rn_asc = 1)
			)), 0) as rate_per_day
	`
	var rate *float64
	err := r.db.QueryRow(ctx, query, segmentID, days).Scan(&rate)
	if err != nil {
		return 0, err
	}
	if rate == nil {
		return 0, nil
	}
	return *rate, nil
}

func getWeatheringColor(depth float64) string {
	switch {
	case depth < 2.0:
		return "#4CAF50"
	case depth < 5.0:
		return "#8BC34A"
	case depth < 10.0:
		return "#FFEB3B"
	case depth < 20.0:
		return "#FF9800"
	case depth < 40.0:
		return "#F44336"
	default:
		return "#880E4F"
	}
}
