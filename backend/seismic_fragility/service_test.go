package seismic_fragility

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
)

func TestIDAResult_Struct(t *testing.T) {
	result := IDAResult{
		Results: nil,
		Err:     nil,
	}
	if result.Results != nil {
		t.Errorf("expected nil results, got %v", result.Results)
	}
	if result.Err != nil {
		t.Errorf("expected nil error, got %v", result.Err)
	}
}

func TestIDAJob_Struct(t *testing.T) {
	segID := uuid.New()
	job := IDAJob{
		ID:         "test-job-1",
		Status:     "running",
		SegmentIDs: []uuid.UUID{segID},
		Results:    nil,
		Error:      "",
	}
	if job.ID != "test-job-1" {
		t.Errorf("expected job id 'test-job-1', got '%s'", job.ID)
	}
	if job.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", job.Status)
	}
	if len(job.SegmentIDs) != 1 {
		t.Errorf("expected 1 segment id, got %d", len(job.SegmentIDs))
	}
}

func TestFragilityHandler_New(t *testing.T) {
	service := &FragilityService{}
	handler := NewFragilityHandler(service)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.service != service {
		t.Error("handler service mismatch")
	}
	if handler.idaJobs == nil {
		t.Error("expected initialized idaJobs map")
	}
}

func TestFragilityHandler_IDAJobsConcurrency(t *testing.T) {
	service := &FragilityService{}
	handler := NewFragilityHandler(service)

	var wg sync.WaitGroup
	jobCount := 10

	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			jobID := uuid.New().String()
			handler.idaJobsMu.Lock()
			handler.idaJobs[jobID] = &IDAJob{
				ID:     jobID,
				Status: "running",
			}
			handler.idaJobsMu.Unlock()

			handler.idaJobsMu.RLock()
			_, exists := handler.idaJobs[jobID]
			handler.idaJobsMu.RUnlock()
			if !exists {
				t.Errorf("job %s should exist", jobID)
			}
		}(i)
	}

	wg.Wait()

	handler.idaJobsMu.RLock()
	count := len(handler.idaJobs)
	handler.idaJobsMu.RUnlock()

	if count != jobCount {
		t.Errorf("expected %d jobs, got %d", jobCount, count)
	}
}

func TestComputeIDAAsync_Basic(t *testing.T) {
	service := &FragilityService{}

	resultChan := make(chan IDAResult, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	ctx := context.Background()
	segID := uuid.New()

	go func() {
		service.AnalyzeIncrementalDynamicAsync(ctx, segID, resultChan, &wg)
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case result := <-resultChan:
		if result.Err == nil {
			t.Error("expected error for nil repo/segment, got nil")
		}
	case <-resultChan:
	}
}

func TestAnalyzeBatchIDA_NoSegments(t *testing.T) {
	service := &FragilityService{}
	ctx := context.Background()

	results, err := service.AnalyzeBatchIDA(ctx, []uuid.UUID{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestNewFragilityService(t *testing.T) {
	cfg := &config.Config{}
	service := NewFragilityService(nil, cfg)
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

func TestRegionName_Basic(t *testing.T) {
	name := RegionName(41.9028, 12.4964)
	if name == "" {
		t.Error("expected non-empty region name")
	}
}

func TestBuildDefaultHistoricalEarthquakes(t *testing.T) {
	quakes := BuildDefaultHistoricalEarthquakes()
	if len(quakes) == 0 {
		t.Error("expected non-empty earthquake list")
	}
	for i, q := range quakes {
		if q.Magnitude <= 0 {
			t.Errorf("earthquake %d: expected positive magnitude, got %f", i, q.Magnitude)
		}
		if q.Year == 0 {
			t.Errorf("earthquake %d: expected non-zero year", i, q.Year)
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
		diff := result - tt.expected
		if diff < -0.001 || diff > 0.001 {
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
