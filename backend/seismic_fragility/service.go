package seismic_fragility

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type FragilityService struct {
	repo *repository.Repository
	cfg  *config.SeismicConfig
}

func NewFragilityService(repo *repository.Repository, cfg *config.Config) *FragilityService {
	return &FragilityService{repo: repo, cfg: &cfg.Seismic}
}

func (s *FragilityService) AnalyzeAqueductSeismicRisk(
	ctx context.Context,
	aqueductID uuid.UUID,
) (*models.AqueductSeismicRisk, error) {

	aq, err := s.repo.GetAqueductByID(ctx, aqueductID)
	if err != nil {
		return nil, err
	}

	segments, err := s.repo.GetAllSegmentsWithStatus(ctx, &aqueductID)
	if err != nil {
		return nil, err
	}

	lat := 41.9028
	lng := 12.4964
	if aq.GeoPath != nil {
		if arr, ok := aq.GeoPath["segments"].([]interface{}); ok && len(arr) > 0 {
			lat = 41.9 + float64(int(aqueductID.ID()[0]))/500.0
			lng = 12.5 + float64(int(aqueductID.ID()[1]))/500.0
		}
	}

	result := AssessSeismicRisk(segments, lat, lng, aq.LengthKM, s.cfg)

	siteClassProbMap := make(map[string]interface{})
	for c, p := range result.SiteClassProbabilities {
		siteClassProbMap[c] = Round3(p)
	}

	uncertaintyMap := map[string]interface{}{
		"site_class_probabilities":   siteClassProbMap,
		"soil_amplification_mean":    Round3(result.SoilAmplification),
		"soil_amplification_std":     Round3(result.SoilAmpStd),
		"risk_interval_low":          Round3(result.RiskInterval.Low),
		"risk_interval_high":         Round3(result.RiskInterval.High),
		"risk_uncertainty_band":      Round3(result.RiskInterval.Uncertainty),
		"liquefaction_potential":     Round3(result.LiquefactionPotential),
		"beta_uncertainty_range":     []float64{s.cfg.BetaUncertaintyMin, s.cfg.BetaUncertaintyMax},
		"bayesian_site_estimation":   true,
	}

	analysisResult := &models.AqueductSeismicRisk{
		ID:                    uuid.New(),
		AqueductID:            aqueductID,
		Region:                RegionName(lat, lng),
		PeakGroundAccel475Yr:  Round3(result.PGA475),
		PeakGroundAccel2475Yr: Round3(result.PGA2475),
		OverallRiskLevel:      result.OverallRiskLevel,
		SiteClass:             result.SiteClass,
		SoilAmplification:     Round3(result.SoilAmplification),
		PredominantPeriodSec:  Round3(result.PredominantPeriod),
		VulnerableSegments:    result.VulnerableSegCount,
		EstimatedTotalLoss:    Round2(result.TotalExpectedLoss),
		AnalysisTime:          time.Now().UTC(),
		CreatedAt:             time.Now().UTC(),
		AqueductName:          aq.Name,
		AqueductLat:           lat,
		AqueductLng:           lng,
	}

	if analysisResult.AdditionalInfo == nil {
		analysisResult.AdditionalInfo = make(map[string]interface{})
	}
	analysisResult.AdditionalInfo["uncertainty_analysis"] = uncertaintyMap

	return analysisResult, nil
}

func (s *FragilityService) GenerateFragilityCurves(
	ctx context.Context,
	segmentID uuid.UUID,
) ([]models.SeismicFragilityPoint, error) {

	seg, err := s.repo.GetSegmentByIDWithStatus(ctx, segmentID)
	if err != nil {
		return nil, err
	}

	return GenerateFragilityCurve(seg, s.cfg), nil
}

func (s *FragilityService) AnalyzeIncrementalDynamic(ctx context.Context, segmentID uuid.UUID) ([]models.SeismicVulnerability, error) {
	seg, err := s.repo.GetSegmentByIDWithStatus(ctx, segmentID)
	if err != nil {
		return nil, err
	}

	return ComputeIDA(seg, s.cfg)
}

func (s *FragilityService) AnalyzeIncrementalDynamicAsync(
	ctx context.Context,
	segmentID uuid.UUID,
	resultChan chan<- IDAResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	results, err := s.AnalyzeIncrementalDynamic(ctx, segmentID)
	resultChan <- IDAResult{Results: results, Err: err}
}

func (s *FragilityService) AnalyzeBatchIDA(ctx context.Context, segmentIDs []uuid.UUID) ([]models.SeismicVulnerability, error) {
	resultChan := make(chan IDAResult, len(segmentIDs))
	var wg sync.WaitGroup

	for _, segID := range segmentIDs {
		wg.Add(1)
		go func(id uuid.UUID) {
			results, err := s.AnalyzeIncrementalDynamic(ctx, id)
			resultChan <- IDAResult{Results: results, Err: err}
			wg.Done()
		}(segID)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var allResults []models.SeismicVulnerability
	for res := range resultChan {
		if res.Err != nil {
			return nil, res.Err
		}
		allResults = append(allResults, res.Results...)
	}

	return allResults, nil
}

func (s *FragilityService) GetAllHistoricalEarthquakes(ctx context.Context) ([]models.HistoricalEarthquake, error) {
	list, err := s.repo.GetAllHistoricalEarthquakes(ctx)
	if err != nil || len(list) == 0 {
		return BuildDefaultHistoricalEarthquakes(), nil
	}
	return list, nil
}

func (s *FragilityService) SaveRiskResult(ctx context.Context, risk *models.AqueductSeismicRisk) error {
	return s.repo.InsertSeismicRiskResult(ctx, risk)
}

func (s *FragilityService) GetAllSeismicRisks(ctx context.Context) ([]models.AqueductSeismicRisk, error) {
	list, err := s.repo.GetAllSeismicRisks(ctx)
	if err != nil || len(list) == 0 {
		return s.generateAllSeismicRisks(ctx)
	}
	return list, nil
}

func (s *FragilityService) generateAllSeismicRisks(ctx context.Context) ([]models.AqueductSeismicRisk, error) {
	aqs, err := s.repo.GetAllAqueducts(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]models.AqueductSeismicRisk, 0, len(aqs))
	for i := range aqs {
		r, e := s.AnalyzeAqueductSeismicRisk(ctx, aqs[i].ID)
		if e == nil {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (s *FragilityService) SaveVulnerability(ctx context.Context, v *models.SeismicVulnerability) error {
	return s.repo.InsertSeismicVulnerability(ctx, v)
}
