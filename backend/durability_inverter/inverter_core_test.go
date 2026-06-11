package durability_inverter

import (
	"fmt"
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

func TestSolveInversion_Convergence(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	ages := []float64{100, 500, 1000, 1500, 2000, 2500}
	for _, age := range ages {
		t.Run(fmt.Sprintf("age_%.0f", age), func(t *testing.T) {
			refFormula := formulas[0]
			refHyp := hyps[0]
			simDepth := SimulateWeatheringDepth(refHyp, age, cfg)
			simStrength := SimulateStrengthRetention(refHyp, age, cfg) * refFormula.OriginalFyMPa
			simPH := SimulatePH(refHyp, age, cfg)

			result := SolveInversion(hyps, simDepth, simStrength, simPH, age, cfg)

			if result.BestIdx < 0 || result.BestIdx >= len(hyps) {
				t.Errorf("age=%.0f: bestIdx out of bounds: %d", age, result.BestIdx)
			}
			if result.Candidates[0].FormulaName != refFormula.FormulaName {
				t.Logf("age=%.0f: best match=%s, expected=%s (score gap=%.4f)",
					age, result.Candidates[0].FormulaName, refFormula.FormulaName,
					result.Candidates[0].MatchScore-result.Candidates[1].MatchScore)
			}
			if len(result.Candidates) == 0 {
				t.Fatalf("age=%.0f: no candidates returned", age)
			}
		})
	}
}

func TestSolveInversion_Uniqueness(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	for fi := range formulas {
		t.Run(fmt.Sprintf("formula_%d", fi), func(t *testing.T) {
			refFormula := formulas[fi]
			refHyp := hyps[fi]
			age := 2000.0
			simDepth := SimulateWeatheringDepth(refHyp, age, cfg)
			simStrength := SimulateStrengthRetention(refHyp, age, cfg) * refFormula.OriginalFyMPa
			simPH := SimulatePH(refHyp, age, cfg)

			result := SolveInversion(hyps, simDepth, simStrength, simPH, age, cfg)
			conf := ComputeConfidence(result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, cfg)

			if result.Candidates[0].FormulaName != refFormula.FormulaName {
				t.Errorf("formula %d: best match=%s, expected=%s",
					fi, result.Candidates[0].FormulaName, refFormula.FormulaName)
			}

			if len(result.Candidates) >= 2 {
				scoreGap := result.Candidates[0].MatchScore - result.Candidates[1].MatchScore
				if scoreGap < 0.01 {
					t.Errorf("formula %d: uniqueness gap too small: %.4f", fi, scoreGap)
				}
			}

			if conf.Confidence < 0.5 {
				t.Errorf("formula %d: confidence too low for known input: %.4f", fi, conf.Confidence)
			}
		})
	}
}

func TestSolveInversion_ArchaeologicalConsistency(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	archaeologicalCases := []struct {
		name           string
		weathering     float64
		strength       float64
		ph             float64
		age            float64
		expectedEra    string
		minLimeRatio   float64
		maxLimeRatio   float64
	}{
		{"庞贝遗址_公元1世纪", 12.0, 7.2, 9.8, 1950, "罗马帝国时期", 0.8, 1.2},
		{"Ostia港口_公元2世纪", 14.5, 6.8, 9.5, 1850, "罗马帝国时期", 0.85, 1.3},
		{"罗马广场_共和时期", 18.0, 5.8, 9.2, 2100, "罗马共和国时期", 0.9, 1.4},
		{"高架水道_克劳狄时期", 16.0, 6.2, 9.3, 1980, "罗马帝国时期", 0.8, 1.25},
	}

	for _, tc := range archaeologicalCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SolveInversion(hyps, tc.weathering, tc.strength, tc.ph, tc.age, cfg)

			best := result.Candidates[0]
			if best.LimeRatio < tc.minLimeRatio || best.LimeRatio > tc.maxLimeRatio {
				t.Errorf("%s: lime ratio %.2f outside expected range [%.2f, %.2f]",
					tc.name, best.LimeRatio, tc.minLimeRatio, tc.maxLimeRatio)
			}

			if best.MatchScore < 0.6 {
				t.Errorf("%s: match score too low for archaeological data: %.4f", tc.name, best.MatchScore)
			}

			conf := ComputeConfidence(result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, cfg)
			if conf.Confidence < 0.4 {
				t.Logf("%s: confidence %.4f for archaeological data", tc.name, conf.Confidence)
			}
		})
	}
}

func TestSolveInversion_WeatheringAccuracy(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	weatheringLevels := []struct {
		name      string
		weathering float64
		strength  float64
		ph        float64
		tolerance float64
	}{
		{"轻度风化", 5.0, 18.0, 11.5, 0.15},
		{"中度风化", 15.0, 8.5, 9.8, 0.12},
		{"重度风化", 30.0, 4.5, 8.5, 0.10},
		{"极重度风化", 50.0, 2.5, 8.0, 0.08},
	}

	age := 2000.0
	for _, tc := range weatheringLevels {
		t.Run(tc.name, func(t *testing.T) {
			result := SolveInversion(hyps, tc.weathering, tc.strength, tc.ph, age, cfg)

			bestIdx := result.BestIdx
			simulatedDepth := result.SimDepths[bestIdx]
			depthError := math.Abs(simulatedDepth-tc.weathering) / tc.weathering

			if depthError > tc.tolerance {
				t.Errorf("%s: depth simulation error %.2f%% exceeds tolerance %.2f%% (sim=%.2f, obs=%.2f)",
					tc.name, depthError*100, tc.tolerance*100, simulatedDepth, tc.weathering)
			}

			simulatedStrength := result.SimStrengths[bestIdx]
			strengthError := math.Abs(simulatedStrength-tc.strength) / math.Max(1.0, tc.strength)
			if strengthError > tc.tolerance {
				t.Errorf("%s: strength simulation error %.2f%% exceeds tolerance %.2f%%",
					tc.name, strengthError*100, tc.tolerance*100)
			}

			simulatedPH := result.SimPHs[bestIdx]
			phError := math.Abs(simulatedPH - tc.ph)
			if phError > 1.0 {
				t.Errorf("%s: pH simulation error %.2f exceeds tolerance 1.0", tc.name, phError)
			}
		})
	}
}

func TestSolveInversion_BoundaryConditions(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	boundaryCases := []struct {
		name       string
		weathering float64
		strength   float64
		ph         float64
		age        float64
	}{
		{"零输入", 0.0, 0.0, 7.0, 1.0},
		{"极高风化", 100.0, 1.0, 7.5, 3000.0},
		{"极高强度", 10.0, 50.0, 12.0, 50.0},
		{"极年轻", 5.0, 20.0, 12.5, 10.0},
		{"极古老", 40.0, 3.0, 8.0, 5000.0},
	}

	for _, tc := range boundaryCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SolveInversion(hyps, tc.weathering, tc.strength, tc.ph, tc.age, cfg)

			if result == nil {
				t.Fatalf("%s: returned nil result", tc.name)
			}
			if len(result.Candidates) != len(hyps) {
				t.Errorf("%s: expected %d candidates, got %d", tc.name, len(hyps), len(result.Candidates))
			}
			for i, cand := range result.Candidates {
				if cand.MatchScore < 0 || cand.MatchScore > 1 {
					t.Errorf("%s: candidate %d match score out of bounds: %f", tc.name, i, cand.MatchScore)
				}
			}
		})
	}
}

func TestSolveInversion_AnomalyCases(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	anomalyCases := []struct {
		name       string
		weathering float64
		strength   float64
		ph         float64
		age        float64
	}{
		{"负风化值", -5.0, 10.0, 9.0, 2000.0},
		{"负强度", 15.0, -5.0, 9.0, 2000.0},
		{"pH超出范围", 15.0, 10.0, 5.0, 2000.0},
		{"负年龄", 15.0, 10.0, 9.0, -100.0},
		{"pH极高", 15.0, 10.0, 14.0, 2000.0},
	}

	for _, tc := range anomalyCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SolveInversion(hyps, tc.weathering, tc.strength, tc.ph, tc.age, cfg)

			if result == nil {
				t.Fatalf("%s: should not return nil for anomalous input", tc.name)
			}

			for i, cand := range result.Candidates {
				if math.IsNaN(cand.MatchScore) || math.IsInf(cand.MatchScore, 0) {
					t.Errorf("%s: candidate %d has invalid match score: %f", tc.name, i, cand.MatchScore)
				}
			}

			for i, r := range result.Residuals {
				if math.IsNaN(r) || math.IsInf(r, 0) {
					t.Errorf("%s: residual %d is invalid: %f", tc.name, i, r)
				}
			}
		})
	}
}

func TestComputeConfidence_Reliability(t *testing.T) {
	cfg := defaultConfig()
	formulas := BuildDefaultFormulas()
	hyps := BuildHypotheses(formulas, cfg)

	testCases := []struct {
		name           string
		weathering     float64
		strength       float64
		ph             float64
		age            float64
		minConfidence  float64
	}{
		{"理想匹配", 15.0, 6.5, 9.5, 2000.0, 0.6},
		{"接近候选", 14.0, 7.0, 9.6, 2000.0, 0.4},
		{"高噪声", 150.0, 6.5, 9.5, 2000.0, 0.3},
		{"模糊数据", 15.0, 7.0, 9.0, 2000.0, 0.3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SolveInversion(hyps, tc.weathering, tc.strength, tc.ph, tc.age, cfg)
			conf := ComputeConfidence(result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, cfg)

			if conf.Confidence < tc.minConfidence {
				t.Errorf("%s: confidence %.4f below minimum %.4f", tc.name, conf.Confidence, tc.minConfidence)
			}
			if conf.Confidence < 0.25 || conf.Confidence > 0.98 {
				t.Errorf("%s: confidence %.4f out of expected bounds [0.25, 0.98]", tc.name, conf.Confidence)
			}
			if conf.BayesianPosterior <= 0 || conf.BayesianPosterior > 1 {
				t.Errorf("%s: bayesian posterior %.4f out of bounds (0, 1]", tc.name, conf.BayesianPosterior)
			}
		})
	}
}
