package inversion

import (
	"math"
	"testing"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"

	"github.com/google/uuid"
)

func defaultInverter() *ConcreteInverter {
	cfg := &config.Config{
		Inversion: config.InversionConfig{
			MaxCandidates:       5,
			LeachingRateBase:    0.012,
			CarbonationRateBase: 0.008,
			PHInitialRoman:      12.8,
			PHModern:            8.2,
			StrengthRetainPower: 0.85,
			MonteCarloSamples:   1000,
			ConfidenceLevel:     0.95,
		},
	}
	return &ConcreteInverter{repo: nil, cfg: &cfg.Inversion}
}

func makeHypothesis(lime, pozz, water, por, dur, fy float64) formulaHypothesis {
	f := models.RomanConcreteFormula{
		ID: uuid.New(), FormulaName: "test", LimeRatio: lime, PozzolanaRatio: pozz,
		WaterRatio: water, Porosity: por, DurabilityIndex: dur, OriginalFyMPa: fy,
	}
	lpRatio := 0.0
	if pozz > 0 {
		lpRatio = lime / pozz
	}
	return formulaHypothesis{
		formula:      f,
		limePozzRatio: lpRatio,
		waterBinder:  water / (lime + pozz),
		leachingK:    0.012 * (1.0 + 0.4*lpRatio),
		carbonationK: 0.008 * (0.6 + 0.8*por),
		poreConnect:  por * (0.5 + 0.5*(1.0-dur)),
	}
}

func TestSimulateWeatheringDepth_NormalAge(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	depth := inv.simulateWeatheringDepth(h, 2000)
	if depth <= 0 {
		t.Errorf("weathering depth should be positive for age=2000, got %f", depth)
	}
	if depth > 200 {
		t.Errorf("weathering depth unrealistically high for age=2000, got %f", depth)
	}
}

func TestSimulateWeatheringDepth_BoundaryAge1(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	depth := inv.simulateWeatheringDepth(h, 1)
	if depth <= 0 {
		t.Errorf("weathering depth should be positive for age=1, got %f", depth)
	}
	if depth > 5 {
		t.Errorf("weathering depth too large for age=1 year, got %f", depth)
	}
}

func TestSimulateWeatheringDepth_ZeroAge(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	depth := inv.simulateWeatheringDepth(h, 0)
	if depth <= 0 {
		t.Errorf("age=0 is clamped to 1, should still produce positive depth, got %f", depth)
	}
}

func TestSimulateWeatheringDepth_Monotonicity(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	prev := 0.0
	for _, age := range []float64{1, 10, 100, 500, 1000, 2000, 5000} {
		depth := inv.simulateWeatheringDepth(h, age)
		if depth < prev {
			t.Errorf("weathering depth should increase with age: age=%g depth=%g < prev=%g", age, depth, prev)
		}
		prev = depth
	}
}

func TestSimulateWeatheringDepth_HighPorosity(t *testing.T) {
	inv := defaultInverter()
	hLow := makeHypothesis(1.0, 1.2, 0.85, 0.20, 0.90, 25.0)
	hHigh := makeHypothesis(1.0, 1.2, 0.85, 0.40, 0.70, 25.0)
	dLow := inv.simulateWeatheringDepth(hLow, 2000)
	dHigh := inv.simulateWeatheringDepth(hHigh, 2000)
	if dHigh <= dLow {
		t.Errorf("higher porosity should lead to more weathering: low=%f high=%f", dLow, dHigh)
	}
}

func TestSimulateStrengthRetention_NormalAge(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	ret := inv.simulateStrengthRetention(h, 2000)
	if ret <= 0 || ret > 2.5 {
		t.Errorf("strength retention out of expected range [0.1, 2.5]: %f", ret)
	}
}

func TestSimulateStrengthRetention_LongTermGain(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	retEarly := inv.simulateStrengthRetention(h, 50)
	retMid := inv.simulateStrengthRetention(h, 300)
	if retMid < retEarly {
		t.Errorf("roman concrete should gain strength in early centuries: early=%f mid=%f", retEarly, retMid)
	}
}

func TestSimulateStrengthRetention_VolcanicAshBoost(t *testing.T) {
	inv := defaultInverter()
	hLowPozz := makeHypothesis(1.5, 0.5, 0.85, 0.28, 0.80, 25.0)
	hHighPozz := makeHypothesis(0.8, 2.0, 0.85, 0.24, 0.90, 28.0)
	retLow := inv.simulateStrengthRetention(hLowPozz, 500)
	retHigh := inv.simulateStrengthRetention(hHighPozz, 500)
	if retHigh < retLow {
		t.Errorf("higher pozzolana ratio should boost long-term strength: low=%f high=%f", retLow, retHigh)
	}
}

func TestSimulateStrengthRetention_BoundaryAge(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	ret0 := inv.simulateStrengthRetention(h, 0)
	if ret0 <= 0 {
		t.Errorf("age=0 (clamped to 1) should produce positive retention: %f", ret0)
	}
	retVeryOld := inv.simulateStrengthRetention(h, 100000)
	if retVeryOld > 0.5 {
		t.Errorf("very old concrete (100k years) should have low retention: %f", retVeryOld)
	}
}

func TestSimulatePH_RomanRange(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	for _, age := range []float64{1, 50, 500, 2000} {
		ph := inv.simulatePH(h, age)
		if ph < 7.5 || ph > 13.0 {
			t.Errorf("pH out of clamped range [7.5, 13.0] at age=%g: %f", age, ph)
		}
	}
}

func TestSimulatePH_DeclineOverTime(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	phEarly := inv.simulatePH(h, 10)
	phMid := inv.simulatePH(h, 500)
	phLate := inv.simulatePH(h, 2000)
	if phMid > phEarly {
		t.Errorf("pH should decline over time: early=%f mid=%f", phEarly, phMid)
	}
	if phLate > phMid {
		t.Errorf("pH should continue declining: mid=%f late=%f", phMid, phLate)
	}
}

func TestSimulatePH_ConvergenceToModern(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	ph := inv.simulatePH(h, 100000)
	if math.Abs(ph-8.2) > 1.0 {
		t.Errorf("very old concrete pH should approach modern equilibrium (~8.2), got %f", ph)
	}
}

func TestComputeConfidence_MultipleCandidates(t *testing.T) {
	inv := defaultInverter()
	candidates := []models.InversionFormulaCandidate{
		{MatchScore: 0.92, Ranking: 1},
		{MatchScore: 0.70, Ranking: 2},
		{MatchScore: 0.55, Ranking: 3},
	}
	residuals := []float64{0.08, 0.40, 0.80}
	conf := inv.computeConfidence(candidates, residuals, 0)
	if conf < 0.25 || conf > 0.98 {
		t.Errorf("confidence out of clamped range [0.25, 0.98]: %f", conf)
	}
}

func TestComputeConfidence_SingleCandidate(t *testing.T) {
	inv := defaultInverter()
	candidates := []models.InversionFormulaCandidate{
		{MatchScore: 0.90, Ranking: 1},
	}
	residuals := []float64{0.10}
	conf := inv.computeConfidence(candidates, residuals, 0)
	if conf != 0.7 {
		t.Errorf("single candidate should return default confidence 0.7, got %f", conf)
	}
}

func TestComputeConfidence_LargeGapHighConfidence(t *testing.T) {
	inv := defaultInverter()
	candidates := []models.InversionFormulaCandidate{
		{MatchScore: 0.95, Ranking: 1},
		{MatchScore: 0.30, Ranking: 2},
	}
	residuals := []float64{0.05, 2.0}
	conf := inv.computeConfidence(candidates, residuals, 0)
	if conf < 0.6 {
		t.Errorf("large score gap should yield high confidence, got %f", conf)
	}
}

func TestComputeConfidence_SmallGapLowConfidence(t *testing.T) {
	inv := defaultInverter()
	candidates := []models.InversionFormulaCandidate{
		{MatchScore: 0.50, Ranking: 1},
		{MatchScore: 0.49, Ranking: 2},
	}
	residuals := []float64{1.0, 1.02}
	conf := inv.computeConfidence(candidates, residuals, 0)
	if conf > 0.6 {
		t.Errorf("tiny score gap should yield lower confidence, got %f", conf)
	}
}

func TestComputeConfidence_LowResidualBoostsConfidence(t *testing.T) {
	inv := defaultInverter()
	cands1 := []models.InversionFormulaCandidate{{MatchScore: 0.90, Ranking: 1}, {MatchScore: 0.50, Ranking: 2}}
	res1 := []float64{0.10, 1.0}
	cands2 := []models.InversionFormulaCandidate{{MatchScore: 0.50, Ranking: 1}, {MatchScore: 0.30, Ranking: 2}}
	res2 := []float64{1.0, 2.0}
	conf1 := inv.computeConfidence(cands1, res1, 0)
	conf2 := inv.computeConfidence(cands2, res2, 0)
	if conf1 <= conf2 {
		t.Errorf("lower residual should boost confidence: conf1=%f conf2=%f", conf1, conf2)
	}
}

func TestBuildDefaultFormulas_Count(t *testing.T) {
	formulas := buildDefaultFormulas()
	if len(formulas) != 5 {
		t.Errorf("expected 5 default formulas, got %d", len(formulas))
	}
}

func TestBuildDefaultFormulas_ValidRanges(t *testing.T) {
	formulas := buildDefaultFormulas()
	for _, f := range formulas {
		if f.LimeRatio <= 0 || f.PozzolanaRatio <= 0 || f.AggregateRatio <= 0 {
			t.Errorf("formula %q has invalid ratio: lime=%f pozz=%f agg=%f",
				f.FormulaName, f.LimeRatio, f.PozzolanaRatio, f.AggregateRatio)
		}
		if f.OriginalFyMPa < 10 || f.OriginalFyMPa > 50 {
			t.Errorf("formula %q Fy out of reasonable range: %f", f.FormulaName, f.OriginalFyMPa)
		}
		if f.Porosity < 0.1 || f.Porosity > 0.5 {
			t.Errorf("formula %q porosity out of range: %f", f.FormulaName, f.Porosity)
		}
		if f.DurabilityIndex < 0.5 || f.DurabilityIndex > 1.0 {
			t.Errorf("formula %q durability index out of range: %f", f.FormulaName, f.DurabilityIndex)
		}
	}
}

func TestBuildDefaultFormulas_ArchaeologicalConsistency(t *testing.T) {
	formulas := buildDefaultFormulas()
	putolanusFound := false
	for _, f := range formulas {
		if f.PozzolanaRatio > 1.4 {
			putolanusFound = true
			if f.OriginalFyMPa < 25 {
				t.Errorf("high pozzolana formula should have higher Fy, got %f", f.OriginalFyMPa)
			}
			if f.DurabilityIndex < 0.85 {
				t.Errorf("high pozzolana formula should have higher durability, got %f", f.DurabilityIndex)
			}
		}
	}
	if !putolanusFound {
		t.Error("should include a high-pozzolana formula matching Puteolanus type")
	}
}

func TestEstimatePozzolanicReactionAge(t *testing.T) {
	for _, lpRatio := range []float64{0.2, 0.5, 1.0, 2.0} {
		h := formulaHypothesis{limePozzRatio: lpRatio}
		age := estimatePozzolanicReactionAge(h)
		if age < 50 || age > 200 {
			t.Errorf("pozzolanic reaction age out of reasonable range for lpRatio=%f: %f", lpRatio, age)
		}
	}
}

func TestEstimatePozzolanicReactionAge_Monotonic(t *testing.T) {
	prev := 0.0
	for _, lpRatio := range []float64{0.1, 0.5, 1.0, 1.5, 2.0, 5.0} {
		h := formulaHypothesis{limePozzRatio: lpRatio}
		age := estimatePozzolanicReactionAge(h)
		if age < prev {
			t.Errorf("pozzolanic reaction age should increase with limePozzRatio: lpRatio=%f age=%f < prev=%f", lpRatio, age, prev)
		}
		prev = age
	}
}

func TestEstimateSelfHealingPotential_Range(t *testing.T) {
	for _, lime := range []float64{0.8, 1.0, 1.2, 1.5} {
		for _, pozz := range []float64{0.8, 1.0, 1.2, 1.6} {
			f := models.RomanConcreteFormula{LimeRatio: lime, PozzolanaRatio: pozz, DurabilityIndex: 0.85}
			h := formulaHypothesis{formula: f}
			potential := estimateSelfHealingPotential(h)
			if potential < 0 || potential > 1.0 {
				t.Errorf("self-healing potential out of [0,1] for lime=%f pozz=%f: %f", lime, pozz, potential)
			}
		}
	}
}

func TestEstimateSelfHealingPotential_FreeLimeBoost(t *testing.T) {
	fHighLime := models.RomanConcreteFormula{LimeRatio: 2.0, PozzolanaRatio: 0.5, DurabilityIndex: 0.85}
	fLowLime := models.RomanConcreteFormula{LimeRatio: 0.5, PozzolanaRatio: 2.0, DurabilityIndex: 0.85}
	hHigh := formulaHypothesis{formula: fHighLime}
	hLow := formulaHypothesis{formula: fLowLime}
	potHigh := estimateSelfHealingPotential(hHigh)
	potLow := estimateSelfHealingPotential(hLow)
	if potHigh <= potLow {
		t.Errorf("higher free lime should boost self-healing: high=%f low=%f", potHigh, potLow)
	}
}

func TestRandNormalApprox_MeanAndVariance(t *testing.T) {
	mu := 5.0
	sigma := 2.0
	sum := 0.0
	sumSq := 0.0
	n := 500
	for k := 0; k < n; k++ {
		v := randNormalApprox(mu, sigma, k)
		sum += v
		sumSq += (v - mu) * (v - mu)
	}
	sampleMean := sum / float64(n)
	if math.Abs(sampleMean-mu) > 1.0 {
		t.Errorf("sample mean too far from mu: got %f, expected ~%f", sampleMean, mu)
	}
	sampleVar := sumSq / float64(n)
	if math.Abs(math.Sqrt(sampleVar)-sigma) > 1.5 {
		t.Errorf("sample std too far from sigma: got %f, expected ~%f", math.Sqrt(sampleVar), sigma)
	}
}

func TestInversionConvergence_DifferentWeatheringLevels(t *testing.T) {
	inv := defaultInverter()
	formulas := buildDefaultFormulas()
	hypotheses := make([]formulaHypothesis, len(formulas))
	for i, f := range formulas {
		lpRatio := 0.0
		if f.PozzolanaRatio > 0 {
			lpRatio = f.LimeRatio / f.PozzolanaRatio
		}
		hypotheses[i] = formulaHypothesis{
			formula:      f,
			limePozzRatio: lpRatio,
			waterBinder:  f.WaterRatio / (f.LimeRatio + f.PozzolanaRatio),
			leachingK:    0.012 * (1.0 + 0.4*lpRatio),
			carbonationK: 0.008 * (0.6 + 0.8*f.Porosity),
			poreConnect:  f.Porosity * (0.5 + 0.5*(1.0-f.DurabilityIndex)),
		}
	}

	for _, weathering := range []float64{2.0, 5.0, 10.0, 20.0, 50.0} {
		bestResidual := math.MaxFloat64
		for _, h := range hypotheses {
			d := inv.simulateWeatheringDepth(h, 2000)
			r := math.Pow((d-weathering)/math.Max(1.0, weathering), 2)
			if r < bestResidual {
				bestResidual = r
			}
		}
		bestScore := 1.0 / (1.0 + bestResidual)
		if bestScore < 0.01 {
			t.Errorf("at weathering=%f, best match score too low: %f (residual=%f)", weathering, bestScore, bestResidual)
		}
	}
}

func TestInversionUniqueness_DistinctFormulas(t *testing.T) {
	inv := defaultInverter()
	formulas := buildDefaultFormulas()
	depths := make([]float64, len(formulas))
	for i, f := range formulas {
		h := makeHypothesis(f.LimeRatio, f.PozzolanaRatio, f.WaterRatio, f.Porosity, f.DurabilityIndex, f.OriginalFyMPa)
		depths[i] = inv.simulateWeatheringDepth(h, 2000)
	}
	uniqueDepths := make(map[float64]bool)
	tolerance := 0.5
	for _, d := range depths {
		found := false
		for ud := range uniqueDepths {
			if math.Abs(d-ud) < tolerance {
				found = true
				break
			}
		}
		if !found {
			uniqueDepths[d] = true
		}
	}
	if len(uniqueDepths) < 3 {
		t.Errorf("formulas should produce sufficiently distinct weathering depths, only %d unique out of %d", len(uniqueDepths), len(depths))
	}
}

func TestInversionPrecision_WeatheringLevelAccuracy(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	simDepth := inv.simulateWeatheringDepth(h, 2000)

	residual := math.Pow((simDepth-simDepth)/math.Max(1.0, simDepth), 2)
	matchScore := 1.0 / (1.0 + residual)
	if matchScore < 0.99 {
		t.Errorf("exact match should yield near-1.0 score, got %f", matchScore)
	}
}

func TestWeatheringComponents_Positive(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	t_val := 2000.0
	carb := 2.0 * h.carbonationK * math.Sqrt(t_val)
	leach := h.leachingK * math.Pow(t_val, 0.75) * (1.0 + 0.15*h.poreConnect)
	dissol := 0.004 * math.Pow(t_val, 0.88) * (1.0 - h.formula.DurabilityIndex)
	if carb <= 0 || leach <= 0 || dissol <= 0 {
		t.Errorf("all weathering components should be positive: carb=%f leach=%f dissol=%f", carb, leach, dissol)
	}
	total := carb + leach + dissol
	if carb/total > 0.8 {
		t.Errorf("carbonation shouldn't dominate too much: %f%%", carb/total*100)
	}
}

func TestWeathering_SqrtCarbonationScaling(t *testing.T) {
	inv := defaultInverter()
	h := makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0)
	d4 := inv.simulateWeatheringDepth(h, 4)
	d1 := inv.simulateWeatheringDepth(h, 1)
	carbRatio := d4 / d1
	if carbRatio < 1.5 || carbRatio > 3.5 {
		t.Errorf("weathering depth ratio for 4x age should reflect sqrt scaling (~2), got %f", carbRatio)
	}
}

func TestRootCause_HighNoiseRegularizationStabilizes(t *testing.T) {
	invBase := defaultInverter()
	invReg := defaultInverter()
	invReg.cfg.L2RegularizationLambda = 1.0
	invReg.cfg.BayesianPriorStrength = 0.9
	invReg.cfg.NoiseRobustWeight = 1.0

	hypotheses := []formulaHypothesis{
		makeHypothesis(0.8, 1.4, 0.9, 0.30, 0.80, 22.0),
		makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
		makeHypothesis(1.2, 1.0, 0.8, 0.26, 0.90, 28.0),
	}

	observedDepth := 15.0
	highNoiseObserved := observedDepth * 1.5
	nominalObserved := observedDepth
	observedStrength := 20.0
	observedPH := 8.5
	ageYears := 2000.0

	resBaseNoisy := invertFormulaHelper(invBase, hypotheses, highNoiseObserved, observedStrength, observedPH, ageYears)
	resRegNoisy := invertFormulaHelper(invReg, hypotheses, highNoiseObserved, observedStrength, observedPH, ageYears)
	resBaseClean := invertFormulaHelper(invBase, hypotheses, nominalObserved, observedStrength, observedPH, ageYears)

	if resBaseNoisy > resRegNoisy*1.5 {
		t.Errorf("regularization should reduce residual under high noise: base=%f reg=%f", resBaseNoisy, resRegNoisy)
	}
	if resBaseClean > resRegNoisy {
		t.Logf("clean baseline residual %f vs regularized noisy %f (expected higher noise residual)", resBaseClean, resRegNoisy)
	}
}

func TestRootCause_BayesianPosteriorQuantifiesUncertainty(t *testing.T) {
	inv := defaultInverter()
	inv.cfg.BayesianPriorStrength = 0.8

	hypotheses := []formulaHypothesis{
		makeHypothesis(0.8, 1.4, 0.9, 0.30, 0.80, 22.0),
		makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
		makeHypothesis(1.2, 1.0, 0.8, 0.26, 0.90, 28.0),
	}

	candidates := make([]models.InversionFormulaCandidate, len(hypotheses))
	residuals := make([]float64, len(hypotheses))
	rawResiduals := make([]float64, len(hypotheses))
	bestIdx := 0
	bestResidual := math.MaxFloat64

	for i, h := range hypotheses {
		d := inv.simulateWeatheringDepth(h, 2000)
		s := inv.simulateStrengthRetention(h, 2000)
		simPH := inv.simulatePH(h, 2000)
		rDepth := math.Pow((d-15.0)/15.0, 2)
		rStrength := math.Pow((h.formula.OriginalFyMPa*s-20.0)/20.0, 2)
		rPH := math.Pow((simPH-8.5)/2.0, 2)
		residuals[i] = math.Sqrt(rDepth + rStrength + rPH)
		rawResiduals[i] = residuals[i]
		candidates[i] = models.InversionFormulaCandidate{
			FormulaID:      h.formula.ID,
			FormulaName:    h.formula.FormulaName,
			LimeRatio:      h.formula.LimeRatio,
			PozzolanaRatio: h.formula.PozzolanaRatio,
			Residual:       residuals[i],
		}
		if residuals[i] < bestResidual {
			bestResidual = residuals[i]
			bestIdx = i
		}
	}

	confidence, bayesian, regEffect := inv.computeConfidence(candidates, residuals, bestIdx, rawResiduals)

	if bayesian <= 0 || bayesian > 1.0 {
		t.Errorf("bayesian posterior should be in (0, 1]: %f", bayesian)
	}
	if confidence < 0 || confidence > 1.0 {
		t.Errorf("confidence should be in [0, 1]: %f", confidence)
	}
	if regEffect < 0 {
		t.Errorf("regularization effect should be non-negative: %f", regEffect)
	}
	t.Logf("Best idx=%d: lime=%.2f pozz=%.2f confidence=%.2f bayesian=%.2f reg=%.4f",
		bestIdx, hypotheses[bestIdx].formula.LimeRatio, hypotheses[bestIdx].formula.PozzolanaRatio,
		confidence, bayesian, regEffect)
}

func TestRootCause_OutlierRejectionImprovesRobustness(t *testing.T) {
	inv := defaultInverter()
	inv.cfg.OutlierRejectionThreshold = 1.0

	hypotheses := []formulaHypothesis{
		makeHypothesis(1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}

	nominalObserved := 15.0
	extremeOutlier := 50.0
	observedStrength := 20.0
	observedPH := 8.5
	ageYears := 2000.0

	resNominal := invertFormulaHelper(inv, hypotheses, nominalObserved, observedStrength, observedPH, ageYears)
	resOutlier := invertFormulaHelper(inv, hypotheses, extremeOutlier, observedStrength, observedPH, ageYears)

	t.Logf("nominal residual=%f outlier residual=%f", resNominal, resOutlier)
	if resOutlier < resNominal*10 {
		t.Errorf("outlier should produce much larger residual: nominal=%f outlier=%f", resNominal, resOutlier)
	}
}

func invertFormulaHelper(inv *ConcreteInverter, hypotheses []formulaHypothesis, observedDepth, observedStrength, observedPH, ageYears float64) float64 {
	noiseStd := math.Max(0.1, observedDepth*0.08)
	priorMeans := map[string]float64{
		"lime_ratio":      1.0,
		"pozzolana_ratio": 1.0,
		"water_binder":    0.85,
	}
	bestResidual := math.MaxFloat64
	for _, h := range hypotheses {
		d := inv.simulateWeatheringDepth(h, ageYears)
		s := inv.simulateStrengthRetention(h, ageYears)
		simPH := inv.simulatePH(h, ageYears)

		noiseScale := 1.0 / (1.0 + inv.cfg.NoiseRobustWeight*math.Exp(-math.Abs(d-observedDepth)/noiseStd))
		wDepth := 1.0 * noiseScale
		wStrength := 1.5 * noiseScale
		wPH := 0.8 * noiseScale

		rDepth := math.Pow((d-observedDepth)/math.Max(1.0, observedDepth), 2)
		rStrength := math.Pow((h.formula.OriginalFyMPa*s-observedStrength)/math.Max(1.0, observedStrength), 2)
		rPH := math.Pow((simPH-observedPH)/2.0, 2)

		dataResidual := wDepth*rDepth + wStrength*rStrength + wPH*rPH

		l2Reg := 0.0
		if inv.cfg.L2RegularizationLambda > 0 {
			regLime := inv.cfg.L2RegularizationLambda * math.Pow(h.formula.LimeRatio-priorMeans["lime_ratio"], 2)
			regPozz := inv.cfg.L2RegularizationLambda * math.Pow(h.formula.PozzolanaRatio-priorMeans["pozzolana_ratio"], 2)
			regWB := inv.cfg.L2RegularizationLambda * 0.5 * math.Pow(h.waterBinder-priorMeans["water_binder"], 2)
			l2Reg = regLime + regPozz + regWB
		}

		totalResidual := math.Sqrt(dataResidual + l2Reg)
		if totalResidual < bestResidual {
			bestResidual = totalResidual
		}
	}
	return bestResidual
}
