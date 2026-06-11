package material_predictor

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func TestAgingDataPoint_Struct(t *testing.T) {
	dp := AgingDataPoint{
		TempC:      25.0,
		HumidityPct: 50.0,
		Days:       30,
		Cycles:     100,
		Retention:  0.95,
	}
	if dp.TempC != 25.0 {
		t.Errorf("expected temp 25.0, got %f", dp.TempC)
	}
	if dp.Retention != 0.95 {
		t.Errorf("expected retention 0.95, got %f", dp.Retention)
	}
}

func TestLifetimePrediction_Struct(t *testing.T) {
	pred := LifetimePrediction{
		MaterialID:   uuid.New(),
		Scenario:     "TEMPERATE",
		PredictedYears: 50.0,
		ConfidenceLow: 40.0,
		ConfidenceHigh: 60.0,
	}
	if pred.Scenario != "TEMPERATE" {
		t.Errorf("expected scenario TEMPERATE, got %s", pred.Scenario)
	}
	if pred.PredictedYears != 50.0 {
		t.Errorf("expected 50.0 years, got %f", pred.PredictedYears)
	}
}

func TestPredictorService_New(t *testing.T) {
	cfg := &config.Config{}
	service := NewPredictorService(nil, cfg)
	if service == nil {
		t.Fatal("expected non-nil service")
	}
	if service.repo != nil {
		t.Error("expected nil repo")
	}
	if service.cfg == nil {
		t.Error("expected non-nil cfg")
	}
}

func TestPredictorHandler_New(t *testing.T) {
	service := &PredictorService{}
	handler := NewPredictorHandler(service)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.service != service {
		t.Error("handler service mismatch")
	}
}

func TestPredictMaterialLifetime_InvalidMaterial(t *testing.T) {
	service := NewPredictorService(nil, &config.Config{})
	ctx := context.Background()
	matID := uuid.New()

	_, err := service.PredictMaterialLifetime(ctx, matID, "TEMPERATE")
	if err == nil {
		t.Error("expected error for nil repo, got nil")
	}
}

func TestArrheniusFactor_Basic(t *testing.T) {
	ea := 0.8
	tempK := 298.15
	refTempK := 298.15

	factor := ArrheniusFactor(ea, tempK, refTempK)
	if factor != 1.0 {
		t.Errorf("expected factor 1.0 at same temp, got %f", factor)
	}
}

func TestArrheniusFactor_HigherTemp(t *testing.T) {
	ea := 0.8
	tempK := 323.15
	refTempK := 298.15

	factor := ArrheniusFactor(ea, tempK, refTempK)
	if factor <= 1.0 {
		t.Errorf("expected factor > 1.0 at higher temp, got %f", factor)
	}
}

func TestScenarioFactor_ValidScenarios(t *testing.T) {
	tests := []struct {
		scenario string
		expected float64
	}{
		{"TEMPERATE", 1.0},
		{"TROPICAL", 1.3},
		{"DESERT", 1.5},
		{"ARCTIC", 0.7},
		{"COASTAL", 1.4},
	}
	for _, tt := range tests {
		factor := ScenarioFactor(tt.scenario)
		if factor != tt.expected {
			t.Errorf("ScenarioFactor(%s) = %f, expected %f", tt.scenario, factor, tt.expected)
		}
	}
}

func TestScenarioFactor_Default(t *testing.T) {
	factor := ScenarioFactor("INVALID_SCENARIO")
	if factor != 1.0 {
		t.Errorf("expected default factor 1.0, got %f", factor)
	}
}

func TestRound1(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.23, 1.2},
		{1.25, 1.3},
		{0.0, 0.0},
		{-1.23, -1.2},
	}
	for _, tt := range tests {
		result := Round1(tt.input)
		if result != tt.expected {
			t.Errorf("Round1(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestRound2(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.234, 1.23},
		{1.235, 1.24},
		{0.0, 0.0},
		{-1.234, -1.23},
	}
	for _, tt := range tests {
		result := Round2(tt.input)
		if result != tt.expected {
			t.Errorf("Round2(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestRound3(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.2345, 1.235},
		{1.2344, 1.234},
		{0.0, 0.0},
	}
	for _, tt := range tests {
		result := Round3(tt.input)
		diff := result - tt.expected
		if diff < -0.0001 || diff > 0.0001 {
			t.Errorf("Round3(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestRound4(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.23456, 1.2346},
		{1.23454, 1.2345},
		{0.0, 0.0},
	}
	for _, tt := range tests {
		result := Round4(tt.input)
		diff := result - tt.expected
		if diff < -0.00001 || diff > 0.00001 {
			t.Errorf("Round4(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestGenerateAcceleratedAgingData(t *testing.T) {
	mat := &models.RepairMaterial{
		MaterialType: "OPC_CONCRETE",
	}
	data := GenerateAcceleratedAgingData(mat)
	if len(data) == 0 {
		t.Error("expected non-empty aging data")
	}
	for i, dp := range data {
		if dp.Days <= 0 {
			t.Errorf("data point %d: expected positive days, got %d", i, dp.Days)
		}
		if dp.Retention <= 0 || dp.Retention > 1.0 {
			t.Errorf("data point %d: expected retention in (0,1], got %f", i, dp.Retention)
		}
	}
}
