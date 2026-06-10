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
}

type TimescaleConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DB       string
}

type MQTTConfig struct {
	Broker      string
	ClientID    string
	Username    string
	Password    string
	AlertTopic  string
}

type ServerConfig struct {
	Host string
	Port int
}

type ThresholdConfig struct {
	LoadCapacityThreshold    float64
	SettlementLimitMM        float64
	WeatheringAccelRatio     float64
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
		},
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8080),
		},
		Threshold: ThresholdConfig{
			LoadCapacityThreshold: getEnvFloat("LOAD_CAPACITY_THRESHOLD", 0.50),
			SettlementLimitMM:     getEnvFloat("SETTLEMENT_LIMIT_MM", 20.0),
			WeatheringAccelRatio:  getEnvFloat("WEATHERING_ACCELERATION_RATIO", 1.5),
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
