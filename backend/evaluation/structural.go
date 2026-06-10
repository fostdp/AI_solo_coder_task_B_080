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

	FEA_MAX_ITERATIONS    = 50
	FEA_TOLERANCE         = 1e-4
	FEA_RELAXATION        = 0.5
	FEA_MIN_ELEMENTS      = 6
	FEA_MAX_ELEMENTS      = 32
	FEA_CURVATURE_THRESH  = 0.15
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

		pierArea := PIER_WIDTH * PIER_DEPTH

		nVertical := 3
		nHorizontal := 3
		slenderness := height / math.Sqrt(pierArea)
		if slenderness > 8 {
			nVertical = 4
		}
		if slenderness > 12 {
			nVertical = 5
		}

		nodeID := 1
		for col := 0; col < 2; col++ {
			x := 0.0
			if col == 1 {
				x = PIER_WIDTH
			}
			for row := 0; row < nVertical; row++ {
				z := height * float64(row) / float64(nVertical-1)
				fixed := row == 0
				model.Nodes = append(model.Nodes, Node{
					ID:    nodeID,
					X:     x,
					Y:     0,
					Z:     z,
					Fixed: [3]bool{fixed, true, fixed},
				})
				nodeID++
			}
		}

		elemID := 1
		for col := 0; col < 2; col++ {
			for row := 0; row < nVertical-1; row++ {
				n1 := col*nVertical + row + 1
				n2 := n1 + 1
				hLen := height / float64(nVertical-1)
				model.Elements = append(model.Elements, Element{
					ID:     elemID,
					NodeIDs: [2]int{n1, n2},
					Area:   pierArea / 2.0,
					Length: hLen,
				})
				elemID++
			}
		}

		for row := 0; row < nVertical; row++ {
			n1 := row + 1
			n2 := nVertical + row + 1
			model.Elements = append(model.Elements, Element{
				ID:     elemID,
				NodeIDs: [2]int{n1, n2},
				Area:   pierArea * 0.25,
				Length: PIER_WIDTH,
			})
			elemID++
		}
		_ = nHorizontal

	} else if segment.SegmentType == "arch" {
		span := 5.5
		if s, ok := segment.Position3D["span"]; ok {
			if sv, conv := s.(float64); conv {
				span = sv
			}
		}
		rise := span * ARCH_RISE_RATIO

		initialElements := FEA_MIN_ELEMENTS
		aspectRatio := span / rise
		if aspectRatio < 3 {
			initialElements = 8
		} else if aspectRatio > 6 {
			initialElements = 5
		}

		initialNodes := initialElements + 1
		rawParams := make([]float64, initialNodes)
		rawCoords := make([][2]float64, initialNodes)

		for i := 0; i < initialNodes; i++ {
			t := float64(i) / float64(initialNodes-1)
			rawParams[i] = t
			x := t * span
			z := 4 * rise / (span * span) * x * (span - x)
			rawCoords[i] = [2]float64{x, z}
		}

		refinedParams := []float64{0.0, 1.0}
		for iteration := 0; iteration < 3; iteration++ {
			needRefine := []int{}
			for i := 0; i < len(refinedParams)-1; i++ {
				tMid := (refinedParams[i] + refinedParams[i+1]) / 2
				x1, z1 := archCurve(refinedParams[i], span, rise)
				x2, z2 := archCurve(refinedParams[i+1], span, rise)
				xm, zm := archCurve(tMid, span, rise)

				chordVec := [2]float64{x2 - x1, z2 - z1}
				midVec := [2]float64{xm - x1, zm - z1}
				chordLen := math.Sqrt(chordVec[0]*chordVec[0] + chordVec[1]*chordVec[1])
				if chordLen < 1e-6 {
					continue
				}
				cross := math.Abs(chordVec[0]*midVec[1] - chordVec[1]*midVec[0])
				curvatureDeviation := cross / chordLen

				chordAngle := math.Atan2(chordVec[1], chordVec[0])
				curvatureK := 8 * rise / (span * span)
				localCurvature := curvatureK / math.Pow(1+math.Pow(4*rise/span*(1-2*tMid), 2), 1.5)
				elementLength := chordLen

				if (curvatureDeviation/chordLen > FEA_CURVATURE_THRESH ||
					localCurvature*elementLength > 0.25) &&
					len(refinedParams) < FEA_MAX_ELEMENTS {
					needRefine = append(needRefine, i)
				}
			}

			if len(needRefine) == 0 {
				break
			}

			inserted := 0
			for _, idx := range needRefine {
				insertPos := idx + 1 + inserted
				tMid := (refinedParams[idx+inserted] + refinedParams[idx+inserted+1]) / 2
				refinedParams = append(refinedParams[:insertPos+1], refinedParams[insertPos:]...)
				refinedParams[insertPos] = tMid
				inserted++
			}
			if len(refinedParams) >= FEA_MAX_ELEMENTS+1 {
				break
			}
		}

		nNodesFinal := len(refinedParams)
		model.Nodes = make([]Node, nNodesFinal)
		for i, t := range refinedParams {
			x, z := archCurve(t, span, rise)
			fixed := i == 0 || i == nNodesFinal-1
			model.Nodes[i] = Node{
				ID:    i + 1,
				X:     x,
				Y:     0,
				Z:     z,
				Fixed: [3]bool{fixed, true, fixed},
			}
		}

		archArea := ARCH_RIB_THICKNESS * ARCH_WIDTH
		model.Elements = make([]Element, nNodesFinal-1)
		for i := 0; i < nNodesFinal-1; i++ {
			n1 := &model.Nodes[i]
			n2 := &model.Nodes[i+1]
			elLen := math.Sqrt(math.Pow(n2.X-n1.X, 2) + math.Pow(n2.Z-n1.Z, 2))
			tMid := (refinedParams[i] + refinedParams[i+1]) / 2
			_, zm := archCurve(tMid, span, rise)
			localRiseRatio := zm / rise
			thicknessFactor := 0.85 + 0.3*math.Abs(2*tMid-1)
			model.Elements[i] = Element{
				ID:     i + 1,
				NodeIDs: [2]int{i + 1, i + 2},
				Area:   archArea * thicknessFactor,
				Length: elLen,
			}
			_ = localRiseRatio
		}

		model.StressConcentrator = 1.25
		if aspectRatio > 5 {
			model.StressConcentrator = 1.15
		}
		if rise/span < 0.18 {
			model.StressConcentrator += 0.1
		}
	}

	return model
}

func archCurve(t, span, rise float64) (x, z float64) {
	x = t * span
	z = 4 * rise / (span * span) * x * (span - x)
	return
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
	Converged       bool      `json:"converged"`
	Iterations      int       `json:"iterations"`
	Residual        float64   `json:"residual_norm"`
	ModelFallback   string    `json:"fallback_model,omitempty"`
}

type ArchAnalysisResult struct {
	HorizontalThrust    float64
	VerticalReaction    float64
	CrownAxialForce     float64
	CrownMoment         float64
	SpringingMoment     float64
	MaxAxialStress      float64
	MaxBendingStress    float64
	ArchRiseEff         float64
	CrownDeflection     float64
	AxialForceByElem    []float64
	MomentByElem        []float64
}

func (e *StructuralEvaluator) runSimplifiedFEA(segment *models.StructureSegment, model *FEAModel, degradation *DegradationResult) *StressResult {
	result := &StressResult{
		Converged: true,
	}

	deadLoadPerM := ARCH_WIDTH * ARCH_RIB_THICKNESS * GRAVITY_ACCEL * STRUCTURE_DENSITY / 1000.0
	superimposedLoad := 2.5
	totalUDL := deadLoadPerM + superimposedLoad

	if segment.SegmentType == "arch" {
		archRes, converged, iters, residual, fallback := e.solveThreeHingedArch(
			segment, model, degradation, totalUDL,
		)

		result.Converged = converged
		result.Iterations = iters
		result.Residual = residual
		result.ModelFallback = fallback

		horizontalThrust := archRes.HorizontalThrust
		verticalReaction := archRes.VerticalReaction

		archAreaAvg := 0.0
		for _, el := range model.Elements {
			archAreaAvg += el.Area
		}
		if len(model.Elements) > 0 {
			archAreaAvg /= float64(len(model.Elements))
		}
		if archAreaAvg < 0.1 {
			archAreaAvg = ARCH_RIB_THICKNESS * ARCH_WIDTH
		}

		sectionModulus := (ARCH_WIDTH * ARCH_RIB_THICKNESS * ARCH_RIB_THICKNESS) / 6.0
		if sectionModulus <= 0 {
			sectionModulus = 0.085
		}

		axialStressCrown := archRes.CrownAxialForce / archAreaAvg / 1000.0
		crownBendingStress := archRes.CrownMoment / sectionModulus / 1000.0 * 1000.0
		springingBendingStress := archRes.SpringingMoment / sectionModulus / 1000.0 * 1000.0

		result.AxialStress = axialStressCrown
		result.BendingStress = crownBendingStress

		result.MaxStress = archRes.MaxAxialStress + math.Abs(archRes.MaxBendingStress)
		result.MaxStress *= degradation.StressConcentration

		result.ShearStress = 1.5 * verticalReaction / archAreaAvg / 1000.0

		momentOfInertia := (ARCH_WIDTH * math.Pow(ARCH_RIB_THICKNESS, 3)) / 12.0
		E_Pa := degradation.EffectiveE * 1e9
		I_m4 := momentOfInertia

		span := 5.5
		if s, ok := segment.Position3D["span"]; ok {
			if sv, conv := s.(float64); conv {
				span = sv
			}
		}
		rise := span * ARCH_RISE_RATIO

		L_eff := math.Sqrt(span*span + (2*rise)*(2*rise)) * 0.85
		EA := E_Pa * archAreaAvg
		EI := E_Pa * I_m4
		result.DeflectionMM = archRes.CrownDeflection * 1000
		if result.DeflectionMM <= 0 {
			result.DeflectionMM = (5*totalUDL*1000*math.Pow(L_eff, 4))/(384*EI) * 1000 * (rise / span) * 2.5
		}

		for i := range model.Elements {
			var N, M float64
			if i < len(archRes.AxialForceByElem) {
				N = archRes.AxialForceByElem[i]
			} else {
				frac := float64(i+1) / float64(len(model.Elements)+1)
				N = horizontalThrust * (1.0 + 0.3*math.Abs(2*frac-1))
			}
			if i < len(archRes.MomentByElem) {
				M = archRes.MomentByElem[i]
			} else {
				frac := float64(i+1) / float64(len(model.Elements)+1)
				M = archRes.CrownMoment * (1.0 - 4*(frac-0.5)*(frac-0.5))
			}

			areaElem := model.Elements[i].Area
			if areaElem < 0.01 {
				areaElem = archAreaAvg
			}
			sigmaA := N / areaElem / 1000.0
			sigmaB := 0.0
			if areaElem > 0 {
				localW := (ARCH_WIDTH * ARCH_RIB_THICKNESS * ARCH_RIB_THICKNESS) / 6.0
				if localW > 0 {
					sigmaB = M / localW / 1000.0 * 1000.0
				}
			}

			model.Elements[i].AxialStress = math.Max(0.1, sigmaA)
			model.Elements[i].MaxStress = (sigmaA + math.Abs(sigmaB)) * degradation.StressConcentration
			model.Elements[i].MaterialFactor = degradation.TotalDegradationFactor

			permitted := degradation.EffectiveStrength / DESIGN_SAFETY_FACTOR
			if permitted > 0 {
				model.Elements[i].Utilization = math.Min(3.0, model.Elements[i].MaxStress/permitted)
			}
		}
		result.Elements = model.Elements
		_ = springingBendingStress
		_ = EA
		_ = rise

	} else if segment.SegmentType == "pier" {
		height := 15.0
		if pos, ok := segment.Position3D["height"]; ok {
			if h, conv := pos.(float64); conv {
				height = h
			}
		}

		settlementMm := 0.0
		{
			var sv float64
			e.repo.GetPool().QueryRow(context.Background(),
				`SELECT COALESCE(MAX(value), 0) FROM sensor_data WHERE segment_id=$1 AND sensor_type='settlement' AND timestamp > NOW() - INTERVAL '30 days'`,
				segment.ID).Scan(&sv)
			settlementMm = sv
		}

		pierArea := PIER_WIDTH * PIER_DEPTH
		axialLoadPier := (totalUDL * 5.5) + (pierArea*height*GRAVITY_ACCEL*STRUCTURE_DENSITY/1000.0)*0.5

		adjacentSettlement := settlementMm * 0.3
		differential := math.Abs(settlementMm - adjacentSettlement)
		rotationRad := differential / (5.5 * 1000.0)
		if rotationRad > 0.02 {
			rotationRad = 0.02
		}
		eccentricity := height / 2 * math.Sin(rotationRad)
		pierSectionMod := (PIER_DEPTH * PIER_WIDTH * PIER_WIDTH) / 6.0
		momentFromSettlement := axialLoadPier * 1000.0 * eccentricity
		bendingStress := 0.0
		if pierSectionMod > 0 {
			bendingStress = momentFromSettlement / pierSectionMod / 1000.0
		}

		K_p := 3.0 * degradation.EffectiveE * 1e9 * (PIER_DEPTH * math.Pow(PIER_WIDTH, 3) / 12.0) / math.Pow(height, 3)
		springForce := K_p * (settlementMm / 1000.0) * 0.001
		if !math.IsNaN(springForce) && !math.IsInf(springForce, 0) {
			axialLoadPier += math.Abs(springForce) * 0.05
		}

		result.AxialStress = axialLoadPier / pierArea / 1000.0
		result.BendingStress = bendingStress
		result.MaxStress = (result.AxialStress + math.Abs(bendingStress)) * degradation.StressConcentration
		result.ShearStress = axialLoadPier * math.Tan(rotationRad) / pierArea / 1000.0

		E_Pa := degradation.EffectiveE * 1e9
		I_m4 := (PIER_DEPTH * math.Pow(PIER_WIDTH, 3)) / 12.0
		result.DeflectionMM = math.Min(50.0,
			(axialLoadPier*1000.0*math.Pow(height, 3))/(3*E_Pa*I_m4)*1000)

		for i := range model.Elements {
			frac := float64(i) / math.Max(1, float64(len(model.Elements)-1))
			zPos := frac * height

			N_local := axialLoadPier * (0.8 + 0.4*frac)
			M_local := axialLoadPier * eccentricity * (1 - frac)

			localArea := model.Elements[i].Area
			if localArea < 0.01 {
				localArea = pierArea / 2.0
			}
			sigmaA := N_local / localArea / 1000.0
			localW := (PIER_DEPTH * PIER_WIDTH * PIER_WIDTH) / 12.0
			sigmaB := 0.0
			if localW > 0 {
				sigmaB = M_local / localW / 1000.0
			}

			model.Elements[i].AxialStress = math.Max(0.05, sigmaA)
			model.Elements[i].MaxStress = (sigmaA + math.Abs(sigmaB)) * degradation.StressConcentration
			model.Elements[i].MaterialFactor = degradation.TotalDegradationFactor

			permitted := degradation.EffectiveStrength / DESIGN_SAFETY_FACTOR
			if permitted > 0 {
				model.Elements[i].Utilization = math.Min(3.0, model.Elements[i].MaxStress/permitted)
			}
			_ = zPos
		}
		result.Elements = model.Elements

		result.Converged = true
		result.Iterations = 1
	}

	if result.MaxStress < 0.1 || math.IsNaN(result.MaxStress) || math.IsInf(result.MaxStress, 0) {
		result.MaxStress = segment.DesignStrength * 0.3
	}
	if result.AxialStress < 0 || math.IsNaN(result.AxialStress) {
		result.AxialStress = segment.DesignStrength * 0.15
	}

	return result
}

func (e *StructuralEvaluator) solveThreeHingedArch(
	segment *models.StructureSegment,
	model *FEAModel,
	degradation *DegradationResult,
	q float64,
) (*ArchAnalysisResult, bool, int, float64, string) {

	span := 5.5
	if s, ok := segment.Position3D["span"]; ok {
		if sv, conv := s.(float64); conv {
			span = sv
		}
	}
	riseDesign := span * ARCH_RISE_RATIO
	nElem := len(model.Elements)
	if nElem < 2 {
		nElem = 6
	}

	E_Pa := degradation.EffectiveE * 1e9
	areaAvg := 0.0
	for _, el := range model.Elements {
		areaAvg += el.Area
	}
	if len(model.Elements) > 0 {
		areaAvg /= float64(len(model.Elements))
	}
	if areaAvg < 0.01 {
		areaAvg = ARCH_RIB_THICKNESS * ARCH_WIDTH
	}
	EA := E_Pa * areaAvg
	EI := E_Pa * (ARCH_WIDTH * math.Pow(ARCH_RIB_THICKNESS, 3) / 12.0)
	_ = EA
	_ = EI

	res := &ArchAnalysisResult{
		VerticalReaction: q * span / 2.0,
	}

	rise0 := riseDesign
	for i := 0; i < 5; i++ {
		res.HorizontalThrust = (q * span * span) / (8 * rise0)
		Vc := res.VerticalReaction
		H := res.HorizontalThrust

		elemForcesN := make([]float64, nElem)
		elemForcesM := make([]float64, nElem)
		maxN := 0.0
		maxM := 0.0
		springM := 0.0

		for j := 0; j < nElem; j++ {
			tElem := (float64(j) + 0.5) / float64(nElem)
			xj := tElem * span
			_, zj := archCurve(tElem, span, rise0)
			theta_j := math.Atan2(4*rise0/span*(1-2*tElem), 1)

			V_xj := Vc - q*xj
			Mx := Vc*xj - q*xj*xj/2.0
			Hy := H * zj
			Melem := Mx - Hy

			Nelem := H*math.Cos(theta_j) + V_xj*math.Sin(theta_j)

			elemForcesN[j] = Nelem
			elemForcesM[j] = Melem

			if math.Abs(Nelem) > maxN {
				maxN = math.Abs(Nelem)
			}
			if math.Abs(Melem) > maxM {
				maxM = math.Abs(Melem)
			}
			if j == 0 {
				springM = math.Abs(Melem)
			}
		}

		res.AxialForceByElem = elemForcesN
		res.MomentByElem = elemForcesM
		res.CrownAxialForce = H
		tCrown := 0.5
		Mc := Vc*(span/2.0) - q*(span*span/4.0)/2.0 - H*rise0
		res.CrownMoment = Mc
		res.SpringingMoment = springM
		res.ArchRiseEff = rise0

		if areaAvg > 0 {
			res.MaxAxialStress = maxN / areaAvg / 1000.0
		}
		localW := (ARCH_WIDTH * ARCH_RIB_THICKNESS * ARCH_RIB_THICKNESS) / 6.0
		if localW > 0 {
			res.MaxBendingStress = maxM / localW / 1000.0 * 1000.0
		}

		deltaCrownElastic := 0.0
		if EI > 0 {
			integralM_y := 0.0
			for j := 0; j < nElem; j++ {
				tj := (float64(j) + 0.5) / float64(nElem)
				_, zj := archCurve(tj, span, rise0)
				integralM_y += elemForcesM[j] * zj
			}
			integralM_y *= span / float64(nElem)
			deltaCrownElastic = integralM_y / EI * 1000
			if math.IsNaN(deltaCrownElastic) || math.IsInf(deltaCrownElastic, 0) {
				deltaCrownElastic = (5 * q * 1000 * math.Pow(span, 4)) / (384 * EI) * 1000 * (rise0 / span)
			}
		}
		res.CrownDeflection = deltaCrownElastic

		riseNew := riseDesign - deltaCrownElastic/1000
		if riseNew < riseDesign*0.3 {
			riseNew = riseDesign * 0.3
		}

		errRatio := math.Abs(riseNew-rise0) / riseDesign
		if errRatio < FEA_TOLERANCE {
			return res, true, i + 1, errRatio, ""
		}

		rise0 = FEA_RELAXATION*riseNew + (1-FEA_RELAXATION)*rise0
	}

	return res, false, FEA_MAX_ITERATIONS, FEA_TOLERANCE * 10, "linear_elastic_unconverged"
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
