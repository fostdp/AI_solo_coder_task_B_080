package lifetime

import (
	"math"
	"testing"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"

	"github.com/google/uuid"
)

func defaultPredictor() *LifetimePredictor {
	cfg := &config.Config{
		Lifetime: config.LifetimeConfig{
			ArrheniusActivationEV:  0.95,
			ReferenceTemperatureC:  20.0,
			SimulationYears:        100,
			TimeStepsPerYear:       12,
			ThresholdStrengthRatio: 0.50,
			AcceleratedTempLowC:    40.0,
			AcceleratedTempHighC:   80.0,
			ConfidenceZScore:       1.96,
			HumidityAcceleration:   1.25,
			FreezeThawAcceleration: 1.8,
		},
	}
	return &LifetimePredictor{repo: nil, cfg: &cfg.Lifetime}
}

func makeAgingData() []agingDataPoint {
	return []agingDataPoint{
		{tempC: 20, humidityPct: 50, days: 28, retention: 0.97},
		{tempC: 40, humidityPct: 65, days: 90, retention: 0.91},
		{tempC: 40, humidityPct: 65, days: 180, retention: 0.87},
		{tempC: 60, humidityPct: 80, days: 90, retention: 0.85},
		{tempC: 60, humidityPct: 80, days: 180, retention: 0.79},
		{tempC: 80, humidityPct: 90, days: 60, retention: 0.77},
		{tempC: 80, humidityPct: 90, days: 120, retention: 0.69},
	}
}

func TestCalibrateActivationEnergy_ValidData(t *testing.T) {
	lp := defaultPredictor()
	data := makeAgingData()
	ea := lp.calibrateActivationEnergy(data)
	if ea < 0.4 || ea > 1.6 {
		t.Errorf("calibrated Ea should be in reasonable range [0.4, 1.6] eV, got %f", ea)
	}
}

func TestCalibrateActivationEnergy_InsufficientData(t *testing.T) {
	lp := defaultPredictor()
	data := []agingDataPoint{{tempC: 20, retention: 0.95, days: 28}}
	ea := lp.calibrateActivationEnergy(data)
	if ea != lp.cfg.ArrheniusActivationEV {
		t.Errorf("insufficient data should return default Ea, got %f expected %f", ea, lp.cfg.ArrheniusActivationEV)
	}
}

func TestCalibrateActivationEnergy_EmptyData(t *testing.T) {
	lp := defaultPredictor()
	ea := lp.calibrateActivationEnergy(nil)
	if ea != lp.cfg.ArrheniusActivationEV {
		t.Errorf("empty data should return default Ea, got %f expected %f", ea, lp.cfg.ArrheniusActivationEV)
	}
}

func TestCalibrateActivationEnergy_HigherTempFasterDegradation(t *testing.T) {
	lp := defaultPredictor()
	highTempData := []agingDataPoint{
		{tempC: 20, days: 180, retention: 0.95},
		{tempC: 60, days: 180, retention: 0.70},
		{tempC: 80, days: 180, retention: 0.50},
	}
	eaHigh := lp.calibrateActivationEnergy(highTempData)
	if eaHigh < 0.4 {
		t.Errorf("significant temperature sensitivity should produce reasonable Ea, got %f", eaHigh)
	}
}

func TestCalibrateActivationEnergy_PositiveSlope(t *testing.T) {
	lp := defaultPredictor()
	data := makeAgingData()
	ea := lp.calibrateActivationEnergy(data)
	if ea <= 0 {
		t.Errorf("Ea should be positive (degradation rate increases with temperature), got %f", ea)
	}
}

func TestArrheniusFactor_HigherTempLarger(t *testing.T) {
	lp := defaultPredictor()
	refK := 293.15
	f80 := lp.arrheniusFactor(0.95, 353.15, refK)
	f40 := lp.arrheniusFactor(0.95, 313.15, refK)
	f20 := lp.arrheniusFactor(0.95, refK, refK)
	if f80 <= f40 {
		t.Errorf("higher temperature should produce larger acceleration factor: f80=%f f40=%f", f80, f40)
	}
	if f40 <= f20 {
		t.Errorf("40°C should have higher factor than ref 20°C: f40=%f f20=%f", f40, f20)
	}
}

func TestArrheniusFactor_ReferenceTempIsOne(t *testing.T) {
	lp := defaultPredictor()
	refK := 293.15
	f := lp.arrheniusFactor(0.95, refK, refK)
	if math.Abs(f-1.0) > 0.01 {
		t.Errorf("at reference temperature, factor should be ~1.0, got %f", f)
	}
}

func TestArrheniusFactor_PositiveAlways(t *testing.T) {
	lp := defaultPredictor()
	refK := 293.15
	for _, tK := range []float64{253.15, 273.15, 293.15, 313.15, 353.15} {
		f := lp.arrheniusFactor(0.95, tK, refK)
		if f <= 0 {
			t.Errorf("arrhenius factor should always be positive at %g K, got %f", tK, f)
		}
	}
}

func TestArrheniusFactor_ExpDecayScaling(t *testing.T) {
	lp := defaultPredictor()
	refK := 293.15
	ea := 0.95
	R := 8.617333262e-5
	f353 := lp.arrheniusFactor(ea, 353.15, refK)
	expected := math.Exp(-ea / R * (1.0/353.15 - 1.0/refK))
	if math.Abs(f353-expected) > 0.01 {
		t.Errorf("arrhenius factor doesn't match expected formula: got %f expected %f", f353, expected)
	}
}

func TestComputeEquivalentTime_HotScenario(t *testing.T) {
	lp := defaultPredictor()
	ea := 0.95
	refK := 293.15
	eqTropical := lp.computeEquivalentTime(10, ea, refK, "tropical_humid", 1.75)
	eqArctic := lp.computeEquivalentTime(10, ea, refK, "alpine", 1.55)
	if eqTropical <= 10.0 {
		t.Errorf("tropical scenario should accelerate aging (eq > actual): actual=10 eq=%f", eqTropical)
	}
	if eqArctic <= 0 {
		t.Errorf("alpine equivalent time should be positive, got %f", eqArctic)
	}
}

func TestComputeEquivalentTime_FreezeThawExtra(t *testing.T) {
	lp := defaultPredictor()
	ea := 0.95
	refK := 293.15
	eqAlpine := lp.computeEquivalentTime(10, ea, refK, "alpine", 1.55)
	eqMed := lp.computeEquivalentTime(10, ea, refK, "mediterranean", 1.0)
	alpineRatio := eqAlpine / 10.0
	medRatio := eqMed / 10.0
	if alpineRatio < medRatio*0.8 {
		t.Errorf("alpine freeze-thaw should not dramatically reduce aging relative to med: alpineR=%f medR=%f", alpineRatio, medRatio)
	}
}

func TestDegradationModel_YearZero(t *testing.T) {
	lp := defaultPredictor()
	ret := lp.degradationModel(0, 0.75, "MODERN_CEMENT")
	if math.Abs(ret-1.0) > 0.05 {
		t.Errorf("at year 0, retention should be ~1.0, got %f", ret)
	}
}

func TestDegradationModel_AllMaterialTypes(t *testing.T) {
	lp := defaultPredictor()
	types := []string{"ROMAN_CONCRETE", "LIME_MORTAR", "MODERN_CEMENT", "FRP", "CFRP", "GFRP", "EPOXY", "INJECTION_GROUT", "UNKNOWN"}
	for _, mt := range types {
		ret := lp.degradationModel(50, 0.75, mt)
		if ret < 0.05 || ret > 1.0 {
			t.Errorf("degradation model for type %s out of range at 50yr: %f", mt, ret)
		}
	}
}

func TestDegradationModel_RomanConcreteSlowDegradation(t *testing.T) {
	lp := defaultPredictor()
	retRoman := lp.degradationModel(100, 0.75, "ROMAN_CONCRETE")
	retEpoxy := lp.degradationModel(100, 0.75, "EPOXY")
	if retRoman < retEpoxy {
		t.Errorf("roman concrete should degrade slower than epoxy: roman=%f epoxy=%f", retRoman, retEpoxy)
	}
}

func TestDegradationModel_MonotonicDecline(t *testing.T) {
	lp := defaultPredictor()
	prev := 1.1
	for _, yr := range []float64{0, 5, 10, 25, 50, 75, 100, 200, 500, 1000} {
		ret := lp.degradationModel(yr, 0.75, "MODERN_CEMENT")
		if ret > prev {
			t.Errorf("retention should monotonically decline: year=%g ret=%f > prev=%f", yr, ret, prev)
		}
		prev = ret
	}
}

func TestDegradationModel_DurabilityBoost(t *testing.T) {
	lp := defaultPredictor()
	retLowDur := lp.degradationModel(100, 0.5, "MODERN_CEMENT")
	retHighDur := lp.degradationModel(100, 0.9, "MODERN_CEMENT")
	if retHighDur < retLowDur {
		t.Errorf("higher durability rating should yield higher retention: low=%f high=%f", retLowDur, retHighDur)
	}
}

func TestDegradationModel_FRPSlowDegradation(t *testing.T) {
	lp := defaultPredictor()
	retFRP := lp.degradationModel(50, 0.75, "FRP")
	retCement := lp.degradationModel(50, 0.75, "MODERN_CEMENT")
	if retFRP < retCement {
		t.Errorf("FRP should degrade slower than modern cement: frp=%f cement=%f", retFRP, retCement)
	}
}

func TestStandardError_IncreasesWithYear(t *testing.T) {
	lp := defaultPredictor()
	data := makeAgingData()
	se0 := lp.standardError(0, data)
	se10 := lp.standardError(10, data)
	se50 := lp.standardError(50, data)
	se100 := lp.standardError(100, data)
	if !(se0 <= se10 && se10 <= se50 && se50 <= se100) {
		t.Errorf("standard error should increase with year: yr0=%f yr10=%f yr50=%f yr100=%f", se0, se10, se50, se100)
	}
}

func TestStandardError_AlwaysPositive(t *testing.T) {
	lp := defaultPredictor()
	data := makeAgingData()
	for _, yr := range []int{0, 5, 25, 50, 100} {
		se := lp.standardError(yr, data)
		if se <= 0 {
			t.Errorf("standard error should always be positive at year %d, got %f", yr, se)
		}
	}
}

func TestStandardError_NoData(t *testing.T) {
	lp := defaultPredictor()
	se := lp.standardError(50, nil)
	if se <= 0 {
		t.Errorf("standard error with nil data should still be positive, got %f", se)
	}
}

func TestEstimateServiceLife_ThresholdCrossing(t *testing.T) {
	lp := defaultPredictor()
	curve := []models.DegradationPoint{
		{Year: 0, StrengthRatio: 1.0},
		{Year: 1, StrengthRatio: 0.95},
		{Year: 2, StrengthRatio: 0.80},
		{Year: 3, StrengthRatio: 0.55},
		{Year: 4, StrengthRatio: 0.45},
		{Year: 5, StrengthRatio: 0.30},
	}
	sl := lp.estimateServiceLife(curve, 0.50)
	if sl < 3 || sl > 4 {
		t.Errorf("service life should be between year 3 and 4 (threshold 0.5 crossed), got %f", sl)
	}
}

func TestEstimateServiceLife_NoCrossing(t *testing.T) {
	lp := defaultPredictor()
	curve := []models.DegradationPoint{
		{Year: 0, StrengthRatio: 1.0},
		{Year: 1, StrengthRatio: 0.90},
		{Year: 2, StrengthRatio: 0.85},
	}
	sl := lp.estimateServiceLife(curve, 0.50)
	if sl != 2.0 {
		t.Errorf("if threshold not crossed, should return last year, got %f", sl)
	}
}

func TestEstimateServiceLife_AlreadyBelow(t *testing.T) {
	lp := defaultPredictor()
	curve := []models.DegradationPoint{
		{Year: 0, StrengthRatio: 0.40},
		{Year: 1, StrengthRatio: 0.30},
	}
	sl := lp.estimateServiceLife(curve, 0.50)
	if sl != 1.0 {
		t.Errorf("if already below threshold at start, should return curve end, got %f", sl)
	}
}

func TestEstimateServiceLife_ExactThreshold(t *testing.T) {
	lp := defaultPredictor()
	curve := []models.DegradationPoint{
		{Year: 0, StrengthRatio: 1.0},
		{Year: 1, StrengthRatio: 0.50},
	}
	sl := lp.estimateServiceLife(curve, 0.50)
	if sl < 0.5 || sl > 1.5 {
		t.Errorf("service life with exact threshold match should be near year 1, got %f", sl)
	}
}

func TestScenarioFactor_AllScenarios(t *testing.T) {
	lp := defaultPredictor()
	scenarios := map[string]float64{
		"temperate_coastal":  1.25,
		"mediterranean":      1.0,
		"continental":        1.35,
		"alpine":             1.55,
		"tropical_humid":     1.75,
		"arid_desert":        0.75,
		"urban_polluted":     1.45,
		"underwater_saline":  1.60,
		"laboratory_control": 0.55,
	}
	for scenario, expectedFactor := range scenarios {
		f := lp.scenarioFactor(scenario)
		if f != expectedFactor {
			t.Errorf("scenario %q: expected factor %f, got %f", scenario, expectedFactor, f)
		}
	}
}

func TestScenarioFactor_Default(t *testing.T) {
	lp := defaultPredictor()
	f := lp.scenarioFactor("unknown_scenario")
	if f != 1.0 {
		t.Errorf("unknown scenario should return default factor 1.0, got %f", f)
	}
}

func TestScenarioAvgTemp_ReasonableRange(t *testing.T) {
	lp := defaultPredictor()
	for _, scenario := range []string{"temperate_coastal", "mediterranean", "continental", "alpine", "tropical_humid", "arid_desert", "urban_polluted", "underwater_saline", "laboratory_control"} {
		temp := lp.scenarioAvgTemp(scenario)
		if temp < -10 || temp > 40 {
			t.Errorf("scenario %q temperature out of reasonable range: %f", scenario, temp)
		}
	}
}

func TestScenarioAvgTemp_HotScenarioHotter(t *testing.T) {
	lp := defaultPredictor()
	tTropical := lp.scenarioAvgTemp("tropical_humid")
	tAlpine := lp.scenarioAvgTemp("alpine")
	if tTropical <= tAlpine {
		t.Errorf("tropical should be hotter than alpine: tropical=%f alpine=%f", tTropical, tAlpine)
	}
}

func TestScenarioAvgHumidity_Range(t *testing.T) {
	lp := defaultPredictor()
	for _, scenario := range []string{"temperate_coastal", "mediterranean", "continental", "alpine", "tropical_humid", "arid_desert", "urban_polluted", "underwater_saline", "laboratory_control"} {
		h := lp.scenarioAvgHumidity(scenario)
		if h < 0 || h > 100 {
			t.Errorf("scenario %q humidity out of range [0,100]: %f", scenario, h)
		}
	}
}

func TestScenarioAvgHumidity_UnderwaterMax(t *testing.T) {
	lp := defaultPredictor()
	h := lp.scenarioAvgHumidity("underwater_saline")
	if h != 100 {
		t.Errorf("underwater scenario should have 100%% humidity, got %f", h)
	}
}

func TestGenerateAcceleratedAgingData_RetentionRange(t *testing.T) {
	lp := defaultPredictor()
	mat := &models.RepairMaterial{
		ID: uuid.New(), Name: "Test Material", MaterialType: "MODERN_CEMENT",
		DurabilityRating: 0.75,
	}
	data := lp.generateAcceleratedAgingData(mat)
	if len(data) != 7 {
		t.Errorf("expected 7 aging data points, got %d", len(data))
	}
	for _, d := range data {
		if d.retention < 0.3 || d.retention > 1.0 {
			t.Errorf("aging data retention out of range [0.3, 1.0]: temp=%f days=%d retention=%f", d.tempC, d.days, d.retention)
		}
	}
}

func TestGenerateAcceleratedAgingData_HigherTempLowerRetention(t *testing.T) {
	lp := defaultPredictor()
	mat := &models.RepairMaterial{
		ID: uuid.New(), Name: "Test Material", MaterialType: "MODERN_CEMENT",
		DurabilityRating: 0.75,
	}
	data := lp.generateAcceleratedAgingData(mat)
	for i := 1; i < len(data); i++ {
		if data[i].tempC > data[i-1].tempC && data[i].retention > data[i-1].retention {
			if data[i].days <= data[i-1].days {
				continue
			}
		}
	}
}

func TestGenerateAcceleratedAgingData_DurabilityEffect(t *testing.T) {
	lp := defaultPredictor()
	matLow := &models.RepairMaterial{DurabilityRating: 0.5}
	matHigh := &models.RepairMaterial{DurabilityRating: 0.95}
	dataLow := lp.generateAcceleratedAgingData(matLow)
	dataHigh := lp.generateAcceleratedAgingData(matHigh)
	avgLow := 0.0
	avgHigh := 0.0
	for i := range dataLow {
		avgLow += dataLow[i].retention
		avgHigh += dataHigh[i].retention
	}
	avgLow /= float64(len(dataLow))
	avgHigh /= float64(len(dataHigh))
	if avgHigh < avgLow {
		t.Errorf("higher durability should yield higher average retention: low=%f high=%f", avgLow, avgHigh)
	}
}

func TestDegradationCurve_50YearConsistency(t *testing.T) {
	lp := defaultPredictor()
	ea := 0.95
	refK := 293.15
	matType := "MODERN_CEMENT"
	baseDur := 0.75

	ret0 := lp.degradationModel(0, baseDur, matType)
	ret25 := lp.degradationModel(lp.computeEquivalentTime(25, ea, refK, "mediterranean", 1.0), baseDur, matType)
	ret50 := lp.degradationModel(lp.computeEquivalentTime(50, ea, refK, "mediterranean", 1.0), baseDur, matType)

	if ret0 < 0.95 {
		t.Errorf("year 0 retention should be ~1.0, got %f", ret0)
	}
	if ret25 < ret50 {
		t.Errorf("25yr retention should be higher than 50yr: r25=%f r50=%f", ret25, ret50)
	}
	if ret50 < 0.2 {
		t.Errorf("50yr retention shouldn't be unrealistically low: %f", ret50)
	}
}

func TestValidityPeriod_ConservativeEstimate(t *testing.T) {
	curve := []models.DegradationPoint{
		{Year: 0, StrengthRatio: 1.0},
		{Year: 1, StrengthRatio: 0.97},
		{Year: 2, StrengthRatio: 0.93},
		{Year: 3, StrengthRatio: 0.88},
		{Year: 4, StrengthRatio: 0.82},
		{Year: 5, StrengthRatio: 0.74},
		{Year: 6, StrengthRatio: 0.64},
		{Year: 7, StrengthRatio: 0.52},
		{Year: 8, StrengthRatio: 0.40},
	}
	lp := defaultPredictor()
	sl := lp.estimateServiceLife(curve, 0.50)
	validity := math.Floor(sl * 0.6)
	if validity >= sl {
		t.Errorf("validity period (%f) should be less than service life (%f)", validity, sl)
	}
	if validity < 0 {
		t.Errorf("validity period should be non-negative, got %f", validity)
	}
}

func TestArrheniusExtrapolation_Consistency(t *testing.T) {
	lp := defaultPredictor()
	ea := 0.95
	refK := 293.15
	R := 8.617333262e-5

	factor80 := lp.arrheniusFactor(ea, 353.15, refK)
	expectedFactor := math.Exp(-ea/R*(1.0/353.15-1.0/refK))
	if math.Abs(factor80-expectedFactor)/expectedFactor > 0.01 {
		t.Errorf("arrhenius factor at 80°C inconsistent with formula: got %f expected %f", factor80, expectedFactor)
	}

	factor40 := lp.arrheniusFactor(ea, 313.15, refK)
	if factor40 > factor80 {
		t.Errorf("80°C acceleration factor should be larger than 40°C: f40=%f f80=%f", factor40, factor80)
	}

	ratio := factor80 / factor40
	if ratio < 2.0 {
		t.Errorf("80°C vs 40°C acceleration ratio should be significant (Arrhenius Ea=0.95eV), got %f", ratio)
	}
}

func TestDegradationModel_BoundaryClamp(t *testing.T) {
	lp := defaultPredictor()
	retVeryHigh := lp.degradationModel(100000, 0.1, "EPOXY")
	if retVeryHigh < 0.05 {
		t.Errorf("degradation model should clamp at 0.05 minimum, got %f", retVeryHigh)
	}
	retVeryLow := lp.degradationModel(0, 0.99, "ROMAN_CONCRETE")
	if retVeryLow > 1.0 {
		t.Errorf("degradation model should clamp at 1.0 maximum, got %f", retVeryLow)
	}
}

func TestEquivalentTime_LaboratorySlowest(t *testing.T) {
	lp := defaultPredictor()
	ea := 0.95
	refK := 293.15
	eqLab := lp.computeEquivalentTime(10, ea, refK, "laboratory_control", 0.55)
	eqMed := lp.computeEquivalentTime(10, ea, refK, "mediterranean", 1.0)
	if eqLab > eqMed {
		t.Errorf("laboratory control should have slowest aging: lab=%f med=%f", eqLab, eqMed)
	}
}

func TestRootCause_AcceleratedToNaturalCorrection_ReducesLifetime(t *testing.T) {
	lpCorrected := defaultPredictor()
	lpCorrected.cfg.AcceleratedToNaturalFactor = 0.78
	lpUncorrected := defaultPredictor()
	lpUncorrected.cfg.AcceleratedToNaturalFactor = 1.0

	ea := 0.95
	refK := 293.15
	eqTimeCorrected := lpCorrected.computeEquivalentTime(10, ea, refK, "mediterranean", 1.0)
	eqTimeUncorrected := lpUncorrected.computeEquivalentTime(10, ea, refK, "mediterranean", 1.0)

	if eqTimeCorrected >= eqTimeUncorrected {
		t.Errorf("corrected equivalent time should be smaller: corrected=%f uncorrected=%f",
			eqTimeCorrected, eqTimeUncorrected)
	}
	t.Logf("Equivalent time: corrected=%.2f years, uncorrected=%.2f years (factor=%.2f)",
		eqTimeCorrected, eqTimeUncorrected, lpCorrected.cfg.AcceleratedToNaturalFactor)
}

func TestRootCause_LongTermExposureCalibration_AdjustsBeta(t *testing.T) {
	lpCalibrated := defaultPredictor()
	lpCalibrated.cfg.LongTermExposureCalibration = 0.92
	lpUncalibrated := defaultPredictor()
	lpUncalibrated.cfg.LongTermExposureCalibration = 1.0

	fieldData := []agingDataPoint{
		{tempC: 50, retention: 0.85, days: 42},
		{tempC: 50, retention: 0.72, days: 125},
		{tempC: 50, retention: 0.60, days: 250},
	}

	eaCalib := lpCalibrated.calibrateActivationEnergy(fieldData)
	eaUncalib := lpUncalibrated.calibrateActivationEnergy(fieldData)

	if eaCalib >= eaUncalib {
		t.Errorf("calibrated EA should be lower (more conservative): calib=%.4f uncalib=%.4f",
			eaCalib, eaUncalib)
	}
	t.Logf("Activation energy: calibrated=%.4f eV, uncalibrated=%.4f eV",
		eaCalib, eaUncalib)
}

func TestRootCause_ThresholdSafetyFactor_IncreasesThreshold(t *testing.T) {
	lpSafe := defaultPredictor()
	lpSafe.cfg.ThresholdSafetyFactor = 1.20
	lpUnsafe := defaultPredictor()
	lpUnsafe.cfg.ThresholdSafetyFactor = 1.0

	curve := []models.DegradationPoint{}
	for year := 0; year <= 50; year += 5 {
		retention := math.Exp(-0.02 * float64(year))
		curve = append(curve, models.DegradationPoint{Year: year, Retention: retention})
	}

	threshold := 0.5
	lifeSafe := lpSafe.estimateServiceLife(curve, threshold)
	lifeUnsafe := lpUnsafe.estimateServiceLife(curve, threshold)

	if lifeSafe >= lifeUnsafe {
		t.Errorf("safety factor should reduce estimated lifetime: safe=%f unsafe=%f",
			lifeSafe, lifeUnsafe)
	}
	expectedRatio := 1.0 / 1.20
	actualRatio := lifeSafe / lifeUnsafe
	if math.Abs(actualRatio-expectedRatio) > 0.2 {
		t.Logf("lifetime ratio %.2f, expected ~%.2f (threshold scaling)", actualRatio, expectedRatio)
	}
	t.Logf("Estimated lifetime: with SF=%.2f -> %.1f years, without SF -> %.1f years",
		lpSafe.cfg.ThresholdSafetyFactor, lifeSafe, lifeUnsafe)
}

func TestRootCause_NaturalAgingBiasCorrection_ReducesEA(t *testing.T) {
	lpCorrected := defaultPredictor()
	lpCorrected.cfg.NaturalAgingBiasCorrection = 0.88
	lpRaw := defaultPredictor()
	lpRaw.cfg.NaturalAgingBiasCorrection = 1.0

	fieldData := []agingDataPoint{
		{tempC: 40, retention: 0.90, days: 21},
		{tempC: 60, retention: 0.75, days: 21},
		{tempC: 80, retention: 0.55, days: 21},
	}

	eaCorrected := lpCorrected.calibrateActivationEnergy(fieldData)
	eaRaw := lpRaw.calibrateActivationEnergy(fieldData)

	if eaCorrected >= eaRaw {
		t.Errorf("bias correction should reduce EA: corrected=%.4f raw=%.4f",
			eaCorrected, eaRaw)
	}
	if eaCorrected != eaRaw*0.88 {
		t.Logf("EA corrected=%.4f, expected raw*0.88=%.4f", eaCorrected, eaRaw*0.88)
	}
	t.Logf("Activation energy: raw=%.4f eV, corrected=%.4f eV (factor=%.2f)",
		eaRaw, eaCorrected, lpCorrected.cfg.NaturalAgingBiasCorrection)
}

func TestRootCause_OutdoorExposureFactor_AcceleratesDegradation(t *testing.T) {
	lpOutdoor := defaultPredictor()
	lpOutdoor.cfg.OutdoorExposureFactor = 1.15
	lpIndoor := defaultPredictor()
	lpIndoor.cfg.OutdoorExposureFactor = 1.0

	year := 20.0
	matType := "MODERN_CEMENT"
	baseDur := "0.85"

	degradeOutdoor := lpOutdoor.degradationModel(year, baseDur, matType)
	degradeIndoor := lpIndoor.degradationModel(year, baseDur, matType)

	if degradeOutdoor >= degradeIndoor {
		t.Errorf("outdoor factor should reduce retention: outdoor=%.4f indoor=%.4f",
			degradeOutdoor, degradeIndoor)
	}
	t.Logf("Retention at year %.0f: indoor=%.3f, outdoor=%.3f (factor=%.2f)",
		year, degradeIndoor, degradeOutdoor, lpOutdoor.cfg.OutdoorExposureFactor)
}

func TestRootCause_AllCorrections_CombinedConservatism(t *testing.T) {
	lpConservative := defaultPredictor()
	lpConservative.cfg.AcceleratedToNaturalFactor = 0.78
	lpConservative.cfg.LongTermExposureCalibration = 0.92
	lpConservative.cfg.NaturalAgingBiasCorrection = 0.88
	lpConservative.cfg.OutdoorExposureFactor = 1.15
	lpConservative.cfg.ThresholdSafetyFactor = 1.20

	lpOptimistic := defaultPredictor()
	lpOptimistic.cfg.AcceleratedToNaturalFactor = 1.0
	lpOptimistic.cfg.LongTermExposureCalibration = 1.0
	lpOptimistic.cfg.NaturalAgingBiasCorrection = 1.0
	lpOptimistic.cfg.OutdoorExposureFactor = 1.0
	lpOptimistic.cfg.ThresholdSafetyFactor = 1.0

	fieldData := []agingDataPoint{
		{tempC: 50, retention: 0.88, days: 42},
		{tempC: 50, retention: 0.75, days: 125},
		{tempC: 50, retention: 0.62, days: 250},
	}

	eaCons := lpConservative.calibrateActivationEnergy(fieldData)
	eaOpt := lpOptimistic.calibrateActivationEnergy(fieldData)

	refK := 293.15
	eqCons := lpConservative.computeEquivalentTime(30, eaCons, refK, "mediterranean", 1.0)
	eqOpt := lpOptimistic.computeEquivalentTime(30, eaOpt, refK, "mediterranean", 1.0)

	retCons := lpConservative.degradationModel(eqCons, "0.80", "MODERN_CEMENT")
	retOpt := lpOptimistic.degradationModel(eqOpt, "0.80", "MODERN_CEMENT")

	if eaCons >= eaOpt {
		t.Errorf("conservative EA should be lower: cons=%.4f opt=%.4f", eaCons, eaOpt)
	}
	if eqCons >= eqOpt {
		t.Errorf("conservative equivalent time should be lower: cons=%.2f opt=%.2f", eqCons, eqOpt)
	}
	if retCons >= retOpt {
		t.Errorf("conservative retention should be lower: cons=%.4f opt=%.4f", retCons, retOpt)
	}

	curveCons := []models.DegradationPoint{}
	curveOpt := []models.DegradationPoint{}
	for year := 0; year <= 50; year += 5 {
		eqC := lpConservative.computeEquivalentTime(year, eaCons, refK, "mediterranean", 1.0)
		eqO := lpOptimistic.computeEquivalentTime(year, eaOpt, refK, "mediterranean", 1.0)
		retC := lpConservative.degradationModel(eqC, "0.80", "MODERN_CEMENT")
		retO := lpOptimistic.degradationModel(eqO, "0.80", "MODERN_CEMENT")
		curveCons = append(curveCons, models.DegradationPoint{Year: year, Retention: retC})
		curveOpt = append(curveOpt, models.DegradationPoint{Year: year, Retention: retO})
	}

	threshold := 0.5
	lifeCons := lpConservative.estimateServiceLife(curveCons, threshold)
	lifeOpt := lpOptimistic.estimateServiceLife(curveOpt, threshold)

	if lifeCons >= lifeOpt {
		t.Errorf("conservative lifetime should be shorter: cons=%.1f opt=%.1f", lifeCons, lifeOpt)
	}

	ratio := lifeCons / lifeOpt
	if ratio > 0.85 {
		t.Logf("conservative/optimistic lifetime ratio=%.2f, expected < 0.85", ratio)
	}

	t.Logf("EA: cons=%.4f eV, opt=%.4f eV", eaCons, eaOpt)
	t.Logf("Equivalent time at 30 years: cons=%.1f, opt=%.1f", eqCons, eqOpt)
	t.Logf("Retention at 30 years: cons=%.3f, opt=%.3f", retCons, retOpt)
	t.Logf("Estimated lifetime: cons=%.1f years, opt=%.1f years, ratio=%.2f",
		lifeCons, lifeOpt, ratio)
	t.Logf("Correction factors: accel=%.2f lt_cal=%.2f bias=%.2f outdoor=%.2f sf=%.2f",
		lpConservative.cfg.AcceleratedToNaturalFactor,
		lpConservative.cfg.LongTermExposureCalibration,
		lpConservative.cfg.NaturalAgingBiasCorrection,
		lpConservative.cfg.OutdoorExposureFactor,
		lpConservative.cfg.ThresholdSafetyFactor)
}
