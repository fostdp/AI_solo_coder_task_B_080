package aqueduct_comparator

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func TestComparisonResult_Struct(t *testing.T) {
	result := ComparisonResult{
		Ranking:        map[string]interface{}{"score": 0.8},
		Recommendation: "test recommendation",
	}
	if result.Recommendation != "test recommendation" {
		t.Errorf("expected 'test recommendation', got %s", result.Recommendation)
	}
	if result.Ranking["score"] != 0.8 {
		t.Errorf("expected score 0.8, got %v", result.Ranking["score"])
	}
}

func TestComparatorService_New(t *testing.T) {
	cfg := &config.Config{}
	service := NewComparatorService(nil, cfg)
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

func TestComparatorHandler_New(t *testing.T) {
	service := &ComparatorService{}
	handler := NewComparatorHandler(service)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.service != service {
		t.Error("handler service mismatch")
	}
}

func TestCompareAqueducts_NoRepo(t *testing.T) {
	service := NewComparatorService(nil, &config.Config{})
	ctx := context.Background()
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	_, err := service.CompareAqueducts(ctx, ids)
	if err == nil {
		t.Error("expected error for nil repo, got nil")
	}
}

func TestClassifyHeritageType_NewAqueduct(t *testing.T) {
	aq := &models.Aqueduct{
		ID:               uuid.New(),
		Name:             "Test Aqueduct",
		ConstructionYear: -50,
		LengthKM:         50.0,
		HeightM:          20.0,
	}
	td := &models.AqueductTourismData{
		HistoricalSignificance: 0.8,
	}
	heritageType := ClassifyHeritageType(aq, td)
	if heritageType == "" {
		t.Error("expected non-empty heritage type")
	}
}

func TestBuildDefaultTourismData(t *testing.T) {
	aq := &models.Aqueduct{
		ID:               uuid.New(),
		Name:             "Claudian Aqueduct",
		ConstructionYear: -38,
		LengthKM:         68.8,
		HeightM:          28.0,
	}
	td := BuildDefaultTourismData(aq)
	if td == nil {
		t.Fatal("expected non-nil tourism data")
	}
	if td.AqueductID != aq.ID {
		t.Errorf("expected aqueduct id %v, got %v", aq.ID, td.AqueductID)
	}
	if td.MaxDailyVisitors <= 0 {
		t.Errorf("expected positive max daily visitors, got %d", td.MaxDailyVisitors)
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

func TestGenerateRecommendation_EmptyDetails(t *testing.T) {
	details := []models.AqueductTourismData{}
	ranking := map[string]interface{}{}
	rec := GenerateRecommendation(details, ranking)
	if rec == "" {
		t.Error("expected non-empty recommendation")
	}
}

func TestBuildRadarData(t *testing.T) {
	cfg := &config.TourismConfig{}
	details := []models.AqueductTourismData{
		{
			AqueductID:   uuid.New(),
			AqueductName: "Aqueduct A",
			HeritageValue: 0.8,
			SafetyScore:   0.7,
		},
		{
			AqueductID:   uuid.New(),
			AqueductName: "Aqueduct B",
			HeritageValue: 0.6,
			SafetyScore:   0.9,
		},
	}
	radarData := BuildRadarData(cfg, details)
	if radarData == nil {
		t.Fatal("expected non-nil radar data")
	}
	if labels, ok := radarData["labels"].([]string); !ok || len(labels) == 0 {
		t.Error("expected non-empty radar labels")
	}
	if datasets, ok := radarData["datasets"].([]interface{}); !ok || len(datasets) != 2 {
		t.Error("expected 2 radar datasets")
	}
}

func TestComputePriorityRanking(t *testing.T) {
	cfg := &config.TourismConfig{}
	details := []models.AqueductTourismData{
		{
			AqueductID:   uuid.New(),
			AqueductName: "High Value Aqueduct",
			HeritageValue: 0.9,
			SafetyScore:   0.8,
		},
		{
			AqueductID:   uuid.New(),
			AqueductName: "Low Value Aqueduct",
			HeritageValue: 0.3,
			SafetyScore:   0.4,
		},
	}
	ranking := ComputePriorityRanking(cfg, details)
	if ranking == nil {
		t.Fatal("expected non-nil ranking")
	}
	if scores, ok := ranking["scores"].(map[string]float64); ok {
		if len(scores) != 2 {
			t.Errorf("expected 2 scores, got %d", len(scores))
		}
	} else {
		t.Error("expected scores map in ranking")
	}
}

func TestCompareAqueducts_Core(t *testing.T) {
	cfg := &config.TourismConfig{}
	id1 := uuid.New()
	id2 := uuid.New()
	details := []models.AqueductTourismData{
		{
			AqueductID:   id1,
			AqueductName: "Aqueduct One",
			HeritageValue: 0.85,
			SafetyScore:   0.75,
		},
		{
			AqueductID:   id2,
			AqueductName: "Aqueduct Two",
			HeritageValue: 0.65,
			SafetyScore:   0.85,
		},
	}
	result := CompareAqueducts(cfg, details)
	if result.RadarData == nil {
		t.Error("expected non-nil radar data")
	}
	if result.Ranking == nil {
		t.Error("expected non-nil ranking")
	}
	if result.Recommendation == "" {
		t.Error("expected non-empty recommendation")
	}
}

func TestSplitString(t *testing.T) {
	tests := []struct {
		input    string
		sep      string
		expected []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{"a", ",", []string{"a"}},
		{"", ",", []string{}},
	}
	for _, tt := range tests {
		result := splitString(tt.input, tt.sep)
		if len(result) != len(tt.expected) {
			t.Errorf("splitString(%q, %q) = %v, expected %v", tt.input, tt.sep, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitString(%q, %q)[%d] = %q, expected %q", tt.input, tt.sep, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestEnrichTourismDataWithHeritage(t *testing.T) {
	cfg := &config.TourismConfig{}
	aq := &models.Aqueduct{
		ID:               uuid.New(),
		Name:             "Historic Aqueduct",
		ConstructionYear: -100,
		LengthKM:         80.0,
		HeightM:          25.0,
	}
	td := &models.AqueductTourismData{
		AqueductID:             aq.ID,
		HistoricalSignificance:  0.9,
		CurrentConditionScore:   0.7,
		VisibilityScore:         0.8,
		PhotographicValue:       0.85,
	}
	enriched := EnrichTourismDataWithHeritage(cfg, aq, td)
	if enriched == nil {
		t.Fatal("expected non-nil enriched data")
	}
	if enriched.HeritageValue <= 0 {
		t.Errorf("expected positive heritage value, got %f", enriched.HeritageValue)
	}
	if enriched.HeritageType == "" {
		t.Error("expected non-empty heritage type")
	}
}
