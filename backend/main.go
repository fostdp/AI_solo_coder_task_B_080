package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"

	"aqueduct-monitor/config"
	"aqueduct-monitor/database"
	"aqueduct-monitor/evaluation"
	"aqueduct-monitor/handlers"
	"aqueduct-monitor/inversion"
	"aqueduct-monitor/lifetime"
	"aqueduct-monitor/metrics"
	"aqueduct-monitor/mqtt"
	"aqueduct-monitor/pipeline"
	"aqueduct-monitor/recommendation"
	"aqueduct-monitor/repository"
	"aqueduct-monitor/seismic"
	"aqueduct-monitor/tourism"
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

	inverter := inversion.NewConcreteInverter(repo, cfg)
	seismicAnalyzer := seismic.NewVulnerabilityAnalyzer(repo, cfg)
	lifePredictor := lifetime.NewLifetimePredictor(repo, cfg)
	tourismPlanner := tourism.NewTourismPlanner(repo, cfg)

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

	appMetrics := metrics.NewMetrics()

	pipe := pipeline.NewPipeline(cfg, repo, mqttClient)
	pipe.SetMetrics(appMetrics)
	pipeCtx, pipeCancel := context.WithCancel(context.Background())
	defer pipeCancel()
	go func() {
		if err := pipe.Start(pipeCtx); err != nil && err != context.Canceled {
			log.Printf("Pipeline error: %v", err)
		}
	}()

	h := handlers.New(repo, cfg, evaluator, recommender, mqttClient, pipe, appMetrics)
	fh := handlers.NewFeatureHandlers(h, inverter, seismicAnalyzer, lifePredictor, tourismPlanner)

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

	r.Use(gzip.Gzip(gzip.DefaultCompression))

	r.Use(prometheusMiddleware(appMetrics))

	gin.SetMode(gin.ReleaseMode)

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"version":   "1.0.0",
			"service":   "aqueduct-monitor",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	pprofGroup := r.Group("/debug/pprof")
	{
		pprofGroup.GET("/", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/cmdline", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/profile", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/symbol", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/trace", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/heap", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/goroutine", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/block", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/mutex", gin.WrapF(http.DefaultServeMux.ServeHTTP))
		pprofGroup.GET("/threadcreate", gin.WrapF(http.DefaultServeMux.ServeHTTP))
	}

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

		// ============================================
		// Feature 1: 古罗马混凝土耐久性反演
		// ============================================
		api.POST("/inversion/invert", fh.InvertConcrete)
		api.GET("/inversion/formulas", fh.GetFormulas)
		api.GET("/inversion/aqueducts/:aqueduct_id", fh.GetAqueductInversionResults)

		// ============================================
		// Feature 2: 地震易损性评估
		// ============================================
		api.GET("/seismic/earthquakes", fh.GetHistoricalEarthquakes)
		api.GET("/seismic/risks/:aqueduct_id", fh.AnalyzeSeismicRisk)
		api.GET("/seismic/risks", fh.GetAllSeismicRisks)
		api.GET("/seismic/fragility/:segment_id", fh.GetFragilityCurve)
		api.GET("/seismic/ida/:segment_id", fh.AnalyzeIncrementalDynamic)

		// ============================================
		// Feature 3: 修复材料长期性能预测
		// ============================================
		api.POST("/lifetime/predict", fh.PredictMaterialLifetime)
		api.GET("/lifetime/materials/:material_id", fh.GetMaterialPredictions)

		// ============================================
		// Feature 4: 多水道对比与旅游规划
		// ============================================
		api.POST("/tourism/compare", fh.CompareAqueducts)
		api.GET("/tourism/comparisons", fh.GetRecentComparisons)
	}

	r.GET("/", func(c *gin.Context) {
		c.File("../frontend/index.html")
	})
	r.Static("/css", "../frontend/css")
	r.Static("/js", "../frontend/js")
	r.StaticFile("/favicon.ico", "../frontend/favicon.ico")

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
		log.Println("  === 新增Feature API ===")
		log.Println("  【古罗马混凝土反演】")
		log.Println("    POST /api/inversion/invert         - 混凝土配比反演")
		log.Println("    GET  /api/inversion/formulas       - 古罗马配方库")
		log.Println("    GET  /api/inversion/aqueducts/:id  - 水道反演历史")
		log.Println("  【地震易损性】")
		log.Println("    GET  /api/seismic/earthquakes      - 历史地震数据")
		log.Println("    GET  /api/seismic/risks/:id        - 水道地震风险分析")
		log.Println("    GET  /api/seismic/risks            - 全水道地震风险地图")
		log.Println("    GET  /api/seismic/fragility/:id    - 段易损性曲线")
		log.Println("    GET  /api/seismic/ida/:id          - 增量动力分析")
		log.Println("  【长期性能预测】")
		log.Println("    POST /api/lifetime/predict         - 修复材料寿命预测")
		log.Println("    GET  /api/lifetime/materials/:id   - 材料预测历史")
		log.Println("  【旅游规划对比】")
		log.Println("    POST /api/tourism/compare          - 多水道对比分析")
		log.Println("    GET  /api/tourism/comparisons      - 历史对比记录")
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

func prometheusMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()

		c.Next()

		duration := time.Since(start)
		statusCode := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method

		if path != "" {
			m.ObserveHTTP(method, path, statusCode, duration)
		}
	}
}
