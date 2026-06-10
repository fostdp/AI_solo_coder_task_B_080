package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"

	"aqueduct-monitor/config"
	"aqueduct-monitor/database"
	"aqueduct-monitor/evaluation"
	"aqueduct-monitor/handlers"
	"aqueduct-monitor/mqtt"
	"aqueduct-monitor/recommendation"
	"aqueduct-monitor/repository"
)

func main() {
	log.Println("=")
	log.Println("  古罗马水道工程结构健康与现代修复评估系统")
	log.Println("  Aqueduct Structural Health Monitoring System")
	log.Println("=")
	log.Println()

	cfg := config.Load()

	if err := database.Connect(&cfg.Timescale); err != nil {
		log.Fatalf("Failed to connect to TimescaleDB: %v", err)
	}
	defer database.Close()

	repo := repository.New(database.GetPool())

	evaluator := evaluation.NewStructuralEvaluator(repo, cfg)
	recommender := recommendation.NewRepairRecommender(repo)

	mqttClient, err := mqtt.NewAlertPublisher(&cfg.MQTT, repo)
	if err != nil {
		log.Printf("Warning: MQTT initialization issue: %v", err)
	}
	if mqttClient != nil {
		defer mqttClient.Close()
		go func() {
			time.Sleep(15 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mqttClient.RepublishPendingAlerts(ctx)
		}()
	}

	h := handlers.New(repo, cfg, evaluator, recommender, mqttClient)

	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[%s] %s %d %s %s\n",
			param.TimeStamp.Format("15:04:05"),
			param.Method,
			param.StatusCode,
			param.Path,
			param.Latency.Round(time.Millisecond),
		)
	}))
	r.Use(gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"version":   "1.0.0",
			"service":   "aqueduct-monitor",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	api := r.Group("/api")
	{
		api.POST("/dtu/submit", h.SubmitSensorData)

		api.GET("/aqueducts", h.GetAqueducts)
		api.GET("/aqueducts/:id", h.GetAqueductDetail)

		api.GET("/segments", h.GetAllSegments)
		api.GET("/segments/:id", h.GetSegmentDetail)
		api.GET("/segments/:segment_id/repair", h.GetRepairRecommendation)

		api.GET("/sensors/:sensor_id/trend", h.GetSensorTrend)

		api.GET("/alerts", h.GetAlerts)
		api.GET("/stats", h.GetStats)

		api.GET("/materials", h.GetRepairMaterials)

		api.POST("/evaluation/run", h.RunFullEvaluation)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	cronScheduler := cron.New()
	_, err = cronScheduler.AddFunc("0 * * * *", func() {
		log.Println("[Cron] Running hourly structural evaluation...")
		ctx := context.Background()
		segments, err := repo.GetAllSegmentsWithStatus(ctx, nil)
		if err != nil {
			log.Printf("[Cron] ERROR loading segments: %v", err)
			return
		}
		alertCount := 0
		for i := range segments {
			alerts, err := evaluator.EvaluateSegment(ctx, segments[i].ID)
			if err == nil {
				for _, a := range alerts {
					if mqttClient != nil {
						mqttClient.PublishAlert(ctx, a)
					}
					alertCount++
				}
			}
		}
		log.Printf("[Cron] Hourly evaluation completed: %d segments, %d new alerts", len(segments), alertCount)
	})
	if err != nil {
		log.Printf("Warning: Could not schedule hourly evaluation cron: %v", err)
	}
	cronScheduler.Start()
	defer cronScheduler.Stop()

	go func() {
		log.Printf("✓ HTTP Server starting on %s", addr)
		log.Println("  API Routes:")
		log.Println("    POST /api/dtu/submit          - DTU传感器数据上报")
		log.Println("    GET  /api/aqueducts           - 水道列表")
		log.Println("    GET  /api/aqueducts/:id       - 水道详情")
		log.Println("    GET  /api/segments            - 结构段列表（3D模型用）")
		log.Println("    GET  /api/segments/:id        - 结构段详情+1年趋势")
		log.Println("    GET  /api/segments/:id/repair - 修复方案推荐")
		log.Println("    GET  /api/sensors/:id/trend   - 传感器历史趋势")
		log.Println("    GET  /api/alerts              - 告警列表")
		log.Println("    GET  /api/stats               - 综合统计")
		log.Println("    GET  /api/materials           - 修复材料库")
		log.Println("    POST /api/evaluation/run      - 触发全量评估")
		log.Println()

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("\nReceived signal: %v. Shutting down gracefully...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}

	log.Println("Server exited successfully")
}
