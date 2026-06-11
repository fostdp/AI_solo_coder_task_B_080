package material_predictor

import (
	"math"
	"testing"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func defaultLifetimeConfig() *config.LifetimeConfig {
	return &config.LifetimeConfig{
		ArrheniusActivationEV:       0.95,
		ReferenceTemperatureC:       20.0,
		SimulationYears:             100,
		TimeStepsPerYear:            12,
		ThresholdStrengthRatio:      0.50,
		AcceleratedTempLowC:         40.0,
		AcceleratedTempHighC:        80.0,
		ConfidenceZScore:            1.96,
		HumidityAcceleration:        1.25,
		FreezeThawAcceleration:      1.8,
		AcceleratedToNaturalFactor:  0.78,
		LongTermExposureCalibration: 0.92,
		NaturalAgingBiasCorrection:  0.88,
		OutdoorExposureFactor:       1.15,
		ThresholdSafetyFactor:       1.20,
	}
}

func makeTestMaterial() *models.RepairMaterial {
	return &models.RepairMaterial{
		ID:                 "test-mat-1",
		Name:               "古罗马混凝土修补砂浆",
		MaterialType:       "ROMAN_CONCRETE",
		CompressiveStrength: 25.0,
		DurabilityRating:   0.85,
		CompatibilityRating: 9.0,
		AestheticMatch:     8.0,
		CostPerUnit:        850.0,
		Unit:               "m³",
		Description:        "传统石灰火山灰砂浆",
	}
}

func makeAgingData() []AgingDataPoint {
	return []AgingDataPoint{
		{TempC: 20, HumidityPct: 60, Days: 30, Retention: 0.98},
		{TempC: 40, HumidityPct: 70, Days: 90, Retention: 0.92},
		{TempC: 60, HumidityPct: 80, Days: 180, Retention: 0.82},
		{TempC: 80, HumidityPct: 90, Days: 360, Retention: 0.68},
	}
}

func TestCalibrateActivationEnergy_Basic(t *testing.T) {
	cfg := defaultLifetimeConfig()
	data := makeAgingData()
	ea := CalibrateActivationEnergy(data, cfg)
	if ea < 0.4 || ea > 1.8 {
		t.Errorf("activation energy out of expected range: %f", ea)
	}
}

func TestCalibrateActivationEnergy_InsufficientData(t *testing.T) {
	cfg := defaultLifetimeConfig()
	data := []AgingDataPoint{
		{TempC: 20, Days: 30, Retention: 0.98},
		{TempC: 40, Days: 30, Retention: 0.92},
	}
	ea := CalibrateActivationEnergy(data, cfg)
	if ea != cfg.ArrheniusActivationEV {
		t.Errorf("expected default activation energy for insufficient data, got %f", ea)
	}
}

func TestCalibrateActivationEnergy_BiasCorrection(t *testing.T) {
	cfg := defaultLifetimeConfig()
	data := makeAgingData()
	cfg.NaturalAgingBiasCorrection = 1.0
	eaNoBias := CalibrateActivationEnergy(data, cfg)
	cfg.NaturalAgingBiasCorrection = 0.5
	eaWithBias := CalibrateActivationEnergy(data, cfg)
	if eaWithBias >= eaNoBias {
		t.Errorf("bias correction < 1 should reduce EA: noBias=%f, withBias=%f", eaNoBias, eaWithBias)
	}
}

func TestArrheniusFactor_TempDependence(t *testing.T) {
	ea := 0.75
	refTemp := 293.15
	lowTemp := 273.15
	highTemp := 313.15
	factorLow := ArrheniusFactor(ea, lowTemp, refTemp)
	factorHigh := ArrheniusFactor(ea, highTemp, refTemp)
	if factorLow >= 1.0 {
		t.Errorf("lower temp should have factor < 1, got %f", factorLow)
	}
	if factorHigh <= 1.0 {
		t.Errorf("higher temp should have factor > 1, got %f", factorHigh)
	}
}

func TestArrheniusFactor_ReferenceTemp(t *testing.T) {
	ea := 0.75
	refTemp := 293.15
	factor := ArrheniusFactor(ea, refTemp, refTemp)
	if math.Abs(factor-1.0) > 1e-9 {
		t.Errorf("at reference temp factor should be 1, got %f", factor)
	}
}

func TestScenarioAvgTemp_AllScenarios(t *testing.T) {
	cfg := defaultLifetimeConfig()
	scenarios := []string{
		"mediterranean", "temperate_coastal", "continental",
		"alpine", "tropical_humid", "urban_polluted",
		"underwater_saline", "laboratory_control", "arid_desert",
	}
	for _, s := range scenarios {
		temp := ScenarioAvgTemp(s, cfg)
		if temp < -20 || temp > 40 {
			t.Errorf("unrealistic temp for scenario %s: %f", s, temp)
		}
	}
}

func TestScenarioAvgTemp_Default(t *testing.T) {
	cfg := defaultLifetimeConfig()
	temp := ScenarioAvgTemp("unknown_scenario", cfg)
	if temp != 15.0 {
		t.Errorf("expected default temp 15 for unknown scenario, got %f", temp)
	}
}

func TestScenarioAvgHumidity_AllScenarios(t *testing.T) {
	scenarios := []string{
		"mediterranean", "temperate_coastal", "continental",
		"alpine", "tropical_humid", "urban_polluted",
		"underwater_saline", "laboratory_control", "arid_desert",
	}
	for _, s := range scenarios {
		hum := ScenarioAvgHumidity(s)
		if hum < 0 || hum > 100 {
			t.Errorf("unrealistic humidity for scenario %s: %f", s, hum)
		}
	}
}

func TestDegradationModel_MonotonicDecrease(t *testing.T) {
	cfg := defaultLifetimeConfig()
	prev := 2.0
	for year := 0; year <= 100; year += 10 {
		d := DegradationModel(float64(year), 0.75, "ROMAN_CONCRETE", cfg)
		if d > prev {
			t.Errorf("degradation should decrease monotonically: year=%d, d=%f, prev=%f", year, d, prev)
		}
		prev = d
	}
}

func TestDegradationModel_YearZero(t *testing.T) {
	cfg := defaultLifetimeConfig()
	d := DegradationModel(0, 0.75, "ROMAN_CONCRETE", cfg)
	if math.Abs(d-1.0) > 0.01 {
		t.Errorf("at year 0 degradation should be ~1.0, got %f", d)
	}
}

func TestDegradationModel_LongTermCalibration(t *testing.T) {
	cfg := defaultLifetimeConfig()
	cfg.LongTermExposureCalibration = 1.0
	dNoCal := DegradationModel(50, 0.75, "ROMAN_CONCRETE", cfg)
	cfg.LongTermExposureCalibration = 0.5
	dWithCal := DegradationModel(50, 0.75, "ROMAN_CONCRETE", cfg)
	if dWithCal >= dNoCal {
		t.Errorf("lower calibration should result in lower retention: noCal=%f, withCal=%f", dNoCal, dWithCal)
	}
}

func TestDegradationModel_DifferentMaterials(t *testing.T) {
	cfg := defaultLifetimeConfig()
	materials := []string{"ROMAN_CONCRETE", "LIME_MORTAR", "MODERN_CEMENT", "FRP", "EPOXY", "INJECTION_GROUT"}
	results := make(map[string]float64)
	for _, mat := range materials {
		results[mat] = DegradationModel(50, 0.75, mat, cfg)
		if results[mat] < 0.05 || results[mat] > 1.0 {
			t.Errorf("degradation out of bounds for %s: %f", mat, results[mat])
		}
	}
	if results["FRP"] < results["MODERN_CEMENT"] {
		t.Errorf("FRP should have higher retention than modern cement: FRP=%f, MODERN_CEMENT=%f", results["FRP"], results["MODERN_CEMENT"])
	}
	if results["EPOXY"] > results["ROMAN_CONCRETE"] {
		t.Errorf("epoxy should degrade faster than roman concrete: EPOXY=%f, ROMAN_CONCRETE=%f", results["EPOXY"], results["ROMAN_CONCRETE"])
	}
}

func TestArrheniusExtrapolationAccuracy(t *testing.T) {
	cfg := defaultLifetimeConfig()
	data := makeAgingData()

	ea := CalibrateActivationEnergy(data, cfg)
	if ea < 0.4 || ea > 1.8 {
		t.Errorf("calibrated activation energy out of expected range: %f", ea)
	}

	refTempK := cfg.ReferenceTemperatureC + 273.15
	testTemps := []struct {
		name     string
		tempC    float64
		minFactor float64
		maxFactor float64
	}{
		{"低温外推_0°C", 0.0, 0.1, 0.5},
		{"常温内插_20°C", 20.0, 0.8, 1.2},
		{"中温外推_40°C", 40.0, 1.5, 5.0},
		{"高温外推_60°C", 60.0, 5.0, 20.0},
		{"极端高温_80°C", 80.0, 15.0, 60.0},
	}

	for _, tc := range testTemps {
		t.Run(tc.name, func(t *testing.T) {
			tempK := tc.tempC + 273.15
			factor := ArrheniusFactor(ea, tempK, refTempK)

			if factor < tc.minFactor || factor > tc.maxFactor {
				t.Errorf("%s: Arrhenius factor %.4f outside expected range [%.4f, %.4f]",
					tc.name, factor, tc.minFactor, tc.maxFactor)
			}
			if math.IsNaN(factor) || math.IsInf(factor, 0) {
				t.Errorf("%s: Arrhenius factor is invalid: %f", tc.name, factor)
			}
		})
	}

	prevFactor := 0.0
	for _, tempC := range []float64{0.0, 20.0, 40.0, 60.0, 80.0} {
		tempK := tempC + 273.15
		factor := ArrheniusFactor(ea, tempK, refTempK)
		if factor <= prevFactor {
			t.Errorf("Arrhenius factor should increase with temperature: %.0f°C=%.4f, prev=%.4f",
				tempC, factor, prevFactor)
		}
		prevFactor = factor
	}
}

func TestPredictMaterialLifetime_50YearCurveConsistency(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()
	forecastYears := 100

	pred, err := PredictMaterialLifetime(mat, "mediterranean", 0.5, forecastYears, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pred == nil {
		t.Fatal("expected non-nil prediction")
	}

	if len(pred.DegradationCurve) != forecastYears+1 {
		t.Errorf("expected %d curve points, got %d", forecastYears+1, len(pred.DegradationCurve))
	}

	shortTermPoints := make(map[int]float64)
	for _, pt := range pred.DegradationCurve {
		if pt.Year <= 10 {
			shortTermPoints[pt.Year] = pt.StrengthRatio
		}
	}

	for year := 1; year <= 10; year++ {
		if shortTermPoints[year] >= shortTermPoints[year-1]+0.01 {
			t.Errorf("short-term degradation should decrease: year %d=%.4f, year %d=%.4f",
				year-1, shortTermPoints[year-1], year, shortTermPoints[year])
		}
	}

	strength10yr := pred.KeyMetrics["strength_at_10yr"].(float64)
	strength25yr := pred.KeyMetrics["strength_at_25yr"].(float64)
	strength50yr := pred.KeyMetrics["strength_at_50yr"].(float64)

	if strength10yr <= 0 || strength10yr > 1 {
		t.Errorf("10yr strength out of bounds: %f", strength10yr)
	}
	if strength25yr <= 0 || strength25yr > 1 {
		t.Errorf("25yr strength out of bounds: %f", strength25yr)
	}
	if strength50yr <= 0 || strength50yr > 1 {
		t.Errorf("50yr strength out of bounds: %f", strength50yr)
	}

	if strength10yr <= strength25yr || strength25yr <= strength50yr {
		t.Errorf("strength should decrease over time: 10yr=%.4f, 25yr=%.4f, 50yr=%.4f",
			strength10yr, strength25yr, strength50yr)
	}

	shortTermRate := pred.KeyMetrics["degradation_rate_10y"].(float64)
	if shortTermRate <= 0 {
		t.Errorf("degradation rate should be positive: %f", shortTermRate)
	}

	year50Strength := 0.0
	for _, pt := range pred.DegradationCurve {
		if pt.Year == 50 {
			year50Strength = pt.StrengthRatio
			break
		}
	}
	if math.Abs(year50Strength-strength50yr) > 0.001 {
		t.Errorf("50yr strength mismatch: curve=%.4f, metrics=%.4f", year50Strength, strength50yr)
	}
}

func TestPredictMaterialLifetime_ValidityConservativeness(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()

	testCases := []struct {
		name           string
		scenario       string
		threshold      float64
		safetyFactor   float64
		minLifeYears   float64
		maxLifeYears   float64
	}{
		{"温和环境", "mediterranean", 0.5, 1.0, 30, 150},
		{"温和环境_保守", "mediterranean", 0.5, 2.0, 10, 80},
		{"严酷环境", "tropical_humid", 0.5, 1.0, 15, 100},
		{"实验室", "laboratory_control", 0.5, 1.0, 50, 200},
		{"高阈值", "mediterranean", 0.7, 1.0, 5, 60},
		{"低阈值", "mediterranean", 0.3, 1.0, 50, 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.ThresholdSafetyFactor = tc.safetyFactor
			pred, err := PredictMaterialLifetime(mat, tc.scenario, tc.threshold, 100, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if pred.ServiceLifeYears < tc.minLifeYears {
				t.Errorf("%s: service life %.1f years below minimum %.1f",
					tc.name, pred.ServiceLifeYears, tc.minLifeYears)
			}
			if pred.ServiceLifeYears > tc.maxLifeYears {
				t.Logf("%s: service life %.1f years above typical maximum %.1f",
					tc.name, pred.ServiceLifeYears, tc.maxLifeYears)
			}

			if pred.ServiceLifeYears <= 0 {
				t.Errorf("%s: service life should be positive: %f", tc.name, pred.ServiceLifeYears)
			}

			if len(pred.ConfidenceBounds) != 2 {
				t.Errorf("%s: expected 2 confidence bounds, got %d", tc.name, len(pred.ConfidenceBounds))
			}

			for year := 0; year <= 100; year++ {
				lower := pred.ConfidenceBounds[0][year]
				upper := pred.ConfidenceBounds[1][year]
				actual := pred.DegradationCurve[year].StrengthRatio

				if lower > actual+0.001 {
					t.Errorf("%s, year %d: lower bound %.4f > actual %.4f", tc.name, year, lower, actual)
				}
				if upper < actual-0.001 {
					t.Errorf("%s, year %d: upper bound %.4f < actual %.4f", tc.name, year, upper, actual)
				}
				if lower < 0.05 {
					t.Errorf("%s, year %d: lower bound too low: %.4f", tc.name, year, lower)
				}
				if upper > 1.0 {
					t.Errorf("%s, year %d: upper bound too high: %.4f", tc.name, year, upper)
				}
			}

			thresholdWithSafety := tc.threshold * tc.safetyFactor
			lifeAtThreshold := 0.0
			for i := 1; i < len(pred.DegradationCurve); i++ {
				prev := pred.DegradationCurve[i-1]
				cur := pred.DegradationCurve[i]
				if prev.StrengthRatio >= thresholdWithSafety && cur.StrengthRatio <= thresholdWithSafety {
					if prev.StrengthRatio != cur.StrengthRatio {
						tVal := (thresholdWithSafety - prev.StrengthRatio) / (cur.StrengthRatio - prev.StrengthRatio)
						lifeAtThreshold = float64(i-1) + math.Max(0, math.Min(1, tVal))
					} else {
						lifeAtThreshold = float64(i)
					}
					break
				}
			}
			if lifeAtThreshold > 0 && math.Abs(pred.ServiceLifeYears-lifeAtThreshold) > 1.0 {
				t.Errorf("%s: service life mismatch: predicted=%.1f, interpolated=%.1f",
					tc.name, pred.ServiceLifeYears, lifeAtThreshold)
			}
		})
	}
}

func TestPredictMaterialLifetime_SafetyFactorEffect(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()

	cfg.ThresholdSafetyFactor = 1.0
	predNoSF, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	cfg.ThresholdSafetyFactor = 2.0
	predWithSF, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)

	if predWithSF.ServiceLifeYears >= predNoSF.ServiceLifeYears {
		t.Errorf("higher safety factor should give shorter life: noSF=%f, withSF=%f",
			predNoSF.ServiceLifeYears, predWithSF.ServiceLifeYears)
	}

	conservatismRatio := predNoSF.ServiceLifeYears / math.Max(0.1, predWithSF.ServiceLifeYears)
	if conservatismRatio < 1.2 {
		t.Errorf("safety factor 2.0 should provide at least 20%% conservatism: ratio=%.2f", conservatismRatio)
	}
}

func TestPredictMaterialLifetime_AllScenarios(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()

	scenarios := []string{
		"arid_desert", "mediterranean", "temperate_coastal",
		"continental", "alpine", "tropical_humid",
		"urban_polluted", "underwater_saline", "laboratory_control",
	}

	results := make(map[string]*LifetimePrediction)
	for _, s := range scenarios {
		pred, err := PredictMaterialLifetime(mat, s, 0.5, 100, cfg)
		if err != nil {
			t.Fatalf("scenario %s: unexpected error: %v", s, err)
		}
		results[s] = pred

		if pred.ServiceLifeYears <= 0 {
			t.Errorf("scenario %s: service life should be positive: %f", s, pred.ServiceLifeYears)
		}
		if pred.ActivationEnergyEV < 0.4 || pred.ActivationEnergyEV > 1.8 {
			t.Errorf("scenario %s: activation energy out of range: %f", s, pred.ActivationEnergyEV)
		}
		if pred.AcceleratedFactor <= 0 {
			t.Errorf("scenario %s: accelerated factor should be positive: %f", s, pred.AcceleratedFactor)
		}
	}

	if results["laboratory_control"].ServiceLifeYears <= results["tropical_humid"].ServiceLifeYears {
		t.Errorf("lab should have longer life than tropical humid: lab=%.1f, tropical=%.1f",
			results["laboratory_control"].ServiceLifeYears, results["tropical_humid"].ServiceLifeYears)
	}

	if results["arid_desert"].ServiceLifeYears <= results["underwater_saline"].ServiceLifeYears {
		t.Errorf("arid should have longer life than underwater saline: arid=%.1f, underwater=%.1f",
			results["arid_desert"].ServiceLifeYears, results["underwater_saline"].ServiceLifeYears)
	}
}

func TestPredictMaterialLifetime_BoundaryConditions(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()

	boundaryCases := []struct {
		name          string
		scenario      string
		threshold     float64
		forecastYears int
	}{
		{"极低阈值", "mediterranean", 0.1, 100},
		{"极高阈值", "mediterranean", 0.9, 100},
		{"极短预测", "mediterranean", 0.5, 5},
		{"极长预测", "mediterranean", 0.5, 200},
		{"未知场景", "unknown_scenario", 0.5, 100},
	}

	for _, tc := range boundaryCases {
		t.Run(tc.name, func(t *testing.T) {
			pred, err := PredictMaterialLifetime(mat, tc.scenario, tc.threshold, tc.forecastYears, cfg)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if pred == nil {
				t.Fatalf("%s: returned nil prediction", tc.name)
			}
			if pred.ServiceLifeYears <= 0 {
				t.Errorf("%s: service life should be positive: %f", tc.name, pred.ServiceLifeYears)
			}
			if len(pred.DegradationCurve) != tc.forecastYears+1 {
				t.Errorf("%s: expected %d curve points, got %d",
					tc.name, tc.forecastYears+1, len(pred.DegradationCurve))
			}
		})
	}
}

func TestPredictMaterialLifetime_AnomalyCases(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()

	anomalyCases := []struct {
		name          string
		scenario      string
		threshold     float64
		forecastYears int
	}{
		{"负阈值", "mediterranean", -0.5, 100},
		{"阈值>1", "mediterranean", 1.5, 100},
		{"负预测年数", "mediterranean", 0.5, -10},
		{"空场景", "", 0.5, 100},
	}

	for _, tc := range anomalyCases {
		t.Run(tc.name, func(t *testing.T) {
			pred, err := PredictMaterialLifetime(mat, tc.scenario, tc.threshold, tc.forecastYears, cfg)

			if tc.threshold < 0 || tc.forecastYears < 0 {
				if err == nil && pred != nil {
					for _, pt := range pred.DegradationCurve {
						if math.IsNaN(pt.StrengthRatio) || math.IsInf(pt.StrengthRatio, 0) {
							t.Errorf("%s: degradation curve has invalid value at year %d: %f",
								tc.name, pt.Year, pt.StrengthRatio)
						}
					}
				}
			}

			if err == nil && pred != nil {
				if math.IsNaN(pred.ServiceLifeYears) || math.IsInf(pred.ServiceLifeYears, 0) {
					t.Errorf("%s: service life is invalid: %f", tc.name, pred.ServiceLifeYears)
				}
				if math.IsNaN(pred.ActivationEnergyEV) || math.IsInf(pred.ActivationEnergyEV, 0) {
					t.Errorf("%s: activation energy is invalid: %f", tc.name, pred.ActivationEnergyEV)
				}
			}
		})
	}
}

func TestComputeEquivalentTime_Basic(t *testing.T) {
	cfg := defaultLifetimeConfig()
	refTempK := cfg.ReferenceTemperatureC + 273.15
	eqTime := ComputeEquivalentTime(10, 0.75, refTempK, "mediterranean", 1.0, cfg)
	if eqTime <= 0 {
		t.Errorf("expected positive equivalent time, got %f", eqTime)
	}
}

func TestComputeEquivalentTime_ScenarioFactor(t *testing.T) {
	cfg := defaultLifetimeConfig()
	refTempK := cfg.ReferenceTemperatureC + 273.15
	eqMild := ComputeEquivalentTime(10, 0.75, refTempK, "laboratory_control", 1.0, cfg)
	eqHarsh := ComputeEquivalentTime(10, 0.75, refTempK, "tropical_humid", 1.0, cfg)
	if eqHarsh <= eqMild {
		t.Errorf("harsher scenario should give higher equivalent time: mild=%f, harsh=%f", eqMild, eqHarsh)
	}
}

func TestScenarioFactor_AllScenarios(t *testing.T) {
	scenarios := []string{
		"arid_desert", "mediterranean", "temperate_coastal",
		"continental", "alpine", "tropical_humid",
		"urban_polluted", "underwater_saline", "laboratory_control",
	}

	factors := make(map[string]float64)
	for _, s := range scenarios {
		factors[s] = ScenarioFactor(s)
		if factors[s] <= 0 {
			t.Errorf("scenario %s: factor should be positive: %f", s, factors[s])
		}
	}

	if factors["laboratory_control"] >= factors["mediterranean"] {
		t.Errorf("lab should have lower factor than mediterranean: lab=%.2f, med=%.2f",
			factors["laboratory_control"], factors["mediterranean"])
	}
	if factors["tropical_humid"] <= factors["mediterranean"] {
		t.Errorf("tropical should have higher factor than mediterranean: tropical=%.2f, med=%.2f",
			factors["tropical_humid"], factors["mediterranean"])
	}
}

func TestStandardError_IncreasesWithTime(t *testing.T) {
	data := makeAgingData()
	prevErr := -1.0
	for year := 0; year <= 100; year += 10 {
		err := StandardError(year, data)
		if err < 0 {
			t.Errorf("year %d: standard error should be non-negative: %f", year, err)
		}
		if err < prevErr {
			t.Errorf("standard error should increase with time: year %d=%f, prev=%f", year, err, prevErr)
		}
		prevErr = err
	}
}

func TestStandardError_DataSpread(t *testing.T) {
	dataLowSpread := []AgingDataPoint{
		{TempC: 20, Days: 30, Retention: 0.90},
		{TempC: 40, Days: 90, Retention: 0.89},
		{TempC: 60, Days: 180, Retention: 0.88},
		{TempC: 80, Days: 360, Retention: 0.87},
	}
	dataHighSpread := []AgingDataPoint{
		{TempC: 20, Days: 30, Retention: 0.95},
		{TempC: 40, Days: 90, Retention: 0.85},
		{TempC: 60, Days: 180, Retention: 0.70},
		{TempC: 80, Days: 360, Retention: 0.50},
	}

	errLow := StandardError(50, dataLowSpread)
	errHigh := StandardError(50, dataHighSpread)

	if errHigh <= errLow {
		t.Errorf("higher data spread should give higher standard error: low=%.4f, high=%.4f", errLow, errHigh)
	}
}

func TestGenerateAcceleratedAgingData(t *testing.T) {
	mat := makeTestMaterial()
	data := GenerateAcceleratedAgingData(mat)

	if len(data) == 0 {
		t.Fatal("expected non-empty aging data")
	}

	prevRetention := 2.0
	prevDays := 0
	for i, d := range data {
		if d.Retention < 0.05 || d.Retention > 1.0 {
			t.Errorf("data point %d: retention out of bounds: %f", i, d.Retention)
		}
		if d.Retention > prevRetention+0.01 {
			t.Errorf("data point %d: retention should not increase: %.4f > %.4f", i, d.Retention, prevRetention)
		}
		if d.Days < prevDays {
			t.Errorf("data point %d: days should be increasing: %d < %d", i, d.Days, prevDays)
		}
		if d.TempC < 20 || d.TempC > 100 {
			t.Errorf("data point %d: temperature out of expected range: %f", i, d.TempC)
		}
		prevRetention = d.Retention
		prevDays = d.Days
	}
}

func TestPredictMaterialLifetime_KeyMetrics(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()

	pred, err := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requiredMetrics := []string{
		"initial_strength", "strength_at_10yr", "strength_at_25yr",
		"strength_at_50yr", "degradation_rate_10y",
		"scenario_factor", "threshold_safety",
	}

	for _, key := range requiredMetrics {
		if _, ok := pred.KeyMetrics[key]; !ok {
			t.Errorf("missing key metric: %s", key)
		}
	}

	initStrength := pred.KeyMetrics["initial_strength"].(float64)
	strength10yr := pred.KeyMetrics["strength_at_10yr"].(float64)
	strength50yr := pred.KeyMetrics["strength_at_50yr"].(float64)

	if initStrength <= strength10yr || strength10yr <= strength50yr {
		t.Errorf("strength metrics should decrease over time: init=%.4f, 10yr=%.4f, 50yr=%.4f",
			initStrength, strength10yr, strength50yr)
	}

	scenarioFactor := pred.KeyMetrics["scenario_factor"].(float64)
	expectedFactor := ScenarioFactor("mediterranean")
	if math.Abs(scenarioFactor-expectedFactor) > 0.01 {
		t.Errorf("scenario factor mismatch: metrics=%.4f, expected=%.4f", scenarioFactor, expectedFactor)
	}

	thresholdSafety := pred.KeyMetrics["threshold_safety"].(float64)
	if math.Abs(thresholdSafety-cfg.ThresholdSafetyFactor) > 0.01 {
		t.Errorf("threshold safety mismatch: metrics=%.4f, expected=%.4f", thresholdSafety, cfg.ThresholdSafetyFactor)
	}
}

func TestPredictMaterialLifetime_NotesGeneration(t *testing.T) {
	cfg := defaultLifetimeConfig()

	testCases := []struct {
		name             string
		durabilityRating float64
		scenario         string
		expectNote       bool
		noteSubstring    string
	}{
		{"低耐久性", 0.3, "mediterranean", true, "耐久等级较低"},
		{"短寿命", 0.5, "tropical_humid", true, "使用寿命较短"},
		{"良好性能", 0.9, "mediterranean", true, "长期性能良好"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mat := makeTestMaterial()
			mat.DurabilityRating = tc.durabilityRating
			pred, err := PredictMaterialLifetime(mat, tc.scenario, 0.5, 100, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.expectNote && pred.Notes == "" {
				t.Errorf("%s: expected note but got empty", tc.name)
			}
		})
	}
}

func TestRound2_Round3_Round4(t *testing.T) {
	if Round2(1.234) != 1.23 {
		t.Errorf("Round2(1.234) should be 1.23, got %f", Round2(1.234))
	}
	if Round3(1.2345) != 1.235 {
		t.Errorf("Round3(1.2345) should be 1.235, got %f", Round3(1.2345))
	}
	if Round4(1.23456) != 1.2346 {
		t.Errorf("Round4(1.23456) should be 1.2346, got %f", Round4(1.23456))
	}
}

func TestFormatFloat(t *testing.T) {
	if FormatFloat(1.234, 2) != "1.23" {
		t.Errorf("FormatFloat(1.234, 2) should be '1.23', got '%s'", FormatFloat(1.234, 2))
	}
	if FormatFloat(1.2, 3) != "1.200" {
		t.Errorf("FormatFloat(1.2, 3) should be '1.200', got '%s'", FormatFloat(1.2, 3))
	}
}
