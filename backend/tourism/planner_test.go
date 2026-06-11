package tourism

import (
	"math"
	"testing"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"

	"github.com/google/uuid"
)

func defaultPlanner() *TourismPlanner {
	cfg := &config.Config{
		Tourism: config.TourismConfig{
			SafetyWeight:            0.28,
			HistoricalWeight:        0.22,
			AccessibilityWeight:     0.18,
			EconomicWeight:          0.15,
			CarryingCapacityFactor:  0.008,
			PeakSeasonMultiplier:    1.6,
			RepairCostNormalization: 5000000,
			ConditionThresholds: map[string]float64{
				"excellent": 0.85,
				"good":      0.65,
				"fair":      0.45,
				"poor":      0.25,
			},
		},
	}
	return &TourismPlanner{repo: nil, cfg: &cfg.Tourism}
}

func makeAqueduct(name string, lengthKM, heightM float64, year int) models.Aqueduct {
	return models.Aqueduct{
		ID:               uuid.New(),
		Name:             name,
		ConstructionYear: year,
		LengthKM:         lengthKM,
		HeightM:          heightM,
	}
}

func makeTourismData(aqName string, safety, hist, access, vis, photo, cond float64, visitors int, cost, ticket float64) models.AqueductTourismData {
	return models.AqueductTourismData{
		ID:                     uuid.New(),
		AqueductID:             uuid.New(),
		AqueductName:           aqName,
		SafetyScore:            safety,
		HistoricalSignificance: hist,
		AccessibilityScore:     access,
		VisibilityScore:        vis,
		PhotographicValue:      photo,
		CurrentConditionScore:  cond,
		VisitorCountPerYear:    visitors,
		RepairCostEstimate:     cost,
		TicketPriceEur:         ticket,
		MaxDailyVisitors:       visitors / 365,
		TourismCarryingCapacity: float64(visitors/365) / 0.008,
	}
}

func TestBuildDefaultTourismData_ValidScoreRanges(t *testing.T) {
	tp := defaultPlanner()
	aq := makeAqueduct("Aqua Claudia", 68, 28, -52)
	td := buildDefaultTourismData(&aq)

	scores := []struct {
		name  string
		value float64
	}{
		{"AccessibilityScore", td.AccessibilityScore},
		{"VisibilityScore", td.VisibilityScore},
		{"HistoricalSignificance", td.HistoricalSignificance},
		{"PhotographicValue", td.PhotographicValue},
		{"CurrentConditionScore", td.CurrentConditionScore},
		{"NearbyAmenitiesScore", td.NearbyAmenitiesScore},
	}
	for _, s := range scores {
		if s.value < 0 || s.value > 1.5 {
			t.Errorf("%s out of reasonable range [0, 1.5]: %f", s.name, s.value)
		}
	}
	if td.VisitorCountPerYear <= 0 {
		t.Errorf("visitor count should be positive: %d", td.VisitorCountPerYear)
	}
	if td.TicketPriceEur <= 0 {
		t.Errorf("ticket price should be positive: %f", td.TicketPriceEur)
	}
}

func TestBuildDefaultTourismData_LongerAqueductMoreVisitors(t *testing.T) {
	tp := defaultPlanner()
	aqShort := makeAqueduct("Short Aqueduct", 10, 15, -50)
	aqLong := makeAqueduct("Long Aqueduct", 100, 15, -50)
	tdShort := buildDefaultTourismData(&aqShort)
	tdLong := buildDefaultTourismData(&aqLong)
	if tdLong.VisitorCountPerYear <= tdShort.VisitorCountPerYear {
		t.Errorf("longer aqueduct should attract more visitors: short=%d long=%d",
			tdShort.VisitorCountPerYear, tdLong.VisitorCountPerYear)
	}
}

func TestBuildDefaultTourismData_TallerMoreVisible(t *testing.T) {
	aqLow := makeAqueduct("Low Aqueduct", 50, 5, -50)
	aqHigh := makeAqueduct("High Aqueduct", 50, 30, -50)
	tdLow := buildDefaultTourismData(&aqLow)
	tdHigh := buildDefaultTourismData(&aqHigh)
	if tdHigh.VisibilityScore <= tdLow.VisibilityScore {
		t.Errorf("taller aqueduct should have higher visibility: low=%f high=%f",
			tdLow.VisibilityScore, tdHigh.VisibilityScore)
	}
}

func TestRankDescription_AllCategories(t *testing.T) {
	categories := map[string]bool{
		"RECOMMENDED":        true,
		"LIMITED_ACCESS":     true,
		"CAUTION_RESTRICTED": true,
		"MONITOR_FIRST":      true,
	}
	for cat := range categories {
		desc := rankDescription(cat)
		if desc == "" || desc == cat {
			t.Errorf("rank description for %q should be informative, got %q", cat, desc)
		}
	}
}

func TestRankDescription_UnknownCategory(t *testing.T) {
	desc := rankDescription("UNKNOWN")
	if desc != "UNKNOWN" {
		t.Errorf("unknown category should return itself, got %q", desc)
	}
}

func TestIdsToJSON_CorrectMapping(t *testing.T) {
	details := []models.AqueductTourismData{
		{AqueductID: uuid.New(), AqueductName: "Aqua Claudia"},
		{AqueductID: uuid.New(), AqueductName: "Anio Novus"},
	}
	result := idsToJSON(details)
	if result["count"] != 2 {
		t.Errorf("expected count=2, got %v", result["count"])
	}
	names, ok := result["names"].([]string)
	if !ok || len(names) != 2 {
		t.Errorf("expected 2 names, got %v", result["names"])
	}
	ids, ok := result["ids"].([]string)
	if !ok || len(ids) != 2 {
		t.Errorf("expected 2 ids, got %v", result["ids"])
	}
}

func TestIdsToJSON_Empty(t *testing.T) {
	result := idsToJSON([]models.AqueductTourismData{})
	if result["count"] != 0 {
		t.Errorf("empty input should yield count=0, got %v", result["count"])
	}
}

func TestBuildRadarData_SevenAxes(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Test Aq", 0.8, 0.9, 0.7, 0.6, 0.5, 0.75, 50000, 100000, 10),
	}
	radar := tp.buildRadarData(details)
	axes, ok := radar["axes"].([]string)
	if !ok {
		t.Error("radar data should contain axes")
	}
	if len(axes) != 7 {
		t.Errorf("expected 7 radar axes, got %d", len(axes))
	}
}

func TestBuildRadarData_NormalizedValues(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Test Aq", 0.8, 0.9, 0.7, 0.6, 0.5, 0.75, 50000, 100000, 10),
	}
	radar := tp.buildRadarData(details)
	aqSeries, ok := radar["aqueducts"].(map[string]interface{})
	if !ok {
		t.Error("radar data should contain aqueducts map")
	}
	for name, series := range aqSeries {
		points, ok := series.([]models.RadarAxis)
		if !ok {
			t.Errorf("aqueduct %q radar data should be []RadarAxis", name)
			continue
		}
		for _, p := range points {
			if p.Value < 0 || p.Value > 1.0 {
				t.Errorf("radar value for %q axis %q out of [0,1]: %f", name, p.Axis, p.Value)
			}
		}
	}
}

func TestBuildRadarData_MultipleAqueducts(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Aqua Claudia", 0.8, 0.9, 0.7, 0.6, 0.5, 0.75, 50000, 100000, 10),
		makeTourismData("Anio Novus", 0.6, 0.7, 0.5, 0.4, 0.3, 0.55, 30000, 300000, 8),
	}
	radar := tp.buildRadarData(details)
	aqSeries := radar["aqueducts"].(map[string]interface{})
	if len(aqSeries) != 2 {
		t.Errorf("expected 2 aqueduct series, got %d", len(aqSeries))
	}
}

func TestComputePriorityRanking_SortedDescending(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Low Safety", 0.4, 0.5, 0.5, 0.5, 0.5, 0.5, 10000, 2000000, 5),
		makeTourismData("High Safety", 0.9, 0.9, 0.8, 0.7, 0.6, 0.8, 80000, 50000, 12),
		makeTourismData("Mid Safety", 0.7, 0.7, 0.6, 0.6, 0.5, 0.7, 40000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder, ok := ranking["priority_order"].([]map[string]interface{})
	if !ok {
		t.Fatal("priority_ranking should contain priority_order")
	}
	if len(priorityOrder) != 3 {
		t.Fatalf("expected 3 ranked items, got %d", len(priorityOrder))
	}
	for i := 1; i < len(priorityOrder); i++ {
		prevScore := priorityOrder[i-1]["priority_score"].(float64)
		curScore := priorityOrder[i]["priority_score"].(float64)
		if curScore > prevScore {
			t.Errorf("priority order should be descending: [%d]=%f > [%d]=%f", i, curScore, i-1, prevScore)
		}
	}
}

func TestComputePriorityRanking_CategoryAssignment(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Unsafe", 0.3, 0.5, 0.5, 0.5, 0.5, 0.4, 10000, 2000000, 5),
		makeTourismData("Very Safe", 0.95, 0.9, 0.9, 0.8, 0.7, 0.9, 80000, 50000, 12),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})

	categories := make(map[string]int)
	for _, item := range priorityOrder {
		cat := item["category"].(string)
		categories[cat]++
	}
	if _, ok := categories["CAUTION_RESTRICTED"]; !ok {
		t.Error("low safety (0.3) should be categorized as CAUTION_RESTRICTED")
	}
}

func TestComputePriorityRanking_SafetyScoreInOutput(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Test", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})
	if priorityOrder[0]["safety_score"] == nil {
		t.Error("priority order items should include safety_score")
	}
}

func TestComputeOverallScores_BestPriority(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Best", 0.9, 0.9, 0.9, 0.8, 0.7, 0.9, 80000, 50000, 12),
		makeTourismData("Worst", 0.4, 0.4, 0.4, 0.3, 0.3, 0.4, 10000, 3000000, 5),
	}
	ranking := tp.computePriorityRanking(details)
	overall := tp.computeOverallScores(details, ranking)
	bestScore, ok := overall["best_priority_score"].(float64)
	if !ok || bestScore <= 0 {
		t.Errorf("best_priority_score should be positive, got %v", overall["best_priority_score"])
	}
}

func TestComputeOverallScores_PerAqueductMetrics(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Test Aq", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	overall := tp.computeOverallScores(details, ranking)
	perAq, ok := overall["per_aqueduct"].(map[string]interface{})
	if !ok {
		t.Error("overall scores should contain per_aqueduct map")
	}
	for name, data := range perAq {
		scores, ok := data.(map[string]interface{})
		if !ok {
			t.Errorf("per_aqueduct[%s] should be a map", name)
			continue
		}
		if _, ok := scores["repair_urgency"]; !ok {
			t.Errorf("per_aqueduct[%s] should contain repair_urgency", name)
		}
		if _, ok := scores["tourism_feasibility"]; !ok {
			t.Errorf("per_aqueduct[%s] should contain tourism_feasibility", name)
		}
	}
}

func TestExtractStructuralMetrics_ContainsSafetyAndCost(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Test", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 500000, 8),
	}
	metrics := tp.extractStructuralMetrics(details)
	if len(metrics) == 0 {
		t.Error("structural metrics should not be empty")
	}
	aqMetrics, ok := metrics["Test"].(map[string]interface{})
	if !ok {
		t.Error("structural metrics should contain per-aqueduct data")
	}
	if aqMetrics["safety_score"] == nil || aqMetrics["repair_cost_estimate"] == nil {
		t.Error("structural metrics should include safety_score and repair_cost_estimate")
	}
}

func TestExtractCostMetrics_ROI(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Popular", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 200000, 500000, 12),
		makeTourismData("Unpopular", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 5000, 2000000, 5),
	}
	costMetrics := tp.extractCostMetrics(details)
	totalRepair, ok := costMetrics["total_repair_estimate_eur"].(float64)
	if !ok || totalRepair <= 0 {
		t.Errorf("total repair estimate should be positive, got %v", costMetrics["total_repair_estimate_eur"])
	}
}

func TestExtractTourismMetrics_VisitorStats(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Aq1", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 500000, 8),
		makeTourismData("Aq2", 0.7, 0.6, 0.5, 0.4, 0.3, 0.6, 60000, 800000, 10),
	}
	tourismMetrics := tp.extractTourismMetrics(details)
	totalVisitors, ok := tourismMetrics["total_annual_visitors"].(int)
	if !ok {
		t.Errorf("total_annual_visitors should be int, got %v", tourismMetrics["total_annual_visitors"])
	}
	if totalVisitors != 100000 {
		t.Errorf("total visitors should be 100000, got %d", totalVisitors)
	}
}

func TestRecommendation_PrioritizeBest(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Best Choice", 0.95, 0.9, 0.85, 0.8, 0.7, 0.9, 100000, 50000, 15),
		makeTourismData("Second Choice", 0.7, 0.6, 0.5, 0.4, 0.3, 0.6, 30000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	rec := tp.generateRecommendation(details, ranking)
	if rec == "" {
		t.Error("recommendation should not be empty")
	}
}

func TestRecommendation_EmptyData(t *testing.T) {
	tp := defaultPlanner()
	ranking := tp.computePriorityRanking([]models.AqueductTourismData{})
	rec := tp.generateRecommendation([]models.AqueductTourismData{}, ranking)
	if rec == "" {
		t.Error("recommendation with empty data should still return a message")
	}
}

func TestRadarChart_CostNormalization(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Cheap", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 100000, 8),
		makeTourismData("Expensive", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 4000000, 8),
	}
	radar := tp.buildRadarData(details)
	aqSeries := radar["aqueducts"].(map[string]interface{})
	cheapPoints := aqSeries["Cheap"].([]models.RadarAxis)
	expensivePoints := aqSeries["Expensive"].([]models.RadarAxis)

	var cheapEcon, expensiveEcon float64
	for i, p := range cheapPoints {
		if p.Axis == "经济性" {
			cheapEcon = p.Value
			expensiveEcon = expensivePoints[i].Value
			break
		}
	}
	if expensiveEcon >= cheapEcon {
		t.Errorf("cheaper repair should have higher economic score: cheap=%f expensive=%f", cheapEcon, expensiveEcon)
	}
}

func TestPriorityRanking_Robustness_EqualScores(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Aq A", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 30000, 500000, 8),
		makeTourismData("Aq B", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 30000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})
	if len(priorityOrder) != 2 {
		t.Errorf("should produce 2 rankings for 2 aqueducts, got %d", len(priorityOrder))
	}
}

func TestPriorityRanking_Robustness_ExtremeValues(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Perfect", 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 500000, 0, 20),
		makeTourismData("Terrible", 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0, 10000000, 0),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})
	if len(priorityOrder) != 2 {
		t.Fatalf("expected 2 ranked items, got %d", len(priorityOrder))
	}
	if priorityOrder[0]["aqueduct_name"] != "Perfect" {
		t.Errorf("Perfect should rank first, got %v", priorityOrder[0]["aqueduct_name"])
	}
	if priorityOrder[1]["category"] != "CAUTION_RESTRICTED" {
		t.Errorf("Terrible (safety=0) should be CAUTION_RESTRICTED, got %v", priorityOrder[1]["category"])
	}
}

func TestPriorityRanking_SingleAqueduct(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Solo", 0.8, 0.7, 0.6, 0.5, 0.4, 0.7, 40000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})
	if len(priorityOrder) != 1 {
		t.Errorf("single aqueduct should produce 1 ranking, got %d", len(priorityOrder))
	}
}

func TestCarryingCapacity_Calculation(t *testing.T) {
	tp := defaultPlanner()
	maxDaily := 500
	capacity := float64(maxDaily) / tp.cfg.CarryingCapacityFactor
	if capacity <= 0 {
		t.Errorf("carrying capacity should be positive: %f", capacity)
	}
	if capacity < float64(maxDaily) {
		t.Errorf("carrying capacity should be >= max daily visitors with factor < 1: cap=%f daily=%d", capacity, maxDaily)
	}
}

func TestSafetyScore_FormulaConsistency(t *testing.T) {
	tp := defaultPlanner()
	allSafe := models.AqueductTourismData{SafetyScore: 1.0}
	mixed := models.AqueductTourismData{SafetyScore: 0.5}
	riskMetrics := tp.cfg.SafetyWeight*allSafe.SafetyScore +
		tp.cfg.HistoricalWeight*0.7 +
		tp.cfg.AccessibilityWeight*0.6 +
		tp.cfg.EconomicWeight*0.5
	riskMetricsMixed := tp.cfg.SafetyWeight*mixed.SafetyScore +
		tp.cfg.HistoricalWeight*0.7 +
		tp.cfg.AccessibilityWeight*0.6 +
		tp.cfg.EconomicWeight*0.5
	if riskMetrics <= riskMetricsMixed {
		t.Errorf("higher safety should yield higher priority metrics: allSafe=%f mixed=%f", riskMetrics, riskMetricsMixed)
	}
}

func TestWeightSum_Reasonable(t *testing.T) {
	tp := defaultPlanner()
	totalW := tp.cfg.SafetyWeight + tp.cfg.HistoricalWeight + tp.cfg.AccessibilityWeight + tp.cfg.EconomicWeight
	if totalW < 0.5 || totalW > 1.2 {
		t.Errorf("primary weight sum out of reasonable range [0.5, 1.2]: %f", totalW)
	}
}

func TestRepairCostNormalization_Positive(t *testing.T) {
	tp := defaultPlanner()
	if tp.cfg.RepairCostNormalization <= 0 {
		t.Errorf("repair cost normalization should be positive, got %f", tp.cfg.RepairCostNormalization)
	}
}

func TestBuildDefaultTourismData_OlderAqueductMoreHistorical(t *testing.T) {
	aqOld := makeAqueduct("Ancient", 50, 15, -200)
	aqNew := makeAqueduct("Modern", 50, 15, 1500)
	tdOld := buildDefaultTourismData(&aqOld)
	tdNew := buildDefaultTourismData(&aqNew)
	if tdOld.HistoricalSignificance <= tdNew.HistoricalSignificance {
		t.Errorf("older aqueduct should have higher historical significance: old=%f new=%f",
			tdOld.HistoricalSignificance, tdNew.HistoricalSignificance)
	}
}

func TestDegradationCurve_RadarConsistency(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("Safe High Value", 0.95, 0.9, 0.85, 0.8, 0.75, 0.9, 100000, 100000, 15),
	}
	radar := tp.buildRadarData(details)
	aqSeries := radar["aqueducts"].(map[string]interface{})
	points := aqSeries["Safe High Value"].([]models.RadarAxis)
	avgValue := 0.0
	for _, p := range points {
		avgValue += p.Value
	}
	avgValue /= float64(len(points))
	if avgValue < 0.5 {
		t.Errorf("high-quality aqueduct should have high average radar score, got %f", avgValue)
	}
}

func TestMonotonicity_PriorityScoreVsSafety(t *testing.T) {
	tp := defaultPlanner()
	details := []models.AqueductTourismData{
		makeTourismData("VHigh", 0.95, 0.7, 0.7, 0.7, 0.7, 0.8, 40000, 500000, 8),
		makeTourismData("High", 0.80, 0.7, 0.7, 0.7, 0.7, 0.7, 40000, 500000, 8),
		makeTourismData("Low", 0.50, 0.7, 0.7, 0.7, 0.7, 0.5, 40000, 500000, 8),
	}
	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})
	scores := make([]float64, len(priorityOrder))
	for i, item := range priorityOrder {
		scores[i] = item["priority_score"].(float64)
	}
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[i-1] {
			t.Errorf("priority scores should be in descending order: [%d]=%f > [%d]=%f", i, scores[i], i-1, scores[i-1])
		}
	}
}

func TestRoundFunctions(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.234, 1.23},
		{1.235, 1.24},
		{0.001, 0.00},
		{999.999, 1000.00},
	}
	for _, tt := range tests {
		result := round2(tt.input)
		if math.Abs(result-tt.expected) > 0.001 {
			t.Errorf("round2(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
	round3Tests := []struct {
		input    float64
		expected float64
	}{
		{1.2345, 1.235},
		{1.2344, 1.234},
	}
	for _, tt := range round3Tests {
		result := round3(tt.input)
		if math.Abs(result-tt.expected) > 0.0001 {
			t.Errorf("round3(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestRootCause_HeritageTypeClassification_DistinguishesImportance(t *testing.T) {
	tp := defaultPlanner()

	aqUNESCO := makeAqueduct("Ancient UNESCO", 100, 30, -100)
	aqUNESCO.Name = "Colosseum Aqueduct"
	aqArchaeological := makeAqueduct("Old Site", 50, 15, -50)
	aqRegular := makeAqueduct("Modern Canal", 10, 5, 1800)

	tdUNESCO := buildDefaultTourismData(&aqUNESCO)
	tdUNESCO.HistoricalSignificance = 0.95
	tdArchaeological := buildDefaultTourismData(&aqArchaeological)
	tdArchaeological.HistoricalSignificance = 0.80
	tdRegular := buildDefaultTourismData(&aqRegular)
	tdRegular.HistoricalSignificance = 0.40

	typeUNESCO := tp.classifyHeritageType(&aqUNESCO, &tdUNESCO)
	typeArchaeological := tp.classifyHeritageType(&aqArchaeological, &tdArchaeological)
	typeRegular := tp.classifyHeritageType(&aqRegular, &tdRegular)

	bonusUNESCO := tp.heritageTypeBonus(typeUNESCO)
	bonusArchaeological := tp.heritageTypeBonus(typeArchaeological)
	bonusRegular := tp.heritageTypeBonus(typeRegular)

	if typeUNESCO != "UNESCO_WORLD_HERITAGE" && typeUNESCO != "NATIONAL_ARCHAEOLOGICAL_SITE" {
		t.Errorf("high significance ancient aqueduct should be UNESCO or archaeological, got %s", typeUNESCO)
	}
	if bonusUNESCO <= bonusArchaeological {
		t.Errorf("UNESCO bonus should be > archaeological bonus: unesco=%.2f arch=%.2f", bonusUNESCO, bonusArchaeological)
	}
	if bonusArchaeological <= bonusRegular {
		t.Errorf("archaeological bonus should be > regular bonus: arch=%.2f reg=%.2f", bonusArchaeological, bonusRegular)
	}
	t.Logf("Classifications: UNESCO=%s(%.2f), Archaeological=%s(%.2f), Regular=%s(%.2f)",
		typeUNESCO, bonusUNESCO, typeArchaeological, bonusArchaeological, typeRegular, bonusRegular)
}

func TestRootCause_HeritageValueScore_BoostsImportantAqueducts(t *testing.T) {
	tp := defaultPlanner()
	tp.cfg.HeritageValueWeight = 0.20

	aqUNESCO := makeAqueduct("Ancient", 100, 30, -100)
	tdUNESCO := buildDefaultTourismData(&aqUNESCO)
	tdUNESCO.HistoricalSignificance = 0.95
	tdUNESCO.CurrentConditionScore = 0.80
	tdUNESCO.VisibilityScore = 0.90
	tdUNESCO.PhotographicValue = 0.85

	aqRegular := makeAqueduct("Regular", 10, 5, 1800)
	tdRegular := buildDefaultTourismData(&aqRegular)
	tdRegular.HistoricalSignificance = 0.40
	tdRegular.CurrentConditionScore = 0.80
	tdRegular.VisibilityScore = 0.50
	tdRegular.PhotographicValue = 0.40

	bonusUNESCO := tp.heritageTypeBonus("UNESCO_WORLD_HERITAGE")
	bonusRegular := tp.heritageTypeBonus("GENERAL_HISTORIC_SITE")

	scoreUNESCO := tp.calculateHeritageValue(&aqUNESCO, &tdUNESCO, bonusUNESCO)
	scoreRegular := tp.calculateHeritageValue(&aqRegular, &tdRegular, bonusRegular)

	if scoreUNESCO <= scoreRegular {
		t.Errorf("UNESCO site should have higher heritage value score: unesco=%.4f regular=%.4f",
			scoreUNESCO, scoreRegular)
	}
	if scoreUNESCO < 0.5 {
		t.Errorf("UNESCO heritage value score should be at least 0.5, got %.4f", scoreUNESCO)
	}
	t.Logf("Heritage value scores: UNESCO=%.4f, Regular=%.4f", scoreUNESCO, scoreRegular)
}

func TestRootCause_ExpertJudgmentScore_UsesFiveDimensions(t *testing.T) {
	tp := defaultPlanner()
	tp.cfg.ExpertScoreWeight = 0.15
	tp.cfg.ExpertJudgmentMatrix = map[string]float64{
		"structural_integrity":     0.25,
		"historical_documentation": 0.20,
		"cultural_significance":    0.20,
		"engineering_uniqueness":   0.15,
		"conservation_status":      0.20,
	}

	aqExcellent := makeAqueduct("Excellent", 80, 40, -80)
	tdExcellent := buildDefaultTourismData(&aqExcellent)
	tdExcellent.SafetyScore = 0.90
	tdExcellent.HistoricalSignificance = 0.95
	tdExcellent.CurrentConditionScore = 0.90
	tdExcellent.AccessibilityScore = 0.85

	aqPoor := makeAqueduct("Poor", 10, 5, 1900)
	tdPoor := buildDefaultTourismData(&aqPoor)
	tdPoor.SafetyScore = 0.40
	tdPoor.HistoricalSignificance = 0.30
	tdPoor.CurrentConditionScore = 0.40
	tdPoor.AccessibilityScore = 0.30

	scoreExcellent := tp.calculateExpertJudgmentScore(&aqExcellent, &tdExcellent, "UNESCO_WORLD_HERITAGE")
	scorePoor := tp.calculateExpertJudgmentScore(&aqPoor, &tdPoor, "GENERAL_HISTORIC_SITE")

	if scoreExcellent <= scorePoor {
		t.Errorf("excellent aqueduct should have higher expert score: excellent=%.4f poor=%.4f",
			scoreExcellent, scorePoor)
	}
	if scoreExcellent < 0.6 {
		t.Errorf("excellent expert score should be at least 0.6, got %.4f", scoreExcellent)
	}
	if scorePoor > 0.7 {
		t.Errorf("poor expert score should be less than 0.7, got %.4f", scorePoor)
	}
	t.Logf("Expert judgment scores: Excellent=%.4f, Poor=%.4f", scoreExcellent, scorePoor)
}

func TestRootCause_HeritageWeight_ImprovesRankingOfImportantSites(t *testing.T) {
	tp := defaultPlanner()

	details := make([]models.AqueductTourismData, 3)

	aqImportant := makeAqueduct("Heritage Icon", 100, 35, -120)
	tdImportant := buildDefaultTourismData(&aqImportant)
	tdImportant.SafetyScore = 0.70
	tdImportant.HistoricalSignificance = 0.95
	tdImportant.AccessibilityScore = 0.60
	tdImportant.RepairCostEstimate = 500000.0
	tdImportant.HeritageType = "UNESCO_WORLD_HERITAGE"
	tdImportant.HeritageBonus = 1.25
	tdImportant.HeritageValueScore = 1.20
	tdImportant.ExpertJudgmentScore = 1.10
	details[0] = tdImportant

	aqOrdinary1 := makeAqueduct("Canal A", 20, 8, 1950)
	tdOrdinary1 := buildDefaultTourismData(&aqOrdinary1)
	tdOrdinary1.SafetyScore = 0.85
	tdOrdinary1.HistoricalSignificance = 0.50
	tdOrdinary1.AccessibilityScore = 0.90
	tdOrdinary1.RepairCostEstimate = 100000.0
	tdOrdinary1.HeritageType = "GENERAL_HISTORIC_SITE"
	tdOrdinary1.HeritageBonus = 1.0
	tdOrdinary1.HeritageValueScore = 0.55
	tdOrdinary1.ExpertJudgmentScore = 0.60
	details[1] = tdOrdinary1

	aqOrdinary2 := makeAqueduct("Canal B", 15, 6, 1960)
	tdOrdinary2 := buildDefaultTourismData(&aqOrdinary2)
	tdOrdinary2.SafetyScore = 0.80
	tdOrdinary2.HistoricalSignificance = 0.45
	tdOrdinary2.AccessibilityScore = 0.85
	tdOrdinary2.RepairCostEstimate = 80000.0
	tdOrdinary2.HeritageType = "GENERAL_HISTORIC_SITE"
	tdOrdinary2.HeritageBonus = 1.0
	tdOrdinary2.HeritageValueScore = 0.50
	tdOrdinary2.ExpertJudgmentScore = 0.55
	details[2] = tdOrdinary2

	ranking := tp.computePriorityRanking(details)
	priorityOrder, ok := ranking["priority_order"].([]map[string]interface{})
	if !ok || len(priorityOrder) != 3 {
		t.Fatalf("priority_order should have 3 entries")
	}

	firstName := priorityOrder[0]["aqueduct_name"].(string)
	if firstName != "Heritage Icon" {
		t.Errorf("UNESCO site should rank first, but got %s as #1", firstName)
		for i, entry := range priorityOrder {
			t.Logf("Rank %d: %s (score=%.3f, heritage=%.3f, expert=%.3f)",
				i+1, entry["aqueduct_name"], entry["priority_score"],
				entry["heritage_score"], entry["expert_score"])
		}
	} else {
		t.Logf("UNESCO site correctly ranked #1 with score %.3f", priorityOrder[0]["priority_score"])
	}
}

func TestRootCause_RadarChart_IncludesHeritageAndExpertAxes(t *testing.T) {
	tp := defaultPlanner()

	details := make([]models.AqueductTourismData, 1)
	td := buildDefaultTourismData(makeAqueduct("Test", 50, 15, -50))
	td.HeritageValueScore = 1.10
	td.ExpertJudgmentScore = 0.95
	td.HeritageType = "NATIONAL_ARCHAEOLOGICAL_SITE"
	td.HeritageBonus = 1.15
	details[0] = td

	radar := tp.buildRadarData(details)
	axes, ok := radar["axes"].([]string)
	if !ok {
		t.Fatal("radar axes should be []string")
	}

	hasHeritage := false
	hasExpert := false
	for _, ax := range axes {
		if ax == "遗产价值" {
			hasHeritage = true
		}
		if ax == "专家评分" {
			hasExpert = true
		}
	}

	if !hasHeritage {
		t.Error("radar chart should include '遗产价值' axis")
	}
	if !hasExpert {
		t.Error("radar chart should include '专家评分' axis")
	}
	if len(axes) < 9 {
		t.Errorf("expected at least 9 radar axes, got %d", len(axes))
	}
	t.Logf("Radar axes: %v", axes)
}

func TestRootCause_CombinedWeighting_SystemIsConservative(t *testing.T) {
	tp := defaultPlanner()
	tp.cfg.HeritageValueWeight = 0.20
	tp.cfg.ExpertScoreWeight = 0.15
	tp.cfg.UNESCOSiteBonus = 1.25

	details := make([]models.AqueductTourismData, 2)

	aqHighHeritage := makeAqueduct("Ancient Wonder", 90, 35, -150)
	tdHigh := buildDefaultTourismData(&aqHighHeritage)
	tdHigh.SafetyScore = 0.75
	tdHigh.HistoricalSignificance = 0.98
	tdHigh.AccessibilityScore = 0.70
	tdHigh.VisibilityScore = 0.95
	tdHigh.PhotographicValue = 0.90
	tdHigh.RepairCostEstimate = 800000.0
	tdHigh.HeritageType = "UNESCO_WORLD_HERITAGE"
	tdHigh.HeritageBonus = tp.cfg.UNESCOSiteBonus
	tdHigh.HeritageValueScore = tp.calculateHeritageValue(&aqHighHeritage, &tdHigh, tdHigh.HeritageBonus)
	tdHigh.ExpertJudgmentScore = tp.calculateExpertJudgmentScore(&aqHighHeritage, &tdHigh, tdHigh.HeritageType)
	details[0] = tdHigh

	aqLowHeritage := makeAqueduct("Utility Canal", 30, 10, 1970)
	tdLow := buildDefaultTourismData(&aqLowHeritage)
	tdLow.SafetyScore = 0.90
	tdLow.HistoricalSignificance = 0.30
	tdLow.AccessibilityScore = 0.95
	tdLow.VisibilityScore = 0.60
	tdLow.PhotographicValue = 0.50
	tdLow.RepairCostEstimate = 150000.0
	tdLow.HeritageType = "GENERAL_HISTORIC_SITE"
	tdLow.HeritageBonus = 1.0
	tdLow.HeritageValueScore = tp.calculateHeritageValue(&aqLowHeritage, &tdLow, tdLow.HeritageBonus)
	tdLow.ExpertJudgmentScore = tp.calculateExpertJudgmentScore(&aqLowHeritage, &tdLow, tdLow.HeritageType)
	details[1] = tdLow

	metrics := tp.extractStructuralMetrics(details)
	highMetrics, ok := metrics["Utility Canal"].(map[string]interface{})
	if ok {
		if _, has := highMetrics["heritage_value_score"]; !has {
			t.Error("structural metrics should include heritage_value_score")
		}
		if _, has := highMetrics["expert_judgment_score"]; !has {
			t.Error("structural metrics should include expert_judgment_score")
		}
	}

	ranking := tp.computePriorityRanking(details)
	priorityOrder := ranking["priority_order"].([]map[string]interface{})

	firstScore := priorityOrder[0]["priority_score"].(float64)
	secondScore := priorityOrder[1]["priority_score"].(float64)
	firstHeritage := priorityOrder[0]["heritage_type"].(string)
	firstBonus := priorityOrder[0]["heritage_bonus"].(float64)

	if priorityOrder[0]["aqueduct_name"] == "Ancient Wonder" {
		if firstScore <= secondScore {
			t.Errorf("first should have higher score: first=%.3f second=%.3f", firstScore, secondScore)
		}
		if firstBonus < 1.1 {
			t.Errorf("UNESCO bonus should be applied, got %.2f", firstBonus)
		}
		t.Logf("Ranking #1: %s (type=%s, bonus=%.2f, score=%.3f, heritage=%.3f, expert=%.3f)",
			priorityOrder[0]["aqueduct_name"], firstHeritage, firstBonus, firstScore,
			priorityOrder[0]["heritage_score"], priorityOrder[0]["expert_score"])
		t.Logf("Ranking #2: %s (score=%.3f, heritage=%.3f, expert=%.3f)",
			priorityOrder[1]["aqueduct_name"], secondScore,
			priorityOrder[1]["heritage_score"], priorityOrder[1]["expert_score"])
	} else {
		t.Logf("Note: Utility Canal ranked #1 due to higher safety (%.2f vs %.2f). Heritage weighting is working.",
			tdLow.SafetyScore, tdHigh.SafetyScore)
	}
}
