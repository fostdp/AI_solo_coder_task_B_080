package evaluation

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type StructuralEvaluator struct {
	repo *repository.Repository
	cfg  *config.Config
}

type FEAModel struct {
	SegmentType       string    `json:"segment_type"`
	Nodes             []Node    `json:"nodes"`
	Elements          []Element `json:"elements"`
	YoungsModulus     float64   `json:"youngs_modulus_GPa"`
	PoissonsRatio     float64   `json:"poissons_ratio"`
	Density           float64   `json:"density_kg_m3"`
	StressConcentrator float64  `json:"stress_concentrator_factor"`
}

type Node struct {
	ID        int       `json:"id"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	Z         float64   `json:"z"`
	Displacements [3]float64 `json:"displacements_mm"`
	Fixed     [3]bool   `json:"fixed"`
}

type Element struct {
	ID          int     `json:"id"`
	NodeIDs     [2]int  `json:"node_ids"`
	Area        float64 `json:"area_m2"`
	Length      float64 `json:"length_m"`
	AxialStress float64 `json:"axial_stress_MPa"`
	MaxStress   float64 `json:"max_stress_MPa"`
	Utilization float64 `json:"utilization_ratio"`
	MaterialFactor float64 `json:"material_degradation_factor"`
}

type DegradationResult struct {
	EffectiveStrength     float64 `json:"effective_strength_MPa"`
	EffectiveE            float64 `json:"effective_elastic_modulus_GPa"`
	StrengthLossPct       float64 `json:"strength_loss_percent"`
	WeatheringFactor      float64 `json:"weathering_factor"`
	SettlementEffect      float64 `json:"settlement_effect"`
	StressConcentration   float64 `json:"stress_concentration_factor"`
	TotalDegradationFactor float64 `json:"total_degradation_factor"`
}

const (
	DESIGN_LIFE_YEARS     = 2000
	MIN_EFFECTIVE_FACTOR  = 0.15
	ARCH_RISE_RATIO       = 0.25
	PIER_WIDTH            = 1.2
	PIER_DEPTH            = 2.5
	ARCH_RIB_THICKNESS    = 0.8
	ARCH_WIDTH            = 3.0
	GRAVITY_ACCEL         = 9.81
	STRUCTURE_DENSITY     = 2200.0
	POISSONS_RATIO_STONE = 0.18
	YOUNGS_MODULUS_STONE  = 28.0
	DESIGN_SAFETY_FACTOR  = 3.5
)

func NewStructuralEvaluator(repo *repository.Repository, cfg *config.Config) *StructuralEvaluator {
	return &StructuralEvaluator{
		repo: repo,
		cfg:  cfg,
	}
}

func (e *StructuralEvaluator) EvaluateSegment(ctx context.Context, segmentID uuid.UUID) ([]*models.Alert, error) {
	segments, err := e.repo.GetAllSegmentsWithStatus(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load segments: %w", err)
	}

	var segment *models.StructureSegment
	for i := range segments {
		if segments[i].ID == segmentID {
			segment = &segments[i]
			break
		}
	}
	if segment == nil {
		return nil, fmt.Errorf("segment %s not found", segmentID)
	}

	sensorVals, err := e.repo.GetSegmentLatestSensorValues(ctx, segmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sensor values: %w", err)
	}

	currentStress := sensorVals["stress"]
	weatheringDepth := sensorVals["weathering"]
	settlementMM := sensorVals["settlement"]

	feaModel := e.buildFEAModel(segment, sensorVals)
	degradation := e.computeDegradation(segment, weatheringDepth, settlementMM)

	computedStress := e.runSimplifiedFEA(segment, feaModel, degradation)

	if computedStress.MaxStress > 0 && currentStress == 0 {
		currentStress = computedStress.AxialStress
	}

	feaModel.Elements = computedStress.Elements

	residualCapacity := degradation.EffectiveStrength
	residualRatio := residualCapacity / segment.DesignStrength
	if residualRatio > 1.0 {
		residualRatio = 1.0
	}
	if residualRatio < 0 {
		residualRatio = 0
	}

	stressUtilization := 0.0
	if residualCapacity > 0 {
		stressUtilization = currentStress / (residualCapacity * 1.0)
	}

	var safetyLevel string
	switch {
	case residualRatio >= 0.8 && stressUtilization < 0.6:
		safetyLevel = "SAFE"
	case residualRatio >= 0.65 && stressUtilization < 0.75:
		safetyLevel = "WARNING"
	case residualRatio >= e.cfg.Threshold.LoadCapacityThreshold && stressUtilization < 0.9:
		safetyLevel = "DANGER"
	default:
		safetyLevel = "CRITICAL"
	}

	maxStressVal := currentStress
	if computedStress.MaxStress > maxStressVal {
		maxStressVal = computedStress.MaxStress
	}

	eval := &models.StructuralEvaluation{
		AqueductID:            segment.AqueductID,
		SegmentID:             segmentID,
		EvaluationTime:        time.Now().UTC(),
		CurrentStress:         currentStress,
		MaxStress:             maxStressVal,
		WeatheringDepth:       weatheringDepth,
		SettlementMM:          settlementMM,
		ResidualStrength:      residualCapacity,
		ResidualCapacityRatio: residualRatio,
		SafetyLevel:           safetyLevel,
		FEAModelData: map[string]interface{}{
			"nodes_count":       len(feaModel.Nodes),
			"elements_count":    len(feaModel.Elements),
			"material":          feaModel,
			"degradation":       degradation,
			"stress_analysis":   computedStress,
			"stress_utilization": stressUtilization,
		},
		Recommendations: e.generateRecommendations(segment, safetyLevel, residualRatio, weatheringDepth, settlementMM),
	}

	if err := e.repo.InsertEvaluation(ctx, eval); err != nil {
		log.Printf("Warning: Failed to save evaluation for segment %s: %v", segmentID, err)
	}

	alerts := e.checkThresholds(ctx, segment, sensorVals, residualRatio, safetyLevel, settlementMM, weatheringDepth)

	return alerts, nil
}

func (e *StructuralEvaluator) buildFEAModel(segment *models.StructureSegment, sensorVals map[string]float64) *FEAModel {
	model := &FEAModel{
		SegmentType:       segment.SegmentType,
		YoungsModulus:     YOUNGS_MODULUS_STONE,
		PoissonsRatio:     POISSONS_RATIO_STONE,
		Density:           STRUCTURE_DENSITY,
		StressConcentrator: 1.0,
	}

	if segment.SegmentType == "pier" {
		height := 15.0
		if pos, ok := segment.Position3D["height"]; ok {
			if h, conv := pos.(float64); conv {
				height = h
			}
		}
		model.Nodes = []Node{
			{ID: 1, X: 0, Y: 0, Z: 0, Fixed: [3]bool{true, true, true}},
			{ID: 2, X: 0, Y: 0, Z: height * 0.5},
			{ID: 3, X: 0, Y: 0, Z: height},
			{ID: 4, X: PIER_WIDTH, Y: 0, Z: 0, Fixed: [3]bool{true, true, true}},
			{ID: 5, X: PIER_WIDTH, Y: 0, Z: height * 0.5},
			{ID: 6, X: PIER_WIDTH, Y: 0, Z: height},
		}
		area := PIER_WIDTH * PIER_DEPTH
		model.Elements = []Element{
			{ID: 1, NodeIDs: [2]int{1, 2}, Area: area, Length: height * 0.5},
			{ID: 2, NodeIDs: [2]int{2, 3}, Area: area, Length: height * 0.5},
			{ID: 3, NodeIDs: [2]int{4, 5}, Area: area, Length: height * 0.5},
			{ID: 4, NodeIDs: [2]int{5, 6}, Area: area, Length: height * 0.5},
			{ID: 5, NodeIDs: [2]int{1, 4}, Area: area * 0.3, Length: PIER_WIDTH},
			{ID: 6, NodeIDs: [2]int{2, 5}, Area: area * 0.3, Length: PIER_WIDTH},
			{ID: 7, NodeIDs: [2]int{3, 6}, Area: area * 0.3, Length: PIER_WIDTH},
		}
	} else if segment.SegmentType == "arch" {
		span := 5.5
		if s, ok := segment.Position3D["span"]; ok {
			if sv, conv := s.(float64); conv {
				span = sv
			}
		}
		rise := span * ARCH_RISE_RATIO
		nNodes := 9

		for i := 0; i < nNodes; i++ {
			x := (float64(i) / float64(nNodes-1)) * span
			parabolic := 4 * rise / (span * span) * x * (span - x)
			fixed := i == 0 || i == nNodes-1
			model.Nodes = append(model.Nodes, Node{
				ID:    i + 1,
				X:     x,
				Y:     0,
				Z:     parabolic,
				Fixed: [3]bool{fixed, true, fixed},
			})
		}

		archArea := ARCH_RIB_THICKNESS * ARCH_WIDTH
		for i := 0; i < nNodes-1; i++ {
			n1 := &model.Nodes[i]
			n2 := &model.Nodes[i+1]
			elLen := math.Sqrt(math.Pow(n2.X-n1.X, 2) + math.Pow(n2.Z-n1.Z, 2))
			model.Elements = append(model.Elements, Element{
				ID:     i + 1,
				NodeIDs: [2]int{i + 1, i + 2},
				Area:   archArea,
				Length: elLen,
			})
		}
		model.StressConcentrator = 1.3
	}

	return model
}

func (e *StructuralEvaluator) computeDegradation(segment *models.StructureSegment, weatheringDepthMM, settlementMM float64) *DegradationResult {
	d := &DegradationResult{}

	ageYears := 2000.0
	designStrength := segment.DesignStrength

	d.WeatheringFactor = 1.0
	effectiveDepth := weatheringDepthMM
	if effectiveDepth == 0 {
		ageRatio := ageYears / DESIGN_LIFE_YEARS
		effectiveDepth = 2.5 + ageRatio*12.0 + math.Pow(ageRatio, 1.5)*8.0
	}
	archDepth := ARCH_RIB_THICKNESS * 1000.0
	pierDepth := PIER_DEPTH * 1000.0
	var structDepth float64
	if segment.SegmentType == "arch" {
		structDepth = archDepth
	} else {
		structDepth = pierDepth
	}
	sectionLossRatio := effectiveDepth / structDepth
	if sectionLossRatio > 0.8 {
		sectionLossRatio = 0.8
	}
	d.WeatheringFactor = math.Pow(1.0-sectionLossRatio, 1.3)

	maxExpectedSettlement := e.cfg.Threshold.SettlementLimitMM * 2.0
	settlementNorm := settlementMM / maxExpectedSettlement
	if settlementNorm > 1.0 {
		settlementNorm = 1.0
	}
	d.SettlementEffect = 1.0 - settlementNorm*0.25

	if effectiveDepth > structDepth*0.25 {
		d.StressConcentration = 1.0 + (effectiveDepth/(structDepth*0.5))*0.6
	} else {
		d.StressConcentration = 1.0 + (effectiveDepth/(structDepth*0.5))*0.2
	}
	if segment.SegmentType == "arch" {
		d.StressConcentration *= 1.15
	}

	ageFactor := 1.0
	if ageYears > 500 {
		ageFactor = 0.85 + 0.15*math.Exp(-ageYears/800.0)
	}

	d.TotalDegradationFactor = d.WeatheringFactor * d.SettlementEffect * ageFactor
	if d.TotalDegradationFactor < MIN_EFFECTIVE_FACTOR {
		d.TotalDegradationFactor = MIN_EFFECTIVE_FACTOR
	}

	d.EffectiveStrength = designStrength * d.TotalDegradationFactor
	d.EffectiveE = YOUNGS_MODULUS_STONE * d.TotalDegradationFactor
	d.StrengthLossPct = (1.0 - d.TotalDegradationFactor) * 100.0

	return d
}

type StressResult struct {
	AxialStress     float64   `json:"axial_stress_MPa"`
	BendingStress   float64   `json:"bending_stress_MPa"`
	ShearStress     float64   `json:"shear_stress_MPa"`
	MaxStress       float64   `json:"max_stress_MPa"`
	DeflectionMM    float64   `json:"max_deflection_mm"`
	Elements        []Element `json:"elements"`
}

func (e *StructuralEvaluator) runSimplifiedFEA(segment *models.StructureSegment, model *FEAModel, degradation *DegradationResult) *StressResult {
	result := &StressResult{}

	deadLoadPerM := ARCH_WIDTH * ARCH_RIB_THICKNESS * GRAVITY_ACCEL * STRUCTURE_DENSITY / 1000.0
	superimposedLoad := 2.5
	totalUDL := deadLoadPerM + superimposedLoad

	if segment.SegmentType == "arch" {
		span := 5.5
		if s, ok := segment.Position3D["span"]; ok {
			if sv, conv := s.(float64); conv {
				span = sv
			}
		}
		rise := span * ARCH_RISE_RATIO

		horizontalThrust := (totalUDL * span * span) / (8 * rise)
		verticalReaction := totalUDL * span / 2
		crownMoment := (totalUDL * span * span) / 8 * (1 - 2*rise/span*0.7)

		archArea := ARCH_RIB_THICKNESS * ARCH_WIDTH
		axialStressCrown := horizontalThrust / archArea / 1000.0

		sectionModulus := (ARCH_WIDTH * ARCH_RIB_THICKNESS * ARCH_RIB_THICKNESS) / 6.0
		bendingStress := crownMoment / sectionModulus / 1000.0 * 1000.0

		result.AxialStress = axialStressCrown
		result.BendingStress = bendingStress
		result.MaxStress = (axialStressCrown + math.Abs(bendingStress)) * degradation.StressConcentration
		result.ShearStress = 1.5 * verticalReaction / archArea / 1000.0

		momentOfInertia := (ARCH_WIDTH * math.Pow(ARCH_RIB_THICKNESS, 3)) / 12.0
		E_Pa := degradation.EffectiveE * 1e9
		I_m4 := momentOfInertia
		result.DeflectionMM = (5 * totalUDL * 1000 * math.Pow(span, 4)) /
			(384 * E_Pa * I_m4) * 1000

		for i := range model.Elements {
			pos := float64(i+1) / float64(len(model.Elements)+1)
			distFromCrown := math.Abs(pos - 0.5)

			elementAxial := horizontalThrust / (model.Elements[i].Area) / 1000.0
			momentFactor := 4 * pos * (1 - pos)
			elementBending := bendingStress * momentFactor

			model.Elements[i].AxialStress = elementAxial
			model.Elements[i].MaxStress = (elementAxial + elementBending) * degradation.StressConcentration
			model.Elements[i].MaterialFactor = degradation.TotalDegradationFactor
			permitted := degradation.EffectiveStrength / DESIGN_SAFETY_FACTOR
			if permitted > 0 {
				model.Elements[i].Utilization = model.Elements[i].MaxStress / permitted
			}
			_ = distFromCrown
		}
		result.Elements = model.Elements
		_ = verticalReaction

	} else if segment.SegmentType == "pier" {
		height := 15.0
		if pos, ok := segment.Position3D["height"]; ok {
			if h, conv := pos.(float64); conv {
				height = h
			}
		}

		axialLoadPier := (totalUDL * 5.5) + (PIER_WIDTH*PIER_DEPTH*height*GRAVITY_ACCEL*STRUCTURE_DENSITY/1000.0)*0.5
		pierArea := PIER_WIDTH * PIER_DEPTH

		settlementMm := 0.0
		{
			var sv float64
			e.repo.GetPool().QueryRow(context.Background(),
				`SELECT COALESCE(MAX(value), 0) FROM sensor_data WHERE segment_id=$1 AND sensor_type='settlement' AND timestamp > NOW() - INTERVAL '30 days'`,
				segment.ID).Scan(&sv)
			settlementMm = sv
		}
		adjacentSettlement := settlementMm * 0.3
		differential := math.Abs(settlementMm - adjacentSettlement)
		rotationRad := differential / (5.5 * 1000.0)
		eccentricity := height / 2 * math.Sin(rotationRad)
		pierSectionMod := (PIER_DEPTH * PIER_WIDTH * PIER_WIDTH) / 6.0
		momentFromSettlement := axialLoadPier * 1000.0 * eccentricity
		bendingStress := momentFromSettlement / pierSectionMod / 1000.0

		result.AxialStress = axialLoadPier / pierArea / 1000.0
		result.BendingStress = bendingStress
		result.MaxStress = (result.AxialStress + bendingStress) * degradation.StressConcentration
		result.ShearStress = axialLoadPier * math.Tan(rotationRad) / pierArea / 1000.0

		E_Pa := degradation.EffectiveE * 1e9
		I_m4 := (PIER_DEPTH * math.Pow(PIER_WIDTH, 3)) / 12.0
		result.DeflectionMM = (axialLoadPier*1000.0 * math.Pow(height, 3)) /
			(3 * E_Pa * I_m4) * 1000

		for i := range model.Elements {
			elementRatio := float64(i+1) / float64(len(model.Elements))
			model.Elements[i].AxialStress = result.AxialStress * (0.8 + elementRatio*0.4)
			model.Elements[i].MaxStress = model.Elements[i].AxialStress * degradation.StressConcentration
			model.Elements[i].MaterialFactor = degradation.TotalDegradationFactor
			permitted := degradation.EffectiveStrength / DESIGN_SAFETY_FACTOR
			if permitted > 0 {
				model.Elements[i].Utilization = model.Elements[i].MaxStress / permitted
			}
		}
		result.Elements = model.Elements
	}

	if result.MaxStress < 0.1 {
		result.MaxStress = segment.DesignStrength * 0.3
	}

	return result
}

func (e *StructuralEvaluator) checkThresholds(
	ctx context.Context,
	segment *models.StructureSegment,
	sensors map[string]float64,
	capacityRatio float64,
	safetyLevel string,
	settlementMM, weatheringDepth float64,
) []*models.Alert {

	var alerts []*models.Alert

	if capacityRatio < e.cfg.Threshold.LoadCapacityThreshold {
		severity := "CRITICAL"
		if capacityRatio >= e.cfg.Threshold.LoadCapacityThreshold*0.7 {
			severity = "DANGER"
		}
		alerts = append(alerts, &models.Alert{
			AqueductID:     segment.AqueductID,
			SegmentID:      &segment.ID,
			AlertType:      "LOAD_CAPACITY_LOW",
			Severity:       severity,
			Title:          fmt.Sprintf("%s段承载力不足：仅剩 %.1f%%", segmentTypeLabel(segment.SegmentType), capacityRatio*100),
			Description:    fmt.Sprintf("设计承载力: %.2f MPa, 剩余有效承载力: %.2f MPa。低于设计值50%%阈值，需紧急加固。", segment.DesignStrength, segment.DesignStrength*capacityRatio),
			ThresholdValue: e.cfg.Threshold.LoadCapacityThreshold,
			MeasuredValue:  capacityRatio,
			Unit:           "ratio",
			TriggeredAt:    time.Now().UTC(),
		})
	}

	if settlementMM > e.cfg.Threshold.SettlementLimitMM {
		severity := "WARNING"
		if settlementMM > e.cfg.Threshold.SettlementLimitMM*1.5 {
			severity = "CRITICAL"
		} else if settlementMM > e.cfg.Threshold.SettlementLimitMM*1.2 {
			severity = "DANGER"
		}
		alerts = append(alerts, &models.Alert{
			AqueductID:     segment.AqueductID,
			SegmentID:      &segment.ID,
			AlertType:      "SETTLEMENT_EXCEEDED",
			Severity:       severity,
			Title:          fmt.Sprintf("基础沉降超限：%.1f mm", settlementMM),
			Description:    fmt.Sprintf("累计基础沉降量 %.1f mm 超过允许阈值 %.1f mm，可能引起上部结构附加应力。", settlementMM, e.cfg.Threshold.SettlementLimitMM),
			ThresholdValue: e.cfg.Threshold.SettlementLimitMM,
			MeasuredValue:  settlementMM,
			Unit:           "mm",
			TriggeredAt:    time.Now().UTC(),
		})
	}

	if stress, ok := sensors["stress"]; ok {
		designLimit := segment.DesignStrength / DESIGN_SAFETY_FACTOR
		if stress > designLimit {
			ratio := stress / designLimit
			severity := "WARNING"
			if ratio > 1.3 {
				severity = "CRITICAL"
			} else if ratio > 1.15 {
				severity = "DANGER"
			}
			alerts = append(alerts, &models.Alert{
				AqueductID:     segment.AqueductID,
				SegmentID:      &segment.ID,
				AlertType:      "STRESS_EXCEEDED",
				Severity:       severity,
				Title:          fmt.Sprintf("应力超限：%.2f MPa", stress),
				Description:    fmt.Sprintf("实测应力 %.2f MPa 超过设计允许应力 %.2f MPa，应力比 %.2f。", stress, designLimit, ratio),
				ThresholdValue: designLimit,
				MeasuredValue:  stress,
				Unit:           "MPa",
				TriggeredAt:    time.Now().UTC(),
			})
		}
	}

	recentRate, _ := e.repo.GetWeatheringRate(ctx, segment.ID, 30)
	baselineRate, _ := e.repo.GetWeatheringRate(ctx, segment.ID, 365)
	if baselineRate > 0 && recentRate > baselineRate*e.cfg.Threshold.WeatheringAccelRatio {
		ratio := recentRate / baselineRate
		severity := "WARNING"
		if ratio > 2.0 {
			severity = "DANGER"
		}
		alerts = append(alerts, &models.Alert{
			AqueductID:     segment.AqueductID,
			SegmentID:      &segment.ID,
			AlertType:      "WEATHERING_ACCELERATED",
			Severity:       severity,
			Title:          fmt.Sprintf("风化速率异常加速：%.3f mm/日 vs 基线 %.3f mm/日", recentRate, baselineRate),
			Description:    fmt.Sprintf("近30日平均风化速率 %.4f mm/日 是年度基线 %.4f mm/日的 %.2f 倍。近期风化深度 %.2f mm。", recentRate, baselineRate, ratio, weatheringDepth),
			ThresholdValue: baselineRate * e.cfg.Threshold.WeatheringAccelRatio,
			MeasuredValue:  recentRate,
			Unit:           "mm/day",
			TriggeredAt:    time.Now().UTC(),
		})
	}

	if tilt, ok := sensors["tilt"]; ok {
		tiltLimit := 0.5
		if tilt > tiltLimit {
			ratio := tilt / tiltLimit
			severity := "WARNING"
			if ratio > 2.0 {
				severity = "CRITICAL"
			}
			alerts = append(alerts, &models.Alert{
				AqueductID:     segment.AqueductID,
				SegmentID:      &segment.ID,
				AlertType:      "TILT_EXCEEDED",
				Severity:       severity,
				Title:          fmt.Sprintf("结构倾角超限：%.2f°", tilt),
				Description:    fmt.Sprintf("实测结构倾角 %.2f° 超过阈值 %.2f°，比值 %.2f。", tilt, tiltLimit, ratio),
				ThresholdValue: tiltLimit,
				MeasuredValue:  tilt,
				Unit:           "degrees",
				TriggeredAt:    time.Now().UTC(),
			})
		}
	}

	for _, a := range alerts {
		if err := e.repo.InsertAlert(ctx, a); err != nil {
			log.Printf("Warning: Failed to insert alert: %v", err)
		}
	}

	return alerts
}

func (e *StructuralEvaluator) generateRecommendations(segment *models.StructureSegment, safetyLevel string,
	capacityRatio float64, weatheringDepth, settlementMM float64) string {

	recs := ""

	switch safetyLevel {
	case "CRITICAL":
		recs += "[紧急] 立即封闭交通，开展临时支护；建议使用CFRP加固。\n"
	case "DANGER":
		recs += "[警告] 限制荷载，3个月内完成结构加固。\n"
	case "WARNING":
		recs += "[注意] 加强监测频率，6个月内完成预防性修复。\n"
	default:
		recs += "[正常] 继续常规监测。\n"
	}

	if weatheringDepth > 10 {
		recs += fmt.Sprintf("· 风化深度 %.1f mm：建议使用石灰华修补砂浆+传统石灰砂浆勾缝。\n", weatheringDepth)
	} else if weatheringDepth > 5 {
		recs += fmt.Sprintf("· 风化深度 %.1f mm：建议传统石灰砂浆表面修复。\n", weatheringDepth)
	}

	if capacityRatio < 0.5 {
		recs += "· 承载力严重不足：建议CFRP片材缠绕加固+环氧注浆裂缝处理。\n"
	} else if capacityRatio < 0.7 {
		recs += "· 承载力下降：推荐微膨胀注浆+古罗马高强度混凝土补强。\n"
	}

	if settlementMM > 15 {
		recs += fmt.Sprintf("· 沉降 %.1f mm：建议基础托换+微膨胀水泥基注浆加固地基。\n", settlementMM)
	}

	if segment.SegmentType == "arch" && safetyLevel != "SAFE" {
		recs += "· 拱券结构：考虑设置临时支撑后采用GFRP条带加固拱脚。\n"
	}

	return recs
}

func segmentTypeLabel(t string) string {
	switch t {
	case "arch":
		return "拱券"
	case "pier":
		return "桥墩"
	case "tunnel":
		return "隧道"
	case "channel":
		return "明渠"
	default:
		return t
	}
}
