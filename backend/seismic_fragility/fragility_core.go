package seismic_fragility

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

type FragilityParams struct {
	Med   float64
	Beta  float64
	Alpha float64
}

type SiteAmplificationResult struct {
	MeanAmp   float64
	StdAmp    float64
	BestClass string
}

type RiskInterval struct {
	Low       float64
	High      float64
	Mean      float64
	Uncertainty float64
}

type SeismicAnalysisResult struct {
	OverallRiskLevel      string
	RiskMetrics           float64
	VulnerableSegCount    int
	TotalExpectedLoss     float64
	PGA475                float64
	PGA2475               float64
	PredominantPeriod     float64
	SiteClass             string
	SoilAmplification     float64
	SoilAmpStd            float64
	LiquefactionPotential float64
	RiskInterval          RiskInterval
	SiteClassProbabilities map[string]float64
}

func LognormalCDF(x, med, beta float64) float64 {
	if x <= 0 {
		return 0.0
	}
	z := (math.Log(x/med) + 0.5*beta*beta) / beta
	return 0.5 * math.Erfc(-z / math.Sqrt2)
}

func ComputeAttenuation(distKm, mag float64) float64 {
	return 1.0 / (1.0 + 0.018*distKm + 0.00006*distKm*distKm) * math.Pow(10.0, 0.3*(mag-5.5))
}

func EstimateSiteClass(lat, lng float64) string {
	v := math.Abs(math.Sin(lat*2.7) * math.Cos(lng*1.9))
	switch {
	case v < 0.25:
		return "A"
	case v < 0.5:
		return "B"
	case v < 0.75:
		return "C"
	default:
		return "D"
	}
}

func BayesianSiteClassProbabilities(lat, lng float64, cfg *config.SeismicConfig) map[string]float64 {
	probs := make(map[string]float64)
	v := math.Abs(math.Sin(lat*2.7) * math.Cos(lng*1.9))

	classes := []string{"A", "B", "C", "D", "E"}
	likelihoods := make(map[string]float64)
	for _, c := range classes {
		center := 0.1
		switch c {
		case "A":
			center = 0.125
		case "B":
			center = 0.375
		case "C":
			center = 0.625
		case "D":
			center = 0.875
		case "E":
			center = 0.95
		}
		unc := cfg.SiteClassUncertainty
		likelihoods[c] = math.Exp(-0.5 * math.Pow((v-center)/math.Max(0.01, unc), 2))
	}

	totalProb := 0.0
	for _, c := range classes {
		prior := 0.1
		if p, ok := cfg.SoilTypePrior[c]; ok {
			prior = p
		}
		probs[c] = likelihoods[c] * prior
		totalProb += probs[c]
	}

	for _, c := range classes {
		probs[c] = probs[c] / math.Max(1e-10, totalProb)
	}

	return probs
}

func SoilAmplificationFactor(siteClass string, pga float64) float64 {
	ratio := 1.0
	switch siteClass {
	case "A":
		ratio = 0.9
	case "B":
		ratio = 1.0
	case "C":
		ratio = 1.25 - 0.3*pga
	case "D":
		ratio = 1.6 - 0.6*pga
	case "E":
		ratio = 2.0 - 1.0*pga
	}
	return math.Max(0.8, ratio)
}

func ExpectedSoilAmplificationWithUncertainty(
	siteProbs map[string]float64,
	pga float64,
	cfg *config.SeismicConfig,
) SiteAmplificationResult {
	meanAmp := 0.0
	meanSq := 0.0
	bestClass := "C"
	bestProb := 0.0
	for class, prob := range siteProbs {
		amp := SoilAmplificationFactor(class, pga)
		meanAmp += prob * amp
		meanSq += prob * amp * amp
		if prob > bestProb {
			bestProb = prob
			bestClass = class
		}
	}
	variance := meanSq - meanAmp*meanAmp
	std := math.Sqrt(math.Max(0, variance))
	uncertaintyFactor := 1.0 + cfg.SoilAmpUncertainty
	return SiteAmplificationResult{
		MeanAmp:   meanAmp * uncertaintyFactor,
		StdAmp:    std,
		BestClass: bestClass,
	}
}

func SegmentFragilityParams(seg *models.StructureSegment, cfg *config.SeismicConfig) FragilityParams {
	return SegmentFragilityParamsWithUncertainty(seg, 0.5, cfg)
}

func SegmentFragilityParamsWithUncertainty(seg *models.StructureSegment, uncertaintyLevel float64, cfg *config.SeismicConfig) FragilityParams {
	baseMed := 0.35
	if seg.SegmentType == "pier" {
		baseMed = 0.42
	} else if seg.SegmentType == "arch" {
		baseMed = 0.28
	}
	capFactor := 1.0
	if seg.CapacityRatio > 0 {
		capFactor = 0.4 + 0.8*seg.CapacityRatio
	}
	settleFactor := 1.0 - 0.015*seg.SettlementMM
	if settleFactor < 0.4 {
		settleFactor = 0.4
	}
	med := baseMed * capFactor * settleFactor * cfg.CapacityReductionFactor
	beta := 0.45 - 0.15*(med/baseMed) + 0.08*(1.0-cfg.DuctilityFactor/5.0)

	betaUnc := cfg.BetaUncertaintyMin + uncertaintyLevel*(cfg.BetaUncertaintyMax-cfg.BetaUncertaintyMin)
	beta += betaUnc * (med / (med + 0.5))

	beta = math.Max(0.25, math.Min(0.85, beta))
	return FragilityParams{Med: med, Beta: beta, Alpha: betaUnc}
}

func CheckLiquefactionPotential(pga float64, siteClass string, cfg *config.SeismicConfig) float64 {
	if !cfg.UseLiquefactionCheck {
		return 0.0
	}
	siteFactor := 0.0
	switch siteClass {
	case "D":
		siteFactor = 1.0
	case "E":
		siteFactor = 1.5
	case "C":
		siteFactor = 0.4
	default:
		return 0.0
	}
	potential := math.Max(0, (pga-0.15)/0.6) * siteFactor
	return math.Max(0, math.Min(1.0, potential))
}

func PGAToMagnitudeApprox(pga, distKm float64) float64 {
	if pga <= 0 {
		return 3.0
	}
	return 5.5 + math.Log10(pga*(1.0+0.018*distKm))/0.3
}

func EvaluateSegmentRisk(seg *models.StructureSegment, pga, period, soilAmp float64, cfg *config.SeismicConfig) float64 {
	specAccel := pga * cfg.DuctilityFactor * soilAmp
	params := SegmentFragilityParams(seg, cfg)
	extProb := LognormalCDF(specAccel, params.Med*2.0, params.Beta*1.1)
	modProb := LognormalCDF(specAccel, params.Med*1.4, params.Beta*1.05)
	risk := 0.10*LognormalCDF(specAccel, params.Med, params.Beta) +
		0.25*modProb +
		0.45*extProb +
		0.20*LognormalCDF(specAccel, params.Med*3.2, params.Beta*1.15)
	return math.Max(0.0, math.Min(1.0, risk))
}

func EstimateRepairCost(seg *models.StructureSegment, state string, prob float64) float64 {
	if prob < 0.01 {
		return 0
	}
	baseUnitCost := 1200.0
	var multiplier float64
	switch state {
	case "Slight":
		multiplier = 0.05
	case "Moderate":
		multiplier = 0.18
	case "Extensive":
		multiplier = 0.55
	case "Complete":
		multiplier = 1.2
	default:
		multiplier = 0.2
	}
	vol := 15.0
	if seg.SegmentType == "arch" {
		vol = 25.0
	}
	return baseUnitCost * vol * multiplier * prob
}

func EstimateDowntime(state string, prob float64) int {
	if prob < 0.01 {
		return 0
	}
	daysMap := map[string]float64{
		"Slight":    10.0,
		"Moderate":  45.0,
		"Extensive": 180.0,
		"Complete":  540.0,
	}
	d, ok := daysMap[state]
	if !ok {
		d = 60
	}
	return int(d * math.Min(1.0, prob*1.5))
}

func CapacitySpectrumData(seg *models.StructureSegment, state string, cfg *config.SeismicConfig) map[string]interface{} {
	threshold, ok := cfg.DamageThresholds[state]
	if !ok {
		threshold = 0.2
	}
	saPoints := make([][2]float64, 0, 8)
	for i := 0; i <= 7; i++ {
		d := 0.01 + 0.02*float64(i)
		sa := threshold * (1.0 - math.Exp(-d*15.0))
		saPoints = append(saPoints, [2]float64{Round4(d), Round3(sa)})
	}
	return map[string]interface{}{
		"yield_disp_m":  threshold * 0.6,
		"ultimate_sa_g": threshold,
		"curve_points": saPoints,
		"ductility":    cfg.DuctilityFactor,
	}
}

func DemandSpectrumData(pga float64) map[string]interface{} {
	points := make([][2]float64, 0, 12)
	for i := 0; i < 12; i++ {
		t := 0.05 + 0.15*float64(i)
		var sa float64
		if t < 0.15 {
			sa = pga * (0.6 + 2.67*t/0.15)
		} else if t < 0.5 {
			sa = pga * 3.27
		} else {
			sa = pga * 3.27 * 0.5 / t
		}
		points = append(points, [2]float64{Round2(t), Round3(sa)})
	}
	return map[string]interface{}{
		"pga_g":         pga,
		"corner_period": 0.5,
		"curve_points":  points,
	}
}

func RegionName(lat, lng float64) string {
	regions := []string{"Latium", "Tuscany", "Campania", "Umbria", "Abruzzo", "Lazio Coast"}
	idx := int(math.Abs(lat*13.0 + lng*7.0)) % len(regions)
	return regions[idx]
}

func AnalyzeSegmentBatch(
	segments []models.StructureSegment,
	pga, period, soilAmp float64,
	cfg *config.SeismicConfig,
) (totalRiskLow, totalRiskHigh, totalRisk float64, vulnerableCount int, totalLoss float64) {
	soilAmpStd := cfg.SoilAmpUncertainty * soilAmp
	for i := range segments {
		risk := EvaluateSegmentRisk(&segments[i], pga, period, soilAmp, cfg)
		riskLow := EvaluateSegmentRisk(&segments[i], pga, period, math.Max(0.8, soilAmp-soilAmpStd), cfg)
		riskHigh := EvaluateSegmentRisk(&segments[i], pga, period, soilAmp+soilAmpStd, cfg)

		totalRiskLow += riskLow
		totalRiskHigh += riskHigh
		totalRisk += risk

		if risk >= 0.35 {
			vulnerableCount++
		}
		totalLoss += segments[i].DesignLoadCapacity * 120.0 * risk
	}
	return
}

func GenerateFragilityCurve(
	seg *models.StructureSegment,
	cfg *config.SeismicConfig,
) []models.SeismicFragilityPoint {
	params := SegmentFragilityParams(seg, cfg)

	nLevels := cfg.PGAComputedLevels
	points := make([]models.SeismicFragilityPoint, nLevels)
	pgaMin := 0.01
	pgaMax := 1.5

	for i := 0; i < nLevels; i++ {
		pga := pgaMin + (pgaMax-pgaMin)*math.Pow(float64(i)/float64(nLevels-1), 1.3)
		mag := PGAToMagnitudeApprox(pga, 30.0)
		points[i] = models.SeismicFragilityPoint{
			PGA:       Round3(pga),
			Magnitude: Round1(mag),
			Slight:    Round3(LognormalCDF(pga, params.Med*1.0, params.Beta)),
			Moderate:  Round3(LognormalCDF(pga, params.Med*1.4, params.Beta*1.05)),
			Extensive: Round3(LognormalCDF(pga, params.Med*2.0, params.Beta*1.1)),
			Complete:  Round3(LognormalCDF(pga, params.Med*3.2, params.Beta*1.15)),
		}
	}

	return points
}

type IDAResult struct {
	Results []models.SeismicVulnerability
	Err     error
}

func ComputeIDAAsync(
	seg *models.StructureSegment,
	cfg *config.SeismicConfig,
	resultChan chan<- IDAResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	result, err := ComputeIDA(seg, cfg)
	resultChan <- IDAResult{Results: result, Err: err}
}

func ComputeIDA(
	seg *models.StructureSegment,
	cfg *config.SeismicConfig,
) ([]models.SeismicVulnerability, error) {
	params := SegmentFragilityParams(seg, cfg)
	states := cfg.DamageStates
	pgaList := []float64{0.05, 0.1, 0.15, 0.2, 0.3, 0.4, 0.6, 0.8, 1.0, 1.2}

	results := make([]models.SeismicVulnerability, 0, len(states)*len(pgaList))
	for si, state := range states {
		magFactor := 1.0 + 0.45*float64(si)
		for _, pga := range pgaList {
			prob := LognormalCDF(pga, params.Med*magFactor, params.Beta*(1.0+0.08*float64(si)))
			repCost := EstimateRepairCost(seg, state, prob)
			downtime := EstimateDowntime(state, prob)

			results = append(results, models.SeismicVulnerability{
				ID:                   uuid.New(),
				AqueductID:           seg.AqueductID,
				SegmentID:            &seg.ID,
				AnalysisTime:         time.Now().UTC(),
				DamageState:          state,
				Magnitude:            Round1(PGAToMagnitudeApprox(pga, 30)),
				PGAG:                 Round3(pga),
				Probability:          Round3(prob),
				FragilityCurveParams: map[string]interface{}{"med_g": params.Med, "beta": params.Beta, "mag_factor": magFactor},
				CapacitySpectrum:     CapacitySpectrumData(seg, state, cfg),
				DemandSpectrum:       DemandSpectrumData(pga),
				ExpectedRepairCost:   Round2(repCost),
				ExpectedDowntimeDays: downtime,
				CreatedAt:            time.Now().UTC(),
			})
		}
	}
	return results, nil
}

func ComputeIDAParallel(
	segments []models.StructureSegment,
	cfg *config.SeismicConfig,
) ([]models.SeismicVulnerability, error) {
	resultChan := make(chan IDAResult, len(segments))
	var wg sync.WaitGroup

	for i := range segments {
		wg.Add(1)
		go ComputeIDAAsync(&segments[i], cfg, resultChan, &wg)
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

func BuildDefaultHistoricalEarthquakes() []models.HistoricalEarthquake {
	list := []struct {
		name, date string
		mag        float64
		lat, lng   float64
		depth      float64
		intensity  float64
		region     string
		damage     string
	}{
		{"公元512年大地震", "512-06-08", 6.9, 42.1, 12.8, 12, 9, "Latium", "Claudia水道部分拱券坍塌"},
		{"公元801年地震", "801-04-29", 6.5, 41.9, 12.5, 8, 8, "Roma", "Anio Novus水道桥墩倾斜"},
		{"1349年地震群", "1349-09-09", 6.7, 41.7, 13.0, 10, 9, "Latium Southeast", "多处拱券结构严重损毁"},
		{"1695年地震", "1695-01-14", 6.2, 42.4, 13.2, 15, 8, "Abruzzo边界", "Marta水道风化加剧"},
		{"1915年 Avezzano地震", "1915-01-13", 7.0, 42.0, 13.4, 15, 10, "Fucino盆地", "多条水道沉降异常"},
		{"2016年 Amatrice地震", "2016-08-24", 6.0, 42.7, 13.2, 8, 7, "Central Apennines", "结构监测数据异常"},
	}
	result := make([]models.HistoricalEarthquake, len(list))
	for i, e := range list {
		result[i] = models.HistoricalEarthquake{
			ID:                   uuid.New(),
			EventName:            e.name,
			EventDate:            e.date,
			Magnitude:            e.mag,
			EpicenterLat:         e.lat,
			EpicenterLng:         e.lng,
			DepthKm:              e.depth,
			IntensityMSK:         e.intensity,
			Region:               e.region,
			HistoricalSources:    "Roman Historical Records / INGV Catalog",
			DamageDescription:    e.damage,
			AffectedAqueducts:    map[string]interface{}{"count": 3 + i%4},
			CreatedAt:            time.Now().UTC(),
		}
	}
	sort.Slice(result, func(a, b int) bool { return result[a].Magnitude > result[b].Magnitude })
	return result
}

func AssessSeismicRisk(
	segments []models.StructureSegment,
	lat, lng float64,
	aqueductLengthKM float64,
	cfg *config.SeismicConfig,
) *SeismicAnalysisResult {
	distanceToEpicentralZone := 80.0 + 40.0*math.Sin(lat/255.0*6.28)

	pga475 := cfg.ReturnPeriod475PGA * ComputeAttenuation(distanceToEpicentralZone, 5.5)
	pga2475 := cfg.ReturnPeriod2475PGA * ComputeAttenuation(distanceToEpicentralZone, 7.0)

	siteProbs := BayesianSiteClassProbabilities(lat, lng, cfg)
	ampResult := ExpectedSoilAmplificationWithUncertainty(siteProbs, pga475, cfg)
	predominantPeriod := 0.6 + 0.002*aqueductLengthKM*1000
	liquefactionPotential := CheckLiquefactionPotential(pga475, ampResult.BestClass, cfg)

	totalRiskLow, totalRiskHigh, totalRisk, vulnerableCount, totalLoss := AnalyzeSegmentBatch(
		segments, pga475, predominantPeriod, ampResult.MeanAmp, cfg,
	)

	n := float64(len(segments))
	avgRiskLow := totalRiskLow / math.Max(1.0, n)
	avgRiskHigh := totalRiskHigh / math.Max(1.0, n)
	avgRisk := totalRisk / math.Max(1.0, n)
	riskUncertainty := avgRiskHigh - avgRiskLow

	riskMetrics := 0.35*(1.0-float64(vulnerableCount)/math.Max(1.0, n)) +
		0.25*math.Min(1.0, pga475*1.5) +
		0.25*ampResult.MeanAmp/2.0 +
		0.15*(1.0-math.Min(1.0, totalLoss/2e7))

	if liquefactionPotential > 0.3 {
		riskMetrics = riskMetrics + liquefactionPotential*0.15
	}

	overallRiskLevel := "LOW"
	switch {
	case riskMetrics >= 0.70:
		overallRiskLevel = "VERY_HIGH"
	case riskMetrics >= 0.50:
		overallRiskLevel = "HIGH"
	case riskMetrics >= 0.30:
		overallRiskLevel = "MODERATE"
	case riskMetrics >= 0.15:
		overallRiskLevel = "LOW"
	default:
		overallRiskLevel = "VERY_LOW"
	}

	return &SeismicAnalysisResult{
		OverallRiskLevel:      overallRiskLevel,
		RiskMetrics:           riskMetrics,
		VulnerableSegCount:    vulnerableCount,
		TotalExpectedLoss:     totalLoss,
		PGA475:                pga475,
		PGA2475:               pga2475,
		PredominantPeriod:     predominantPeriod,
		SiteClass:             ampResult.BestClass,
		SoilAmplification:     ampResult.MeanAmp,
		SoilAmpStd:            ampResult.StdAmp,
		LiquefactionPotential: liquefactionPotential,
		RiskInterval: RiskInterval{
			Low:         avgRiskLow,
			High:        avgRiskHigh,
			Mean:        avgRisk,
			Uncertainty: riskUncertainty,
		},
		SiteClassProbabilities: siteProbs,
	}
}

func Round1(v float64) float64 { return math.Round(v*10) / 10 }
func Round2(v float64) float64 { return math.Round(v*100) / 100 }
func Round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func Round4(v float64) float64 { return math.Round(v*10000) / 10000 }
