package aqueduct_comparator

import (
	"fmt"
	"math"
	"testing"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func defaultConfig() *config.TourismConfig {
	return &config.TourismConfig{
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
		HeritageValueWeight: 0.20,
		ExpertScoreWeight:   0.15,
		UNESCOSiteBonus:     1.25,
		ArchaeologicalSiteBonus: 1.15,
		RareArchitectureBonus:   1.10,
		ExpertJudgmentMatrix: map[string]float64{
			"structural_integrity":     0.25,
			"historical_documentation": 0.20,
			"cultural_significance":    0.20,
			"engineering_uniqueness":   0.15,
			"conservation_status":      0.20,
		},
	}
}

func makeTestAqueduct(name string, year int, length, height float64) models.Aqueduct {
	return models.Aqueduct{
		ID:               uuid.New(),
		Name:             name,
		LatinName:        name + " Latin",
		ConstructionYear: year,
		LengthKM:         length,
		HeightM:          height,
		StartLocation:    "Source",
		EndLocation:      "Rome",
		Description:      "Test aqueduct",
	}
}

func makeTestTourismData(aqName string, safety, historic, access, visible, photo, condition float64, cost float64, capacity float64, visitors int) models.AqueductTourismData {
	aqID := uuid.New()
	return models.AqueductTourismData{
		ID:                       uuid.New(),
		AqueductID:               aqID,
		AqueductName:             aqName,
		SafetyScore:              safety,
		HistoricalSignificance:   historic,
		AccessibilityScore:       access,
		VisibilityScore:          visible,
		PhotographicValue:        photo,
		CurrentConditionScore:    condition,
		RepairCostEstimate:       cost,
		TourismCarryingCapacity:  capacity,
		VisitorCountPerYear:      visitors,
		TicketPriceEur:           8.0,
		NearbyAmenitiesScore:     0.6,
		MaxDailyVisitors:         500,
		GuidedTourAvailable:      true,
		WheelchairAccessible:     true,
		PublicTransportAccess:    true,
		HeritageType:             "REGIONAL_HERITAGE_SITE",
		HeritageValueScore:       0.65,
		ExpertJudgmentScore:      0.70,
		HeritageBonus:            1.05,
	}
}

func TestBuildRadarData_Comprehensiveness(t *testing.T) {
	cfg := defaultConfig()
	expectedAxes := []string{"结构安全", "历史意义", "可达性", "可见度", "摄影价值", "经济性", "承载能力", "遗产价值", "专家评分"}

	tests := []struct {
		name      string
		numAq     int
		wantAxes  int
		wantEach  int
	}{
		{"single_aqueduct", 1, 9, 9},
		{"three_aqueducts", 3, 9, 9},
		{"five_aqueducts", 5, 9, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := make([]models.AqueductTourismData, tt.numAq)
			for i := 0; i < tt.numAq; i++ {
				details[i] = makeTestTourismData(
					fmt.Sprintf("水道-%d", i+1),
					0.8-float64(i)*0.1,
					0.85-float64(i)*0.05,
					0.75+float64(i)*0.03,
					0.70,
					0.65,
					0.80,
					1000000+float64(i)*500000,
					3000+float64(i)*500,
					100000+i*20000,
				)
			}

			radar := BuildRadarData(cfg, details)

			axes, ok := radar["axes"].([]string)
			if !ok {
				t.Fatal("axes should be []string")
			}
			if len(axes) != tt.wantAxes {
				t.Errorf("expected %d axes, got %d", tt.wantAxes, len(axes))
			}
			for i, ax := range axes {
				if ax != expectedAxes[i] {
					t.Errorf("axis %d: expected %s, got %s", i, expectedAxes[i], ax)
				}
			}

			aqSeries, ok := radar["aqueducts"].(map[string]interface{})
			if !ok {
				t.Fatal("aqueducts should be map[string]interface{}")
			}
			if len(aqSeries) != tt.numAq {
				t.Errorf("expected %d aqueduct series, got %d", tt.numAq, len(aqSeries))
			}

			for aqName, series := range aqSeries {
				points, ok := series.([]models.RadarAxis)
				if !ok {
					t.Fatalf("%s: series should be []models.RadarAxis", aqName)
				}
				if len(points) != tt.wantEach {
					t.Errorf("%s: expected %d points, got %d", aqName, tt.wantEach, len(points))
				}

				for i, pt := range points {
					if pt.Axis != expectedAxes[i] {
						t.Errorf("%s axis %d: expected axis %s, got %s", aqName, i, expectedAxes[i], pt.Axis)
					}
					if pt.Value < 0 || pt.Value > 1.0 {
						t.Errorf("%s axis %s: value %.3f out of range [0,1]", aqName, pt.Axis, pt.Value)
					}
					if pt.Max <= 0 {
						t.Errorf("%s axis %s: max %.3f should be positive", aqName, pt.Axis, pt.Max)
					}
				}
			}
		})
	}
}

func TestBuildRadarData_Consistency(t *testing.T) {
	cfg := defaultConfig()

	t.Run("identical_inputs_identical_outputs", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道A", 0.8, 0.85, 0.75, 0.7, 0.65, 0.8, 1000000, 3000, 100000),
			makeTestTourismData("水道B", 0.8, 0.85, 0.75, 0.7, 0.65, 0.8, 1000000, 3000, 100000),
		}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		ptsA := aqSeries["水道A"].([]models.RadarAxis)
		ptsB := aqSeries["水道B"].([]models.RadarAxis)

		for i := range ptsA {
			if math.Abs(ptsA[i].Value-ptsB[i].Value) > 0.001 {
				t.Errorf("axis %s: A=%.4f, B=%.4f, should be equal", ptsA[i].Axis, ptsA[i].Value, ptsB[i].Value)
			}
		}
	})

	t.Run("higher_safety_higher_score", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道高", 0.9, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
			makeTestTourismData("水道低", 0.5, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
		}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		ptsHigh := aqSeries["水道高"].([]models.RadarAxis)
		ptsLow := aqSeries["水道低"].([]models.RadarAxis)

		for _, pt := range ptsHigh {
			if pt.Axis == "结构安全" && pt.Value <= 0.88 {
				t.Errorf("高安全水道结构安全值过低: %.4f", pt.Value)
			}
		}
		for _, pt := range ptsLow {
			if pt.Axis == "结构安全" && pt.Value >= 0.52 {
				t.Errorf("低安全水道结构安全值过高: %.4f", pt.Value)
			}
		}
	})

	t.Run("higher_cost_lower_economic_score", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道低成本", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 100000, 3000, 100000),
			makeTestTourismData("水道高成本", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 5000000, 3000, 100000),
		}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		ptsLow := aqSeries["水道低成本"].([]models.RadarAxis)
		ptsHigh := aqSeries["水道高成本"].([]models.RadarAxis)

		var lowEcon, highEcon float64
		for _, pt := range ptsLow {
			if pt.Axis == "经济性" {
				lowEcon = pt.Value
			}
		}
		for _, pt := range ptsHigh {
			if pt.Axis == "经济性" {
				highEcon = pt.Value
			}
		}
		if lowEcon <= highEcon {
			t.Errorf("低成本水道经济性应更高: 低成本=%.4f, 高成本=%.4f", lowEcon, highEcon)
		}
	})
}

func TestBuildRadarData_BoundaryConditions(t *testing.T) {
	cfg := defaultConfig()

	t.Run("extreme_high_scores", func(t *testing.T) {
		detail := makeTestTourismData("完美水道", 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 0, 5000, 50000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["完美水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Value > 1.001 {
				t.Errorf("axis %s: value %.4f exceeds 1.0", pt.Axis, pt.Value)
			}
		}
	})

	t.Run("extreme_low_scores", func(t *testing.T) {
		detail := makeTestTourismData("濒危水道", 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 10000000, 100, 1000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["濒危水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Value < -0.001 {
				t.Errorf("axis %s: value %.4f below 0", pt.Axis, pt.Value)
			}
		}
	})

	t.Run("zero_repair_cost", func(t *testing.T) {
		detail := makeTestTourismData("零成本水道", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 0, 3000, 100000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["零成本水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Axis == "经济性" && math.Abs(pt.Value-1.0) > 0.001 {
				t.Errorf("零成本水道经济性应为1.0, 实际=%.4f", pt.Value)
			}
		}
	})

	t.Run("very_high_repair_cost", func(t *testing.T) {
		detail := makeTestTourismData("高成本水道", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 1e9, 3000, 100000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["高成本水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Axis == "经济性" && pt.Value > 0.01 {
				t.Errorf("极高成本水道经济性应接近0, 实际=%.4f", pt.Value)
			}
		}
	})
}

func TestBuildRadarData_AnomalyCases(t *testing.T) {
	cfg := defaultConfig()

	t.Run("negative_safety_score", func(t *testing.T) {
		detail := makeTestTourismData("异常水道", -0.5, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["异常水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Axis == "结构安全" && pt.Value > 0 {
				t.Errorf("负安全值应被标准化, 实际=%.4f", pt.Value)
			}
		}
	})

	t.Run("negative_repair_cost", func(t *testing.T) {
		detail := makeTestTourismData("负成本水道", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, -1000000, 3000, 100000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["负成本水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Axis == "经济性" && pt.Value < 0.99 {
				t.Errorf("负成本应被视为极低, 经济性应接近1.0, 实际=%.4f", pt.Value)
			}
		}
	})

	t.Run("zero_carrying_capacity", func(t *testing.T) {
		detail := makeTestTourismData("零容量水道", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 0, 100000)
		details := []models.AqueductTourismData{detail}

		radar := BuildRadarData(cfg, details)
		aqSeries := radar["aqueducts"].(map[string]interface{})
		pts := aqSeries["零容量水道"].([]models.RadarAxis)

		for _, pt := range pts {
			if pt.Axis == "承载能力" && pt.Value > 0.001 {
				t.Errorf("零承载能力值应接近0, 实际=%.4f", pt.Value)
			}
		}
	})
}

func TestComputePriorityRanking_Robustness(t *testing.T) {
	cfg := defaultConfig()

	t.Run("correct_sorting_order", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道C", 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 3000000, 2000, 50000),
			makeTestTourismData("水道A", 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 500000, 4000, 200000),
			makeTestTourismData("水道B", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 1500000, 3000, 100000),
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder, ok := result["priority_order"].([]map[string]interface{})
		if !ok {
			t.Fatal("priority_order should be []map[string]interface{}")
		}

		if len(priorityOrder) != 3 {
			t.Fatalf("expected 3 ranked items, got %d", len(priorityOrder))
		}

		if priorityOrder[0]["aqueduct_name"] != "水道A" {
			t.Errorf("排名第1应为水道A, 实际=%s", priorityOrder[0]["aqueduct_name"])
		}
		if priorityOrder[1]["aqueduct_name"] != "水道B" {
			t.Errorf("排名第2应为水道B, 实际=%s", priorityOrder[1]["aqueduct_name"])
		}
		if priorityOrder[2]["aqueduct_name"] != "水道C" {
			t.Errorf("排名第3应为水道C, 实际=%s", priorityOrder[2]["aqueduct_name"])
		}

		scores := make([]float64, 3)
		for i, item := range priorityOrder {
			scores[i] = item["priority_score"].(float64)
		}
		if scores[0] <= scores[1] || scores[1] <= scores[2] {
			t.Errorf("分数应严格递减: [%.4f, %.4f, %.4f]", scores[0], scores[1], scores[2])
		}
	})

	t.Run("ranking_stability_same_scores", func(t *testing.T) {
		for run := 0; run < 5; run++ {
			details := []models.AqueductTourismData{
				makeTestTourismData("水道1", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
				makeTestTourismData("水道2", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
				makeTestTourismData("水道3", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
			}

			result := ComputePriorityRanking(cfg, details)
			priorityOrder := result["priority_order"].([]map[string]interface{})

			scores := make([]float64, 3)
			for i, item := range priorityOrder {
				scores[i] = item["priority_score"].(float64)
			}

			maxDiff := 0.0
			for i := 1; i < len(scores); i++ {
				if d := math.Abs(scores[i] - scores[0]); d > maxDiff {
					maxDiff = d
				}
			}
			if maxDiff > 0.001 {
				t.Errorf("相同输入分数应一致, 最大差异=%.4f", maxDiff)
			}
		}
	})

	t.Run("category_assignment_correctness", func(t *testing.T) {
		testCases := []struct {
			name        string
			safety      float64
			wantCat     string
		}{
			{"优秀安全", 0.9, "RECOMMENDED"},
			{"良好安全", 0.7, "RECOMMENDED"},
			{"中等安全", 0.6, "LIMITED_ACCESS"},
			{"较低安全", 0.55, "LIMITED_ACCESS"},
			{"危险安全", 0.4, "CAUTION_RESTRICTED"},
			{"极低安全", 0.2, "CAUTION_RESTRICTED"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				details := []models.AqueductTourismData{
					makeTestTourismData(tc.name, tc.safety, 0.8, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
				}

				result := ComputePriorityRanking(cfg, details)
				priorityOrder := result["priority_order"].([]map[string]interface{})
				cat := priorityOrder[0]["category"].(string)

				if cat != tc.wantCat {
					t.Errorf("safety=%.2f: 期望类别=%s, 实际=%s", tc.safety, tc.wantCat, cat)
				}
			})
		}
	})

	t.Run("monitor_first_for_low_rank", func(t *testing.T) {
		details := make([]models.AqueductTourismData, 6)
		for i := 0; i < 6; i++ {
			safety := 0.85 - float64(i)*0.08
			details[i] = makeTestTourismData(
				fmt.Sprintf("水道-%d", i+1),
				safety,
				0.8, 0.7, 0.7, 0.7, 0.7,
				1000000, 3000, 100000,
			)
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})

		monitorCount := 0
		for _, item := range priorityOrder {
			if item["category"] == "MONITOR_FIRST" {
				monitorCount++
			}
		}

		if monitorCount < 2 {
			t.Errorf("6个水道中应有至少2个MONITOR_FIRST, 实际=%d", monitorCount)
		}

		for i, item := range priorityOrder {
			cat := item["category"].(string)
			rank := item["rank"].(int)
			if rank == i+1 && rank <= 4 && cat == "MONITOR_FIRST" {
				t.Errorf("排名前4不应为MONITOR_FIRST, rank=%d, cat=%s", rank, cat)
			}
		}
	})
}

func TestComputePriorityRanking_BoundaryConditions(t *testing.T) {
	cfg := defaultConfig()

	t.Run("single_aqueduct", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("唯一水道", 0.8, 0.8, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})

		if len(priorityOrder) != 1 {
			t.Errorf("expected 1 result, got %d", len(priorityOrder))
		}
		if priorityOrder[0]["rank"].(int) != 1 {
			t.Errorf("rank should be 1, got %d", priorityOrder[0]["rank"].(int))
		}
		score := priorityOrder[0]["priority_score"].(float64)
		if score <= 0 || score >= 1 {
			t.Errorf("score out of range: %.4f", score)
		}
	})

	t.Run("large_number_of_aqueducts", func(t *testing.T) {
		details := make([]models.AqueductTourismData, 20)
		for i := 0; i < 20; i++ {
			details[i] = makeTestTourismData(
				fmt.Sprintf("水道-%02d", i+1),
				0.9-float64(i)*0.03,
				0.85-float64(i)*0.02,
				0.7, 0.7, 0.7, 0.7,
				float64(i+1)*500000,
				3000+float64(i)*100,
				100000+i*5000,
			)
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})

		if len(priorityOrder) != 20 {
			t.Errorf("expected 20 results, got %d", len(priorityOrder))
		}

		for i, item := range priorityOrder {
			if item["rank"].(int) != i+1 {
				t.Errorf("rank %d: expected %d, got %d", i, i+1, item["rank"].(int))
			}
		}
	})

	t.Run("all_extremely_good", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道1", 0.95, 0.95, 0.95, 0.95, 0.95, 0.95, 100000, 5000, 200000),
			makeTestTourismData("水道2", 0.92, 0.92, 0.92, 0.92, 0.92, 0.92, 200000, 4800, 180000),
			makeTestTourismData("水道3", 0.90, 0.90, 0.90, 0.90, 0.90, 0.90, 300000, 4500, 160000),
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})

		for _, item := range priorityOrder {
			cat := item["category"].(string)
			if cat == "CAUTION_RESTRICTED" || cat == "MONITOR_FIRST" {
				t.Errorf("优秀水道不应为%s类别", cat)
			}
		}
	})

	t.Run("all_extremely_poor", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道1", 0.4, 0.4, 0.4, 0.4, 0.4, 0.4, 5000000, 500, 10000),
			makeTestTourismData("水道2", 0.35, 0.35, 0.35, 0.35, 0.35, 0.35, 6000000, 400, 8000),
			makeTestTourismData("水道3", 0.3, 0.3, 0.3, 0.3, 0.3, 0.3, 7000000, 300, 5000),
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})

		for _, item := range priorityOrder {
			cat := item["category"].(string)
			if cat != "CAUTION_RESTRICTED" {
				t.Errorf("危险水道应为CAUTION_RESTRICTED, 实际=%s", cat)
			}
		}
	})
}

func TestComputePriorityRanking_AnomalyCases(t *testing.T) {
	cfg := defaultConfig()

	t.Run("negative_safety_category", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("异常水道", -1.0, 0.7, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})
		cat := priorityOrder[0]["category"].(string)

		if cat != "CAUTION_RESTRICTED" {
			t.Errorf("负安全值应为CAUTION_RESTRICTED, 实际=%s", cat)
		}
	})

	t.Run("extreme_scores_no_panic", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("极端水道", 1e6, -1e6, 1e6, -1e6, 1e6, 0.5, 1e12, 1e9, 0),
		}

		result := ComputePriorityRanking(cfg, details)
		priorityOrder := result["priority_order"].([]map[string]interface{})

		score := priorityOrder[0]["priority_score"].(float64)
		if math.IsNaN(score) || math.IsInf(score, 0) {
			t.Errorf("极端输入不应产生NaN或Inf, score=%v", score)
		}
	})
}

func TestExtractTourismMetrics_CarryingCapacityRationality(t *testing.T) {
	t.Run("occupancy_rate_calculation", func(t *testing.T) {
		testCases := []struct {
			name        string
			visitors    int
			capacity    float64
			wantMinPct  float64
			wantMaxPct  float64
		}{
			{"低客流", 36500, 1000, 0, 20},
			{"中客流", 182500, 1000, 20, 70},
			{"高客流", 365000, 1000, 70, 150},
			{"极高客流", 730000, 1000, 150, 300},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				details := []models.AqueductTourismData{
					makeTestTourismData("测试水道", 0.8, 0.8, 0.7, 0.7, 0.7, 0.7, 1000000, tc.capacity, tc.visitors),
				}

				metrics := ExtractTourismMetrics(details)
				perAq := metrics["per_aqueduct"].(map[string]interface{})
				aqMetrics := perAq["测试水道"].(map[string]interface{})
				occupancy := aqMetrics["occupancy_rate_pct"].(float64)

				if occupancy < tc.wantMinPct || occupancy > tc.wantMaxPct {
					t.Errorf("访客=%d, 容量=%.0f: 期望入住率在[%.1f, %.1f]%%, 实际=%.1f%%",
						tc.visitors, tc.capacity, tc.wantMinPct, tc.wantMaxPct, occupancy)
				}

				carryingCap := aqMetrics["carrying_capacity"].(float64)
				if math.Abs(carryingCap-tc.capacity) > 0.001 {
					t.Errorf("承载能力不匹配: 期望=%.0f, 实际=%.0f", tc.capacity, carryingCap)
				}

				annualVisitors := aqMetrics["annual_visitors"].(int)
				if annualVisitors != tc.visitors {
					t.Errorf("年访客不匹配: 期望=%d, 实际=%d", tc.visitors, annualVisitors)
				}
			})
		}
	})

	t.Run("zero_capacity_handling", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("零容量水道", 0.8, 0.8, 0.7, 0.7, 0.7, 0.7, 1000000, 0, 100000),
		}

		metrics := ExtractTourismMetrics(details)
		perAq := metrics["per_aqueduct"].(map[string]interface{})
		aqMetrics := perAq["零容量水道"].(map[string]interface{})
		occupancy := aqMetrics["occupancy_rate_pct"].(float64)

		if math.IsInf(occupancy, 0) || math.IsNaN(occupancy) {
			t.Errorf("零容量不应产生Inf或NaN, 入住率=%v", occupancy)
		}
	})

	t.Run("total_visitors_sum", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("水道A", 0.8, 0.8, 0.7, 0.7, 0.7, 0.7, 1000000, 3000, 100000),
			makeTestTourismData("水道B", 0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 2000000, 2500, 150000),
			makeTestTourismData("水道C", 0.6, 0.6, 0.7, 0.7, 0.7, 0.7, 3000000, 2000, 80000),
		}

		metrics := ExtractTourismMetrics(details)
		total := metrics["total_annual_visitors"].(int)

		if total != 100000+150000+80000 {
			t.Errorf("总访客不匹配: 期望=%d, 实际=%d", 330000, total)
		}
	})
}

func TestClassifyHeritageType_VariousAges(t *testing.T) {
	testCases := []struct {
		name       string
		year       int
		histScore  float64
		height     float64
		length     float64
		wantType   string
	}{
		{"UNESCO_超古老", -500, 0.8, 20, 20, "UNESCO_WORLD_HERITAGE"},
		{"UNESCO_高历史分", 100, 0.95, 15, 15, "UNESCO_WORLD_HERITAGE"},
		{"国家考古_古老", 100, 0.8, 15, 15, "NATIONAL_ARCHAEOLOGICAL_SITE"},
		{"国家考古_较高历史", 500, 0.85, 15, 15, "NATIONAL_ARCHAEOLOGICAL_SITE"},
		{"工程奇迹_高大长", 1000, 0.5, 30, 40, "RARE_ENGINEERING_MONUMENT"},
		{"地区遗产_较老", 500, 0.5, 15, 15, "REGIONAL_HERITAGE_SITE"},
		{"一般历史_年轻", 1500, 0.4, 10, 10, "GENERAL_HISTORIC_SITE"},
		{"一般历史_低分", 1800, 0.3, 10, 10, "GENERAL_HISTORIC_SITE"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			aq := makeTestAqueduct(tc.name, tc.year, tc.length, tc.height)
			tData := &models.AqueductTourismData{
				HistoricalSignificance: tc.histScore,
			}

			result := ClassifyHeritageType(&aq, tData)
			if result != tc.wantType {
				t.Errorf("year=%d, hist=%.2f: 期望=%s, 实际=%s", tc.year, tc.histScore, tc.wantType, result)
			}
		})
	}
}

func TestCompareAqueducts_Integration(t *testing.T) {
	cfg := defaultConfig()

	t.Run("full_comparison_workflow", func(t *testing.T) {
		details := []models.AqueductTourismData{
			makeTestTourismData("克劳狄水道", 0.85, 0.95, 0.75, 0.85, 0.90, 0.80, 2000000, 4000, 250000),
			makeTestTourismData("加尔水道", 0.90, 0.90, 0.70, 0.95, 0.95, 0.85, 1500000, 3500, 200000),
			makeTestTourismData("图拉真水道", 0.75, 0.85, 0.80, 0.75, 0.80, 0.70, 3000000, 3000, 150000),
			makeTestTourismData("亚历山大水道", 0.65, 0.80, 0.65, 0.70, 0.75, 0.60, 4000000, 2500, 100000),
		}

		result := CompareAqueducts(cfg, details)

		if result.RadarData == nil {
			t.Error("RadarData 不应为 nil")
		}
		if result.Ranking == nil {
			t.Error("Ranking 不应为 nil")
		}
		if result.StructuralMetrics == nil {
			t.Error("StructuralMetrics 不应为 nil")
		}
		if result.CostMetrics == nil {
			t.Error("CostMetrics 不应为 nil")
		}
		if result.TourismMetrics == nil {
			t.Error("TourismMetrics 不应为 nil")
		}
		if result.OverallScores == nil {
			t.Error("OverallScores 不应为 nil")
		}
		if result.Recommendation == "" {
			t.Error("Recommendation 不应为空")
		}
		if result.AqueductIDs == nil {
			t.Error("AqueductIDs 不应为 nil")
		}

		radarData := result.RadarData
		axes, _ := radarData["axes"].([]string)
		if len(axes) != 9 {
			t.Errorf("雷达图应有9个轴, 实际=%d", len(axes))
		}

		ranking := result.Ranking
		priorityOrder, _ := ranking["priority_order"].([]map[string]interface{})
		if len(priorityOrder) != 4 {
			t.Errorf("应有4个水道排名, 实际=%d", len(priorityOrder))
		}

		topName := priorityOrder[0]["aqueduct_name"].(string)
		if topName != "加尔水道" && topName != "克劳狄水道" {
			t.Errorf("排名第一应为加尔水道或克劳狄水道, 实际=%s", topName)
		}

		tourismMetrics := result.TourismMetrics
		totalVisitors, _ := tourismMetrics["total_annual_visitors"].(int)
		if totalVisitors != 250000+200000+150000+100000 {
			t.Errorf("总访客不匹配: 期望=%d, 实际=%d", 700000, totalVisitors)
		}

		costMetrics := result.CostMetrics
		totalRepair, _ := costMetrics["total_repair_estimate_eur"].(float64)
		expectedTotal := 2000000 + 1500000 + 3000000 + 4000000
		if math.Abs(totalRepair-float64(expectedTotal)) > 1 {
			t.Errorf("总修复成本不匹配: 期望=%d, 实际=%.0f", expectedTotal, totalRepair)
		}

		overallScores := result.OverallScores
		bestScore, _ := overallScores["best_priority_score"].(float64)
		if bestScore <= 0 || bestScore >= 1 {
			t.Errorf("最佳优先级分数不合理: %.4f", bestScore)
		}

		aqIds := result.AqueductIDs
		count, _ := aqIds["count"].(int)
		if count != 4 {
			t.Errorf("水道数量不匹配: 期望=4, 实际=%d", count)
		}

		if !containsString(result.Recommendation, "建议优先开放") {
			t.Errorf("建议应包含'建议优先开放', 实际=%s", result.Recommendation)
		}
	})
}

func TestRankDescription_AllCategories(t *testing.T) {
	testCases := []struct {
		cat  string
		want string
	}{
		{"RECOMMENDED", "优先对外开放"},
		{"LIMITED_ACCESS", "限制流量开放"},
		{"CAUTION_RESTRICTED", "仅专业导览开放"},
		{"MONITOR_FIRST", "暂缓开放"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tc := range testCases {
		t.Run(tc.cat, func(t *testing.T) {
			desc := RankDescription(tc.cat)
			if !containsString(desc, tc.want) {
				t.Errorf("%s: 期望包含'%s', 实际=%s", tc.cat, tc.want, desc)
			}
		})
	}
}

func TestHeritageTypeBonus_AllTypes(t *testing.T) {
	cfg := defaultConfig()

	testCases := []struct {
		heritageType string
		wantBonus    float64
	}{
		{"UNESCO_WORLD_HERITAGE", 1.25},
		{"NATIONAL_ARCHAEOLOGICAL_SITE", 1.15},
		{"RARE_ENGINEERING_MONUMENT", 1.10},
		{"REGIONAL_HERITAGE_SITE", 1.05},
		{"GENERAL_HISTORIC_SITE", 1.0},
		{"UNKNOWN_TYPE", 1.0},
	}

	for _, tc := range testCases {
		t.Run(tc.heritageType, func(t *testing.T) {
			bonus := HeritageTypeBonus(cfg, tc.heritageType)
			if math.Abs(bonus-tc.wantBonus) > 0.001 {
				t.Errorf("%s: 期望bonus=%.2f, 实际=%.2f", tc.heritageType, tc.wantBonus, bonus)
			}
		})
	}
}

func TestCalculateHeritageValue_Components(t *testing.T) {
	cfg := defaultConfig()

	t.Run("higher_historical_higher_value", func(t *testing.T) {
		aq1 := makeTestAqueduct("古老水道1", -100, 20, 15)
		tData1 := &models.AqueductTourismData{
			HistoricalSignificance: 0.9,
			CurrentConditionScore:  0.8,
			VisibilityScore:        0.7,
			PhotographicValue:      0.7,
		}

		aq2 := makeTestAqueduct("古老水道2", -100, 20, 15)
		tData2 := &models.AqueductTourismData{
			HistoricalSignificance: 0.5,
			CurrentConditionScore:  0.8,
			VisibilityScore:        0.7,
			PhotographicValue:      0.7,
		}

		v1 := CalculateHeritageValue(cfg, &aq1, tData1, 1.0)
		v2 := CalculateHeritageValue(cfg, &aq2, tData2, 1.0)

		if v1 <= v2 {
			t.Errorf("高历史分应有更高遗产价值: v1=%.4f, v2=%.4f", v1, v2)
		}
	})

	t.Run("bonus_multiplies_value", func(t *testing.T) {
		aq := makeTestAqueduct("测试水道", -100, 20, 15)
		tData := &models.AqueductTourismData{
			HistoricalSignificance: 0.8,
			CurrentConditionScore:  0.8,
			VisibilityScore:        0.7,
			PhotographicValue:      0.7,
		}

		v1 := CalculateHeritageValue(cfg, &aq, tData, 1.0)
		v2 := CalculateHeritageValue(cfg, &aq, tData, 1.25)

		if math.Abs(v2/v1-1.25) > 0.01 {
			t.Errorf("bonus应有倍增效果: v1=%.4f, v2=%.4f, ratio=%.4f", v1, v2, v2/v1)
		}
	})
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
