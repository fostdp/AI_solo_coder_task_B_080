package seismic_fragility

import (
	"math"
	"sync"
	"testing"
	"time"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func defaultSeismicConfig() *config.SeismicConfig {
	return &config.SeismicConfig{
		FragilityCurveBeta:        0.4,
		SlightDamageMedianPGA:     0.15,
		ModerateDamageMedianPGA:   0.30,
		ExtensiveDamageMedianPGA:  0.50,
		CompleteDamageMedianPGA:   0.75,
		CapacityReductionSlight:   0.85,
		CapacityReductionModerate: 0.60,
		CapacityReductionExtensive: 0.30,
		CapacityReductionComplete: 0.10,
		RiskLevelThresholdVeryLow: 0.001,
		RiskLevelThresholdLow:     0.01,
		RiskLevelThresholdModerate: 0.05,
		RiskLevelThresholdHigh:    0.15,
		ReturnPeriod475yrPGA:      0.25,
		ReturnPeriod2475yrPGA:     0.45,
		SiteClassUncertainty:      0.25,
		SoilAmpUncertainty:        0.20,
		BetaUncertaintyMin:        0.05,
		BetaUncertaintyMax:        0.15,
		UseLiquefactionCheck:      false,
		SoilTypePrior: map[string]float64{
			"A": 0.05, "B": 0.30, "C": 0.40, "D": 0.20, "E": 0.05,
		},
	}
}

func makeTestSegment(id string, lat, lng, period, strength float64) models.StructureSegment {
	return models.StructureSegment{
		ID:                    id,
		Latitude:              lat,
		Longitude:             lng,
		PredominantPeriodSec:  period,
		DesignStrengthMPa:     strength,
		SegmentType:           "arch",
		CapacityRatio:         0.85,
		HeightM:               15.0,
		SpanM:                 5.0,
		MaterialType:          "roman_concrete",
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
	params := SegmentFragilityParamsWithUncertainty(&seg, cfg)
	if params["slight"].Med <= 0 {
		t.Errorf("expected positive median for slight, got %f", params["slight"].Med)
	}
	if params["slight"].Beta <= 0 {
		t.Errorf("expected positive beta for slight, got %f", params["slight"].Beta)
	}
	if params["moderate"].Med <= params["slight"].Med {
		t.Error("moderate median should be > slight median")
	}
}

func TestCheckLiquefactionPotential(t *testing.T) {
	cfg := defaultSeismicConfig()
	tests := []struct {
		name    string
		lat     float64
		lng     float64
		pga     float64
		enabled bool
	}{
		{"disabled", 41.9, 12.5, 0.3, false},
		{"enabled_low", 41.9, 12.5, 0.1, true},
		{"enabled_high", 41.9, 12.5, 0.5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.UseLiquefactionCheck = tt.enabled
			liq := CheckLiquefactionPotential(tt.lat, tt.lng, tt.pga, cfg)
			if tt.enabled && liq < 0 {
				t.Errorf("liquefaction potential should be >= 0, got %f", liq)
			}
			if !tt.enabled && liq != 0 {
				t.Errorf("liquefaction should be 0 when disabled, got %f", liq)
			}
		})
	}
}

func TestAssessSeismicRisk_Basic(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	result := AssessSeismicRisk(&seg, cfg)
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
		cfg.ReturnPeriod475yrPGA = pga
		seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
		result := AssessSeismicRisk(&seg, cfg)
		if !validLevels[result.OverallRiskLevel] {
			t.Errorf("invalid risk level for PGA=%f: %s", pga, result.OverallRiskLevel)
		}
	}
}

func TestComputeIDA_ResultStructure(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegment("test", 41.9, 12.5, 0.8, 25.0)
	vuln, err := ComputeIDA(&seg, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vuln.SegmentID != seg.ID {
		t.Errorf("segment ID mismatch: expected %s, got %s", seg.ID, vuln.SegmentID)
	}
	if len(vuln.FragilityCurve) == 0 {
		t.Error("expected non-empty fragility curve")
	}
	if vuln.MeanDamageIndex < 0 || vuln.MeanDamageIndex > 1 {
		t.Errorf("damage index out of bounds: %f", vuln.MeanDamageIndex)
	}
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
	if result.Vuln.SegmentID != seg.ID {
		t.Errorf("segment ID mismatch in async: expected %s, got %s", seg.ID, result.Vuln.SegmentID)
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
	if len(results) != len(segs) {
		t.Errorf("expected %d results, got %d", len(segs), len(results))
	}
	for i, r := range results {
		if r.SegmentID != segs[i].ID {
			t.Errorf("result %d ID mismatch: expected %s, got %s", i, segs[i].ID, r.SegmentID)
		}
	}
	if duration > 5*time.Second {
		t.Logf("parallel IDA took %v for %d segments", duration, len(segs))
	}
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
	result := AssessSeismicRisk(&seg, cfg)
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
	params := SegmentFragilityParamsWithUncertainty(&seg, cfg)
	for state, p := range params {
		if p.Beta < 0.4+cfg.BetaUncertaintyMin-0.01 || p.Beta > 0.4+cfg.BetaUncertaintyMax+0.01 {
			t.Errorf("beta for %s out of uncertainty range: %f", state, p.Beta)
		}
	}
}
