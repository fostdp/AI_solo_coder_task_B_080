package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Timescale TimescaleConfig
	MQTT      MQTTConfig
	Server    ServerConfig
	Threshold ThresholdConfig
	FEA       FEAConfig
	MADM      MADMConfig
	Pipeline  PipelineConfig
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
