package material_predictor

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type PredictorService struct {
	repo *repository.Repository
	cfg  *config.LifetimeConfig
}

func NewPredictorService(repo *repository.Repository, cfg *config.Config) *PredictorService {
	return &PredictorService{repo: repo, cfg: &cfg.Lifetime}
}

func (s *PredictorService) PredictMaterialLifetime(
	ctx context.Context,
	materialID uuid.UUID,
	scenario string,
) (*models.MaterialLifetimePrediction, error) {

	mat, err := s.getMaterialByID(ctx, materialID)
	if err != nil {
		return nil, err
	}

	pred, err := PredictMaterialLifetime(
		mat, scenario, s.cfg.ThresholdStrengthRatio, s.cfg.SimulationYears, s.cfg,
	)
	if err != nil {
		return nil, err
	}

	totalYears := s.cfg.SimulationYears
	curve := make([]models.DegradationPoint, len(pred.DegradationCurve))
	for i, p := range pred.DegradationCurve {
		se := StandardError(i, GenerateAcceleratedAgingData(mat))
		z := s.cfg.ConfidenceZScore
		low := math.Max(0.05, p.StrengthRatio-z*se)
		high := math.Min(1.0, p.StrengthRatio+z*se)
		curve[i] = models.DegradationPoint{
			Year:              i,
			StrengthRatio:     p.StrengthRatio,
			ConfidenceLow:     Round4(low),
			ConfidenceHigh:    Round4(high),
			EquivalentAgeDays: p.EquivalentAgeDays,
			TemperatureC:      p.TemperatureC,
		}
	}

	strengthAt50 := 0.0
	if 50 < len(curve) {
		strengthAt50 = curve[50].StrengthRatio
	}
	strengthAt100 := 0.0
	if 100 < len(curve) {
		strengthAt100 = curve[100].StrengthRatio
	}

	serviceLife := pred.ServiceLifeYears
	validityYears := int(math.Floor(serviceLife * 0.6))
	refTempK := s.cfg.ReferenceTemperatureC + 273.15

	correctionInfo := map[string]interface{}{
		"accelerated_to_natural_factor": Round4(s.cfg.AcceleratedToNaturalFactor),
		"longterm_exposure_calibration": Round4(s.cfg.LongTermExposureCalibration),
		"natural_aging_bias_correction": Round4(s.cfg.NaturalAgingBiasCorrection),
		"outdoor_exposure_factor":       Round4(s.cfg.OutdoorExposureFactor),
		"threshold_safety_factor":       Round4(s.cfg.ThresholdSafetyFactor),
		"corrected_threshold_ratio":     Round4(s.cfg.ThresholdStrengthRatio * s.cfg.ThresholdSafetyFactor),
		"model_conservative_nature":     "加速老化外推已应用保守修正，预测值偏保守",
	}

	scFactor := ScenarioFactor(scenario)
	timeTempShift := map[string]interface{}{
		"scenario_factor":       scFactor,
		"activation_energy_ev":  pred.ActivationEnergyEV,
		"ref_temperature_c":     s.cfg.ReferenceTemperatureC,
		"scenario_temp_avg_c":   ScenarioAvgTemp(scenario, s.cfg),
		"time_accel_factor_80C": Round4(ArrheniusFactor(pred.ActivationEnergyEV, s.cfg.AcceleratedTempHighC+273.15, refTempK)),
		"time_accel_factor_40C": Round4(ArrheniusFactor(pred.ActivationEnergyEV, s.cfg.AcceleratedTempLowC+273.15, refTempK)),
		"correction_info":       correctionInfo,
	}

	curveJSON := map[string]interface{}{
		"years_total":     totalYears,
		"points":          curve,
		"threshold_ratio": s.cfg.ThresholdStrengthRatio,
	}

	assumptions := "Arrhenius方程: k=A·exp(-Ea/RT); 时间-温度叠加原理(TTSP); 阈值=" +
		"保留强度>50%为有效; 包含湿度和冻融加速因子。" +
		" 已应用加速-自然老化修正因子(" + formatFloat(s.cfg.AcceleratedToNaturalFactor) +
		")和长期暴露校准(" + formatFloat(s.cfg.LongTermExposureCalibration) + ");" +
		" 安全因子=" + formatFloat(s.cfg.ThresholdSafetyFactor) + ", 预测偏保守。"

	result := &models.MaterialLifetimePrediction{
		ID:                     uuid.New(),
		MaterialID:             materialID,
		PredictionTime:         time.Now().UTC(),
		Scenario:               scenario,
		PredictionYears:        totalYears,
		ArrheniusActivationEV:  pred.ActivationEnergyEV,
		TimeTempShiftFactor:    timeTempShift,
		DegradationCurve:       curveJSON,
		StrengthAt50Yr:         Round4(strengthAt50),
		StrengthAt100Yr:        Round4(strengthAt100),
		EstimatedServiceLife:   Round1(serviceLife),
		ThresholdStrengthRatio: s.cfg.ThresholdStrengthRatio,
		ConfidenceIntervalLow:  Round4(math.Max(0.05, strengthAt50-0.1)),
		ConfidenceIntervalHigh: Round4(math.Min(1.0, strengthAt50+0.1)),
		ModelAssumptions:       assumptions,
		CreatedAt:              time.Now().UTC(),
		MaterialName:           mat.Name,
		MaterialType:           mat.MaterialType,
		RepairValidityYears:    validityYears,
	}

	return result, nil
}

func (s *PredictorService) getMaterialByID(ctx context.Context, id uuid.UUID) (*models.RepairMaterial, error) {
	list, err := s.repo.GetAllRepairMaterials(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return &models.RepairMaterial{
		ID: id, Name: "默认修复材料", MaterialType: "MODERN_CEMENT",
		CompressiveStrength: 45.0, DurabilityRating: 0.75,
	}, nil
}

func (s *PredictorService) CalibrateActivationEnergy(
	ctx context.Context,
	materialType string,
	agingData []models.AcceleratedAgingData,
) (float64, error) {
	mpData := make([]AgingDataPoint, len(agingData))
	for i, d := range agingData {
		mpData[i] = AgingDataPoint{
			TempC:       d.TemperatureC,
			HumidityPct: d.HumidityPct,
			Days:        d.TimeHours / 24,
			Cycles:      d.FreezeThawCycles,
			Retention:   d.RetainedRate,
		}
	}
	return CalibrateActivationEnergy(mpData, s.cfg), nil
}

func (s *PredictorService) SavePrediction(ctx context.Context, pred *models.MaterialLifetimePrediction) error {
	return s.repo.InsertLifetimePrediction(ctx, pred)
}

func (s *PredictorService) GetPredictionsBySegment(ctx context.Context, segmentID uuid.UUID, limit int) ([]models.MaterialLifetimePrediction, error) {
	return s.repo.GetLifetimePredictionsBySegment(ctx, segmentID, limit)
}

func (s *PredictorService) GetPredictionsByAqueduct(ctx context.Context, aqueductID uuid.UUID, limit int) ([]models.MaterialLifetimePrediction, error) {
	return s.repo.GetLifetimePredictionsByAqueduct(ctx, aqueductID, limit)
}

func (s *PredictorService) CompareMaterials(
	ctx context.Context,
	materialIDs []uuid.UUID,
	scenario string,
) ([]map[string]interface{}, error) {
	results := make([]map[string]interface{}, 0, len(materialIDs))

	for _, matID := range materialIDs {
		pred, err := s.PredictMaterialLifetime(ctx, matID, scenario)
		if err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"material_id":         matID,
			"material_name":       pred.MaterialName,
			"material_type":       pred.MaterialType,
			"service_life_yrs":    pred.EstimatedServiceLife,
			"strength_at_50yr":    pred.StrengthAt50Yr,
			"activation_energy":   pred.ArrheniusActivationEV,
			"repair_validity_yrs": pred.RepairValidityYears,
		})
	}

	return results, nil
}

func formatFloat(v float64) string {
	if math.Abs(v-math.Trunc(v)) < 0.01 {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func GenerateRepairRecommendation(pred *LifetimePrediction) string {
	if pred.ServiceLifeYears < 10 {
		return "使用寿命不足10年，建议选择更高耐久性材料或增加防护层"
	} else if pred.ServiceLifeYears < 25 {
		return "使用寿命中等，建议每10年进行一次检测评估"
	} else if pred.ServiceLifeYears < 50 {
		return "使用寿命良好，适合一般遗产修复工程"
	}
	return "使用寿命优异，适合重要遗产结构的长期修复"
}
