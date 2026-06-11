package aqueduct_comparator

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type ComparatorService struct {
	repo *repository.Repository
	cfg  *config.TourismConfig
}

func NewComparatorService(repo *repository.Repository, cfg *config.Config) *ComparatorService {
	return &ComparatorService{repo: repo, cfg: &cfg.Tourism}
}

func (s *ComparatorService) CompareAqueducts(
	ctx context.Context,
	aqueductIDs []uuid.UUID,
) (*models.AqueductComparison, error) {

	var aqueducts []models.Aqueduct
	var err error
	if len(aqueductIDs) == 0 {
		aqueducts, err = s.repo.GetAllAqueducts(ctx)
	} else {
		allAqs, e := s.repo.GetAllAqueducts(ctx)
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
		td, e := s.buildAqueductTourismData(ctx, &aqueducts[i])
		if e != nil {
			continue
		}
		details = append(details, *td)
	}

	compResult := CompareAqueducts(s.cfg, details)

	result := &models.AqueductComparison{
		ID:                    uuid.New(),
		ComparisonName:        "多水道综合对比分析",
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

func (s *ComparatorService) buildAqueductTourismData(
	ctx context.Context,
	aq *models.Aqueduct,
) (*models.AqueductTourismData, error) {

	tData, err := s.repo.GetTourismDataByAqueduct(ctx, aq.ID)
	if err != nil || tData == nil {
		tData = BuildDefaultTourismData(aq)
	}

	segments, err := s.repo.GetAllSegmentsWithStatus(ctx, &aq.ID)
	if err != nil {
		return nil, err
	}

	safeCount, warnCount, dangerCount, criticalCount := 0, 0, 0, 0
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

	carryingCap := float64(tData.MaxDailyVisitors) / math.Max(1, s.cfg.CarryingCapacityFactor)

	tData.AqueductName = aq.Name
	tData.SafetyScore = Round3(math.Max(0, math.Min(1.0, safetyScore)))
	tData.RepairCostEstimate = Round2(repCost)
	tData.TourismCarryingCapacity = Round2(carryingCap)

	enriched := EnrichTourismDataWithHeritage(s.cfg, aq, tData)
	return enriched, nil
}

func (s *ComparatorService) GetAllTourismData(ctx context.Context) ([]models.AqueductTourismData, error) {
	aqs, err := s.repo.GetAllAqueducts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]models.AqueductTourismData, 0, len(aqs))
	for i := range aqs {
		td, e := s.buildAqueductTourismData(ctx, &aqs[i])
		if e == nil {
			result = append(result, *td)
		}
	}
	return result, nil
}

func (s *ComparatorService) GetTourismDataByAqueduct(ctx context.Context, aqueductID uuid.UUID) (*models.AqueductTourismData, error) {
	aq, err := s.repo.GetAqueductByID(ctx, aqueductID)
	if err != nil {
		return nil, err
	}
	return s.buildAqueductTourismData(ctx, aq)
}

func (s *ComparatorService) SaveComparison(ctx context.Context, comparison *models.AqueductComparison) error {
	return s.repo.InsertAqueductComparison(ctx, comparison)
}

func (s *ComparatorService) GetComparisons(ctx context.Context, limit int) ([]models.AqueductComparison, error) {
	return s.repo.GetAqueductComparisons(ctx, limit)
}

func (s *ComparatorService) RecommendTourismPlan(
	ctx context.Context,
	preferences map[string]interface{},
) ([]map[string]interface{}, error) {

	tourismDataList, err := s.GetAllTourismData(ctx)
	if err != nil {
		return nil, err
	}

	ranking := ComputePriorityRanking(s.cfg, tourismDataList)

	results := make([]map[string]interface{}, 0, len(tourismDataList))
	for _, td := range tourismDataList {
		score := 0.0
		if scores, ok := ranking["scores"].(map[string]float64); ok {
			score = scores[td.AqueductID.String()]
		}

		results = append(results, map[string]interface{}{
			"aqueduct_id":     td.AqueductID,
			"aqueduct_name":   td.AqueductName,
			"tourism_score":   Round3(score),
			"heritage_value":  Round3(td.HeritageValue),
			"annual_visitors": td.AnnualVisitors,
			"safety_score":    td.SafetyScore,
		})
	}

	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			si := results[i]["tourism_score"].(float64)
			sj := results[j]["tourism_score"].(float64)
			if si < sj {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

func (s *ComparatorService) BuildRadarDataForAqueducts(
	ctx context.Context,
	aqueductIDs []uuid.UUID,
) (map[string]interface{}, error) {
	comparison, err := s.CompareAqueducts(ctx, aqueductIDs)
	if err != nil {
		return nil, err
	}
	return comparison.RadarChartData, nil
}
