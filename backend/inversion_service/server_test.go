package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aqueduct-monitor/config"
	"aqueduct-monitor/durability_inverter"
)

func setupTestServer() *InversionServer {
	cfg := config.DefaultConfig()
	return NewInversionServer(&cfg.Inversion)
}

func TestNewInversionServer(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewInversionServer(&cfg.Inversion)

	if s == nil {
		t.Fatal("server should not be nil")
	}
	if s.cfg == nil {
		t.Error("config should not be nil")
	}
	if s.router == nil {
		t.Error("router should not be nil")
	}
}

func TestHandleHealth(t *testing.T) {
	s := setupTestServer()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	status, ok := resp["status"].(string)
	if !ok || status != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}

	service, ok := resp["service"].(string)
	if !ok || service != "inversion-service" {
		t.Errorf("expected service 'inversion-service', got %v", resp["service"])
	}
}

func TestHandleGetFormulas(t *testing.T) {
	s := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/formulas", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	count, ok := resp["count"].(float64)
	if !ok || count < 3 {
		t.Errorf("expected at least 3 formulas, got count=%v", resp["count"])
	}

	formulas, ok := resp["formulas"].([]interface{})
	if !ok || len(formulas) < 3 {
		t.Errorf("expected formulas array with at least 3 items, got %v", formulas)
	}
}

func TestHandleInvert_Success(t *testing.T) {
	s := setupTestServer()

	reqBody := InversionRequest{
		SegmentID:          "test-segment-123",
		ObservedWeathering: 15.0,
		ObservedStrength:   6.5,
		ObservedPH:         9.5,
		AgeYears:           2000,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/invert", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
		t.Logf("response body: %s", w.Body.String())
	}

	var resp InversionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success=true, got false, message=%s", resp.Message)
	}
	if resp.Result == nil {
		t.Error("result should not be nil")
	}
	if resp.Confidence == nil {
		t.Error("confidence should not be nil")
	}
	if resp.BestFormula == nil {
		t.Error("best_formula should not be nil")
	}
	if len(resp.Candidates) == 0 {
		t.Error("candidates should not be empty")
	}
	if resp.ProcessTimeMs <= 0 {
		t.Error("process_time_ms should be positive")
	}
}

func TestHandleInvert_InvalidBody(t *testing.T) {
	s := setupTestServer()

	req := httptest.NewRequest("POST", "/api/v1/invert", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp InversionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success {
		t.Error("expected success=false for invalid request")
	}
	if resp.Message == "" {
		t.Error("message should not be empty for invalid request")
	}
}

func TestHandleInvert_ValidationError(t *testing.T) {
	s := setupTestServer()

	testCases := []struct {
		name string
		req  InversionRequest
	}{
		{"zero_weathering", InversionRequest{SegmentID: "test", ObservedWeathering: 0, ObservedStrength: 6.5, AgeYears: 2000}},
		{"negative_weathering", InversionRequest{SegmentID: "test", ObservedWeathering: -5, ObservedStrength: 6.5, AgeYears: 2000}},
		{"zero_strength", InversionRequest{SegmentID: "test", ObservedWeathering: 15, ObservedStrength: 0, AgeYears: 2000}},
		{"zero_age", InversionRequest{SegmentID: "test", ObservedWeathering: 15, ObservedStrength: 6.5, AgeYears: 0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tc.req)
			req := httptest.NewRequest("POST", "/api/v1/invert", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleInvert_DefaultPH(t *testing.T) {
	s := setupTestServer()

	reqBody := InversionRequest{
		SegmentID:          "test-segment-123",
		ObservedWeathering: 15.0,
		ObservedStrength:   6.5,
		ObservedPH:         0,
		AgeYears:           2000,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/invert", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleBatchInvert_Success(t *testing.T) {
	s := setupTestServer()

	batchReq := BatchInversionRequest{
		Requests: []InversionRequest{
			{SegmentID: "seg-1", ObservedWeathering: 12.0, ObservedStrength: 7.0, AgeYears: 1800},
			{SegmentID: "seg-2", ObservedWeathering: 15.0, ObservedStrength: 6.5, AgeYears: 2000},
			{SegmentID: "seg-3", ObservedWeathering: 20.0, ObservedStrength: 5.5, AgeYears: 2500},
		},
	}
	bodyBytes, _ := json.Marshal(batchReq)

	req := httptest.NewRequest("POST", "/api/v1/batch-invert", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
		t.Logf("response: %s", w.Body.String())
	}

	var resp BatchInversionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
	if len(resp.Results) != 3 {
		t.Errorf("expected 3 results, got %d", len(resp.Results))
	}
	if resp.TotalTimeMs <= 0 {
		t.Error("total_time_ms should be positive")
	}

	for i, r := range resp.Results {
		if !r.Success {
			t.Errorf("result %d should be successful", i)
		}
		if r.Result == nil {
			t.Errorf("result %d should have result data", i)
		}
	}
}

func TestHandleBatchInvert_InvalidBody(t *testing.T) {
	s := setupTestServer()

	req := httptest.NewRequest("POST", "/api/v1/batch-invert", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleBatchInvert_EmptyBatch(t *testing.T) {
	s := setupTestServer()

	batchReq := BatchInversionRequest{Requests: []InversionRequest{}}
	bodyBytes, _ := json.Marshal(batchReq)

	req := httptest.NewRequest("POST", "/api/v1/batch-invert", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp BatchInversionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestInversionResponse_Structure(t *testing.T) {
	resp := InversionResponse{
		Success:      true,
		Message:      "test",
		Result:       &durability_inverter.InversionResult{},
		Confidence:   &durability_inverter.ConfidenceMetrics{},
		BestFormula:  nil,
		Candidates:   []durability_inverter.InversionFormulaCandidate{},
		ProcessTimeMs: 123,
	}

	if !resp.Success {
		t.Error("success should be true")
	}
	if resp.Message != "test" {
		t.Error("message mismatch")
	}
	if resp.ProcessTimeMs != 123 {
		t.Error("process time mismatch")
	}
}

func TestBatchInversionResponse_Structure(t *testing.T) {
	resp := BatchInversionResponse{
		Success:     true,
		Results:     []InversionResponse{},
		TotalTimeMs: 456,
	}

	if !resp.Success {
		t.Error("success should be true")
	}
	if resp.TotalTimeMs != 456 {
		t.Error("total time mismatch")
	}
}
