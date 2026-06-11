package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"aqueduct-monitor/config"
	"aqueduct-monitor/durability_inverter"
	"aqueduct-monitor/models"
)

type InversionServer struct {
	cfg    *config.InversionConfig
	router *mux.Router
	server *http.Server
}

type InversionRequest struct {
	SegmentID          string  `json:"segment_id"`
	ObservedWeathering float64 `json:"observed_weathering_cm"`
	ObservedStrength   float64 `json:"observed_strength_mpa"`
	ObservedPH         float64 `json:"observed_ph"`
	AgeYears           float64 `json:"age_years"`
}

type InversionResponse struct {
	Success      bool                                `json:"success"`
	Message      string                              `json:"message,omitempty"`
	Result       *durability_inverter.InversionResult `json:"result,omitempty"`
	Confidence   *durability_inverter.ConfidenceMetrics `json:"confidence,omitempty"`
	BestFormula  *models.RomanConcreteFormula         `json:"best_formula,omitempty"`
	Candidates   []models.InversionFormulaCandidate   `json:"candidates,omitempty"`
	ProcessTimeMs int64                              `json:"process_time_ms"`
}

func NewInversionServer(cfg *config.InversionConfig) *InversionServer {
	s := &InversionServer{
		cfg:    cfg,
		router: mux.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *InversionServer) setupRoutes() {
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/api/v1/invert", s.handleInvert).Methods("POST")
	s.router.HandleFunc("/api/v1/formulas", s.handleGetFormulas).Methods("GET")
	s.router.HandleFunc("/api/v1/batch-invert", s.handleBatchInvert).Methods("POST")
}

func (s *InversionServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"service":   "inversion-service",
		"timestamp": time.Now().UTC(),
	})
}

func (s *InversionServer) handleInvert(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Content-Type", "application/json")

	var req InversionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(InversionResponse{
			Success: false,
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.ObservedWeathering <= 0 || req.ObservedStrength <= 0 || req.AgeYears <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(InversionResponse{
			Success: false,
			Message: "Observed weathering, strength, and age must be positive",
		})
		return
	}

	if req.ObservedPH == 0 {
		req.ObservedPH = 9.5
	}

	formulas := durability_inverter.BuildDefaultFormulas()
	hypotheses := durability_inverter.BuildHypotheses(formulas, s.cfg)

	result := durability_inverter.SolveInversion(
		hypotheses,
		req.ObservedWeathering,
		req.ObservedStrength,
		req.ObservedPH,
		req.AgeYears,
		s.cfg,
	)

	confidence := durability_inverter.ComputeConfidence(
		result.Candidates,
		result.Residuals,
		result.BestIdx,
		result.RawResiduals,
		s.cfg,
	)

	bestFormula := &hypotheses[result.BestIdx].Formula
	processTime := time.Since(startTime).Milliseconds()

	json.NewEncoder(w).Encode(InversionResponse{
		Success:       true,
		Result:        result,
		Confidence:    &confidence,
		BestFormula:   bestFormula,
		Candidates:    result.Candidates,
		ProcessTimeMs: processTime,
	})
}

func (s *InversionServer) handleGetFormulas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	formulas := durability_inverter.BuildDefaultFormulas()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":    len(formulas),
		"formulas": formulas,
	})
}

type BatchInversionRequest struct {
	Requests []InversionRequest `json:"requests"`
}

type BatchInversionResponse struct {
	Success     bool                `json:"success"`
	Results     []InversionResponse `json:"results"`
	TotalTimeMs int64               `json:"total_time_ms"`
}

func (s *InversionServer) handleBatchInvert(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Content-Type", "application/json")

	var batchReq BatchInversionRequest
	if err := json.NewDecoder(r.Body).Decode(&batchReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BatchInversionResponse{
			Success: false,
		})
		return
	}

	results := make([]InversionResponse, len(batchReq.Requests))
	formulas := durability_inverter.BuildDefaultFormulas()
	hypotheses := durability_inverter.BuildHypotheses(formulas, s.cfg)

	for i, req := range batchReq.Requests {
		reqStart := time.Now()

		if req.ObservedPH == 0 {
			req.ObservedPH = 9.5
		}

		result := durability_inverter.SolveInversion(
			hypotheses,
			req.ObservedWeathering,
			req.ObservedStrength,
			req.ObservedPH,
			req.AgeYears,
			s.cfg,
		)

		confidence := durability_inverter.ComputeConfidence(
			result.Candidates,
			result.Residuals,
			result.BestIdx,
			result.RawResiduals,
			s.cfg,
		)

		bestFormula := &hypotheses[result.BestIdx].Formula
		results[i] = InversionResponse{
			Success:       true,
			Result:        result,
			Confidence:    &confidence,
			BestFormula:   bestFormula,
			Candidates:    result.Candidates,
			ProcessTimeMs: time.Since(reqStart).Milliseconds(),
		}
	}

	json.NewEncoder(w).Encode(BatchInversionResponse{
		Success:     true,
		Results:     results,
		TotalTimeMs: time.Since(startTime).Milliseconds(),
	})
}

func (s *InversionServer) Start(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Inversion service starting on %s", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down inversion service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %v", err)
	}

	log.Println("Inversion service exited properly")
	return nil
}

func main() {
	addr := flag.String("addr", ":8081", "Server address")
	flag.Parse()

	cfg := config.DefaultConfig()
	server := NewInversionServer(&cfg.Inversion)

	if err := server.Start(*addr); err != nil {
		log.Fatal(err)
	}
}
