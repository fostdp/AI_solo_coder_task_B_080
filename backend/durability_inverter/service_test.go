package durability_inverter

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func TestInverterService_NewLocal(t *testing.T) {
	cfg := &config.Config{}
	service := NewInverterService(nil, cfg)
	if service == nil {
		t.Fatal("expected non-nil service")
	}
	if service.useRemote {
		t.Error("expected local service (useRemote=false)")
	}
	if service.repo != nil {
		t.Error("expected nil repo")
	}
	if service.cfg == nil {
		t.Error("expected non-nil cfg")
	}
}

func TestInverterService_NewRemote(t *testing.T) {
	cfg := &config.Config{}
	service := NewInverterServiceWithRemote(nil, cfg, "http://localhost:8081")
	if service == nil {
		t.Fatal("expected non-nil service")
	}
	if !service.useRemote {
		t.Error("expected remote service (useRemote=true)")
	}
	if service.remoteClient == nil {
		t.Error("expected non-nil remote client")
	}
}

func TestInverterHandler_New(t *testing.T) {
	service := &InverterService{}
	handler := NewInverterHandler(service)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.service != service {
		t.Error("handler service mismatch")
	}
}

func TestInvertConcreteProperties_NoRepo(t *testing.T) {
	service := NewInverterService(nil, &config.Config{})
	ctx := context.Background()
	segID := uuid.New()

	_, err := service.InvertConcreteProperties(ctx, segID, 5.0, 25.0, 2000.0, 9.5)
	if err == nil {
		t.Error("expected error for nil repo, got nil")
	}
}

func TestRemoteInversionClient_New(t *testing.T) {
	baseURL := "http://localhost:8081"
	client := NewRemoteInversionClient(baseURL)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.baseURL != baseURL {
		t.Errorf("expected baseURL %s, got %s", baseURL, client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("expected non-nil http client")
	}
}

func TestRemoteInversionClient_HealthCheck_Fails(t *testing.T) {
	client := NewRemoteInversionClient("http://invalid-url:9999")
	err := client.HealthCheck()
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

func TestBuildDefaultFormulas(t *testing.T) {
	formulas := BuildDefaultFormulas()
	if len(formulas) == 0 {
		t.Error("expected non-empty formulas list")
	}
	for i, f := range formulas {
		if f.FormulaID == uuid.Nil {
			t.Errorf("formula %d: expected non-nil formula ID", i)
		}
		if f.FormulaName == "" {
			t.Errorf("formula %d: expected non-empty formula name", i)
		}
	}
}

func TestBuildHypotheses_Default(t *testing.T) {
	formulas := BuildDefaultFormulas()
	cfg := &config.InversionConfig{}
	hypotheses := BuildHypotheses(formulas, cfg)
	if len(hypotheses) != len(formulas) {
		t.Errorf("expected %d hypotheses, got %d", len(formulas), len(hypotheses))
	}
	for i, h := range hypotheses {
		if h.LimePozzRatio <= 0 {
			t.Errorf("hypothesis %d: expected positive lime-pozz ratio, got %f", i, h.LimePozzRatio)
		}
	}
}

func TestFormulaHypothesis_Struct(t *testing.T) {
	h := FormulaHypothesis{
		LimePozzRatio: 1.2,
		WaterBinder:   0.5,
		LeachingK:     0.001,
		CarbonationK:  0.002,
		PoreConnect:   0.3,
	}
	if h.LimePozzRatio != 1.2 {
		t.Errorf("expected 1.2, got %f", h.LimePozzRatio)
	}
	if h.WaterBinder != 0.5 {
		t.Errorf("expected 0.5, got %f", h.WaterBinder)
	}
}

func TestInvertRequest_Struct(t *testing.T) {
	req := InvertRequest{
		SegmentID:          uuid.New().String(),
		ObservedWeathering: 5.0,
		ObservedStrength:   25.0,
		ObservedPH:         9.5,
		AgeYears:           2000.0,
		SaveResult:         true,
	}
	if req.ObservedWeathering != 5.0 {
		t.Errorf("expected 5.0, got %f", req.ObservedWeathering)
	}
	if !req.SaveResult {
		t.Error("expected SaveResult=true")
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{4.0, 2.0},
		{9.0, 3.0},
		{0.0, 0.0},
		{2.0, 1.414},
	}
	for _, tt := range tests {
		result := Sqrt(tt.input)
		diff := result - tt.expected
		if diff < -0.01 || diff > 0.01 {
			t.Errorf("Sqrt(%f) = %f, expected ~%f", tt.input, result, tt.expected)
		}
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		a, b     float64
		expected float64
	}{
		{3.0, 5.0, 5.0},
		{-1.0, -5.0, -1.0},
		{0.0, 0.0, 0.0},
	}
	for _, tt := range tests {
		result := Max(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Max(%f, %f) = %f, expected %f", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b     float64
		expected float64
	}{
		{3.0, 5.0, 3.0},
		{-1.0, -5.0, -5.0},
		{0.0, 0.0, 0.0},
	}
	for _, tt := range tests {
		result := Min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Min(%f, %f) = %f, expected %f", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{5.0, 5.0},
		{-5.0, 5.0},
		{0.0, 0.0},
	}
	for _, tt := range tests {
		result := Abs(tt.input)
		if result != tt.expected {
			t.Errorf("Abs(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestSolveInversion_Basic(t *testing.T) {
	formulas := BuildDefaultFormulas()
	cfg := &config.InversionConfig{}
	hypotheses := BuildHypotheses(formulas, cfg)

	result := SolveInversion(hypotheses, 5.0, 25.0, 9.5, 2000.0, cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Candidates) == 0 {
		t.Error("expected non-empty formula candidates")
	}
	if result.BestIdx < 0 || result.BestIdx >= len(result.Candidates) {
		t.Errorf("invalid best index: %d (candidates: %d)", result.BestIdx, len(result.Candidates))
	}
}

func TestGetAllFormulas_Service(t *testing.T) {
	service := NewInverterService(nil, &config.Config{})
	ctx := context.Background()

	formulas, err := service.GetAllFormulas(ctx)
	if err != nil {
		t.Logf("expected error or result for nil repo: %v", err)
	}
	if len(formulas) > 0 {
		t.Logf("got %d default formulas", len(formulas))
	}
}

func TestSaveResult_NoRepo(t *testing.T) {
	service := NewInverterService(nil, &config.Config{})
	ctx := context.Background()
	result := &models.ConcreteInversionResult{}

	err := service.SaveResult(ctx, result)
	if err == nil {
		t.Error("expected error for nil repo, got nil")
	}
}

func TestGetResultsByAqueduct_NoRepo(t *testing.T) {
	service := NewInverterService(nil, &config.Config{})
	ctx := context.Background()
	aqID := uuid.New()

	_, err := service.GetResultsByAqueduct(ctx, aqID, 10)
	if err == nil {
		t.Error("expected error for nil repo, got nil")
	}
}

func TestRemoteInversionRequest_Struct(t *testing.T) {
	req := RemoteInversionRequest{
		SegmentID:          uuid.New().String(),
		AqueductID:         uuid.New().String(),
		ObservedWeathering: 5.0,
		ObservedStrength:   25.0,
		ObservedPH:         9.5,
		AgeYears:           2000.0,
	}
	if req.ObservedWeathering != 5.0 {
		t.Errorf("expected 5.0, got %f", req.ObservedWeathering)
	}
}

func TestRemoteBatchRequest_Struct(t *testing.T) {
	req := RemoteBatchRequest{
		Requests: []RemoteInversionRequest{
			{ObservedWeathering: 5.0},
			{ObservedWeathering: 8.0},
		},
	}
	if len(req.Requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(req.Requests))
	}
}
