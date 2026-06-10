package recommendation

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type RepairRecommender struct {
	repo *repository.Repository
}

type MADMCriteria struct {
	Name       string  `json:"name"`
	Weight     float64 `json:"weight"`
	IsBenefit  bool    `json:"is_benefit"`
	MissingRate float64 `json:"missing_rate,omitempty"`
}

type SensitivityResult struct {
	RankStability     float64            `json:"rank_stability"`
	AttributeSens     map[string]float64 `json:"attribute_sensitivity"`
	TopRankChangeRate float64            `json:"top_rank_change_rate"`
	CriticalDecision  bool               `json:"critical_decision"`
	BaseRanking       []string           `json:"base_ranking"`
	ConfidenceScore   float64            `json:"confidence_score"`
}

const (
	KNN_NEIGHBORS = 3
	WEIGHT_MISSING_PENALTY = 0.7
	SENSITIVITY_PERTURB = 0.20
	MISSING_THRESHOLD_HIGH = 0.4
)

type ScenarioContext struct {
	DamageType          string
	DamageSeverity      float64
	StructuralElement   string
	EnvironmentMoist    bool
	LoadBearingCritical bool
	AestheticPriority   bool
	HeritageCompliance  bool
	UrgencyLevel        string
}

const (
	WEIGHT_COMPRESSIVE     = 0.12
	WEIGHT_TENSILE         = 0.10
	WEIGHT_ELASTIC_MODULUS = 0.06
	WEIGHT_DURABILITY      = 0.15
	WEIGHT_COMPATIBILITY   = 0.18
	WEIGHT_COST            = 0.10
	WEIGHT_EASE_OF_USE     = 0.08
	WEIGHT_ENVIRONMENTAL   = 0.07
	WEIGHT_AESTHETIC       = 0.14
)

func NewRepairRecommender(repo *repository.Repository) *RepairRecommender {
	return &RepairRecommender{repo: repo}
}

func (r *RepairRecommender) RecommendForSegment(ctx context.Context, segment *models.StructureSegment) (*models.RepairRecommendation, error) {
	scenario := r.analyzeDamageScenario(segment)

	materials, err := r.repo.GetAllRepairMaterials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load materials: %w", err)
	}

	missingReport := r.imputeMissingData(materials)

	weights := r.getAdjustedWeights(scenario)

	r.applyMissingWeightPenalty(weights, missingReport)

	scoredMaterials := r.runTOPSIS(materials, weights, scenario)

	sensitivity := r.performSensitivityAnalysis(materials, weights, scenario)

	top3 := scoredMaterials
	if len(top3) > 3 {
		top3 = top3[:3]
	}

	damageSeverity := 0.0
	switch segment.SafetyLevel {
	case "CRITICAL":
		damageSeverity = 0.95
	case "DANGER":
		damageSeverity = 0.75
	case "WARNING":
		damageSeverity = 0.50
	default:
		damageSeverity = 0.25
	}

	expectedCost := r.calculateExpectedCost(segment, top3)

	expectedLifespan := r.calculateExpectedLifespan(scoredMaterials[0], damageSeverity)

	constructionNotes := r.generateConstructionNotes(segment, scenario, top3)

	return &models.RepairRecommendation{
		AqueductID:          segment.AqueductID,
		SegmentID:           segment.ID,
		RecommendationTime:  time.Now().UTC(),
		DamageType:          scenario.DamageType,
		DamageSeverity:      damageSeverity,
		RecommendedMaterials: top3,
		DecisionScores: map[string]interface{}{
			"method":               "TOPSIS+KNN_Imputation+Sensitivity",
			"scenario_weights":     weights,
			"damage_analysis":      scenario,
			"weighted_normalized":  r.extractWeightedScores(top3),
			"data_imputation":      missingReport,
			"sensitivity_analysis": sensitivity,
		},
		ExpectedCost:        expectedCost,
		ExpectedLifespan:    expectedLifespan,
		ConstructionNotes:   constructionNotes,
	}, nil
}

func (r *RepairRecommender) analyzeDamageScenario(segment *models.StructureSegment) *ScenarioContext {
	sc := &ScenarioContext{
		StructuralElement:  segment.SegmentType,
		HeritageCompliance: true,
		EnvironmentMoist:   false,
	}

	capacityRatio := segment.CapacityRatio
	if capacityRatio == 0 {
		capacityRatio = 0.85
	}
	weatheringDepth := segment.WeatheringDepth
	settlement := segment.SettlementMM

	var damageTypes []string
	severity := 0.0

	if weatheringDepth >= 20 {
		damageTypes = append(damageTypes, "severe_mortar_weathering")
		severity += 0.35
		sc.AestheticPriority = true
	} else if weatheringDepth >= 10 {
		damageTypes = append(damageTypes, "moderate_weathering")
		severity += 0.20
		sc.AestheticPriority = true
	} else if weatheringDepth >= 3 {
		damageTypes = append(damageTypes, "minor_surface_erosion")
		severity += 0.10
	}

	if capacityRatio < 0.50 {
		damageTypes = append(damageTypes, "severe_structural_degradation")
		severity += 0.40
		sc.LoadBearingCritical = true
	} else if capacityRatio < 0.70 {
		damageTypes = append(damageTypes, "load_capacity_reduction")
		severity += 0.25
		sc.LoadBearingCritical = true
	} else if capacityRatio < 0.85 {
		damageTypes = append(damageTypes, "mild_strength_loss")
		severity += 0.10
	}

	if settlement >= 20 {
		damageTypes = append(damageTypes, "severe_foundation_settlement")
		severity += 0.30
		sc.LoadBearingCritical = true
	} else if settlement >= 10 {
		damageTypes = append(damageTypes, "moderate_settlement")
		severity += 0.15
	} else if settlement >= 5 {
		damageTypes = append(damageTypes, "minor_settlement")
		severity += 0.05
	}

	if segment.CurrentStress > segment.DesignStrength * 0.5 {
		damageTypes = append(damageTypes, "high_stress_state")
		severity += 0.20
	}

	if len(damageTypes) == 0 {
		damageTypes = append(damageTypes, "routine_maintenance")
		severity = 0.05
	}
	if severity > 1.0 {
		severity = 1.0
	}

	switch {
	case severity >= 0.8:
		sc.UrgencyLevel = "CRITICAL"
	case severity >= 0.6:
		sc.UrgencyLevel = "URGENT"
	case severity >= 0.3:
		sc.UrgencyLevel = "SCHEDULED"
	default:
		sc.UrgencyLevel = "PREVENTIVE"
	}

	sc.DamageType = damageTypes[0]
	if len(damageTypes) > 1 {
		for _, d := range damageTypes[1:] {
			sc.DamageType += "+" + d
		}
	}
	sc.DamageSeverity = severity

	return sc
}

func (r *RepairRecommender) getAdjustedWeights(sc *ScenarioContext) []MADMCriteria {
	baseWeights := []MADMCriteria{
		{"compressive_strength", WEIGHT_COMPRESSIVE, true},
		{"tensile_strength", WEIGHT_TENSILE, true},
		{"elastic_modulus", WEIGHT_ELASTIC_MODULUS, true},
		{"durability_rating", WEIGHT_DURABILITY, true},
		{"compatibility_rating", WEIGHT_COMPATIBILITY, true},
		{"cost_per_unit", WEIGHT_COST, false},
		{"ease_of_application", WEIGHT_EASE_OF_USE, true},
		{"environmental_impact", WEIGHT_ENVIRONMENTAL, false},
		{"aesthetic_match", WEIGHT_AESTHETIC, true},
	}

	adjustFactor := func(idx int, factor float64) {
		baseWeights[idx].Weight *= factor
	}

	if sc.LoadBearingCritical {
		adjustFactor(0, 1.8)
		adjustFactor(1, 1.6)
		adjustFactor(3, 1.3)
	}

	if sc.HeritageCompliance {
		adjustFactor(4, 1.7)
		adjustFactor(8, 1.5)
		adjustFactor(7, 1.4)
	}

	if sc.AestheticPriority {
		adjustFactor(8, 1.6)
	}

	if sc.UrgencyLevel == "CRITICAL" || sc.UrgencyLevel == "URGENT" {
		adjustFactor(6, 1.5)
		adjustFactor(0, 1.2)
	}

	if sc.EnvironmentMoist {
		adjustFactor(3, 1.5)
	}

	totalWeight := 0.0
	for i := range baseWeights {
		totalWeight += baseWeights[i].Weight
	}
	for i := range baseWeights {
		baseWeights[i].Weight /= totalWeight
	}

	return baseWeights
}

func (r *RepairRecommender) runTOPSIS(
	materials []models.RepairMaterial,
	criteria []MADMCriteria,
	sc *ScenarioContext,
) []models.RepairMaterial {

	n := len(materials)
	m := len(criteria)

	if n == 0 {
		return materials
	}

	getValue := func(mat *models.RepairMaterial, idx int) float64 {
		switch criteria[idx].Name {
		case "compressive_strength":
			return mat.CompressiveStrength
		case "tensile_strength":
			return mat.TensileStrength
		case "elastic_modulus":
			return mat.ElasticModulus
		case "durability_rating":
			return mat.DurabilityRating
		case "compatibility_rating":
			return mat.CompatibilityRating
		case "cost_per_unit":
			return mat.CostPerUnit
		case "ease_of_application":
			return mat.EaseOfApplication
		case "environmental_impact":
			return mat.EnvironmentalImpact
		case "aesthetic_match":
			return mat.AestheticMatch
		}
		return 0
	}

	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			matrix[i][j] = getValue(&materials[i], j)
		}
	}

	colNorms := make([]float64, m)
	for j := 0; j < m; j++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += matrix[i][j] * matrix[i][j]
		}
		colNorms[j] = math.Sqrt(sum)
	}

	normMatrix := make([][]float64, n)
	for i := range normMatrix {
		normMatrix[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			if colNorms[j] > 0 {
				normMatrix[i][j] = matrix[i][j] / colNorms[j]
			}
		}
	}

	weightedNorm := make([][]float64, n)
	for i := range weightedNorm {
		weightedNorm[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			weightedNorm[i][j] = normMatrix[i][j] * criteria[j].Weight
		}
	}

	idealBest := make([]float64, m)
	idealWorst := make([]float64, m)
	for j := 0; j < m; j++ {
		best := weightedNorm[0][j]
		worst := weightedNorm[0][j]
		for i := 1; i < n; i++ {
			if criteria[j].IsBenefit {
				if weightedNorm[i][j] > best {
					best = weightedNorm[i][j]
				}
				if weightedNorm[i][j] < worst {
					worst = weightedNorm[i][j]
				}
			} else {
				if weightedNorm[i][j] < best {
					best = weightedNorm[i][j]
				}
				if weightedNorm[i][j] > worst {
					worst = weightedNorm[i][j]
				}
			}
		}
		idealBest[j] = best
		idealWorst[j] = worst
	}

	distanceBest := make([]float64, n)
	distanceWorst := make([]float64, n)
	performance := make([]float64, n)

	for i := 0; i < n; i++ {
		sumB := 0.0
		sumW := 0.0
		for j := 0; j < m; j++ {
			sumB += math.Pow(weightedNorm[i][j]-idealBest[j], 2)
			sumW += math.Pow(weightedNorm[i][j]-idealWorst[j], 2)
		}
		distanceBest[i] = math.Sqrt(sumB)
		distanceWorst[i] = math.Sqrt(sumW)

		denom := distanceWorst[i] + distanceBest[i]
		if denom > 0 {
			performance[i] = distanceWorst[i] / denom
		}

		if sc.HeritageCompliance {
			if materials[i].MaterialType == "ROMAN_CONCRETE" {
				performance[i] *= 1.12
			} else if materials[i].MaterialType == "LIME_MORTAR" {
				performance[i] *= 1.08
			}
			if materials[i].MaterialType == "FRP" {
				performance[i] *= 0.92
			}
		}
		if sc.UrgencyLevel == "CRITICAL" {
			if materials[i].MaterialType == "MODERN_CEMENT" || materials[i].MaterialType == "EPOXY" {
				performance[i] *= 1.1
			}
		}
		if performance[i] > 1.0 {
			performance[i] = 0.999
		}
	}

	for i := range materials {
		materials[i].DecisionScore = performance[i]
		materials[i].WeightedScores = make(map[string]float64)
		for j, c := range criteria {
			materials[i].WeightedScores[c.Name] = weightedNorm[i][j]
		}
		materials[i].WeightedScores["distance_best"] = distanceBest[i]
		materials[i].WeightedScores["distance_worst"] = distanceWorst[i]
	}

	sort.Slice(materials, func(i, j int) bool {
		return materials[i].DecisionScore > materials[j].DecisionScore
	})

	log.Printf("TOPSIS recommendation: top=%s score=%.4f", materials[0].Name, materials[0].DecisionScore)
	return materials
}

func (r *RepairRecommender) calculateExpectedCost(segment *models.StructureSegment, materials []models.RepairMaterial) float64 {
	if len(materials) == 0 {
		return 0
	}

	var volumeM3 float64
	var areaM2 float64

	switch segment.SegmentType {
	case "pier":
		pierArea := 1.2 * 2.5
		affectedSection := segment.WeatheringDepth / 1000.0
		if affectedSection > 0.2 {
			affectedSection = 0.2
		}
		if affectedSection < 0.02 {
			affectedSection = 0.02
		}
		volumeM3 = pierArea * affectedSection * 1.15
		areaM2 = 2 * (1.2 + 2.5) * 12 * 0.4
	case "arch":
		affectedSection := segment.WeatheringDepth / 1000.0
		if affectedSection > 0.15 {
			affectedSection = 0.15
		}
		if affectedSection < 0.015 {
			affectedSection = 0.015
		}
		volumeM3 = 5.5 * 0.8 * 3.0 * affectedSection * 1.3
		areaM2 = 5.5 * 3.0 * 1.1
	default:
		volumeM3 = 0.2
		areaM2 = 2.0
	}

	capacityRatio := segment.CapacityRatio
	if capacityRatio == 0 {
		capacityRatio = 0.9
	}
	if capacityRatio < 0.5 {
		volumeM3 *= 1.8
		areaM2 *= 1.5
	} else if capacityRatio < 0.7 {
		volumeM3 *= 1.4
	}

	totalCost := 0.0
	for i, mat := range materials {
		weightFactor := []float64{0.60, 0.30, 0.10}
		var unitCost float64
		switch mat.Unit {
		case "m²", "m2":
			unitCost = mat.CostPerUnit * areaM2
		case "kg":
			unitCost = mat.CostPerUnit * volumeM3 * 1800.0
		default:
			unitCost = mat.CostPerUnit * volumeM3
		}
		if i < len(weightFactor) {
			totalCost += unitCost * weightFactor[i]
		} else {
			totalCost += unitCost * 0.05
		}
	}

	overhead := 1.45
	return totalCost * overhead
}

func (r *RepairRecommender) calculateExpectedLifespan(bestMaterial models.RepairMaterial, severity float64) int {
	baseLife := 100

	durabilityFactor := bestMaterial.DurabilityRating / 10.0
	compatFactor := bestMaterial.CompatibilityRating / 10.0
	materialFactor := 0.5 + 0.5*(durabilityFactor*0.6+compatFactor*0.4)

	severityPenalty := 1.0 - severity*0.55

	resultYears := float64(baseLife) * materialFactor * severityPenalty

	switch bestMaterial.MaterialType {
	case "ROMAN_CONCRETE":
		resultYears *= 1.35
	case "LIME_MORTAR":
		resultYears *= 1.20
	case "FRP":
		resultYears *= 0.75
	case "EPOXY":
		resultYears *= 0.80
	}

	if resultYears < 5 {
		resultYears = 5
	}
	if resultYears > 200 {
		resultYears = 200
	}

	return int(math.Round(resultYears))
}

func (r *RepairRecommender) generateConstructionNotes(segment *models.StructureSegment, sc *ScenarioContext, materials []models.RepairMaterial) string {
	notes := ""

	notes += fmt.Sprintf("【施工阶段】修复紧急度：%s\n", sc.UrgencyLevel)

	if len(materials) > 0 {
		notes += fmt.Sprintf("【推荐主材料】%s (TOPSIS得分: %.3f)\n",
			materials[0].Name, materials[0].DecisionScore)
	}
	if len(materials) > 1 {
		notes += fmt.Sprintf("【备选材料1】%s (%.3f)；【备选材料2】%s (%.3f)\n",
			materials[1].Name, materials[1].DecisionScore,
			materials[2].Name, materials[2].DecisionScore)
	}

	if segment.SegmentType == "arch" {
		notes += "【拱券修复要点】施工前设置可调节临时支撑，对称卸力后进行补强，分阶段落架。\n"
	} else {
		notes += "【桥墩修复要点】基础处理优先，结构补强遵循先下后上原则。\n"
	}

	if sc.HeritageCompliance {
		notes += "【文物保护】遵循《威尼斯宪章》最小干预原则，修复材料可逆可识别。\n"
	}

	switch sc.UrgencyLevel {
	case "CRITICAL":
		notes += "【应急措施】立即搭设临时防护棚，安装实时位移监测，24小时值守。\n"
	case "URGENT":
		notes += "【实施建议】30日内完成现场勘察和施工图，60日内开工。\n"
	default:
		notes += "【实施建议】纳入年度维护计划，结合季节性条件合理安排施工。\n"
	}

	if segment.CapacityRatio > 0 && segment.CapacityRatio < 0.5 {
		notes += "【加固要求】承载力提升至设计值75%以上，需进行第三方荷载试验验收。\n"
	}

	return notes
}

func (r *RepairRecommender) extractWeightedScores(materials []models.RepairMaterial) map[string]map[string]float64 {
	result := make(map[string]map[string]float64)
	for _, m := range materials {
		result[m.Name] = m.WeightedScores
	}
	return result
}

func (r *RepairRecommender) getAttrValue(mat *models.RepairMaterial, name string) float64 {
	switch name {
	case "compressive_strength":
		return mat.CompressiveStrength
	case "tensile_strength":
		return mat.TensileStrength
	case "elastic_modulus":
		return mat.ElasticModulus
	case "durability_rating":
		return mat.DurabilityRating
	case "compatibility_rating":
		return mat.CompatibilityRating
	case "cost_per_unit":
		return mat.CostPerUnit
	case "ease_of_application":
		return mat.EaseOfApplication
	case "environmental_impact":
		return mat.EnvironmentalImpact
	case "aesthetic_match":
		return mat.AestheticMatch
	}
	return 0
}

func (r *RepairRecommender) setAttrValue(mat *models.RepairMaterial, name string, val float64) {
	switch name {
	case "compressive_strength":
		mat.CompressiveStrength = val
	case "tensile_strength":
		mat.TensileStrength = val
	case "elastic_modulus":
		mat.ElasticModulus = val
	case "durability_rating":
		mat.DurabilityRating = val
	case "compatibility_rating":
		mat.CompatibilityRating = val
	case "cost_per_unit":
		mat.CostPerUnit = val
	case "ease_of_application":
		mat.EaseOfApplication = val
	case "environmental_impact":
		mat.EnvironmentalImpact = val
	case "aesthetic_match":
		mat.AestheticMatch = val
	}
}

func (r *RepairRecommender) imputeMissingData(materials []models.RepairMaterial) map[string]interface{} {
	attrNames := []string{
		"compressive_strength", "tensile_strength", "elastic_modulus",
		"durability_rating", "compatibility_rating", "cost_per_unit",
		"ease_of_application", "environmental_impact", "aesthetic_match",
	}

	report := map[string]interface{}{
		"total_materials":   len(materials),
		"attribute_missing": map[string]float64{},
		"imputation_method": map[string]string{},
		"total_imputed":     0,
	}

	missingMatrix := make([][]bool, len(materials))
	for i := range missingMatrix {
		missingMatrix[i] = make([]bool, len(attrNames))
	}

	attrMissingCount := make([]int, len(attrNames))
	for j, name := range attrNames {
		count := 0
		for i := range materials {
			v := r.getAttrValue(&materials[i], name)
			if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				missingMatrix[i][j] = true
				count++
				attrMissingCount[j]++
			}
		}
		missingRate := float64(count) / float64(len(materials))
		report["attribute_missing"].(map[string]float64)[name] = missingRate
		if count > 0 {
			if missingRate > MISSING_THRESHOLD_HIGH {
				report["imputation_method"].(map[string]string)[name] = "global_mean"
			} else {
				report["imputation_method"].(map[string]string)[name] = "knn_k3_weighted"
			}
		}
	}

	for j, name := range attrNames {
		if attrMissingCount[j] == 0 {
			continue
		}

		validVals := make([]float64, 0, len(materials))
		validIdx := make([]int, 0, len(materials))
		for i := range materials {
			if !missingMatrix[i][j] {
				validVals = append(validVals, r.getAttrValue(&materials[i], name))
				validIdx = append(validIdx, i)
			}
		}

		if len(validVals) == 0 {
			continue
		}

		globalMean := 0.0
		for _, v := range validVals {
			globalMean += v
		}
		globalMean /= float64(len(validVals))

		missingRate := float64(attrMissingCount[j]) / float64(len(materials))

		for i := range materials {
			if !missingMatrix[i][j] {
				continue
			}

			imputedVal := 0.0

			if missingRate <= MISSING_THRESHOLD_HIGH && len(validIdx) >= KNN_NEIGHBORS {
				type neighbor struct {
					idx   int
					dist  float64
				}
				neighbors := make([]neighbor, 0, len(validIdx))

				for _, vi := range validIdx {
					dist := 0.0
					commonAttrs := 0
					for k, n2 := range attrNames {
						if k == j {
							continue
						}
						v1 := r.getAttrValue(&materials[i], n2)
						v2 := r.getAttrValue(&materials[vi], n2)
						if (v1 <= 0 || math.IsNaN(v1)) && (v2 <= 0 || math.IsNaN(v2)) {
							continue
						}
						if v1 <= 0 || math.IsNaN(v1) || v2 <= 0 || math.IsNaN(v2) {
							dist += 1.0
							commonAttrs++
							continue
						}
						colMax := math.Abs(v2)
						if colMax < 1e-9 {
							colMax = 1.0
						}
						diff := (v1 - v2) / colMax
						dist += diff * diff
						commonAttrs++
					}
					if commonAttrs > 0 {
						dist = math.Sqrt(dist / float64(commonAttrs))
					}
					neighbors = append(neighbors, neighbor{idx: vi, dist: dist})
				}

				sort.Slice(neighbors, func(a, b int) bool {
					return neighbors[a].dist < neighbors[b].dist
				})

				k := KNN_NEIGHBORS
				if len(neighbors) < k {
					k = len(neighbors)
				}

				weightSum := 0.0
				weightedVal := 0.0
				for n := 0; n < k; n++ {
					nb := neighbors[n]
					w := 1.0 / (nb.dist + 1e-6)
					v := r.getAttrValue(&materials[nb.idx], name)
					weightedVal += w * v
					weightSum += w
				}
				if weightSum > 0 {
					imputedVal = weightedVal / weightSum
				} else {
					imputedVal = globalMean
				}
			} else {
				imputedVal = globalMean
			}

			if imputedVal <= 0 || math.IsNaN(imputedVal) || math.IsInf(imputedVal, 0) {
				imputedVal = math.Max(globalMean, 0.01)
			}

			r.setAttrValue(&materials[i], name, imputedVal)
			report["total_imputed"] = report["total_imputed"].(int) + 1
		}
	}

	log.Printf("Data imputation: filled %d missing values across %d materials",
		report["total_imputed"].(int), len(materials))
	return report
}

func (r *RepairRecommender) applyMissingWeightPenalty(weights []MADMCriteria, report map[string]interface{}) {
	attrMissing, ok := report["attribute_missing"].(map[string]float64)
	if !ok {
		return
	}
	for i := range weights {
		mr, exists := attrMissing[weights[i].Name]
		if exists {
			weights[i].MissingRate = mr
			if mr > 0 {
				penalty := math.Pow(WEIGHT_MISSING_PENALTY, mr*2.5)
				weights[i].Weight *= penalty
			}
		}
	}
	total := 0.0
	for i := range weights {
		total += weights[i].Weight
	}
	if total > 0 {
		for i := range weights {
			weights[i].Weight /= total
		}
	}
}

func (r *RepairRecommender) runTOPSISCore(
	materials []models.RepairMaterial,
	criteria []MADMCriteria,
	sc *ScenarioContext,
) []float64 {
	n := len(materials)
	m := len(criteria)
	if n == 0 {
		return nil
	}

	getValue := func(mat *models.RepairMaterial, idx int) float64 {
		return r.getAttrValue(mat, criteria[idx].Name)
	}

	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			matrix[i][j] = getValue(&materials[i], j)
		}
	}

	colNorms := make([]float64, m)
	for j := 0; j < m; j++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += matrix[i][j] * matrix[i][j]
		}
		colNorms[j] = math.Sqrt(sum)
	}

	normMatrix := make([][]float64, n)
	for i := range normMatrix {
		normMatrix[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			if colNorms[j] > 0 {
				normMatrix[i][j] = matrix[i][j] / colNorms[j]
			}
		}
	}

	weightedNorm := make([][]float64, n)
	for i := range weightedNorm {
		weightedNorm[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			weightedNorm[i][j] = normMatrix[i][j] * criteria[j].Weight
		}
	}

	idealBest := make([]float64, m)
	idealWorst := make([]float64, m)
	for j := 0; j < m; j++ {
		best := weightedNorm[0][j]
		worst := weightedNorm[0][j]
		for i := 1; i < n; i++ {
			if criteria[j].IsBenefit {
				if weightedNorm[i][j] > best {
					best = weightedNorm[i][j]
				}
				if weightedNorm[i][j] < worst {
					worst = weightedNorm[i][j]
				}
			} else {
				if weightedNorm[i][j] < best {
					best = weightedNorm[i][j]
				}
				if weightedNorm[i][j] > worst {
					worst = weightedNorm[i][j]
				}
			}
		}
		idealBest[j] = best
		idealWorst[j] = worst
	}

	performance := make([]float64, n)
	for i := 0; i < n; i++ {
		sumB := 0.0
		sumW := 0.0
		for j := 0; j < m; j++ {
			sumB += math.Pow(weightedNorm[i][j]-idealBest[j], 2)
			sumW += math.Pow(weightedNorm[i][j]-idealWorst[j], 2)
		}
		distanceBest := math.Sqrt(sumB)
		distanceWorst := math.Sqrt(sumW)
		denom := distanceWorst + distanceBest
		if denom > 0 {
			performance[i] = distanceWorst / denom
		}

		if sc.HeritageCompliance {
			if materials[i].MaterialType == "ROMAN_CONCRETE" {
				performance[i] *= 1.12
			} else if materials[i].MaterialType == "LIME_MORTAR" {
				performance[i] *= 1.08
			}
			if materials[i].MaterialType == "FRP" {
				performance[i] *= 0.92
			}
		}
		if sc.UrgencyLevel == "CRITICAL" {
			if materials[i].MaterialType == "MODERN_CEMENT" || materials[i].MaterialType == "EPOXY" {
				performance[i] *= 1.1
			}
		}
		if performance[i] > 1.0 {
			performance[i] = 0.999
		}
	}

	return performance
}

func (r *RepairRecommender) performSensitivityAnalysis(
	materials []models.RepairMaterial,
	criteria []MADMCriteria,
	sc *ScenarioContext,
) *SensitivityResult {
	n := len(materials)
	m := len(criteria)
	if n == 0 {
		return nil
	}

	baseScores := r.runTOPSISCore(materials, criteria, sc)

	baseRanking := make([]int, n)
	for i := range baseRanking {
		baseRanking[i] = i
	}
	sort.Slice(baseRanking, func(a, b int) bool {
		return baseScores[baseRanking[a]] > baseScores[baseRanking[b]]
	})

	baseRankPos := make([]int, n)
	for rank, idx := range baseRanking {
		baseRankPos[idx] = rank
	}

	baseRankingNames := make([]string, n)
	for rank, idx := range baseRanking {
		baseRankingNames[rank] = materials[idx].Name
	}

	perturbDirections := []float64{-SENSITIVITY_PERTURB, SENSITIVITY_PERTURB}
	attrSens := make(map[string]float64)

	totalRankChanges := 0
	totalScenarios := 0
	top3ChangedCount := 0
	baseTop3Set := make(map[int]bool)
	for r := 0; r < n && r < 3; r++ {
		baseTop3Set[baseRanking[r]] = true
	}

	for j := 0; j < m; j++ {
		attrName := criteria[j].Name
		maxSpearmanDiff := 0.0

		for _, perturb := range perturbDirections {
			perturbedWeights := make([]MADMCriteria, m)
			copy(perturbedWeights, criteria)
			perturbedWeights[j].Weight = criteria[j].Weight * (1.0 + perturb)

			total := 0.0
			for k := range perturbedWeights {
				total += perturbedWeights[k].Weight
			}
			if total > 0 {
				for k := range perturbedWeights {
					perturbedWeights[k].Weight /= total
				}
			}

			perturbedScores := r.runTOPSISCore(materials, perturbedWeights, sc)
			perturbedRanking := make([]int, n)
			for i := range perturbedRanking {
				perturbedRanking[i] = i
			}
			sort.Slice(perturbedRanking, func(a, b int) bool {
				return perturbedScores[perturbedRanking[a]] > perturbedScores[perturbedRanking[b]]
			})

			perturbedRankPos := make([]int, n)
			for rank, idx := range perturbedRanking {
				perturbedRankPos[idx] = rank
			}

			sumDiff2 := 0.0
			for i := 0; i < n; i++ {
				diff := float64(baseRankPos[i] - perturbedRankPos[i])
				sumDiff2 += diff * diff
			}
			spearmanCorr := 1.0 - 6.0*sumDiff2/(float64(n)*(float64(n*n)-1.0))
			spearmanDiff := 1.0 - spearmanCorr
			if spearmanDiff < 0 {
				spearmanDiff = 0
			}
			if spearmanDiff > maxSpearmanDiff {
				maxSpearmanDiff = spearmanDiff
			}

			for i := 0; i < n; i++ {
				if baseRankPos[i] != perturbedRankPos[i] {
					totalRankChanges++
				}
			}
			totalScenarios += n

			perturbedTop3Set := make(map[int]bool)
			for rk := 0; rk < n && rk < 3; rk++ {
				perturbedTop3Set[perturbedRanking[rk]] = true
			}
			top3Diff := false
			for k := range baseTop3Set {
				if !perturbedTop3Set[k] {
					top3Diff = true
					break
				}
			}
			for k := range perturbedTop3Set {
				if !baseTop3Set[k] {
					top3Diff = true
					break
				}
			}
			if top3Diff {
				top3ChangedCount++
			}
		}

		attrSens[attrName] = math.Round(maxSpearmanDiff*10000) / 10000
	}

	totalAttrScenarios := m * 2
	rankStability := 1.0
	if totalScenarios > 0 {
		rankStability = 1.0 - float64(totalRankChanges)/float64(totalScenarios)
	}
	if rankStability < 0 {
		rankStability = 0
	}

	top3ChangeRate := float64(top3ChangedCount) / float64(totalAttrScenarios)

	maxAttrSens := 0.0
	for _, v := range attrSens {
		if v > maxAttrSens {
			maxAttrSens = v
		}
	}

	confidence := rankStability*0.5 + (1.0-top3ChangeRate)*0.35 + (1.0-maxAttrSens)*0.15
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	critical := confidence < 0.70 || top3ChangeRate > 0.30

	result := &SensitivityResult{
		RankStability:     math.Round(rankStability*10000) / 10000,
		AttributeSens:     attrSens,
		TopRankChangeRate: math.Round(top3ChangeRate*10000) / 10000,
		CriticalDecision:  critical,
		BaseRanking:       baseRankingNames,
		ConfidenceScore:   math.Round(confidence*10000) / 10000,
	}

	log.Printf("Sensitivity analysis: confidence=%.2f%%, rank_stability=%.2f%%, top3_change=%.1f%%, critical=%v",
		confidence*100, rankStability*100, top3ChangeRate*100, critical)

	return result
}
