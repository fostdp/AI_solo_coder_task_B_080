package lifetime

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/material_predictor"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type LifetimePredictor struct {
	repo *repository.Repository
	cfg  *config.LifetimeConfig
}

func NewLifetimePredictor(repo *repository.Repository, cfg *config.Config) *LifetimePredictor {
	return &LifetimePredictor{repo: repo, cfg: &cfg.Lifetime}
}

type agingDataPoint struct {
	tempC       float64
	humidityPct float64
	days        int
	cycles      int
	retention   float64
}

func (lp *LifetimePredictor) PredictMaterialLifetime(
	ctx context.Context,
	materialID uuid.UUID,
	scenario string,
) (*models.MaterialLifetimePrediction, error) {

	mat, err := lp.getMaterialByID(ctx, materialID)
	if err != nil {
		return nil, err
	}

	pred, err := material_predictor.PredictMaterialLifetime(
		mat, scenario, lp.cfg.ThresholdStrengthRatio, lp.cfg.SimulationYears, lp.cfg,
	)
	if err != nil {
		return nil, err
	}

	totalYears := lp.cfg.SimulationYears
	curve := make([]models.DegradationPoint, len(pred.DegradationCurve))
	for i, p := range pred.DegradationCurve {
		se := material_predictor.StandardError(i, material_predictor.GenerateAcceleratedAgingData(mat))
		z := lp.cfg.ConfidenceZScore
		low := math.Max(0.05, p.StrengthRatio-z*se)
		high := math.Min(1.0, p.StrengthRatio+z*se)
		curve[i] = models.DegradationPoint{
			Year:              i,
			StrengthRatio:     p.StrengthRatio,
			ConfidenceLow:     material_predictor.Round4(low),
			ConfidenceHigh:    material_predictor.Round4(high),
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
	refTempK := lp.cfg.ReferenceTemperatureC + 273.15

	correctionInfo := map[string]interface{}{
		"accelerated_to_natural_factor": material_predictor.Round4(lp.cfg.AcceleratedToNaturalFactor),
		"longterm_exposure_calibration": material_predictor.Round4(lp.cfg.LongTermExposureCalibration),
		"natural_aging_bias_correction": material_predictor.Round4(lp.cfg.NaturalAgingBiasCorrection),
		"outdoor_exposure_factor":       material_predictor.Round4(lp.cfg.OutdoorExposureFactor),
		"threshold_safety_factor":       material_predictor.Round4(lp.cfg.ThresholdSafetyFactor),
		"corrected_threshold_ratio":     material_predictor.Round4(lp.cfg.ThresholdStrengthRatio * lp.cfg.ThresholdSafetyFactor),
		"model_conservative_nature":     "加速老化外推已应用保守修正，预测值偏保守",
	}

	scFactor := material_predictor.ScenarioFactor(scenario)
	timeTempShift := map[string]interface{}{
		"scenario_factor":       scFactor,
		"activation_energy_ev":  pred.ActivationEnergyEV,
		"ref_temperature_c":     lp.cfg.ReferenceTemperatureC,
		"scenario_temp_avg_c":   material_predictor.ScenarioAvgTemp(scenario, lp.cfg),
		"time_accel_factor_80C": material_predictor.Round4(material_predictor.ArrheniusFactor(pred.ActivationEnergyEV, lp.cfg.AcceleratedTempHighC+273.15, refTempK)),
		"time_accel_factor_40C": material_predictor.Round4(material_predictor.ArrheniusFactor(pred.ActivationEnergyEV, lp.cfg.AcceleratedTempLowC+273.15, refTempK)),
		"correction_info":       correctionInfo,
	}

	curveJSON := map[string]interface{}{
		"years_total":     totalYears,
		"points":          curve,
		"threshold_ratio": lp.cfg.ThresholdStrengthRatio,
	}

	assumptions := "Arrhenius方程: k=A·exp(-Ea/RT); 时间-温度叠加原理(TTSP); 阈值=" +
		"保留强度>50%为有效; 包含湿度和冻融加速因子。" +
		" 已应用加速-自然老化修正因子(" + formatFloat(lp.cfg.AcceleratedToNaturalFactor) +
		")和长期暴露校准(" + formatFloat(lp.cfg.LongTermExposureCalibration) + ");" +
		" 安全因子=" + formatFloat(lp.cfg.ThresholdSafetyFactor) + ", 预测偏保守。"

	result := &models.MaterialLifetimePrediction{
		ID:                     uuid.New(),
		MaterialID:             materialID,
		PredictionTime:         time.Now().UTC(),
		Scenario:               scenario,
		PredictionYears:        totalYears,
		ArrheniusActivationEV:  pred.ActivationEnergyEV,
		TimeTempShiftFactor:    timeTempShift,
		DegradationCurve:       curveJSON,
		StrengthAt50Yr:         material_predictor.Round4(strengthAt50),
		StrengthAt100Yr:        material_predictor.Round4(strengthAt100),
		EstimatedServiceLife:   material_predictor.Round1(serviceLife),
		ThresholdStrengthRatio: lp.cfg.ThresholdStrengthRatio,
		ConfidenceIntervalLow:  material_predictor.Round4(math.Max(0.05, strengthAt50-0.1)),
		ConfidenceIntervalHigh: material_predictor.Round4(math.Min(1.0, strengthAt50+0.1)),
		ModelAssumptions:       assumptions,
		CreatedAt:              time.Now().UTC(),
		MaterialName:           mat.Name,
		MaterialType:           mat.MaterialType,
		RepairValidityYears:    validityYears,
	}

	return result, nil
}

func (lp *LifetimePredictor) calibrateActivationEnergy(data []agingDataPoint) float64 {
	mpData := make([]material_predictor.AgingDataPoint, len(data))
	for i, d := range data {
		mpData[i] = material_predictor.AgingDataPoint{
			TempC:       d.tempC,
			HumidityPct: d.humidityPct,
			Days:        d.days,
			Cycles:      d.cycles,
			Retention:   d.retention,
		}
	}
	return material_predictor.CalibrateActivationEnergy(mpData, lp.cfg)
}

func (lp *LifetimePredictor) arrheniusFactor(ea float64, tempK, refTempK float64) float64 {
	return material_predictor.ArrheniusFactor(ea, tempK, refTempK)
}

func (lp *LifetimePredictor) computeEquivalentTime(
	actualYears float64, ea, refTempK float64, scenario string, scenarioFactor float64,
) float64 {
	return material_predictor.ComputeEquivalentTime(actualYears, ea, refTempK, scenario, scenarioFactor, lp.cfg)
}

func (lp *LifetimePredictor) degradationModel(equivYears, baseDurability float64, matType string) float64 {
	return material_predictor.DegradationModel(equivYears, baseDurability, matType, lp.cfg)
}

func (lp *LifetimePredictor) standardError(year int, data []agingDataPoint) float64 {
	mpData := make([]material_predictor.AgingDataPoint, len(data))
	for i, d := range data {
		mpData[i] = material_predictor.AgingDataPoint{
			TempC:       d.tempC,
			HumidityPct: d.humidityPct,
			Days:        d.days,
			Cycles:      d.cycles,
			Retention:   d.retention,
		}
	}
	return material_predictor.StandardError(year, mpData)
}

func (lp *LifetimePredictor) estimateServiceLife(curve []models.DegradationPoint, threshold float64) float64 {
	return material_predictor.EstimateServiceLife(curve, threshold, lp.cfg)
}

func (lp *LifetimePredictor) scenarioFactor(scenario string) float64 {
	return material_predictor.ScenarioFactor(scenario)
}

func (lp *LifetimePredictor) scenarioAvgTemp(scenario string) float64 {
	return material_predictor.ScenarioAvgTemp(scenario, lp.cfg)
}

func (lp *LifetimePredictor) scenarioAvgHumidity(scenario string) float64 {
	return material_predictor.ScenarioAvgHumidity(scenario)
}

func (lp *LifetimePredictor) getMaterialByID(ctx context.Context, id uuid.UUID) (*models.RepairMaterial, error) {
	list, err := lp.repo.GetAllRepairMaterials(ctx)
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

func (lp *LifetimePredictor) generateAcceleratedAgingData(mat *models.RepairMaterial) []agingDataPoint {
	mpData := material_predictor.GenerateAcceleratedAgingData(mat)
	result := make([]agingDataPoint, len(mpData))
	for i, d := range mpData {
		result[i] = agingDataPoint{
			tempC:       d.TempC,
			humidityPct: d.HumidityPct,
			days:        d.Days,
			cycles:      d.Cycles,
			retention:   d.Retention,
		}
	}
	return result
}

func round1(v float64) float64 { return material_predictor.Round1(v) }
func round4(v float64) float64 { return material_predictor.Round4(v) }
func formatFloat(v float64) string {
	if math.Abs(v-math.Trunc(v)) < 0.01 {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}
