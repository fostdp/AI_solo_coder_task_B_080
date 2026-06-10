package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"aqueduct-monitor/config"
)

var Pool *pgxpool.Pool

func Connect(cfg *config.TimescaleConfig) error {
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable&pool_max_conns=50",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DB,
	)

	var err error
	maxAttempts := 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		Pool, err = pgxpool.New(ctx, connStr)
		cancel()
		
		if err == nil {
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			err = Pool.Ping(ctx)
			cancel()
			if err == nil {
				log.Printf("✓ Successfully connected to TimescaleDB (attempt %d)", attempt)
				log.Printf("  Host: %s:%d | Database: %s | Pool: 50 connections", cfg.Host, cfg.Port, cfg.DB)
				return nil
			}
		}
		
		if attempt < maxAttempts {
			log.Printf("Connection attempt %d/%d failed: %v. Retrying in 3s...", attempt, maxAttempts, err)
			time.Sleep(3 * time.Second)
		}
	}

	return fmt.Errorf("failed to connect to TimescaleDB after %d attempts: %w", maxAttempts, err)
}

func Close() {
	if Pool != nil {
		Pool.Close()
		log.Println("Database connection pool closed")
	}
}

func GetPool() *pgxpool.Pool {
	return Pool
}
