package aqueduct_comparator

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

type ComparisonResult struct {
	RadarData         map[string]interface{}
	Ranking           map[string]interface{}
	StructuralMetrics map[string]interface{}
	CostMetrics       map[string]interface{}
	TourismMetrics    map[string]interface{}
	OverallScores     map[string]interface{}
	Recommendation    string
	AqueductIDs       map[string]interface{}
}

func Round2(v float64) float64 { return math.Round(v*100) / 100 }
func Round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func ClassifyHeritageType(aq *models.Aqueduct, tData *models.AqueductTourismData) string {
	age := 2000.0
	if aq.ConstructionYear < 0 {
		age = float64(-aq.ConstructionYear) + 2025.0
	} else {
		age = 2025.0 - float64(aq.ConstructionYear)
	}

	histScore := tData.HistoricalSignificance
	if histScore > 0.90 || age > 2100 {
		return "UNESCO_WORLD_HERITAGE"
	} else if histScore > 0.75 || age > 1900 {
		return "NATIONAL_ARCHAEOLOGICAL_SITE"
	} else if histScore > 0.60 || (aq.HeightM > 25 && aq.LengthKM > 30) {
		return "RARE_ENGINEERING_MONUMENT"
	} else if histScore > 0.45 || age > 1500 {
		return "REGIONAL_HERITAGE_SITE"
	}
	return "GENERAL_HISTORIC_SITE"
}

func HeritageTypeBonus(cfg *config.TourismConfig, heritageType string) float64 {
	switch heritageType {
	case "UNESCO_WORLD_HERITAGE":
		if cfg.UNESCOSiteBonus > 0 {
			return cfg.UNESCOSiteBonus
		}
		return 1.25
	case "NATIONAL_ARCHAEOLOGICAL_SITE":
		if cfg.ArchaeologicalSiteBonus > 0 {
			return cfg.ArchaeologicalSiteBonus
		}
		return 1.15
	case "RARE_ENGINEERING_MONUMENT":
		if cfg.RareArchitectureBonus > 0 {
			return cfg.RareArchitectureBonus
		}
		return 1.10
	case "REGIONAL_HERITAGE_SITE":
		return 1.05
	default:
		return 1.0
	}
}

func CalculateHeritageValue(cfg *config.TourismConfig, aq *models.Aqueduct, tData *models.AqueductTourismData, bonus float64) float64 {
	baseHeritage := 0.35*tData.HistoricalSignificance +
		0.25*tData.CurrentConditionScore +
		0.20*tData.VisibilityScore +
		0.20*tData.PhotographicValue

	age := 2000.0
	if aq.ConstructionYear < 0 {
		age = float64(-aq.ConstructionYear) + 2025.0
	} else {
		age = 2025.0 - float64(aq.ConstructionYear)
	}
	ageFactor := math.Min(1.0, age/2500.0)

	heritageWeight := cfg.HeritageValueWeight
	if heritageWeight <= 0 {
		heritageWeight = 0.5
	}

	score := baseHeritage * bonus * (1.0 + heritageWeight*ageFactor)
	return score
}

func CalculateExpertJudgmentScore(cfg *config.TourismConfig, aq *models.Aqueduct, tData *models.AqueductTourismData, heritageType string) float64 {
	expertMatrix := cfg.ExpertJudgmentMatrix
	if expertMatrix == nil {
		expertMatrix = map[string]float64{
			"structural_integrity":      0.25,
			"historical_documentation":  0.20,
			"cultural_significance":     0.20,
			"engineering_uniqueness":    0.15,
			"conservation_status":       0.20,
		}
	}

	integrityScore := tData.SafetyScore*0.7 + tData.CurrentConditionScore*0.3
	docScore := tData.HistoricalSignificance*0.8 + tData.AccessibilityScore*0.2
	culturalScore := tData.HistoricalSignificance
	uniquenessScore := 0.0
	if aq.HeightM > 25 || aq.LengthKM > 50 {
		uniquenessScore = 0.8
	} else {
		uniquenessScore = 0.5
	}
	conservationScore := tData.CurrentConditionScore

	score := expertMatrix["structural_integrity"]*integrityScore +
		expertMatrix["historical_documentation"]*docScore +
		expertMatrix["cultural_significance"]*culturalScore +
		expertMatrix["engineering_uniqueness"]*uniquenessScore +
		expertMatrix["conservation_status"]*conservationScore

	expertWeight := cfg.ExpertScoreWeight
	if expertWeight > 0 {
		score = score * (1.0 + expertWeight)
	}
	return score
}

func BuildRadarData(cfg *config.TourismConfig, details []models.AqueductTourismData) map[string]interface{} {
	axes := []string{"结构安全", "历史意义", "可达性", "可见度", "摄影价值", "经济性", "承载能力", "遗产价值", "专家评分"}
	maxValues := []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.5, 1.5}

	aqSeries := make(map[string]interface{})
	for _, d := range details {
		normalizedCost := 1.0 - math.Min(1.0, d.RepairCostEstimate/cfg.RepairCostNormalization)
		heritageNormalized := math.Min(1.0, d.HeritageValueScore)
		expertNormalized := math.Min(1.0, d.ExpertJudgmentScore)
		values := []float64{
			d.SafetyScore,
			d.HistoricalSignificance,
			d.AccessibilityScore,
			d.VisibilityScore,
			d.PhotographicValue,
			normalizedCost,
			math.Min(1.0, d.TourismCarryingCapacity/5000.0),
			heritageNormalized,
			expertNormalized,
		}
		points := make([]models.RadarAxis, len(axes))
		for i, ax := range axes {
			points[i] = models.RadarAxis{
				Axis:  ax,
				Label: ax,
				Value: Round3(values[i]),
				Max:   maxValues[i],
			}
		}
		aqSeries[d.AqueductName] = points
	}
	return map[string]interface{}{
		"axes":      axes,
		"aqueducts": aqSeries,
	}
}

func ComputePriorityRanking(cfg *config.TourismConfig, details []models.AqueductTourismData) map[string]interface{} {
	type scored struct {
		name          string
		id            uuid.UUID
		score         float64
		safety        float64
		heritageScore float64
		expertScore   float64
		heritageBonus float64
		heritageType  string
	}
	scoredList := make([]scored, 0, len(details))
	for _, d := range details {
		repairNorm := 1.0 - math.Min(1.0, d.RepairCostEstimate/cfg.RepairCostNormalization)

		heritageWeight := cfg.HeritageValueWeight
		if heritageWeight <= 0 {
			heritageWeight = 0.20
		}
		expertWeight := cfg.ExpertScoreWeight
		if expertWeight <= 0 {
			expertWeight = 0.15
		}

		baseComposite := cfg.SafetyWeight*d.SafetyScore +
			cfg.HistoricalWeight*d.HistoricalSignificance +
			cfg.AccessibilityWeight*d.AccessibilityScore +
			cfg.EconomicWeight*repairNorm +
			0.08*d.VisibilityScore +
			0.09*d.PhotographicValue

		heritageComponent := heritageWeight * d.HeritageValueScore
		expertComponent := expertWeight * d.ExpertJudgmentScore

		totalWeight := 1.0 + heritageWeight + expertWeight
		composite := (baseComposite + heritageComponent + expertComponent) / totalWeight

		scoredList = append(scoredList, scored{
			name: d.AqueductName, id: d.ID, score: Round3(composite), safety: d.SafetyScore,
			heritageScore: d.HeritageValueScore, expertScore: d.ExpertJudgmentScore,
			heritageBonus: d.HeritageBonus, heritageType: d.HeritageType,
		})
	}
	sort.Slice(scoredList, func(a, b int) bool { return scoredList[a].score > scoredList[b].score })

	ranking := make(map[string]interface{})
	recommended := make([]map[string]interface{}, 0)
	for i, s := range scoredList {
		category := "RECOMMENDED"
		if s.safety < 0.5 {
			category = "CAUTION_RESTRICTED"
		} else if s.safety < 0.65 {
			category = "LIMITED_ACCESS"
		} else if i >= len(scoredList)*2/3 {
			category = "MONITOR_FIRST"
		}
		recommended = append(recommended, map[string]interface{}{
			"rank":               i + 1,
			"aqueduct_name":      s.name,
			"id":                 s.id.String(),
			"priority_score":     s.score,
			"safety_score":       s.safety,
			"heritage_score":     s.heritageScore,
			"expert_score":       s.expertScore,
			"heritage_bonus":     s.heritageBonus,
			"heritage_type":      s.heritageType,
			"category":           category,
			"description":        RankDescription(category),
		})
		ranking[s.name] = map[string]interface{}{
			"rank": i + 1, "priority_score": s.score, "category": category,
			"heritage_type": s.heritageType, "heritage_bonus": s.heritageBonus,
		}
	}
	return map[string]interface{}{
		"ranking":        ranking,
		"priority_order": recommended,
	}
}

func ExtractStructuralMetrics(details []models.AqueductTourismData) map[string]interface{} {
	metrics := make(map[string]interface{})
	for _, d := range details {
		metrics[d.AqueductName] = map[string]interface{}{
			"safety_score":          d.SafetyScore,
			"condition_score":       d.CurrentConditionScore,
			"repair_cost_estimate":  d.RepairCostEstimate,
			"visitor_per_year":      d.VisitorCountPerYear,
			"heritage_value_score":  d.HeritageValueScore,
			"expert_judgment_score": d.ExpertJudgmentScore,
			"heritage_type":         d.HeritageType,
			"heritage_bonus":        d.HeritageBonus,
		}
	}
	return metrics
}

func ExtractCostMetrics(details []models.AqueductTourismData) map[string]interface{} {
	totalRepair := 0.0
	for _, d := range details {
		totalRepair += d.RepairCostEstimate
	}
	perAq := make(map[string]interface{})
	for _, d := range details {
		perAq[d.AqueductName] = map[string]interface{}{
			"estimated_repair_eur": d.RepairCostEstimate,
			"ticket_revenue_eur":   d.VisitorCountPerYear * int(d.TicketPriceEur),
			"roi_years":            int(math.Ceil(d.RepairCostEstimate / math.Max(1, float64(d.VisitorCountPerYear)*d.TicketPriceEur*0.15))),
			"cost_vs_total_pct":    Round3(d.RepairCostEstimate / math.Max(1, totalRepair) * 100),
		}
	}
	return map[string]interface{}{
		"total_repair_estimate_eur": Round2(totalRepair),
		"per_aqueduct":              perAq,
	}
}

func ExtractTourismMetrics(details []models.AqueductTourismData) map[string]interface{} {
	totalVisitors := 0
	for _, d := range details {
		totalVisitors += d.VisitorCountPerYear
	}
	perAq := make(map[string]interface{})
	for _, d := range details {
		perAq[d.AqueductName] = map[string]interface{}{
			"annual_visitors":        d.VisitorCountPerYear,
			"carrying_capacity":      d.TourismCarryingCapacity,
			"occupancy_rate_pct":     Round3(float64(d.VisitorCountPerYear) / (365.0 * math.Max(1, d.TourismCarryingCapacity)) * 100),
			"accessibility_score":    d.AccessibilityScore,
			"historical_significance": d.HistoricalSignificance,
			"ticket_price_eur":       d.TicketPriceEur,
		}
	}
	return map[string]interface{}{
		"total_annual_visitors": totalVisitors,
		"per_aqueduct":          perAq,
	}
}

func ComputeOverallScores(details []models.AqueductTourismData, ranking map[string]interface{}) map[string]interface{} {
	scoreMap := make(map[string]interface{})
	overallBest := 0.0
	for _, d := range details {
		stDev := 0.0
		if rankInfo, ok := ranking[d.AqueductName].(map[string]interface{}); ok {
			if v, ok := rankInfo["priority_score"].(float64); ok {
				stDev = v
				if v > overallBest {
					overallBest = v
				}
			}
		}
		scoreMap[d.AqueductName] = map[string]interface{}{
			"priority_score":      stDev,
			"safety_score":        d.SafetyScore,
			"tourism_feasibility": Round3(0.6*d.SafetyScore + 0.4*(1.0-math.Min(1.0, d.RepairCostEstimate/2e6))),
			"repair_urgency":      Round3(1.0 - d.SafetyScore),
		}
	}
	return map[string]interface{}{
		"best_priority_score": Round3(overallBest),
		"per_aqueduct":        scoreMap,
	}
}

func GenerateRecommendation(details []models.AqueductTourismData, ranking map[string]interface{}) string {
	ordered, _ := ranking["priority_order"].([]map[string]interface{})
	if len(ordered) == 0 {
		return "暂无足够数据生成建议"
	}
	best := ordered[0]
	return "建议优先开放" + best["aqueduct_name"].(string) +
		"，结构安全与旅游价值综合最优。"
}

func RankDescription(category string) string {
	switch category {
	case "RECOMMENDED":
		return "优先对外开放，结构安全且旅游价值高"
	case "LIMITED_ACCESS":
		return "限制流量开放，设置警示标识"
	case "CAUTION_RESTRICTED":
		return "仅专业导览开放，需预先加固关键段"
	case "MONITOR_FIRST":
		return "暂缓开放，优先实施结构加固"
	default:
		return category
	}
}

func BuildDefaultTourismData(aq *models.Aqueduct) *models.AqueductTourismData {
	ageFactor := 1.0
	if aq.ConstructionYear < 0 {
		ageFactor = 1.0 - float64(aq.ConstructionYear)/400.0
	} else {
		ageFactor = 0.9 + float64(aq.ConstructionYear)/3000.0
	}
	ageFactor = math.Max(0.5, math.Min(1.1, ageFactor))

	visitors := int(15000 + float64(aq.LengthKM)*8000 + ageFactor*50000)
	price := 6.0 + 4.0*(1.0-math.Min(1.0, float64(len(aq.Name))/20.0))

	return &models.AqueductTourismData{
		ID:                     uuid.New(),
		AqueductID:             aq.ID,
		VisitorCountPerYear:    visitors,
		TicketPriceEur:         Round2(price),
		AccessibilityScore:     Round3(0.45 + 0.002*float64(aq.LengthKM)),
		VisibilityScore:        Round3(0.55 + 0.008*aq.HeightM),
		HistoricalSignificance: Round3(0.60 + 0.25*(1.0-math.Min(1.0, ageFactor))),
		PhotographicValue:      Round3(0.50 + 0.01*aq.HeightM + 0.05*(1.0-math.Min(1.0, ageFactor))),
		CurrentConditionScore:  Round3(0.72 - 0.002*float64(aq.ConstructionYear)),
		ProximityToCityKm:      15.0 + 3.0*math.Sqrt(float64(aq.LengthKM)),
		NearbyAmenitiesScore:   Round3(0.55),
		MaxDailyVisitors:       int(120 + 40*aq.HeightM + float64(visitors)/365.0*0.3),
		GuidedTourAvailable:    true,
		WheelchairAccessible:   len(aq.Name)%2 == 0,
		PublicTransportAccess:  true,
		PeakSeason:             "4月-10月",
		TourismNotes:           "基于水道物理参数模拟的默认旅游数据",
		LastUpdated:            time.Now().UTC(),
		CreatedAt:              time.Now().UTC(),
	}
}

func IdsToJSON(details []models.AqueductTourismData) map[string]interface{} {
	ids := make([]string, 0, len(details))
	names := make([]string, 0, len(details))
	for _, d := range details {
		ids = append(ids, d.AqueductID.String())
		names = append(names, d.AqueductName)
	}
	return map[string]interface{}{"ids": ids, "names": names, "count": len(details)}
}

func EnrichTourismDataWithHeritage(
	cfg *config.TourismConfig,
	aq *models.Aqueduct,
	tData *models.AqueductTourismData,
) *models.AqueductTourismData {
	heritageType := ClassifyHeritageType(aq, tData)
	bonus := HeritageTypeBonus(cfg, heritageType)
	heritageValue := CalculateHeritageValue(cfg, aq, tData, bonus)
	expertScore := CalculateExpertJudgmentScore(cfg, aq, tData, heritageType)

	enriched := *tData
	enriched.HeritageType = heritageType
	enriched.HeritageBonus = Round3(bonus)
	enriched.HeritageValueScore = Round3(heritageValue)
	enriched.ExpertJudgmentScore = Round3(expertScore)
	return &enriched
}

func CompareAqueducts(
	cfg *config.TourismConfig,
	details []models.AqueductTourismData,
) *ComparisonResult {
	radarData := BuildRadarData(cfg, details)
	ranking := ComputePriorityRanking(cfg, details)
	structuralMetrics := ExtractStructuralMetrics(details)
	costMetrics := ExtractCostMetrics(details)
	tourismMetrics := ExtractTourismMetrics(details)
	overallScores := ComputeOverallScores(details, ranking)
	recommendation := GenerateRecommendation(details, ranking)
	aqIds := IdsToJSON(details)

	return &ComparisonResult{
		RadarData:         radarData,
		Ranking:           ranking,
		StructuralMetrics: structuralMetrics,
		CostMetrics:       costMetrics,
		TourismMetrics:    tourismMetrics,
		OverallScores:     overallScores,
		Recommendation:    recommendation,
		AqueductIDs:       aqIds,
	}
}
