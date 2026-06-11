package seismic_fragility

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func defaultSeismicConfig() *config.SeismicConfig {
	return &config.SeismicConfig{
		FragilityCurveBeta:         0.4,
		SlightDamageMedianPGA:      0.15,
		ModerateDamageMedianPGA:    0.30,
		ExtensiveDamageMedianPGA:   0.50,
		CompleteDamageMedianPGA:    0.75,
		CapacityReductionSlight:    0.85,
		CapacityReductionModerate:  0.60,
		CapacityReductionExtensive: 0.30,
		CapacityReductionComplete:  0.10,
		RiskLevelThresholdVeryLow:  0.001,
		RiskLevelThresholdLow:      0.01,
		RiskLevelThresholdModerate: 0.05,
		RiskLevelThresholdHigh:     0.15,
		ReturnPeriod475PGA:         0.35,
		ReturnPeriod2475PGA:        0.75,
		CapacityReductionFactor:    0.65,
		DuctilityFactor:            2.5,
		SiteClassUncertainty:       0.25,
		SoilAmpUncertainty:         0.20,
		BetaUncertaintyMin:         0.05,
		BetaUncertaintyMax:         0.15,
		UseLiquefactionCheck:       false,
		PGAComputedLevels:          20,
		DamageStates:               []string{"Slight", "Moderate", "Extensive", "Complete"},
		DamageThresholds: map[string]float64{
			"Slight":   0.05,
			"Moderate": 0.15,
			"Extensive": 0.30,
			"Complete": 0.60,
		},
		SoilTypePrior: map[string]float64{
			"A": 0.05, "B": 0.30, "C": 0.40, "D": 0.20, "E": 0.05,
		},
	}
}

func makeTestSegment(id string, lat, lng, period, strength float64) models.StructureSegment {
	return models.StructureSegment{
		ID:                   id,
		Latitude:             lat,
		Longitude:            lng,
		PredominantPeriodSec: period,
		DesignStrengthMPa:    strength,
		SegmentType:          "arch",
		CapacityRatio:        0.85,
		HeightM:              15.0,
		SpanM:                5.0,
		MaterialType:         "roman_concrete",
		SettlementMM:         0.0,
	}
}

func TestLognormalCDF_ZeroInput(t *testing.T) {
	if LognormalCDF(0, 0.3, 0.4) != 0 {
		t.Error("LognormalCDF should return 0 for x=0")
	}
}

func TestLognormalCDF_Monotonic(t *testing.T) {
	med, beta := 0.3, 0.4
	prev := -1.0
	for _, x := range []float64{0.01, 0.1, 0.3, 0.5, 1.0, 2.0} {
		v := LognormalCDF(x, med, beta)
		if v <= prev {
			t.Errorf("LognormalCDF should be monotonic increasing: x=%f, v=%f, prev=%f", x, v, prev)
		}
		prev = v
	}
}

func TestLognormalCDF_Bounds(t *testing.T) {
	med, beta := 0.3, 0.4
	for _, x := range []float64{0.001, 1000.0} {
		v := LognormalCDF(x, med, beta)
		if v < 0 || v > 1.0 {
			t.Errorf("LognormalCDF out of bounds for x=%f: %f", x, v)
		}
	}
}

func TestComputeAttenuation_Basic(t *testing.T) {
	att := ComputeAttenuation(10, 6.0)
	if att <= 0 {
		t.Errorf("attenuation should be positive, got %f", att)
	}
	attFar := ComputeAttenuation(100, 6.0)
	if attFar >= att {
		t.Errorf("attenuation should decrease with distance: near=%f, far=%f", att, attFar)
	}
}

func TestComputeAttenuation_MagnitudeEffect(t *testing.T) {
	attSmall := ComputeAttenuation(10, 5.0)
	attLarge := ComputeAttenuation(10, 7.0)
	if attLarge <= attSmall {
		t.Errorf("attenuation should increase with magnitude: small=%f, large=%f", attSmall, attLarge)
	}
}

func TestEstimateSiteClass_AllClasses(t *testing.T) {
	classes := make(map[string]int)
	for lat := 40.0; lat < 43.0; lat += 0.1 {
		for lng := 12.0; lng < 14.0; lng += 0.1 {
			c := EstimateSiteClass(lat, lng)
			classes[c]++
			if c < "A" || c > "E" {
				t.Errorf("invalid site class: %s", c)
			}
		}
	}
	if len(classes) < 2 {
		t.Errorf("should have at least 2 different site classes, got %d", len(classes))
	}
}

func TestBayesianSiteClassProbabilities_Sum(t *testing.T) {
	cfg := defaultSeismicConfig()
	probs := BayesianSiteClassProbabilities(41.9, 12.5, cfg)
	sum := 0.0
	for _, p := range probs {
		sum += p
		if p < 0 || p > 1 {
			t.Errorf("probability out of bounds: %f", p)
		}
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("probabilities should sum to 1, got %f", sum)
	}
}

func TestBayesianSiteClassProbabilities_UsesPrior(t *testing.T) {
	cfg := defaultSeismicConfig()
	cfg.SoilTypePrior = map[string]float64{"A": 0.9, "B": 0.025, "C": 0.025, "D": 0.025, "E": 0.025}
	probs := BayesianSiteClassProbabilities(41.9, 12.5, cfg)
	if probs["A"] < 0.5 {
		t.Errorf("strong prior for A should result in high A probability, got %f", probs["A"])
	}
}

func TestSoilAmplificationFactor_AllClasses(t *testing.T) {
	pga := 0.3
	for _, c := range []string{"A", "B", "C", "D", "E"} {
		amp := SoilAmplificationFactor(c, pga)
		if amp < 0.8 {
			t.Errorf("amplification too low for class %s: %f", c, amp)
		}
	}
}

func TestSoilAmplificationFactor_MonotonicClass(t *testing.T) {
	pga := 0.3
	ampA := SoilAmplificationFactor("A", pga)
	ampB := SoilAmplificationFactor("B", pga)
	ampC := SoilAmplificationFactor("C", pga)
	ampD := SoilAmplificationFactor("D", pga)
	if ampA > ampB || ampB > ampC || ampC > ampD {
		t.Errorf("amplification should increase from A to D: A=%f, B=%f, C=%f, D=%f", ampA, ampB, ampC, ampD)
	}
}

func TestExpectedSoilAmplificationWithUncertainty(t *testing.T) {
	cfg := defaultSeismicConfig()
	probs := map[string]float64{"A": 0.1, "B": 0.3, "C": 0.4, "D": 0.15, "E": 0.05}
	result := ExpectedSoilAmplificationWithUncertainty(probs, 0.3, cfg)
	if result.MeanAmp <= 0 {
		t.Errorf("expected positive mean amplification, got %f", result.MeanAmp)
	}
	if result.StdAmp < 0 {
		t.Errorf("expected non-negative std amplification, got %f", result.StdAmp)
	}
	if result.BestClass == "" {
		t.Error("expected non-empty best class")
	}
}

func TestSegmentFragilityParamsWithUncertainty(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	params := SegmentFragilityParamsWithUncertainty(&seg, 0.5, cfg)
	if params.Med <= 0 {
		t.Errorf("expected positive median, got %f", params.Med)
	}
	if params.Beta <= 0 {
		t.Errorf("expected positive beta, got %f", params.Beta)
	}
	if params.Beta < 0.25 || params.Beta > 0.85 {
		t.Errorf("beta out of expected range [0.25, 0.85]: %f", params.Beta)
	}
}

func TestSegmentFragilityParams_DifferentTypes(t *testing.T) {
	cfg := defaultSeismicConfig()
	segArch := makeTestSegment("arch", 41.9, 12.5, 0.8, 25.0)
	segArch.SegmentType = "arch"
	segPier := makeTestSegment("pier", 41.9, 12.5, 0.8, 25.0)
	segPier.SegmentType = "pier"

	paramsArch := SegmentFragilityParams(&segArch, cfg)
	paramsPier := SegmentFragilityParams(&segPier, cfg)

	if paramsArch.Med >= paramsPier.Med {
		t.Errorf("arch should have lower median PGA than pier: arch=%f, pier=%f", paramsArch.Med, paramsPier.Med)
	}
}

func TestCheckLiquefactionPotential(t *testing.T) {
	cfg := defaultSeismicConfig()
	tests := []struct {
		name      string
		pga       float64
		siteClass string
		enabled   bool
	}{
		{"disabled", 0.3, "D", false},
		{"enabled_low", 0.1, "D", true},
		{"enabled_high", 0.5, "D", true},
		{"site_A_no_liquefaction", 0.5, "A", true},
		{"site_E_high_liquefaction", 0.5, "E", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.UseLiquefactionCheck = tt.enabled
			liq := CheckLiquefactionPotential(tt.pga, tt.siteClass, cfg)
			if tt.enabled && liq < 0 {
				t.Errorf("liquefaction potential should be >= 0, got %f", liq)
			}
			if tt.enabled && liq > 1 {
				t.Errorf("liquefaction potential should be <= 1, got %f", liq)
			}
			if !tt.enabled && liq != 0 {
				t.Errorf("liquefaction should be 0 when disabled, got %f", liq)
			}
			if tt.enabled && tt.siteClass == "A" && liq != 0 {
				t.Errorf("site class A should have 0 liquefaction potential, got %f", liq)
			}
		})
	}
}

func TestAssessSeismicRisk_Basic(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	segments := []models.StructureSegment{seg}
	result := AssessSeismicRisk(segments, 41.9, 12.5, 10.0, cfg)
	if result.OverallRiskLevel == "" {
		t.Error("expected non-empty risk level")
	}
	if result.PGA475 <= 0 {
		t.Errorf("expected positive 475yr PGA, got %f", result.PGA475)
	}
	if result.SiteClass == "" {
		t.Error("expected non-empty site class")
	}
}

func TestAssessSeismicRisk_ValidRiskLevels(t *testing.T) {
	cfg := defaultSeismicConfig()
	validLevels := map[string]bool{
		"VERY_LOW": true, "LOW": true, "MODERATE": true, "HIGH": true, "VERY_HIGH": true,
	}
	for _, pga := range []float64{0.05, 0.15, 0.3, 0.5, 0.8} {
		cfg.ReturnPeriod475PGA = pga
		seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
		segments := []models.StructureSegment{seg}
		result := AssessSeismicRisk(segments, 41.9, 12.5, 10.0, cfg)
		if !validLevels[result.OverallRiskLevel] {
			t.Errorf("invalid risk level for PGA=%f: %s", pga, result.OverallRiskLevel)
		}
	}
}

func TestComputeIDA_ResultStructure(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	vulns, err := ComputeIDA(&seg, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vulns) == 0 {
		t.Error("expected non-empty vulnerability results")
	}
	for _, v := range vulns {
		if v.SegmentID == nil || *v.SegmentID != seg.ID {
			t.Errorf("segment ID mismatch: expected %s, got %v", seg.ID, v.SegmentID)
		}
		if v.Probability < 0 || v.Probability > 1 {
			t.Errorf("probability out of bounds: %f", v.Probability)
		}
		if v.FragilityCurveParams == nil {
			t.Error("expected non-nil fragility curve params")
		}
	}
}

func TestComputeIDA_Accuracy(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	vulns, err := ComputeIDA(&seg, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCount := len(cfg.DamageStates) * 10
	if len(vulns) != expectedCount {
		t.Errorf("expected %d results, got %d", expectedCount, len(vulns))
	}

	pgaLevels := map[float64]bool{}
	for _, v := range vulns {
		pgaLevels[v.PGAG] = true
		if v.Magnitude < 3.0 || v.Magnitude > 9.0 {
			t.Errorf("magnitude out of expected range: %f", v.Magnitude)
		}
	}
	if len(pgaLevels) != 10 {
		t.Errorf("expected 10 distinct PGA levels, got %d", len(pgaLevels))
	}
}

func TestComputeIDA_ComputationalEfficiency(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)

	start := time.Now()
	_, err := ComputeIDA(&seg, cfg)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if duration > 500*time.Millisecond {
		t.Errorf("IDA analysis took too long: %v", duration)
	}
	t.Logf("IDA analysis completed in %v", duration)
}

func TestComputeIDAAsync_Concurrent(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	resultChan := make(chan IDAResult, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go ComputeIDAAsync(&seg, cfg, resultChan, &wg)
	wg.Wait()
	close(resultChan)
	result := <-resultChan
	if result.Err != nil {
		t.Fatalf("unexpected error in async IDA: %v", result.Err)
	}
	if len(result.Results) == 0 {
		t.Error("expected non-empty results in async IDA")
	}
}

func TestComputeIDAParallel_MultipleSegments(t *testing.T) {
	cfg := defaultSeismicConfig()
	segs := []models.StructureSegment{
		makeTestSegment("seg1", 41.9, 12.5, 0.8, 25.0),
		makeTestSegment("seg2", 42.0, 12.6, 0.9, 28.0),
		makeTestSegment("seg3", 41.8, 12.4, 0.7, 22.0),
	}
	start := time.Now()
	results, err := ComputeIDAParallel(segs, cfg)
	duration := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error in parallel IDA: %v", err)
	}

	expectedPerSegment := len(cfg.DamageStates) * 10
	expectedTotal := len(segs) * expectedPerSegment
	if len(results) != expectedTotal {
		t.Errorf("expected %d results, got %d", expectedTotal, len(results))
	}

	for _, r := range results {
		if r.Probability < 0 || r.Probability > 1 {
			t.Errorf("probability out of bounds: %f", r.Probability)
		}
	}

	t.Logf("parallel IDA took %v for %d segments (%d results)", duration, len(segs), len(results))
}

func TestComputeIDAParallel_EmptyInput(t *testing.T) {
	cfg := defaultSeismicConfig()
	results, err := ComputeIDAParallel([]models.StructureSegment{}, cfg)
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestRootCause_SiteClassUncertainty(t *testing.T) {
	cfg := defaultSeismicConfig()
	cfg.SiteClassUncertainty = 0.5
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	segments := []models.StructureSegment{seg}
	result := AssessSeismicRisk(segments, 41.9, 12.5, 10.0, cfg)
	if result.SoilAmpStd <= 0 {
		t.Error("with high uncertainty, std should be positive")
	}
	if len(result.SiteClassProbabilities) != 5 {
		t.Errorf("expected 5 site class probabilities, got %d", len(result.SiteClassProbabilities))
	}
}

func TestRootCause_BetaUncertaintyRange(t *testing.T) {
	cfg := defaultSeismicConfig()
	cfg.BetaUncertaintyMin = 0.1
	cfg.BetaUncertaintyMax = 0.2
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	params := SegmentFragilityParamsWithUncertainty(&seg, 0.5, cfg)
	if params.Beta < 0.25 || params.Beta > 0.85 {
		t.Errorf("beta out of expected range [0.25, 0.85]: %f", params.Beta)
	}
}

func TestComputeIDA_ProbabilityMagnitudeResponse(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	vulns, err := ComputeIDA(&seg, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, state := range cfg.DamageStates {
		t.Run(state, func(t *testing.T) {
			var stateResults []models.SeismicVulnerability
			for _, v := range vulns {
				if v.DamageState == state {
					stateResults = append(stateResults, v)
				}
			}

			if len(stateResults) == 0 {
				t.Fatalf("no results for damage state %s", state)
			}

			for i := 1; i < len(stateResults); i++ {
				if stateResults[i].Magnitude < stateResults[i-1].Magnitude {
					t.Errorf("%s: magnitude should increase with PGA: prev=%f, curr=%f",
						state, stateResults[i-1].Magnitude, stateResults[i].Magnitude)
				}
				if stateResults[i].Probability < stateResults[i-1].Probability-0.01 {
					t.Errorf("%s: probability should generally increase with magnitude: PGA=%f, prob=%f; prev PGA=%f, prob=%f",
						state, stateResults[i].PGAG, stateResults[i].Probability,
						stateResults[i-1].PGAG, stateResults[i-1].Probability)
				}
			}

			lowMagProb := stateResults[0].Probability
			highMagProb := stateResults[len(stateResults)-1].Probability
			if highMagProb < lowMagProb+0.1 {
				t.Logf("%s: probability increase from low to high magnitude: %.4f -> %.4f",
					state, lowMagProb, highMagProb)
			}
		})
	}
}

func TestGenerateFragilityCurve_Monotonic(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	curve := GenerateFragilityCurve(&seg, cfg)

	if len(curve) != cfg.PGAComputedLevels {
		t.Errorf("expected %d curve points, got %d", cfg.PGAComputedLevels, len(curve))
	}

	for i := 1; i < len(curve); i++ {
		if curve[i].PGA <= curve[i-1].PGA {
			t.Errorf("PGA should be increasing: prev=%f, curr=%f", curve[i-1].PGA, curve[i].PGA)
		}
		if curve[i].Slight < curve[i-1].Slight-0.01 {
			t.Errorf("Slight probability should be non-decreasing: prev=%f, curr=%f", curve[i-1].Slight, curve[i].Slight)
		}
		if curve[i].Complete < curve[i-1].Complete-0.01 {
			t.Errorf("Complete probability should be non-decreasing: prev=%f, curr=%f", curve[i-1].Complete, curve[i].Complete)
		}
	}

	for _, pt := range curve {
		for _, prob := range []float64{pt.Slight, pt.Moderate, pt.Extensive, pt.Complete} {
			if prob < 0 || prob > 1 {
				t.Errorf("probability out of bounds at PGA=%f: %f", pt.PGA, prob)
			}
		}
		if pt.Slight < pt.Moderate || pt.Moderate < pt.Extensive || pt.Extensive < pt.Complete {
			t.Errorf("damage state probabilities out of order at PGA=%f: Slight=%.3f, Moderate=%.3f, Extensive=%.3f, Complete=%.3f",
				pt.PGA, pt.Slight, pt.Moderate, pt.Extensive, pt.Complete)
		}
	}
}

func TestAssessSeismicRisk_RiskLevelClassification(t *testing.T) {
	cfg := defaultSeismicConfig()
	testCases := []struct {
		name         string
		pga475       float64
		capacityRatio float64
		settlementMM float64
		minLevel     string
		maxLevel     string
	}{
		{"极低风险", 0.05, 1.0, 0.0, "VERY_LOW", "LOW"},
		{"低风险", 0.15, 0.9, 5.0, "LOW", "MODERATE"},
		{"中等风险", 0.35, 0.7, 15.0, "MODERATE", "HIGH"},
		{"高风险", 0.6, 0.5, 30.0, "MODERATE", "VERY_HIGH"},
		{"极高风险", 0.8, 0.3, 50.0, "HIGH", "VERY_HIGH"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.ReturnPeriod475PGA = tc.pga475
			seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
			seg.CapacityRatio = tc.capacityRatio
			seg.SettlementMM = tc.settlementMM
			segments := []models.StructureSegment{seg}
			result := AssessSeismicRisk(segments, 41.9, 12.5, 10.0, cfg)

			levelOrder := map[string]int{"VERY_LOW": 0, "LOW": 1, "MODERATE": 2, "HIGH": 3, "VERY_HIGH": 4}
			resultLevel := levelOrder[result.OverallRiskLevel]
			minLevel := levelOrder[tc.minLevel]
			maxLevel := levelOrder[tc.maxLevel]

			if resultLevel < minLevel || resultLevel > maxLevel {
				t.Errorf("%s: risk level %s outside expected range [%s, %s], riskMetrics=%.4f",
					tc.name, result.OverallRiskLevel, tc.minLevel, tc.maxLevel, result.RiskMetrics)
			}
		})
	}
}

func TestAssessSeismicRisk_BoundaryConditions(t *testing.T) {
	cfg := defaultSeismicConfig()

	boundaryCases := []struct {
		name       string
		segments   []models.StructureSegment
		lat        float64
		lng        float64
		lengthKM   float64
	}{
		{"单段", []models.StructureSegment{makeTestSegment("seg1", 41.9, 12.5, 0.8, 25.0)}, 41.9, 12.5, 1.0},
		{"多段", []models.StructureSegment{
			makeTestSegment("seg1", 41.9, 12.5, 0.8, 25.0),
			makeTestSegment("seg2", 41.91, 12.51, 0.9, 28.0),
			makeTestSegment("seg3", 41.89, 12.49, 0.7, 22.0),
		}, 41.9, 12.5, 5.0},
		{"极短水道", []models.StructureSegment{makeTestSegment("seg1", 41.9, 12.5, 0.8, 25.0)}, 41.9, 12.5, 0.1},
		{"极长水道", []models.StructureSegment{makeTestSegment("seg1", 41.9, 12.5, 0.8, 25.0)}, 41.9, 12.5, 100.0},
	}

	for _, tc := range boundaryCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AssessSeismicRisk(tc.segments, tc.lat, tc.lng, tc.lengthKM, cfg)
			if result == nil {
				t.Fatalf("%s: returned nil result", tc.name)
			}
			if result.OverallRiskLevel == "" {
				t.Errorf("%s: empty risk level", tc.name)
			}
			if result.RiskMetrics < 0 || result.RiskMetrics > 1 {
				t.Errorf("%s: risk metrics out of bounds: %f", tc.name, result.RiskMetrics)
			}
			if result.PGA475 <= 0 || result.PGA2475 <= 0 {
				t.Errorf("%s: PGA values should be positive: 475=%f, 2475=%f", tc.name, result.PGA475, result.PGA2475)
			}
			if result.PGA2475 <= result.PGA475 {
				t.Errorf("%s: 2475yr PGA should be > 475yr PGA: 475=%f, 2475=%f", tc.name, result.PGA475, result.PGA2475)
			}
		})
	}
}

func TestAssessSeismicRisk_AnomalyCases(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	segments := []models.StructureSegment{seg}

	anomalyCases := []struct {
		name     string
		lat      float64
		lng      float64
		lengthKM float64
	}{
		{"零长度", 41.9, 12.5, 0.0},
		{"负长度", 41.9, 12.5, -10.0},
		{"极高纬度", 90.0, 0.0, 10.0},
		{"极低纬度", -90.0, 0.0, 10.0},
	}

	for _, tc := range anomalyCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AssessSeismicRisk(segments, tc.lat, tc.lng, tc.lengthKM, cfg)

			if result == nil {
				t.Fatalf("%s: should not return nil for anomalous input", tc.name)
			}

			if math.IsNaN(result.RiskMetrics) || math.IsInf(result.RiskMetrics, 0) {
				t.Errorf("%s: risk metrics is invalid: %f", tc.name, result.RiskMetrics)
			}
			if math.IsNaN(result.PGA475) || math.IsInf(result.PGA475, 0) {
				t.Errorf("%s: PGA475 is invalid: %f", tc.name, result.PGA475)
			}
		})
	}
}

func TestEvaluateSegmentRisk_MagnitudeResponse(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)

	pgaLevels := []float64{0.01, 0.05, 0.1, 0.2, 0.3, 0.5, 0.8, 1.2}
	prevRisk := -1.0
	for _, pga := range pgaLevels {
		risk := EvaluateSegmentRisk(&seg, pga, 0.8, 1.0, cfg)
		if risk < 0 || risk > 1 {
			t.Errorf("PGA=%.2f: risk out of bounds: %f", pga, risk)
		}
		if risk < prevRisk-0.01 {
			t.Errorf("risk should generally increase with PGA: PGA=%.2f, risk=%.4f; prev risk=%.4f",
				pga, risk, prevRisk)
		}
		prevRisk = risk
	}

	lowRisk := EvaluateSegmentRisk(&seg, 0.05, 0.8, 1.0, cfg)
	highRisk := EvaluateSegmentRisk(&seg, 0.8, 0.8, 1.0, cfg)
	if highRisk <= lowRisk {
		t.Errorf("high PGA should produce higher risk: low=%.4f, high=%.4f", lowRisk, highRisk)
	}
}

func TestEstimateRepairCost_DamageStates(t *testing.T) {
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)

	states := []string{"Slight", "Moderate", "Extensive", "Complete"}
	prevCost := -1.0
	for _, state := range states {
		cost := EstimateRepairCost(&seg, state, 1.0)
		if cost <= 0 {
			t.Errorf("%s: repair cost should be positive: %f", state, cost)
		}
		if cost < prevCost {
			t.Errorf("repair cost should increase with damage severity: %s=%.0f, prev=%.0f", state, cost, prevCost)
		}
		prevCost = cost
	}

	costZeroProb := EstimateRepairCost(&seg, "Slight", 0.001)
	if costZeroProb != 0 {
		t.Errorf("repair cost should be 0 for negligible probability: %f", costZeroProb)
	}
}

func TestPGAToMagnitudeApprox_Monotonic(t *testing.T) {
	prevMag := -1.0
	for _, pga := range []float64{0.01, 0.05, 0.1, 0.2, 0.5, 1.0, 2.0} {
		mag := PGAToMagnitudeApprox(pga, 30.0)
		if mag < 3.0 || mag > 9.0 {
			t.Errorf("PGA=%.2f: magnitude out of expected range: %f", pga, mag)
		}
		if mag < prevMag {
			t.Errorf("magnitude should increase with PGA: PGA=%.2f, mag=%.2f, prev=%.2f", pga, mag, prevMag)
		}
		prevMag = mag
	}
}

func TestBuildDefaultHistoricalEarthquakes(t *testing.T) {
	earthquakes := BuildDefaultHistoricalEarthquakes()
	if len(earthquakes) == 0 {
		t.Fatal("expected non-empty earthquake list")
	}

	prevMag := 10.0
	for _, eq := range earthquakes {
		if eq.Magnitude < 4.0 || eq.Magnitude > 8.0 {
			t.Errorf("%s: magnitude out of expected range: %f", eq.EventName, eq.Magnitude)
		}
		if eq.Magnitude > prevMag+0.01 {
			t.Errorf("earthquakes should be sorted by magnitude descending: %s=%.1f, prev=%.1f",
				eq.EventName, eq.Magnitude, prevMag)
		}
		prevMag = eq.Magnitude
		if eq.EventName == "" {
			t.Error("earthquake has empty name")
		}
	}
}

func TestRiskInterval_Uncertainty(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	segments := []models.StructureSegment{seg}
	result := AssessSeismicRisk(segments, 41.9, 12.5, 10.0, cfg)

	if result.RiskInterval.Low < 0 || result.RiskInterval.Low > 1 {
		t.Errorf("risk interval low out of bounds: %f", result.RiskInterval.Low)
	}
	if result.RiskInterval.High < 0 || result.RiskInterval.High > 1 {
		t.Errorf("risk interval high out of bounds: %f", result.RiskInterval.High)
	}
	if result.RiskInterval.Mean < result.RiskInterval.Low || result.RiskInterval.Mean > result.RiskInterval.High {
		t.Errorf("risk interval mean should be between low and high: low=%f, mean=%f, high=%f",
			result.RiskInterval.Low, result.RiskInterval.Mean, result.RiskInterval.High)
	}
	if result.RiskInterval.Uncertainty < 0 {
		t.Errorf("risk interval uncertainty should be non-negative: %f", result.RiskInterval.Uncertainty)
	}
}

func TestCapacityDemandSpectrum(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)

	for _, state := range cfg.DamageStates {
		t.Run(fmt.Sprintf("capacity_%s", state), func(t *testing.T) {
			capSpec := CapacitySpectrumData(&seg, state, cfg)
			if capSpec == nil {
				t.Fatalf("%s: capacity spectrum is nil", state)
			}
			if _, ok := capSpec["curve_points"]; !ok {
				t.Errorf("%s: missing curve_points in capacity spectrum", state)
			}
			if _, ok := capSpec["yield_disp_m"]; !ok {
				t.Errorf("%s: missing yield_disp_m in capacity spectrum", state)
			}
			if _, ok := capSpec["ultimate_sa_g"]; !ok {
				t.Errorf("%s: missing ultimate_sa_g in capacity spectrum", state)
			}
		})
	}

	for _, pga := range []float64{0.1, 0.3, 0.5} {
		t.Run(fmt.Sprintf("demand_pga_%.1f", pga), func(t *testing.T) {
			demandSpec := DemandSpectrumData(pga)
			if demandSpec == nil {
				t.Fatalf("PGA=%.1f: demand spectrum is nil", pga)
			}
			if _, ok := demandSpec["curve_points"]; !ok {
				t.Errorf("PGA=%.1f: missing curve_points in demand spectrum", pga)
			}
			if v, ok := demandSpec["pga_g"].(float64); !ok || math.Abs(v-pga) > 0.001 {
				t.Errorf("PGA=%.1f: pga_g mismatch in demand spectrum", pga)
			}
		})
	}
}

func TestAssessSeismicRisk_MultipleSegments(t *testing.T) {
	cfg := defaultSeismicConfig()

	segGood := makeTestSegment("good", 41.9, 12.5, 0.8, 30.0)
	segGood.CapacityRatio = 1.0
	segGood.SettlementMM = 0.0

	segPoor := makeTestSegment("poor", 41.9, 12.5, 0.8, 20.0)
	segPoor.CapacityRatio = 0.4
	segPoor.SettlementMM = 40.0

	segmentsGood := []models.StructureSegment{segGood, segGood, segGood}
	segmentsMixed := []models.StructureSegment{segGood, segPoor, segGood}
	segmentsPoor := []models.StructureSegment{segPoor, segPoor, segPoor}

	resultGood := AssessSeismicRisk(segmentsGood, 41.9, 12.5, 10.0, cfg)
	resultMixed := AssessSeismicRisk(segmentsMixed, 41.9, 12.5, 10.0, cfg)
	resultPoor := AssessSeismicRisk(segmentsPoor, 41.9, 12.5, 10.0, cfg)

	if resultGood.RiskMetrics >= resultMixed.RiskMetrics {
		t.Errorf("good segments should have lower risk than mixed: good=%.4f, mixed=%.4f",
			resultGood.RiskMetrics, resultMixed.RiskMetrics)
	}
	if resultMixed.RiskMetrics >= resultPoor.RiskMetrics {
		t.Errorf("mixed segments should have lower risk than poor: mixed=%.4f, poor=%.4f",
			resultMixed.RiskMetrics, resultPoor.RiskMetrics)
	}
	if resultGood.VulnerableSegCount > resultMixed.VulnerableSegCount {
		t.Errorf("good segments should have fewer vulnerable segments: good=%d, mixed=%d",
			resultGood.VulnerableSegCount, resultMixed.VulnerableSegCount)
	}
	if resultMixed.VulnerableSegCount > resultPoor.VulnerableSegCount {
		t.Errorf("mixed segments should have fewer vulnerable segments than poor: mixed=%d, poor=%d",
			resultMixed.VulnerableSegCount, resultPoor.VulnerableSegCount)
	}
}
