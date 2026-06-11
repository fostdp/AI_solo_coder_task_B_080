package durability_inverter

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func defaultConfig() *config.InversionConfig {
	return &config.InversionConfig{
		MaxCandidates:           5,
		LeachingRateBase:        0.012,
		CarbonationRateBase:     0.008,
		PHInitialRoman:          12.8,
		PHModern:                8.2,
		StrengthRetainPower:     0.85,
		MonteCarloSamples:       1000,
		ConfidenceLevel:         0.95,
		L2RegularizationLambda:  0.15,
		BayesianPriorStrength:   0.8,
		NoiseRobustWeight:       0.6,
		OutlierRejectionThreshold: 2.5,
		MaxIterations:           50,
	}
}

func makeTestFormula(name string, lime, pozz, water, por, dur, fy float64) models.RomanConcreteFormula {
	return models.RomanConcreteFormula{
		FormulaName:     name,
		LimeRatio:       lime,
		PozzolanaRatio:  pozz,
		WaterRatio:      water,
		Porosity:        por,
		DurabilityIndex: dur,
		OriginalFyMPa:   fy,
		AggregateRatio:  3.5,
	}
}

func TestBuildHypotheses_Basic(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
		makeTestFormula("F2", 0.85, 1.6, 0.78, 0.24, 0.90, 28.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	if len(hyps) != 2 {
		t.Fatalf("expected 2 hypotheses, got %d", len(hyps))
	}
	if hyps[0].LimePozzRatio <= 0 {
		t.Errorf("expected positive lime-pozz ratio, got %f", hyps[0].LimePozzRatio)
	}
	if hyps[0].LeachingK <= 0 {
		t.Errorf("expected positive leachingK, got %f", hyps[0].LeachingK)
	}
}

func TestBuildHypotheses_ZeroPozz(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 0, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	if hyps[0].LimePozzRatio != 0 {
		t.Errorf("expected zero lime-pozz ratio for zero pozzolana, got %f", hyps[0].LimePozzRatio)
	}
}

func TestSimulateWeatheringDepth_NormalAge(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	depth := SimulateWeatheringDepth(hyps[0], 2000, cfg)
	if depth <= 0 {
		t.Errorf("weathering depth should be positive for age=2000, got %f", depth)
	}
	if depth > 200 {
		t.Errorf("weathering depth unrealistically high for age=2000, got %f", depth)
	}
}

func TestSimulateWeatheringDepth_BoundaryAge1(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	depth := SimulateWeatheringDepth(hyps[0], 1, cfg)
	if depth <= 0 {
		t.Errorf("weathering depth should be positive for age=1, got %f", depth)
	}
	if depth > 5 {
		t.Errorf("weathering depth too large for age=1 year, got %f", depth)
	}
}

func TestSimulateWeatheringDepth_ZeroAge(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	depth := SimulateWeatheringDepth(hyps[0], 0, cfg)
	if depth <= 0 {
		t.Errorf("age=0 is clamped to 1, should still produce positive depth, got %f", depth)
	}
}

func TestSimulateStrengthRetention_MonotonicDecrease(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	s100 := SimulateStrengthRetention(hyps[0], 100, cfg)
	s1000 := SimulateStrengthRetention(hyps[0], 1000, cfg)
	s2000 := SimulateStrengthRetention(hyps[0], 2000, cfg)
	if s100 < s1000 {
		t.Errorf("strength should decrease with age: s100=%f < s1000=%f", s100, s1000)
	}
	if s1000 < s2000 {
		t.Errorf("strength should decrease with age: s1000=%f < s2000=%f", s1000, s2000)
	}
}

func TestSimulateStrengthRetention_Bounds(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	for _, age := range []float64{1, 100, 1000, 5000} {
		s := SimulateStrengthRetention(hyps[0], age, cfg)
		if s < 0.1 {
			t.Errorf("strength too low for age=%f: %f", age, s)
		}
		if s > 2.5 {
			t.Errorf("strength too high for age=%f: %f", age, s)
		}
	}
}

func TestSimulatePH_Convergence(t *testing.T) {
	cfg := defaultConfig()
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.2, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, cfg)
	ph1 := SimulatePH(hyps[0], 1, cfg)
	ph1000 := SimulatePH(hyps[0], 1000, cfg)
	ph2000 := SimulatePH(hyps[0], 2000, cfg)
	if ph1 < cfg.PHModern || ph1 > cfg.PHInitialRoman {
		t.Errorf("initial pH out of bounds: %f", ph1)
	}
	if ph1000 < ph2000 {
		t.Errorf("pH should decrease with age: ph1000=%f < ph2000=%f", ph1000, ph2000)
	}
	if ph2000 < 7.5 || ph2000 > 13.0 {
		t.Errorf("final pH out of bounds: %f", ph2000)
	}
}

func TestSolveInversion_Basic(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)
	result := SolveInversion(hyps, 15.0, 6.5, 9.5, 2000, cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Candidates) != len(hyps) {
		t.Errorf("expected %d candidates, got %d", len(hyps), len(result.Candidates))
	}
	if result.BestIdx < 0 || result.BestIdx >= len(hyps) {
		t.Errorf("bestIdx out of bounds: %d", result.BestIdx)
	}
	if result.Candidates[0].Ranking != 1 {
		t.Errorf("first candidate should be rank 1, got %d", result.Candidates[0].Ranking)
	}
}

func TestSolveInversion_RegularizationEffect(t *testing.T) {
	cfg := defaultConfig()
	cfg.L2RegularizationLambda = 0.0
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)
	resultNoReg := SolveInversion(hyps, 15.0, 6.5, 9.5, 2000, cfg)
	cfg.L2RegularizationLambda = 1.0
	resultReg := SolveInversion(hyps, 15.0, 6.5, 9.5, 2000, cfg)
	if resultNoReg.RawResiduals[resultNoReg.BestIdx] > resultReg.RawResiduals[resultReg.BestIdx] {
		t.Error("regularization should increase residual relative to raw")
	}
}

func TestSolveInversion_NoiseRobustness(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)
	resultClean := SolveInversion(hyps, 15.0, 6.5, 9.5, 2000, cfg)
	resultNoisy := SolveInversion(hyps, 150.0, 6.5, 9.5, 2000, cfg)
	if resultNoisy.NoiseStd <= resultClean.NoiseStd {
		t.Error("noise std should scale with observed value")
	}
}

func TestComputeConfidence_Basic(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)
	result := SolveInversion(hyps, 15.0, 6.5, 9.5, 2000, cfg)
	conf := ComputeConfidence(result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, cfg)
	if conf.Confidence < 0.25 || conf.Confidence > 0.98 {
		t.Errorf("confidence out of bounds: %f", conf.Confidence)
	}
	if conf.BayesianPosterior <= 0 || conf.BayesianPosterior > 1 {
		t.Errorf("bayesian posterior out of bounds: %f", conf.BayesianPosterior)
	}
}

func TestComputeConfidence_SingleCandidate(t *testing.T) {
	cfg := defaultConfig()
	candidates := []models.InversionFormulaCandidate{{FormulaName: "test"}}
	conf := ComputeConfidence(candidates, []float64{0.1}, 0, []float64{0.1}, cfg)
	if conf.Confidence != 0.7 {
		t.Errorf("expected default confidence 0.7 for single candidate, got %f", conf.Confidence)
	}
	if conf.BayesianPosterior != 0.7 {
		t.Errorf("expected default posterior 0.7 for single candidate, got %f", conf.BayesianPosterior)
	}
}

func TestEstimatePozzolanicReactionAge(t *testing.T) {
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.0, 1.0, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, defaultConfig())
	age := EstimatePozzolanicReactionAge(hyps[0])
	if age < 50 || age > 200 {
		t.Errorf("pozzolanic reaction age out of expected range: %f", age)
	}
}

func TestEstimateSelfHealingPotential(t *testing.T) {
	formulas := []models.RomanConcreteFormula{
		makeTestFormula("F1", 1.5, 1.0, 0.85, 0.28, 0.85, 25.0),
	}
	hyps := BuildHypotheses(formulas, defaultConfig())
	potential := EstimateSelfHealingPotential(hyps[0])
	if potential < 0.2 || potential > 0.95 {
		t.Errorf("self healing potential out of bounds: %f", potential)
	}
}

func TestNewFormula(t *testing.T) {
	f := NewFormula("test", 1.0, 1.2, 3.5, 0.85, 8.5, 1.8, 25.0, 0.28, 0.85)
	if f.FormulaName != "test" {
		t.Errorf("expected name 'test', got %s", f.FormulaName)
	}
	if math.Abs(f.LimeRatio-1.0) > 1e-9 {
		t.Errorf("expected lime ratio 1.0, got %f", f.LimeRatio)
	}
	if f.ID == (uuid.Nil) {
		t.Error("expected non-nil UUID")
	}
}

func TestBuildDefaultFormulas(t *testing.T) {
	formulas := BuildDefaultFormulas()
	if len(formulas) != 5 {
		t.Errorf("expected 5 default formulas, got %d", len(formulas))
	}
	for i, f := range formulas {
		if f.FormulaName == "" {
			t.Errorf("formula %d has empty name", i)
		}
	}
}

func TestMathHelpers(t *testing.T) {
	if math.Abs(Sqrt(4.0)-2.0) > 1e-9 {
		t.Errorf("Sqrt(4) should be 2, got %f", Sqrt(4.0))
	}
	if Max(3.0, 5.0) != 5.0 {
		t.Errorf("Max(3,5) should be 5, got %f", Max(3.0, 5.0))
	}
	if Min(3.0, 5.0) != 3.0 {
		t.Errorf("Min(3,5) should be 3, got %f", Min(3.0, 5.0))
	}
	if Abs(-2.5) != 2.5 {
		t.Errorf("Abs(-2.5) should be 2.5, got %f", Abs(-2.5))
	}
}

func TestRandNormalApprox_Stats(t *testing.T) {
	var sum, sumSq float64
	n := 1000
	for i := 0; i < n; i++ {
		v := RandNormalApprox(0, 1, i)
		sum += v
		sumSq += v * v
	}
	mean := sum / float64(n)
	variance := sumSq/float64(n) - mean*mean
	if math.Abs(mean) > 0.5 {
		t.Errorf("mean too far from 0: %f", mean)
	}
	if variance < 0.3 || variance > 3.0 {
		t.Errorf("variance out of expected range: %f", variance)
	}
}

func TestSolveInversion_RootCause_NoiseRobustness(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)
	result := SolveInversion(hyps, 15.0, 6.5, 9.5, 2000, cfg)
	conf := ComputeConfidence(result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, cfg)
	if conf.RegularizationEffect <= 0 {
		t.Error("regularization should have positive effect when L2 lambda > 0")
	}
	if result.SignalNoiseRatio < 1.0 {
		t.Error("signal-to-noise ratio should be reasonable for valid data")
	}
}
