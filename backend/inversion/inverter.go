package inversion

import (
	"context"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/durability_inverter"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type ConcreteInverter struct {
	repo *repository.Repository
	cfg  *config.InversionConfig
}

func NewConcreteInverter(repo *repository.Repository, cfg *config.Config) *ConcreteInverter {
	return &ConcreteInverter{repo: repo, cfg: &cfg.Inversion}
}

type formulaHypothesis struct {
	formula      models.RomanConcreteFormula
	limePozzRatio float64
	waterBinder  float64
	leachingK    float64
	carbonationK float64
	poreConnect  float64
}

func (inv *ConcreteInverter) InvertConcreteProperties(
	ctx context.Context,
	segmentID uuid.UUID,
	observedWeathering float64,
	observedStrength float64,
	ageYears float64,
	observedPH float64,
) (*models.ConcreteInversionResult, error) {

	segment, err := inv.repo.GetSegmentByIDWithStatus(ctx, segmentID)
	if err != nil {
		return nil, err
	}

	formulas, err := inv.getAllFormulas(ctx)
	if err != nil || len(formulas) == 0 {
		formulas = durability_inverter.BuildDefaultFormulas()
	}

	hypotheses := durability_inverter.BuildHypotheses(formulas, inv.cfg)
	result := durability_inverter.SolveInversion(
		hypotheses, observedWeathering, observedStrength, observedPH, ageYears, inv.cfg,
	)

	confMetrics := durability_inverter.ComputeConfidence(
		result.Candidates, result.Residuals, result.BestIdx, result.RawResiduals, inv.cfg,
	)

	bestHyp := &hypotheses[result.BestIdx]
	bestFormula := &bestHyp.Formula
	bestSim := result.SimDepths[result.BestIdx]

	leachingRate := bestHyp.LeachingK
	carbonationDepth := 2.0 * bestHyp.CarbonationK * durability_inverter.Sqrt(ageYears)

	durabilityMechanism := map[string]interface{}{
		"pozzolanic_reaction_age_years":  durability_inverter.EstimatePozzolanicReactionAge(*bestHyp),
		"calcium_leaching_dominant":      leachingRate > 0.01,
		"carbonation_contribution_pct":   carbonationDepth / durability_inverter.Max(0.1, bestSim) * 100,
		"pore_refinement_index":          1.0 - bestHyp.PoreConnect,
		"self_healing_potential":         durability_inverter.EstimateSelfHealingPotential(*bestHyp),
		"modern_reference_notes":         durability_inverter.GenerateModernReference(bestFormula),
		"bayesian_posterior_probability": confMetrics.BayesianPosterior,
		"l2_regularization_effect":      confMetrics.RegularizationEffect,
		"signal_noise_ratio":             result.SignalNoiseRatio,
		"outlier_rejection_applied":      inv.cfg.OutlierRejectionThreshold > 0,
		"noise_robustness_enabled":       inv.cfg.NoiseRobustWeight > 0,
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
		"count":      len(result.Candidates),
		"candidates": result.Candidates,
	}

	analysisResult := &models.ConcreteInversionResult{
		ID:                       uuid.New(),
		AqueductID:               segment.AqueductID,
		SegmentID:                &segmentID,
		AnalysisTime:             time.Now().UTC(),
		ObservedWeatheringDepth:  observedWeathering,
		ObservedStrength:         observedStrength,
		ObservedMortarPH:         observedPH,
		AgeYears:                 ageYears,
		BestMatchFormulaID:       &bestFormula.ID,
		CandidateFormulas:        candidatesJSON,
		InversionConfidence:      confMetrics.Confidence,
		InferredOriginalFy:       bestFormula.OriginalFyMPa,
		InferredDurabilityMechanism: durabilityMechanism,
		LeachingRate:             leachingRate,
		CarbonationDepth:         carbonationDepth,
		ModernReferenceFormula:   modernRef,
		Notes:                    durability_inverter.GenerateInterpretationNotes(result.Candidates[0], confMetrics.Confidence, bestFormula),
		CreatedAt:                time.Now().UTC(),
		BestMatchFormula:         bestFormula,
	}

	return analysisResult, nil
}

func (inv *ConcreteInverter) simulateWeatheringDepth(h formulaHypothesis, ageYears float64) float64 {
	dh := durability_inverter.FormulaHypothesis{
		Formula:       h.formula,
		LimePozzRatio: h.limePozzRatio,
		WaterBinder:   h.waterBinder,
		LeachingK:     h.leachingK,
		CarbonationK:  h.carbonationK,
		PoreConnect:   h.poreConnect,
	}
	return durability_inverter.SimulateWeatheringDepth(dh, ageYears, inv.cfg)
}

func (inv *ConcreteInverter) simulateStrengthRetention(h formulaHypothesis, ageYears float64) float64 {
	dh := durability_inverter.FormulaHypothesis{
		Formula:       h.formula,
		LimePozzRatio: h.limePozzRatio,
		WaterBinder:   h.waterBinder,
		LeachingK:     h.leachingK,
		CarbonationK:  h.carbonationK,
		PoreConnect:   h.poreConnect,
	}
	return durability_inverter.SimulateStrengthRetention(dh, ageYears, inv.cfg)
}

func (inv *ConcreteInverter) simulatePH(h formulaHypothesis, ageYears float64) float64 {
	dh := durability_inverter.FormulaHypothesis{
		Formula:       h.formula,
		LimePozzRatio: h.limePozzRatio,
		WaterBinder:   h.waterBinder,
		LeachingK:     h.leachingK,
		CarbonationK:  h.carbonationK,
		PoreConnect:   h.poreConnect,
	}
	return durability_inverter.SimulatePH(dh, ageYears, inv.cfg)
}

func (inv *ConcreteInverter) computeConfidence(candidates []models.InversionFormulaCandidate, residuals []float64, bestIdx int, rawResiduals []float64) (float64, float64, float64) {
	cm := durability_inverter.ComputeConfidence(candidates, residuals, bestIdx, rawResiduals, inv.cfg)
	return cm.Confidence, cm.BayesianPosterior, cm.RegularizationEffect
}

func (inv *ConcreteInverter) getAllFormulas(ctx context.Context) ([]models.RomanConcreteFormula, error) {
	return inv.repo.GetAllConcreteFormulas(ctx)
}

func buildDefaultFormulas() []models.RomanConcreteFormula {
	return durability_inverter.BuildDefaultFormulas()
}

func newFormula(name string, lime, pozz, agg, water, fy, fm, em, por, dur float64) models.RomanConcreteFormula {
	return durability_inverter.NewFormula(name, lime, pozz, agg, water, fy, fm, em, por, dur)
}

func estimatePozzolanicReactionAge(h formulaHypothesis) float64 {
	dh := durability_inverter.FormulaHypothesis{
		Formula:       h.formula,
		LimePozzRatio: h.limePozzRatio,
		WaterBinder:   h.waterBinder,
		LeachingK:     h.leachingK,
		CarbonationK:  h.carbonationK,
		PoreConnect:   h.poreConnect,
	}
	return durability_inverter.EstimatePozzolanicReactionAge(dh)
}

func estimateSelfHealingPotential(h formulaHypothesis) float64 {
	dh := durability_inverter.FormulaHypothesis{
		Formula:       h.formula,
		LimePozzRatio: h.limePozzRatio,
		WaterBinder:   h.waterBinder,
		LeachingK:     h.leachingK,
		CarbonationK:  h.carbonationK,
		PoreConnect:   h.poreConnect,
	}
	return durability_inverter.EstimateSelfHealingPotential(dh)
}

func generateModernReference(f *models.RomanConcreteFormula) string {
	return durability_inverter.GenerateModernReference(f)
}

func generateInterpretationNotes(best models.InversionFormulaCandidate, confidence float64, f *models.RomanConcreteFormula) string {
	return durability_inverter.GenerateInterpretationNotes(best, confidence, f)
}

func randNormalApprox(mu, sigma float64, seed int) float64 {
	return durability_inverter.RandNormalApprox(mu, sigma, seed)
}
