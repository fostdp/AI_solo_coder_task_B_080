package material_predictor

import (
	"math"
	"sort"
	"strconv"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

type AgingDataPoint struct {
	TempC       float64
	HumidityPct float64
	Days        int
	Cycles      int
	Retention   float64
}

type PredictionConfig struct {
	MatType       string
	Scenario      string
	Threshold     float64
	ForecastYears int
	BaseData      []AgingDataPoint
}

type LifetimePrediction struct {
	ServiceLifeYears    float64
	DegradationCurve    []models.DegradationPoint
	ConfidenceBounds    [][]float64
	ActivationEnergyEV  float64
	AcceleratedFactor   float64
	KeyMetrics          map[string]interface{}
	Notes               string
}

func CalibrateActivationEnergy(data []AgingDataPoint, cfg *config.LifetimeConfig) float64 {
	if len(data) < 3 {
		return cfg.ArrheniusActivationEV
	}
	sort.Slice(data, func(a, b int) bool { return data[a].TempC < data[b].TempC })

	k := make([]float64, len(data))
	tempInvK := make([]float64, len(data))
	for i, d := range data {
		k[i] = -math.Log(math.Max(0.05, d.Retention)) / math.Max(1.0, float64(d.Days))
		tempInvK[i] = 1.0 / (d.TempC + 273.15)
	}

	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	n := float64(len(data))
	for i := range data {
		sumX += tempInvK[i]
		sumY += math.Log(math.Max(1e-8, k[i]))
		sumXY += tempInvK[i] * math.Log(math.Max(1e-8, k[i]))
		sumX2 += tempInvK[i] * tempInvK[i]
	}
	denom := n*sumX2 - sumX*sumX
	if denom < 1e-12 {
		return cfg.ArrheniusActivationEV
	}
	slope := (n*sumXY - sumX*sumY) / denom
	R_K := 8.617333262e-5
	ea := -slope * R_K

	if cfg.NaturalAgingBiasCorrection > 0 {
		biasFactor := cfg.NaturalAgingBiasCorrection
		ea = ea * biasFactor
	}

	if ea < 0.4 || ea > 1.8 {
		ea = cfg.ArrheniusActivationEV
	}
	return ea
}

func ArrheniusFactor(ea float64, tempK, refTempK float64) float64 {
	R := 8.617333262e-5
	return math.Exp(-ea / R * (1.0/tempK - 1.0/refTempK))
}

func ComputeEquivalentTime(
	actualYears float64, ea, refTempK float64, scenario string, scenarioFactor float64, cfg *config.LifetimeConfig,
) float64 {
	scenarioTempC := ScenarioAvgTemp(scenario, cfg)
	scenarioHumidity := ScenarioAvgHumidity(scenario)
	tempK := scenarioTempC + 273.15
	tempFactor := ArrheniusFactor(ea, tempK, refTempK)

	humidityFactor := 1.0 + 0.02*math.Max(0, scenarioHumidity-60.0)/20.0
	humidityFactor = math.Min(2.5, humidityFactor*cfg.HumidityAcceleration)

	ftFactor := 1.0
	if scenario == "alpine" || scenario == "continental" {
		ftFactor = cfg.FreezeThawAcceleration
	}

	accToNatFactor := cfg.AcceleratedToNaturalFactor
	if accToNatFactor <= 0 {
		accToNatFactor = 1.0
	}

	totalFactor := tempFactor * humidityFactor * ftFactor * scenarioFactor * accToNatFactor
	return actualYears * totalFactor
}

func DegradationModel(equivYears, baseDurability float64, matType string, cfg *config.LifetimeConfig) float64 {
	years := equivYears
	var alpha, beta float64
	switch matType {
	case "ROMAN_CONCRETE", "LIME_MORTAR":
		alpha = 0.02
		beta = 0.65
	case "MODERN_CEMENT":
		alpha = 0.035
		beta = 0.75
	case "FRP", "CFRP", "GFRP":
		alpha = 0.015
		beta = 0.90
	case "EPOXY":
		alpha = 0.055
		beta = 0.70
	case "INJECTION_GROUT":
		alpha = 0.04
		beta = 0.72
	default:
		alpha = 0.03
		beta = 0.75
	}

	ltcFactor := cfg.LongTermExposureCalibration
	if ltcFactor > 0 {
		ltcExponent := 1.0 - math.Min(0.3, ltcFactor*0.2)
		beta = beta * ltcExponent
	}

	outdoorFactor := cfg.OutdoorExposureFactor
	if outdoorFactor > 0 && outdoorFactor != 1.0 {
		alpha = alpha * outdoorFactor
	}

	durBoost := 0.3 * math.Max(0, baseDurability-0.5)
	retention := 1.0 - (alpha*math.Pow(years/50.0, beta) + durBoost*alpha)
	return math.Max(0.05, math.Min(1.0, retention))
}

func StandardError(year int, data []AgingDataPoint) float64 {
	y := float64(year)
	baseErr := 0.03 + 0.003*math.Sqrt(y)
	dataSpread := 0.01
	if len(data) >= 2 {
		avgRet := 0.0
		for _, d := range data {
			avgRet += d.Retention
		}
		avgRet /= float64(len(data))
		varS := 0.0
		for _, d := range data {
			varS += (d.Retention - avgRet) * (d.Retention - avgRet)
		}
		dataSpread = math.Sqrt(varS / float64(len(data)))
	}
	return baseErr + 0.5*dataSpread
}

func EstimateServiceLife(curve []models.DegradationPoint, threshold float64, cfg *config.LifetimeConfig) float64 {
	safetyFactor := cfg.ThresholdSafetyFactor
	if safetyFactor <= 0 {
		safetyFactor = 1.0
	}
	correctedThreshold := threshold * safetyFactor

	for i := 1; i < len(curve); i++ {
		prev := curve[i-1]
		cur := curve[i]
		if prev.StrengthRatio >= correctedThreshold && cur.StrengthRatio <= correctedThreshold {
			if prev.StrengthRatio == cur.StrengthRatio {
				return float64(i)
			}
			t := (correctedThreshold - prev.StrengthRatio) / (cur.StrengthRatio - prev.StrengthRatio)
			return float64(i-1) + math.Max(0, math.Min(1, t))
		}
	}
	return float64(len(curve) - 1)
}

func ScenarioFactor(scenario string) float64 {
	switch scenario {
	case "temperate_coastal":
		return 1.25
	case "mediterranean":
		return 1.0
	case "continental":
		return 1.35
	case "alpine":
		return 1.55
	case "tropical_humid":
		return 1.75
	case "arid_desert":
		return 0.75
	case "urban_polluted":
		return 1.45
	case "underwater_saline":
		return 1.60
	case "laboratory_control":
		return 0.55
	default:
		return 1.0
	}
}

func ScenarioAvgTemp(scenario string, cfg *config.LifetimeConfig) float64 {
	switch scenario {
	case "temperate_coastal":
		return 14.0
	case "mediterranean":
		return 18.0
	case "continental":
		return 10.0
	case "alpine":
		return 5.0
	case "tropical_humid":
		return 26.0
	case "arid_desert":
		return 22.0
	case "urban_polluted":
		return 17.0
	case "underwater_saline":
		return 13.0
	case "laboratory_control":
		return cfg.ReferenceTemperatureC
	default:
		return 15.0
	}
}

func ScenarioAvgHumidity(scenario string) float64 {
	switch scenario {
	case "temperate_coastal":
		return 75
	case "mediterranean":
		return 60
	case "continental":
		return 65
	case "alpine":
		return 70
	case "tropical_humid":
		return 88
	case "arid_desert":
		return 25
	case "urban_polluted":
		return 55
	case "underwater_saline":
		return 100
	case "laboratory_control":
		return 50
	default:
		return 60
	}
}

func GenerateAcceleratedAgingData(mat *models.RepairMaterial) []AgingDataPoint {
	dur := mat.DurabilityRating
	if dur <= 0 {
		dur = 0.75
	}
	return []AgingDataPoint{
		{TempC: 20, HumidityPct: 50, Days: 28, Cycles: 0, Retention: Round4(0.95 + 0.03*dur)},
		{TempC: 40, HumidityPct: 65, Days: 90, Cycles: 0, Retention: Round4(0.88 + 0.05*dur)},
		{TempC: 40, HumidityPct: 65, Days: 180, Cycles: 0, Retention: Round4(0.82 + 0.07*dur)},
		{TempC: 60, HumidityPct: 80, Days: 90, Cycles: 30, Retention: Round4(0.78 + 0.08*dur)},
		{TempC: 60, HumidityPct: 80, Days: 180, Cycles: 60, Retention: Round4(0.70 + 0.10*dur)},
		{TempC: 80, HumidityPct: 90, Days: 60, Cycles: 100, Retention: Round4(0.68 + 0.10*dur)},
		{TempC: 80, HumidityPct: 90, Days: 120, Cycles: 200, Retention: Round4(0.58 + 0.12*dur)},
	}
}

func PredictMaterialLifetime(
	mat *models.RepairMaterial,
	scenario string,
	threshold float64,
	forecastYears int,
	cfg *config.LifetimeConfig,
) (*LifetimePrediction, error) {
	baseData := GenerateAcceleratedAgingData(mat)

	ea := CalibrateActivationEnergy(baseData, cfg)
	refTempK := cfg.ReferenceTemperatureC + 273.15
	scFactor := ScenarioFactor(scenario)

	matType := mat.MaterialType
	if matType == "" {
		matType = "MODERN_CEMENT"
	}

	curve := make([]models.DegradationPoint, forecastYears+1)
	lower := make([]float64, forecastYears+1)
	upper := make([]float64, forecastYears+1)

	labEquiv0 := ComputeEquivalentTime(0.0, ea, refTempK, scenario, scFactor, cfg)
	initRetention := DegradationModel(labEquiv0, mat.DurabilityRating, matType, cfg)

	for year := 0; year <= forecastYears; year++ {
		equivYears := ComputeEquivalentTime(float64(year), ea, refTempK, scenario, scFactor, cfg)
		retention := DegradationModel(equivYears, mat.DurabilityRating, matType, cfg)
		stdErr := StandardError(year, baseData)

		curve[year] = models.DegradationPoint{
			Year:              year,
			StrengthRatio:     Round4(retention),
			EquivalentAgeDays: int(equivYears * 365),
			TemperatureC:      ScenarioAvgTemp(scenario, cfg),
		}
		lower[year] = Round4(math.Max(0.05, retention-2*stdErr))
		upper[year] = Round4(math.Min(1.0, retention+2*stdErr))
	}

	serviceLife := EstimateServiceLife(curve, threshold, cfg)
	accelFactor := ComputeEquivalentTime(1.0, ea, refTempK, scenario, scFactor, cfg)

	notes := ""
	if mat.DurabilityRating < 0.5 {
		notes = "材料耐久等级较低，建议缩短监测周期"
	} else if serviceLife < 20 {
		notes = "预估使用寿命较短，建议考虑替代材料或加强保护措施"
	} else if serviceLife > 50 {
		notes = "材料长期性能良好，符合遗产修复耐久性要求"
	}

	metrics := map[string]interface{}{
		"initial_strength":     Round4(initRetention),
		"strength_at_10yr":     Round4(curve[10].StrengthRatio),
		"strength_at_25yr":     Round4(curve[25].StrengthRatio),
		"strength_at_50yr":     Round4(curve[50].StrengthRatio),
		"degradation_rate_10y": Round4((initRetention - curve[10].StrengthRatio) / 10.0),
		"scenario_factor":      Round4(scFactor),
		"threshold_safety":     Round4(cfg.ThresholdSafetyFactor),
	}

	return &LifetimePrediction{
		ServiceLifeYears:   Round1(serviceLife),
		DegradationCurve:   curve,
		ConfidenceBounds:   [][]float64{lower, upper},
		ActivationEnergyEV: Round4(ea),
		AcceleratedFactor:  Round4(accelFactor),
		KeyMetrics:         metrics,
		Notes:              notes,
	}, nil
}

func FormatFloat(v float64, precision int) string {
	return strconv.FormatFloat(v, 'f', precision, 64)
}

func Round1(v float64) float64 { return math.Round(v*10) / 10 }
func Round2(v float64) float64 { return math.Round(v*100) / 100 }
func Round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func Round4(v float64) float64 { return math.Round(v*10000) / 10000 }
