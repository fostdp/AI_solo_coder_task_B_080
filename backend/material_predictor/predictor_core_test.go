package material_predictor

import (
	"math"
	"testing"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func defaultLifetimeConfig() *config.LifetimeConfig {
	return &config.LifetimeConfig{
		ArrheniusActivationEV:       0.75,
		ReferenceTempC:              20.0,
		HumidityAcceleration:        1.15,
		FreezeThawAcceleration:      1.8,
		MaxPredictionYears:          200,
		PredictionPoints:            100,
		ConfidenceLevel:             0.95,
		AcceleratedToNaturalFactor:   0.78,
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
		DurabilityRating:   8.5,
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
		{TempC: 40, HumidityPct: 70, Days: 30, Retention: 0.92},
		{TempC: 60, HumidityPct: 80, Days: 30, Retention: 0.82},
		{TempC: 80, HumidityPct: 90, Days: 30, Retention: 0.68},
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
		"underwater_saline", "laboratory_control",
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
	if temp != 20.0 {
		t.Errorf("expected default temp 20 for unknown scenario, got %f", temp)
	}
}

func TestScenarioAvgHumidity_AllScenarios(t *testing.T) {
	scenarios := []string{
		"mediterranean", "temperate_coastal", "continental",
		"alpine", "tropical_humid", "urban_polluted",
		"underwater_saline", "laboratory_control",
	}
	for _, s := range scenarios {
		hum := ScenarioAvgHumidity(s)
		if hum < 0 || hum > 100 {
			t.Errorf("unrealistic humidity for scenario %s: %f", s, hum)
		}
	}
}

func TestDegradationModel_MonotonicDecrease(t *testing.T) {
	prev := 2.0
	for year := 0; year <= 100; year += 10 {
		d := DegradationModel(float64(year), 0.75, 293.15, 293.15, defaultLifetimeConfig())
		if d > prev {
			t.Errorf("degradation should decrease monotonically: year=%d, d=%f, prev=%f", year, d, prev)
		}
		prev = d
	}
}

func TestDegradationModel_YearZero(t *testing.T) {
	d := DegradationModel(0, 0.75, 293.15, 293.15, defaultLifetimeConfig())
	if math.Abs(d-1.0) > 0.01 {
		t.Errorf("at year 0 degradation should be ~1.0, got %f", d)
	}
}

func TestDegradationModel_LongTermCalibration(t *testing.T) {
	cfg := defaultLifetimeConfig()
	cfg.LongTermExposureCalibration = 1.0
	dNoCal := DegradationModel(50, 0.75, 293.15, 293.15, cfg)
	cfg.LongTermExposureCalibration = 0.5
	dWithCal := DegradationModel(50, 0.75, 293.15, 293.15, cfg)
	if dWithCal >= dNoCal {
		t.Errorf("lower calibration should result in lower retention: noCal=%f, withCal=%f", dNoCal, dWithCal)
	}
}

func TestEstimateServiceLife_Basic(t *testing.T) {
	cfg := defaultLifetimeConfig()
	life := EstimateServiceLife(0.75, 0.5, 293.15, 293.15, cfg)
	if life < 10 || life > 500 {
		t.Errorf("service life out of expected range: %f", life)
	}
}

func TestEstimateServiceLife_ThresholdEffect(t *testing.T) {
	cfg := defaultLifetimeConfig()
	lifeLow := EstimateServiceLife(0.75, 0.7, 293.15, 293.15, cfg)
	lifeHigh := EstimateServiceLife(0.75, 0.3, 293.15, 293.15, cfg)
	if lifeLow >= lifeHigh {
		t.Errorf("higher threshold should give shorter life: low=%f, high=%f", lifeLow, lifeHigh)
	}
}

func TestPredictMaterialLifetime_Basic(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()
	pred, err := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pred == nil {
		t.Fatal("expected non-nil prediction")
	}
	if pred.ServiceLifeYears <= 0 {
		t.Errorf("expected positive service life, got %f", pred.ServiceLifeYears)
	}
	if len(pred.DegradationCurve) == 0 {
		t.Error("expected non-empty degradation curve")
	}
	if pred.ActivationEnergyEV <= 0 {
		t.Errorf("expected positive activation energy, got %f", pred.ActivationEnergyEV)
	}
}

func TestPredictMaterialLifetime_SafetyFactor(t *testing.T) {
	cfg := defaultLifetimeConfig()
	cfg.ThresholdSafetyFactor = 1.0
	mat := makeTestMaterial()
	predNoSF, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	cfg.ThresholdSafetyFactor = 2.0
	predWithSF, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if predWithSF.ServiceLifeYears >= predNoSF.ServiceLifeYears {
		t.Errorf("higher safety factor should give shorter life: noSF=%f, withSF=%f", predNoSF.ServiceLifeYears, predWithSF.ServiceLifeYears)
	}
}

func TestPredictMaterialLifetime_AcceleratedNaturalFactor(t *testing.T) {
	cfg := defaultLifetimeConfig()
	cfg.AcceleratedToNaturalFactor = 1.0
	mat := makeTestMaterial()
	predNoCorr, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	cfg.AcceleratedToNaturalFactor = 0.5
	predWithCorr, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if predWithCorr.ServiceLifeYears <= predNoCorr.ServiceLifeYears {
		t.Errorf("lower accel-natural factor should give longer life: noCorr=%f, withCorr=%f", predNoCorr.ServiceLifeYears, predWithCorr.ServiceLifeYears)
	}
}

func TestPredictMaterialLifetime_OutdoorExposure(t *testing.T) {
	cfg := defaultLifetimeConfig()
	mat := makeTestMaterial()
	predIndoor, _ := PredictMaterialLifetime(mat, "laboratory_control", 0.5, 100, cfg)
	predOutdoor, _ := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if predOutdoor.ServiceLifeYears >= predIndoor.ServiceLifeYears {
		t.Errorf("outdoor should give shorter life than lab: indoor=%f, outdoor=%f", predIndoor.ServiceLifeYears, predOutdoor.ServiceLifeYears)
	}
}

func TestPredictMaterialLifetime_InvalidMaterial(t *testing.T) {
	cfg := defaultLifetimeConfig()
	_, err := PredictMaterialLifetime(nil, "mediterranean", 0.5, 100, cfg)
	if err == nil {
		t.Error("expected error for nil material")
	}
}

func TestComputeEquivalentTime_Basic(t *testing.T) {
	cfg := defaultLifetimeConfig()
	eqTime := ComputeEquivalentTime(10, 0.75, 293.15, "mediterranean", 1.0, cfg)
	if eqTime <= 0 {
		t.Errorf("expected positive equivalent time, got %f", eqTime)
	}
}

func TestComputeEquivalentTime_ScenarioFactor(t *testing.T) {
	cfg := defaultLifetimeConfig()
	eqMild := ComputeEquivalentTime(10, 0.75, 293.15, "laboratory_control", 1.0, cfg)
	eqHarsh := ComputeEquivalentTime(10, 0.75, 293.15, "tropical_humid", 1.0, cfg)
	if eqHarsh <= eqMild {
		t.Errorf("harsher scenario should give higher equivalent time: mild=%f, harsh=%f", eqMild, eqHarsh)
	}
}

func TestRootCause_AcceleratedNaturalCorrection(t *testing.T) {
	cfg := defaultLifetimeConfig()
	cfg.AcceleratedToNaturalFactor = 0.78
	mat := makeTestMaterial()
	pred, err := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	strength50 := 0.0
	for _, pt := range pred.DegradationCurve {
		if pt.Year == 50 {
			strength50 = pt.StrengthRatio
			break
		}
	}
	if strength50 <= 0 {
		t.Error("expected positive strength at 50 years")
	}
	if pred.ActivationEnergyEV < 0.4 || pred.ActivationEnergyEV > 1.8 {
		t.Errorf("activation energy out of expected range with correction: %f", pred.ActivationEnergyEV)
	}
}

func TestRootCause_LongTermExposureCalibration(t *testing.T) {
	cfg := defaultLifetimeConfig()
	cfg.LongTermExposureCalibration = 0.92
	mat := makeTestMaterial()
	pred, err := PredictMaterialLifetime(mat, "mediterranean", 0.5, 100, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pred.ServiceLifeYears < 30 {
		t.Errorf("service life too short with calibration: %f", pred.ServiceLifeYears)
	}
}

func TestRound2_Round3(t *testing.T) {
	if Round2(1.234) != 1.23 {
		t.Errorf("Round2(1.234) should be 1.23, got %f", Round2(1.234))
	}
	if Round3(1.2345) != 1.235 {
		t.Errorf("Round3(1.2345) should be 1.235, got %f", Round3(1.2345))
	}
}
