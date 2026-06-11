package durability_inverter

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

type FormulaHypothesis struct {
	Formula        models.RomanConcreteFormula
	LimePozzRatio  float64
	WaterBinder    float64
	LeachingK      float64
	CarbonationK   float64
	PoreConnect    float64
}

func BuildHypotheses(formulas []models.RomanConcreteFormula, cfg *config.InversionConfig) []FormulaHypothesis {
	hypotheses := make([]FormulaHypothesis, len(formulas))
	for i, f := range formulas {
		lpRatio := 0.0
		if f.PozzolanaRatio > 0 {
			lpRatio = f.LimeRatio / f.PozzolanaRatio
		}
		hypotheses[i] = FormulaHypothesis{
			Formula:       f,
			LimePozzRatio: lpRatio,
			WaterBinder:   f.WaterRatio / (f.LimeRatio + f.PozzolanaRatio),
			LeachingK:     cfg.LeachingRateBase * (1.0 + 0.4*lpRatio),
			CarbonationK:  cfg.CarbonationRateBase * (0.6 + 0.8*f.Porosity),
			PoreConnect:   f.Porosity * (0.5 + 0.5*(1.0-f.DurabilityIndex)),
		}
	}
	return hypotheses
}

func SimulateWeatheringDepth(h FormulaHypothesis, ageYears float64, cfg *config.InversionConfig) float64 {
	t := math.Max(1.0, ageYears)
	carb := 2.0 * h.CarbonationK * math.Sqrt(t)
	leach := h.LeachingK * math.Pow(t, 0.75) * (1.0 + 0.15*h.PoreConnect)
	dissol := 0.004 * math.Pow(t, 0.88) * (1.0 - h.Formula.DurabilityIndex)
	return carb + leach + dissol
}

func SimulateStrengthRetention(h FormulaHypothesis, ageYears float64, cfg *config.InversionConfig) float64 {
	t := math.Max(1.0, ageYears)
	gainPhase := 1.0 + 0.35*(1.0-math.Exp(-t/150.0))
	lossPhase := 1.0 - 0.12*math.Pow(t/2000.0, cfg.StrengthRetainPower)
	pozzBoost := 1.0 + 0.22*h.LimePozzRatio/(1.0+h.LimePozzRatio)
	total := gainPhase * pozzBoost * lossPhase
	return math.Max(0.1, math.Min(2.5, total))
}

func SimulatePH(h FormulaHypothesis, ageYears float64, cfg *config.InversionConfig) float64 {
	t := math.Max(1.0, ageYears)
	phInitial := cfg.PHInitialRoman
	phFinal := cfg.PHModern
	carbRate := 0.6 / math.Sqrt(t+25.0) * (1.0 + 0.3*h.WaterBinder)
	ph := phFinal + (phInitial-phFinal)*math.Exp(-carbRate*t)
	return math.Max(7.5, math.Min(13.0, ph))
}

type InversionResult struct {
	Candidates       []models.InversionFormulaCandidate
	Residuals        []float64
	RawResiduals     []float64
	SimDepths        []float64
	SimStrengths     []float64
	SimPHs           []float64
	BestIdx          int
	NoiseStd         float64
	SignalNoiseRatio float64
}

func SolveInversion(
	hypotheses []FormulaHypothesis,
	observedWeathering float64,
	observedStrength float64,
	observedPH float64,
	ageYears float64,
	cfg *config.InversionConfig,
) *InversionResult {
	simDepths := make([]float64, len(hypotheses))
	simStrengths := make([]float64, len(hypotheses))
	simPHs := make([]float64, len(hypotheses))
	residuals := make([]float64, len(hypotheses))
	rawResiduals := make([]float64, len(hypotheses))

	noiseStd := math.Max(0.1, observedWeathering*0.08)
	priorMeans := map[string]float64{
		"lime_ratio":      1.0,
		"pozzolana_ratio": 1.0,
		"water_binder":    0.85,
	}

	for i, h := range hypotheses {
		d := SimulateWeatheringDepth(h, ageYears, cfg)
		s := SimulateStrengthRetention(h, ageYears, cfg)
		simDepths[i] = d
		simStrengths[i] = h.Formula.OriginalFyMPa * s
		simPHs[i] = SimulatePH(h, ageYears, cfg)

		noiseScale := 1.0 / (1.0 + cfg.NoiseRobustWeight*math.Exp(-math.Abs(d-observedWeathering)/noiseStd))

		wDepth := 1.0 * noiseScale
		wStrength := 1.5 * noiseScale
		wPH := 0.8 * noiseScale

		rDepth := math.Pow((d-observedWeathering)/math.Max(1.0, observedWeathering), 2)
		rStrength := math.Pow((simStrengths[i]-observedStrength)/math.Max(1.0, observedStrength), 2)
		simPH := simPHs[i]
		rPH := math.Pow((simPH-observedPH)/2.0, 2)

		dataResidual := wDepth*rDepth + wStrength*rStrength + wPH*rPH

		l2Reg := 0.0
		if cfg.L2RegularizationLambda > 0 {
			regLime := cfg.L2RegularizationLambda * math.Pow(h.Formula.LimeRatio-priorMeans["lime_ratio"], 2)
			regPozz := cfg.L2RegularizationLambda * math.Pow(h.Formula.PozzolanaRatio-priorMeans["pozzolana_ratio"], 2)
			regWB := cfg.L2RegularizationLambda * 0.5 * math.Pow(h.WaterBinder-priorMeans["water_binder"], 2)
			l2Reg = regLime + regPozz + regWB
		}

		rawResiduals[i] = math.Sqrt(dataResidual)
		residuals[i] = math.Sqrt(dataResidual + l2Reg)

		if cfg.OutlierRejectionThreshold > 0 && residuals[i] > cfg.OutlierRejectionThreshold*math.Sqrt(rDepth+rStrength+rPH) {
			residuals[i] = cfg.OutlierRejectionThreshold * math.Sqrt(rDepth+rStrength+rPH)
		}
	}

	candidates := make([]models.InversionFormulaCandidate, len(hypotheses))
	for i := range hypotheses {
		matchScore := 1.0 / (1.0 + residuals[i])
		candidates[i] = models.InversionFormulaCandidate{
			FormulaID:       hypotheses[i].Formula.ID,
			FormulaName:     hypotheses[i].Formula.FormulaName,
			LimeRatio:       hypotheses[i].Formula.LimeRatio,
			PozzolanaRatio:  hypotheses[i].Formula.PozzolanaRatio,
			MatchScore:      matchScore,
			Ranking:         0,
			SimulatedDepth:  simDepths[i],
			ResidualError:   residuals[i],
		}
	}
	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].MatchScore > candidates[b].MatchScore
	})
	for i := range candidates {
		candidates[i].Ranking = i + 1
	}

	bestIdx := 0
	for i := range hypotheses {
		if candidates[0].FormulaID == hypotheses[i].Formula.ID {
			bestIdx = i
			break
		}
	}

	signalNoiseRatio := math.Max(0.1, math.Abs(observedWeathering)/noiseStd)

	return &InversionResult{
		Candidates:       candidates,
		Residuals:        residuals,
		RawResiduals:     rawResiduals,
		SimDepths:        simDepths,
		SimStrengths:     simStrengths,
		SimPHs:           simPHs,
		BestIdx:          bestIdx,
		NoiseStd:         noiseStd,
		SignalNoiseRatio: signalNoiseRatio,
	}
}

type ConfidenceMetrics struct {
	Confidence         float64
	BayesianPosterior  float64
	RegularizationEffect float64
}

func ComputeConfidence(
	candidates []models.InversionFormulaCandidate,
	residuals []float64,
	bestIdx int,
	rawResiduals []float64,
	cfg *config.InversionConfig,
) ConfidenceMetrics {
	if len(candidates) < 2 {
		return ConfidenceMetrics{0.7, 0.7, 0.0}
	}
	scoreGap := candidates[0].MatchScore - candidates[1].MatchScore
	bestResidual := residuals[bestIdx]
	bestRawResidual := rawResiduals[bestIdx]
	normResidual := 1.0 / (1.0 + bestResidual)

	bestLikelihood := math.Exp(-0.5 * bestRawResidual * bestRawResidual)
	totalLikelihood := 0.0
	for _, r := range rawResiduals {
		totalLikelihood += math.Exp(-0.5 * r * r)
	}
	bayesianPosterior := bestLikelihood / math.Max(1e-10, totalLikelihood)

	regularizationEffect := 0.0
	if cfg.L2RegularizationLambda > 0 {
		regularizationEffect = 1.0 - math.Min(1.0, (residuals[bestIdx]-rawResiduals[bestIdx])/math.Max(0.01, residuals[bestIdx]))
	}

	monteCarloBonus := 0.0
	if cfg.MonteCarloSamples > 0 {
		mu := bestResidual
		sigma := math.Max(0.01, bestResidual*0.15)
		stabilityCount := 0
		for k := 0; k < 500; k++ {
			simR := math.Abs(RandNormalApprox(mu, sigma, k))
			simScore := 1.0 / (1.0 + simR)
			if simScore >= candidates[0].MatchScore*0.9 {
				stabilityCount++
			}
		}
		monteCarloBonus = float64(stabilityCount) / 500.0 * 0.12
	}

	priorStrength := cfg.BayesianPriorStrength
	conf := (0.25*normResidual + 0.25*math.Min(1.0, scoreGap*2.5) + 0.30*bayesianPosterior + 0.15*regularizationEffect) + monteCarloBonus
	conf = (conf*priorStrength + 0.7*(1.0-priorStrength))
	return ConfidenceMetrics{
		Confidence:         math.Max(0.25, math.Min(0.98, conf)),
		BayesianPosterior:  bayesianPosterior,
		RegularizationEffect: regularizationEffect,
	}
}

func BuildDefaultFormulas() []models.RomanConcreteFormula {
	return []models.RomanConcreteFormula{
		NewFormula("标准罗马混凝土配方", 1.0, 1.2, 3.5, 0.85, 8.5, 1.8, 25.0, 0.28, 0.85),
		NewFormula("高强度火山灰砂浆 (Puteolanus)", 0.85, 1.6, 3.2, 0.78, 10.5, 2.2, 28.0, 0.24, 0.90),
		NewFormula("水下耐水配方 (Signinum)", 1.1, 1.0, 4.0, 0.9, 7.5, 1.5, 22.0, 0.30, 0.88),
		NewFormula("拱券专用砂浆", 1.2, 0.8, 3.8, 0.88, 7.0, 1.4, 20.0, 0.32, 0.80),
		NewFormula("石灰华骨料结构混凝土", 0.95, 1.1, 4.2, 0.82, 9.0, 2.0, 26.0, 0.26, 0.86),
	}
}

func NewFormula(name string, lime, pozz, agg, water, fy, fm, em, por, dur float64) models.RomanConcreteFormula {
	return models.RomanConcreteFormula{
		ID:                   uuid.New(),
		FormulaName:          name,
		LimeRatio:            lime,
		PozzolanaRatio:       pozz,
		AggregateRatio:       agg,
		WaterRatio:           water,
		AggregateType:        "石灰华/火山砾",
		AdditiveType:         "无",
		OriginalFyMPa:        fy,
		OriginalFmMPa:        fm,
		OriginalEmGPa:        em,
		Porosity:             por,
		DurabilityIndex:      dur,
		EraDescription:       "罗马帝国时期 (公元前1世纪 - 公元3世纪)",
		ArchaeologicalSources: "参考庞贝、Ostia、Rome Forum遗址考古发现",
		CreatedAt:            time.Now().UTC(),
	}
}

func EstimatePozzolanicReactionAge(h FormulaHypothesis) float64 {
	return 50.0 + 120.0*h.LimePozzRatio/(1.0+h.LimePozzRatio)
}

func EstimateSelfHealingPotential(h FormulaHypothesis) float64 {
	freeLime := math.Max(0, h.Formula.LimeRatio-0.7*h.Formula.PozzolanaRatio)
	caSource := freeLime / (h.Formula.LimeRatio + h.Formula.PozzolanaRatio)
	return math.Min(0.95, 0.2+0.6*caSource+0.3*h.Formula.DurabilityIndex)
}

func GenerateModernReference(f *models.RomanConcreteFormula) string {
	return ""
}

func GenerateInterpretationNotes(best models.InversionFormulaCandidate, confidence float64, f *models.RomanConcreteFormula) string {
	return ""
}

func RandNormalApprox(mu, sigma float64, seed int) float64 {
	x := float64(seed) / 500.0
	u1 := 0.001 + 0.998*(0.5+0.5*math.Sin(x*12.9898+78.233)*43758.5453)
	u2 := 0.001 + 0.998*(0.5+0.5*math.Sin(x*78.233+12.9898)*43758.5453)
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	return mu + sigma*z
}

func Sqrt(v float64) float64  { return math.Sqrt(v) }
func Max(a, b float64) float64 { return math.Max(a, b) }
func Min(a, b float64) float64 { return math.Min(a, b) }
func Abs(v float64) float64    { return math.Abs(v) }
