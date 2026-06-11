package durability_inverter

import (
	"context"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type InverterService struct {
	repo        *repository.Repository
	cfg         *config.InversionConfig
	useRemote   bool
	remoteClient *RemoteInversionClient
}

func NewInverterService(repo *repository.Repository, cfg *config.Config) *InverterService {
	return &InverterService{
		repo:      repo,
		cfg:       &cfg.Inversion,
		useRemote: false,
	}
}

func NewInverterServiceWithRemote(repo *repository.Repository, cfg *config.Config, remoteURL string) *InverterService {
	return &InverterService{
		repo:         repo,
		cfg:          &cfg.Inversion,
		useRemote:    true,
		remoteClient: NewRemoteInversionClient(remoteURL),
	}
}

func (s *InverterService) InvertConcreteProperties(
	ctx context.Context,
	segmentID uuid.UUID,
	observedWeathering float64,
	observedStrength float64,
	ageYears float64,
	observedPH float64,
) (*models.ConcreteInversionResult, error) {

	segment, err := s.repo.GetSegmentByIDWithStatus(ctx, segmentID)
	if err != nil {
		return nil, err
	}

	if s.useRemote && s.remoteClient != nil {
		return s.invertWithRemote(ctx, segmentID, segment.AqueductID,
			observedWeathering, observedStrength, ageYears, observedPH)
	}

	return s.invertLocal(ctx, segmentID, segment.AqueductID,
		observedWeathering, observedStrength, ageYears, observedPH)
}

func (s *InverterService) invertLocal(
	ctx context.Context,
	segmentID uuid.UUID,
	aqueductID uuid.UUID,
	observedWeathering float64,
	observedStrength float64,
	ageYears float64,
	observedPH float64,
) (*models.ConcreteInversionResult, error) {

	formulas, err := s.getAllFormulas(ctx)
	if err != nil || len(formulas) == 0 {
		formulas = BuildDefaultFormulas()
	}

	hypotheses := BuildHypotheses(formulas, s.cfg)
	result := SolveInversion(
		hypotheses, observedWeathering, observedStrength, observedPH, ageYears, s.cfg,
	)

	confMetrics := ComputeConfidence(
		result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, s.cfg,
	)

	bestHyp := &hypotheses[result.BestIdx]
	bestFormula := &bestHyp.Formula
	bestSim := result.SimDepths[result.BestIdx]

	return s.buildResult(segmentID, aqueductID, observedWeathering, observedStrength,
		observedPH, ageYears, bestFormula, &result, &confMetrics, bestSim, bestHyp), nil
}

func (s *InverterService) invertWithRemote(
	ctx context.Context,
	segmentID uuid.UUID,
	aqueductID uuid.UUID,
	observedWeathering float64,
	observedStrength float64,
	ageYears float64,
	observedPH float64,
) (*models.ConcreteInversionResult, error) {

	remoteResp, err := s.remoteClient.Invert(ctx,
		observedWeathering, observedStrength, observedPH, ageYears, segmentID.String())
	if err != nil {
		return s.invertLocal(ctx, segmentID, aqueductID,
			observedWeathering, observedStrength, ageYears, observedPH)
	}

	bestFormula := remoteResp.BestFormula
	result := remoteResp.Result
	confMetrics := remoteResp.Confidence

	formulas := BuildDefaultFormulas()
	hypotheses := BuildHypotheses(formulas, s.cfg)

	bestHyp := &FormulaHypothesis{Formula: *bestFormula}
	if result != nil && result.BestIdx >= 0 && result.BestIdx < len(hypotheses) {
		bestHyp = &hypotheses[result.BestIdx]
	}

	bestSim := 0.0
	if result != nil && len(result.SimDepths) > result.BestIdx {
		bestSim = result.SimDepths[result.BestIdx]
	}

	return s.buildResult(segmentID, aqueductID, observedWeathering, observedStrength,
		observedPH, ageYears, bestFormula, result, confMetrics, bestSim, bestHyp), nil
}

func (s *InverterService) buildResult(
	segmentID uuid.UUID,
	aqueductID uuid.UUID,
	observedWeathering float64,
	observedStrength float64,
	observedPH float64,
	ageYears float64,
	bestFormula *models.RomanConcreteFormula,
	result *InversionResult,
	confMetrics *ConfidenceMetrics,
	bestSim float64,
	bestHyp *FormulaHypothesis,
) *models.ConcreteInversionResult {

	leachingRate := 0.005
	carbonationDepth := 2.0
	if bestHyp != nil {
		leachingRate = bestHyp.LeachingK
		carbonationDepth = 2.0 * bestHyp.CarbonationK * Sqrt(ageYears)
	}

	pozzolanicAge := 200.0
	selfHealingPotential := 0.7
	poreRefinement := 0.75
	if bestHyp != nil {
		pozzolanicAge = EstimatePozzolanicReactionAge(*bestHyp)
		selfHealingPotential = EstimateSelfHealingPotential(*bestHyp)
		poreRefinement = 1.0 - bestHyp.PoreConnect
	}

	confidence := 0.8
	bayesianPosterior := 0.75
	regularizationEffect := 0.15
	signalNoiseRatio := 10.0
	candidates := make([]models.InversionFormulaCandidate, 0)
	if confMetrics != nil {
		confidence = confMetrics.Confidence
		bayesianPosterior = confMetrics.BayesianPosterior
		regularizationEffect = confMetrics.RegularizationEffect
	}
	if result != nil {
		signalNoiseRatio = result.SignalNoiseRatio
		candidates = result.Candidates
	}

	durabilityMechanism := map[string]interface{}{
		"pozzolanic_reaction_age_years":  pozzolanicAge,
		"calcium_leaching_dominant":      leachingRate > 0.01,
		"carbonation_contribution_pct":   carbonationDepth / Max(0.1, bestSim) * 100,
		"pore_refinement_index":          poreRefinement,
		"self_healing_potential":         selfHealingPotential,
		"modern_reference_notes":         GenerateModernReference(bestFormula),
		"bayesian_posterior_probability": bayesianPosterior,
		"l2_regularization_effect":      regularizationEffect,
		"signal_noise_ratio":             signalNoiseRatio,
		"outlier_rejection_applied":      s.cfg.OutlierRejectionThreshold > 0,
		"noise_robustness_enabled":       s.cfg.NoiseRobustWeight > 0,
		"remote_service_used":            s.useRemote,
	}

	modernRef := map[string]interface{}{
		"modern_opc_comparison": map[string]float64{
			"durability_ratio_roman_opc": bestFormula.DurabilityIndex / 0.7,
			"strength_ratio_roman_opc":   bestFormula.OriginalFyMPa / 35.0,
			"carbon_footprint_pct_opc":   0.35,
		},
		"recommendation": "现代混凝土可参考火山灰掺量比例，降低水化热并提高长龄期耐久性",
	}

	candidatesJSON := map[string]interface{}{
		"count":      len(candidates),
		"candidates": candidates,
	}

	topCandidate := models.InversionFormulaCandidate{}
	if len(candidates) > 0 {
		topCandidate = candidates[0]
	}

	analysisResult := &models.ConcreteInversionResult{
		ID:                       uuid.New(),
		AqueductID:               aqueductID,
		SegmentID:                &segmentID,
		AnalysisTime:             time.Now().UTC(),
		ObservedWeatheringDepth:  observedWeathering,
		ObservedStrength:         observedStrength,
		ObservedMortarPH:         observedPH,
		AgeYears:                 ageYears,
		BestMatchFormulaID:       &bestFormula.ID,
		CandidateFormulas:        candidatesJSON,
		InversionConfidence:      confidence,
		InferredOriginalFy:       bestFormula.OriginalFyMPa,
		InferredDurabilityMechanism: durabilityMechanism,
		LeachingRate:             leachingRate,
		CarbonationDepth:         carbonationDepth,
		ModernReferenceFormula:   modernRef,
		Notes:                    GenerateInterpretationNotes(topCandidate, confidence, bestFormula),
		CreatedAt:                time.Now().UTC(),
		BestMatchFormula:         bestFormula,
	}

	return analysisResult
}

func (s *InverterService) getAllFormulas(ctx context.Context) ([]models.RomanConcreteFormula, error) {
	return s.repo.GetAllConcreteFormulas(ctx)
}

func (s *InverterService) SaveResult(ctx context.Context, result *models.ConcreteInversionResult) error {
	return s.repo.InsertConcreteInversionResult(ctx, result)
}

func (s *InverterService) GetResultsByAqueduct(ctx context.Context, aqueductID uuid.UUID, limit int) ([]models.ConcreteInversionResult, error) {
	return s.repo.GetInversionResultsByAqueduct(ctx, aqueductID, limit)
}

func (s *InverterService) GetAllFormulas(ctx context.Context) ([]models.RomanConcreteFormula, error) {
	list, err := s.repo.GetAllConcreteFormulas(ctx)
	if err != nil || len(list) == 0 {
		return BuildDefaultFormulas(), nil
	}
	return list, nil
}
