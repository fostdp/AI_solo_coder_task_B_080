package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Timescale    TimescaleConfig
	MQTT         MQTTConfig
	Server       ServerConfig
	Threshold    ThresholdConfig
	FEA          FEAConfig
	MADM         MADMConfig
	Pipeline     PipelineConfig
	Inversion    InversionConfig
	Seismic      SeismicConfig
	Lifetime     LifetimeConfig
	Tourism      TourismConfig
}

type TimescaleConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DB       string
}

type MQTTConfig struct {
	Broker     string
	ClientID   string
	Username   string
	Password   string
	AlertTopic string
	QueueDir   string
}

type ServerConfig struct {
	Host string
	Port int
}

type ThresholdConfig struct {
	LoadCapacityThreshold float64
	SettlementLimitMM     float64
	WeatheringAccelRatio  float64
	StressLimitMpa        float64
	DisplacementLimitMM   float64
}

type FEAConfig struct {
	MaxIterations     int
	Tolerance         float64
	Relaxation        float64
	MinElements       int
	MaxElements       int
	CurvatureThresh   float64
	BaseConcreteFy    float64
	BaseStoneFy       float64
	BaseElasticMod    float64
	WeatheringPower   float64
	AgeFactorBase     float64
	AgeFactorRate     float64
	AqueductAgeYears  float64
	FoundationSpringK float64
	PierRotationLimit float64
}

type MADMConfig struct {
	KNNeighbors        int
	MissingPenalty     float64
	MissingThreshold   float64
	SensitivityPerturb float64
	HeritageBonus      map[string]float64
	BaseWeights        map[string]float64
	BenefitAttrs       []string
	CostAttrs          []string
}

type PipelineConfig struct {
	BufferSize         int
	WorkerCountEval    int
	WorkerCountRepair  int
	WorkerCountAlarm   int
	BatchSizeEval      int
}

type InversionConfig struct {
	MaxCandidates             int
	LeachingRateBase          float64
	CarbonationRateBase       float64
	PHInitialRoman            float64
	PHModern                  float64
	StrengthRetainPower       float64
	MonteCarloSamples         int
	ConfidenceLevel           float64
	L2RegularizationLambda    float64
	BayesianPriorStrength     float64
	NoiseRobustWeight         float64
	OutlierRejectionThreshold float64
	MaxIterations             int
}

type SeismicConfig struct {
	MagnitudeMin             float64
	MagnitudeMax             float64
	PGAComputedLevels        int
	DamageStates             []string
	DamageThresholds         map[string]float64
	ReturnPeriod475PGA       float64
	ReturnPeriod2475PGA      float64
	CapacityReductionFactor  float64
	DuctilityFactor          float64
	SiteClassUncertainty     float64
	SoilAmpUncertainty       float64
	BetaUncertaintyMin       float64
	BetaUncertaintyMax       float64
	UseLiquefactionCheck     bool
	SoilTypePrior            map[string]float64
	IDAWorkerCount           int
}

type LifetimeConfig struct {
	ArrheniusActivationEV       float64
	ReferenceTemperatureC       float64
	SimulationYears             int
	TimeStepsPerYear            int
	ThresholdStrengthRatio      float64
	AcceleratedTempLowC         float64
	AcceleratedTempHighC        float64
	ConfidenceZScore            float64
	HumidityAcceleration        float64
	FreezeThawAcceleration      float64
	AcceleratedToNaturalFactor  float64
	LongTermExposureCalibration float64
	NaturalAgingBiasCorrection  float64
	OutdoorExposureFactor       float64
	ThresholdSafetyFactor       float64
}

type TourismConfig struct {
	SafetyWeight             float64
	HistoricalWeight         float64
	AccessibilityWeight      float64
	EconomicWeight           float64
	CarryingCapacityFactor   float64
	PeakSeasonMultiplier     float64
	RepairCostNormalization  float64
	ConditionThresholds      map[string]float64
	HeritageValueWeight      float64
	ExpertScoreWeight        float64
	UNESCOSiteBonus          float64
	ArchaeologicalSiteBonus  float64
	RareArchitectureBonus    float64
	ExpertJudgmentMatrix     map[string]float64
}

func Load() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Could not load .env file, using environment variables: %v", err)
	}

	return &Config{
		Timescale: TimescaleConfig{
			Host:     getEnv("TIMESCALE_HOST", "localhost"),
			Port:     getEnvInt("TIMESCALE_PORT", 5432),
			User:     getEnv("TIMESCALE_USER", "postgres"),
			Password: getEnv("TIMESCALE_PASSWORD", "postgres"),
			DB:       getEnv("TIMESCALE_DB", "aqueduct_monitor"),
		},
		MQTT: MQTTConfig{
			Broker:     getEnv("MQTT_BROKER", "tcp://localhost:1883"),
			ClientID:   getEnv("MQTT_CLIENT_ID", "aqueduct_monitor"),
			Username:   getEnv("MQTT_USERNAME", "admin"),
			Password:   getEnv("MQTT_PASSWORD", "admin"),
			AlertTopic: getEnv("MQTT_ALERT_TOPIC", "aqueduct/alerts"),
			QueueDir:   getEnv("MQTT_QUEUE_DIR", "./mqtt_queue"),
		},
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8080),
		},
		Threshold: ThresholdConfig{
			LoadCapacityThreshold: getEnvFloat("LOAD_CAPACITY_THRESHOLD", 0.50),
			SettlementLimitMM:     getEnvFloat("SETTLEMENT_LIMIT_MM", 20.0),
			WeatheringAccelRatio:  getEnvFloat("WEATHERING_ACCELERATION_RATIO", 1.5),
			StressLimitMpa:        getEnvFloat("STRESS_LIMIT_MPA", 8.0),
			DisplacementLimitMM:   getEnvFloat("DISPLACEMENT_LIMIT_MM", 15.0),
		},
		FEA: FEAConfig{
			MaxIterations:     getEnvInt("FEA_MAX_ITERATIONS", 50),
			Tolerance:         getEnvFloat("FEA_TOLERANCE", 1e-4),
			Relaxation:        getEnvFloat("FEA_RELAXATION", 0.5),
			MinElements:       getEnvInt("FEA_MIN_ELEMENTS", 6),
			MaxElements:       getEnvInt("FEA_MAX_ELEMENTS", 32),
			CurvatureThresh:   getEnvFloat("FEA_CURVATURE_THRESH", 0.15),
			BaseConcreteFy:    getEnvFloat("FEA_BASE_CONCRETE_FY", 15.0),
			BaseStoneFy:       getEnvFloat("FEA_BASE_STONE_FY", 12.0),
			BaseElasticMod:    getEnvFloat("FEA_BASE_EMOD", 25000.0),
			WeatheringPower:   getEnvFloat("FEA_WEATHERING_POWER", 1.3),
			AgeFactorBase:     getEnvFloat("FEA_AGE_BASE", 0.85),
			AgeFactorRate:     getEnvFloat("FEA_AGE_RATE", 0.15),
			AqueductAgeYears:  getEnvFloat("FEA_AQUEDUCT_AGE", 2000.0),
			FoundationSpringK: getEnvFloat("FEA_FOUNDATION_K", 1e6),
			PierRotationLimit: getEnvFloat("FEA_PIER_ROT_LIMIT", 0.02),
		},
		MADM: MADMConfig{
			KNNeighbors:        getEnvInt("MADM_KNN_NEIGHBORS", 3),
			MissingPenalty:     getEnvFloat("MADM_MISSING_PENALTY", 0.7),
			MissingThreshold:   getEnvFloat("MADM_MISSING_THRESHOLD", 0.4),
			SensitivityPerturb: getEnvFloat("MADM_SENSITIVITY_PERTURB", 0.20),
			HeritageBonus: map[string]float64{
				"ROMAN_CONCRETE": 1.12,
				"LIME_MORTAR":    1.08,
				"MODERN_CEMENT":  1.00,
				"FRP":            0.92,
				"EPOXY":          0.95,
			},
			BaseWeights: map[string]float64{
				"compressive_strength":  0.18,
				"tensile_strength":      0.12,
				"elastic_modulus":       0.08,
				"durability_rating":     0.15,
				"compatibility_rating":  0.12,
				"cost_per_unit":         0.10,
				"ease_of_application":   0.08,
				"environmental_impact":  0.07,
				"aesthetic_match":       0.10,
			},
			BenefitAttrs: []string{
				"compressive_strength", "tensile_strength", "elastic_modulus",
				"durability_rating", "compatibility_rating",
				"ease_of_application", "aesthetic_match",
			},
			CostAttrs: []string{
				"cost_per_unit", "environmental_impact",
			},
		},
		Pipeline: PipelineConfig{
			BufferSize:        getEnvInt("PIPE_BUFFER_SIZE", 200),
			WorkerCountEval:   getEnvInt("PIPE_WORKERS_EVAL", 4),
			WorkerCountRepair: getEnvInt("PIPE_WORKERS_REPAIR", 2),
			WorkerCountAlarm:  getEnvInt("PIPE_WORKERS_ALARM", 2),
			BatchSizeEval:     getEnvInt("PIPE_BATCH_EVAL", 50),
		},
		Inversion: InversionConfig{
			MaxCandidates:             getEnvInt("INV_MAX_CANDIDATES", 5),
			LeachingRateBase:          getEnvFloat("INV_LEACHING_RATE", 0.012),
			CarbonationRateBase:       getEnvFloat("INV_CARBONATION_RATE", 0.008),
			PHInitialRoman:            getEnvFloat("INV_PH_INITIAL", 12.8),
			PHModern:                  getEnvFloat("INV_PH_MODERN", 8.2),
			StrengthRetainPower:       getEnvFloat("INV_STRENGTH_POWER", 0.85),
			MonteCarloSamples:         getEnvInt("INV_MC_SAMPLES", 1000),
			ConfidenceLevel:           getEnvFloat("INV_CONFIDENCE", 0.95),
			L2RegularizationLambda:    getEnvFloat("INV_REG_LAMBDA", 0.15),
			BayesianPriorStrength:     getEnvFloat("INV_BAYES_PRIOR", 0.8),
			NoiseRobustWeight:         getEnvFloat("INV_NOISE_WEIGHT", 0.6),
			OutlierRejectionThreshold: getEnvFloat("INV_OUTLIER_THRESH", 2.5),
			MaxIterations:             getEnvInt("INV_MAX_ITER", 50),
		},
		Seismic: SeismicConfig{
			MagnitudeMin:          getEnvFloat("SEIS_MAG_MIN", 4.0),
			MagnitudeMax:          getEnvFloat("SEIS_MAG_MAX", 8.0),
			PGAComputedLevels:     getEnvInt("SEIS_PGA_LEVELS", 20),
			DamageStates:          []string{"Slight", "Moderate", "Extensive", "Complete"},
			DamageThresholds: map[string]float64{
				"Slight":     0.05,
				"Moderate":   0.15,
				"Extensive":  0.30,
				"Complete":   0.60,
			},
			ReturnPeriod475PGA:    getEnvFloat("SEIS_PGA_475", 0.35),
			ReturnPeriod2475PGA:   getEnvFloat("SEIS_PGA_2475", 0.75),
			CapacityReductionFactor: getEnvFloat("SEIS_CAP_REDUCT", 0.65),
			DuctilityFactor:       getEnvFloat("SEIS_DUCTILITY", 2.5),
			SiteClassUncertainty:  getEnvFloat("SEIS_SITE_UNC", 0.25),
			SoilAmpUncertainty:    getEnvFloat("SEIS_SOIL_UNC", 0.20),
			BetaUncertaintyMin:    getEnvFloat("SEIS_BETA_UNC_MIN", 0.05),
			BetaUncertaintyMax:    getEnvFloat("SEIS_BETA_UNC_MAX", 0.15),
			UseLiquefactionCheck:  getEnvBool("SEIS_LIQUEFACTION", false),
			SoilTypePrior: map[string]float64{
				"A": 0.05, "B": 0.30, "C": 0.40, "D": 0.20, "E": 0.05,
			},
			IDAWorkerCount: getEnvInt("SEIS_IDA_WORKERS", 2),
		},
		Lifetime: LifetimeConfig{
			ArrheniusActivationEV:       getEnvFloat("LIFE_ACT_EV", 0.95),
			ReferenceTemperatureC:       getEnvFloat("LIFE_TREF_C", 20.0),
			SimulationYears:             getEnvInt("LIFE_SIM_YEARS", 100),
			TimeStepsPerYear:            getEnvInt("LIFE_STEPS_YEAR", 12),
			ThresholdStrengthRatio:      getEnvFloat("LIFE_THRESHOLD", 0.50),
			AcceleratedTempLowC:         getEnvFloat("LIFE_ACC_TLOW", 40.0),
			AcceleratedTempHighC:        getEnvFloat("LIFE_ACC_THIGH", 80.0),
			ConfidenceZScore:            getEnvFloat("LIFE_ZSCORE", 1.96),
			HumidityAcceleration:        getEnvFloat("LIFE_HUMID_ACCEL", 1.25),
			FreezeThawAcceleration:      getEnvFloat("LIFE_FT_ACCEL", 1.8),
			AcceleratedToNaturalFactor:  getEnvFloat("LIFE_ACC_NATURAL_FACTOR", 0.78),
			LongTermExposureCalibration: getEnvFloat("LIFE_LONGTERM_CALIB", 0.92),
			NaturalAgingBiasCorrection:  getEnvFloat("LIFE_BIAS_CORRECTION", 0.88),
			OutdoorExposureFactor:       getEnvFloat("LIFE_OUTDOOR_FACTOR", 1.15),
			ThresholdSafetyFactor:       getEnvFloat("LIFE_SAFETY_FACTOR", 1.20),
		},
		Tourism: TourismConfig{
			SafetyWeight:            getEnvFloat("TOUR_W_SAFETY", 0.28),
			HistoricalWeight:        getEnvFloat("TOUR_W_HISTORIC", 0.22),
			AccessibilityWeight:     getEnvFloat("TOUR_W_ACCESS", 0.18),
			EconomicWeight:          getEnvFloat("TOUR_W_ECON", 0.15),
			CarryingCapacityFactor:  getEnvFloat("TOUR_CAP_FACTOR", 0.008),
			PeakSeasonMultiplier:    getEnvFloat("TOUR_PEAK_MULT", 1.6),
			RepairCostNormalization: getEnvFloat("TOUR_COST_NORM", 5000000),
			ConditionThresholds: map[string]float64{
				"excellent": 0.85,
				"good":      0.65,
				"fair":      0.45,
				"poor":      0.25,
			},
			HeritageValueWeight:     getEnvFloat("TOUR_W_HERITAGE", 0.20),
			ExpertScoreWeight:       getEnvFloat("TOUR_W_EXPERT", 0.15),
			UNESCOSiteBonus:         getEnvFloat("TOUR_UNESCO_BONUS", 1.25),
			ArchaeologicalSiteBonus: getEnvFloat("TOUR_ARCH_BONUS", 1.15),
			RareArchitectureBonus:   getEnvFloat("TOUR_RARE_BONUS", 1.10),
			ExpertJudgmentMatrix: map[string]float64{
				"structural_integrity": 0.25,
				"historical_documentation": 0.20,
				"cultural_significance": 0.20,
				"engineering_uniqueness": 0.15,
				"conservation_status":   0.20,
			},
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, exists := os.LookupEnv(key); exists {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val, exists := os.LookupEnv(key); exists {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, exists := os.LookupEnv(key); exists {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
