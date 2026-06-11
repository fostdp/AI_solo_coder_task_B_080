package tourism

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/aqueduct_comparator"
	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type TourismPlanner struct {
	repo *repository.Repository
	cfg  *config.TourismConfig
}

func NewTourismPlanner(repo *repository.Repository, cfg *config.Config) *TourismPlanner {
	return &TourismPlanner{repo: repo, cfg: &cfg.Tourism}
}

func (tp *TourismPlanner) CompareAqueducts(
	ctx context.Context,
	aqueductIDs []uuid.UUID,
) (*models.AqueductComparison, error) {

	var aqueducts []models.Aqueduct
	var err error
	if len(aqueductIDs) == 0 {
		aqueducts, err = tp.repo.GetAllAqueducts(ctx)
	} else {
		allAqs, e := tp.repo.GetAllAqueducts(ctx)
		err = e
		if err == nil {
			idSet := make(map[uuid.UUID]bool, len(aqueductIDs))
			for _, id := range aqueductIDs {
				idSet[id] = true
			}
			for i := range allAqs {
				if idSet[allAqs[i].ID] {
					aqueducts = append(aqueducts, allAqs[i])
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}

	details := make([]models.AqueductTourismData, 0, len(aqueducts))
	for i := range aqueducts {
		td, e := tp.buildAqueductTourismData(ctx, &aqueducts[i])
		if e != nil {
			continue
		}
		details = append(details, *td)
	}

	compResult := aqueduct_comparator.CompareAqueducts(tp.cfg, details)

	result := &models.AqueductComparison{
		ID:                    uuid.New(),
		ComparisonName:        "多水道综合对比分析",
		AqueductIDs:           compResult.AqueductIDs,
		AnalysisTime:          time.Now().UTC(),
		StructuralMetrics:     compResult.StructuralMetrics,
		CostMetrics:           compResult.CostMetrics,
		TourismMetrics:        compResult.TourismMetrics,
		RadarChartData:        compResult.RadarData,
		PriorityRanking:       compResult.Ranking,
		OverallScore:          compResult.OverallScores,
		RecommendationSummary: compResult.Recommendation,
		CreatedAt:             time.Now().UTC(),
		AqueductsDetail:       details,
	}
	return result, nil
}

func (tp *TourismPlanner) buildAqueductTourismData(
	ctx context.Context,
	aq *models.Aqueduct,
) (*models.AqueductTourismData, error) {

	tData, err := tp.repo.GetAqueductTourismData(ctx, aq.ID)
	if err != nil || tData == nil {
		tData = aqueduct_comparator.BuildDefaultTourismData(aq)
	}

	segments, err := tp.repo.GetAllSegmentsWithStatus(ctx, &aq.ID)
	if err != nil {
		return nil, err
	}

	safeCount, warnCount, dangerCount, criticalCount := 0, 0, 0, 0
	totalCapacity := 0.0
	totalRatio := 0.0
	for i := range segments {
		switch segments[i].SafetyLevel {
		case "SAFE":
			safeCount++
		case "WARNING":
			warnCount++
		case "DANGER":
			dangerCount++
		case "CRITICAL":
			criticalCount++
		}
		totalCapacity += segments[i].ResidualCapacity
		totalRatio += segments[i].CapacityRatio
	}
	n := float64(len(segments))
	avgRatio := 0.0
	if n > 0 {
		avgRatio = totalRatio / n
	}
	totalCapRatio := avgRatio
	safetyScore := 0.25*float64(safeCount)/math.Max(1, n) +
		0.20*float64(safeCount+warnCount)/math.Max(1, n) +
		0.35*totalCapRatio +
		0.10*(1.0-float64(criticalCount)/math.Max(1, n)) +
		0.10*(1.0-math.Min(1.0, float64(dangerCount+criticalCount)/math.Max(1, n/3)))

	repCost := 0.0
	if n > 0 {
		segUnits := 120000.0
		repCost = segUnits * n * (1.0 - totalCapRatio)
		repCost = math.Max(50000, repCost)
	}

	carryingCap := float64(tData.MaxDailyVisitors) / math.Max(1, tp.cfg.CarryingCapacityFactor)

	tData.AqueductName = aq.Name
	tData.SafetyScore = aqueduct_comparator.Round3(math.Max(0, math.Min(1.0, safetyScore)))
	tData.RepairCostEstimate = aqueduct_comparator.Round2(repCost)
	tData.TourismCarryingCapacity = aqueduct_comparator.Round2(carryingCap)

	enriched := aqueduct_comparator.EnrichTourismDataWithHeritage(tp.cfg, aq, tData)
	_ = totalCapacity
	return enriched, nil
}

func (tp *TourismPlanner) extractStructuralMetrics(details []models.AqueductTourismData) map[string]interface{} {
	return aqueduct_comparator.ExtractStructuralMetrics(details)
}

func (tp *TourismPlanner) extractCostMetrics(details []models.AqueductTourismData) map[string]interface{} {
	return aqueduct_comparator.ExtractCostMetrics(details)
}

func (tp *TourismPlanner) extractTourismMetrics(details []models.AqueductTourismData) map[string]interface{} {
	return aqueduct_comparator.ExtractTourismMetrics(details)
}

func (tp *TourismPlanner) buildRadarData(details []models.AqueductTourismData) map[string]interface{} {
	return aqueduct_comparator.BuildRadarData(tp.cfg, details)
}

func (tp *TourismPlanner) computePriorityRanking(details []models.AqueductTourismData) map[string]interface{} {
	return aqueduct_comparator.ComputePriorityRanking(tp.cfg, details)
}

func (tp *TourismPlanner) computeOverallScores(details []models.AqueductTourismData, ranking map[string]interface{}) map[string]interface{} {
	return aqueduct_comparator.ComputeOverallScores(details, ranking)
}

func (tp *TourismPlanner) generateRecommendation(details []models.AqueductTourismData, ranking map[string]interface{}) string {
	return aqueduct_comparator.GenerateRecommendation(details, ranking)
}

func buildDefaultTourismData(aq *models.Aqueduct) *models.AqueductTourismData {
	return aqueduct_comparator.BuildDefaultTourismData(aq)
}

func idsToJSON(details []models.AqueductTourismData) map[string]interface{} {
	return aqueduct_comparator.IdsToJSON(details)
}

func rankDescription(category string) string {
	return aqueduct_comparator.RankDescription(category)
}

func round2(v float64) float64 { return aqueduct_comparator.Round2(v) }
func round3(v float64) float64 { return aqueduct_comparator.Round3(v) }

func (tp *TourismPlanner) classifyHeritageType(aq *models.Aqueduct, tData *models.AqueductTourismData) string {
	return aqueduct_comparator.ClassifyHeritageType(aq, tData)
}

func (tp *TourismPlanner) heritageTypeBonus(heritageType string) float64 {
	return aqueduct_comparator.HeritageTypeBonus(tp.cfg, heritageType)
}

func (tp *TourismPlanner) calculateHeritageValue(aq *models.Aqueduct, tData *models.AqueductTourismData, bonus float64) float64 {
	return aqueduct_comparator.CalculateHeritageValue(tp.cfg, aq, tData, bonus)
}

func (tp *TourismPlanner) calculateExpertJudgmentScore(aq *models.Aqueduct, tData *models.AqueductTourismData, heritageType string) float64 {
	return aqueduct_comparator.CalculateExpertJudgmentScore(tp.cfg, aq, tData, heritageType)
}
