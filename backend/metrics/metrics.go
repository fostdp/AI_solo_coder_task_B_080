package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	DTUReceived         prometheus.Counter
	DTUByAqueduct       *prometheus.CounterVec
	EvalProcessed       prometheus.Counter
	EvalBySafetyLevel   *prometheus.CounterVec
	EvalDuration        prometheus.Histogram
	EvalErrors          prometheus.Counter
	RepairProcessed     prometheus.Counter
	AlertsPublished     prometheus.Counter
	AlertsBySeverity    *prometheus.CounterVec
	AlertsBuffered      prometheus.Gauge
	PipelineQueueSize   *prometheus.GaugeVec
	DBQueryDuration     prometheus.Histogram
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestsTotal   *prometheus.CounterVec
	FEAIterations       prometheus.Histogram
	FEANonConverge      prometheus.Counter
	SensorValue         *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		DTUReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "aqueduct_dtu_received_total",
			Help: "Total DTU sensor readings received",
		}),
		DTUByAqueduct: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "aqueduct_dtu_received_by_aqueduct_total",
			Help: "DTU readings received by aqueduct ID",
		}, []string{"aqueduct_id", "sensor_type"}),
		EvalProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "aqueduct_eval_processed_total",
			Help: "Total structural evaluations processed",
		}),
		EvalBySafetyLevel: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "aqueduct_eval_safety_level_total",
			Help: "Evaluations by safety level",
		}, []string{"safety_level", "aqueduct_id"}),
		EvalDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "aqueduct_eval_duration_seconds",
			Help:    "Time taken for structural evaluation",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
		}),
		EvalErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "aqueduct_eval_errors_total",
			Help: "Total evaluation errors",
		}),
		RepairProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "aqueduct_repair_processed_total",
			Help: "Total repair recommendations processed",
		}),
		AlertsPublished: promauto.NewCounter(prometheus.CounterOpts{
			Name: "aqueduct_alerts_published_total",
			Help: "Total MQTT alerts published",
		}),
		AlertsBySeverity: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "aqueduct_alerts_severity_total",
			Help: "Alerts by severity level",
		}, []string{"severity", "alert_type"}),
		AlertsBuffered: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "aqueduct_alerts_buffered",
			Help: "Current alerts in offline queue",
		}),
		PipelineQueueSize: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aqueduct_pipeline_queue_size",
			Help: "Current size of each pipeline channel",
		}, []string{"stage"}),
		DBQueryDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "aqueduct_db_query_duration_seconds",
			Help:    "Database query duration",
			Buckets: prometheus.DefBuckets,
		}),
		HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aqueduct_http_request_duration_seconds",
			Help:    "HTTP request duration",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status_code"}),
		HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "aqueduct_http_requests_total",
			Help: "Total HTTP requests",
		}, []string{"method", "path", "status_code"}),
		FEAIterations: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "aqueduct_fea_iterations",
			Help:    "FEA solver iterations to converge",
			Buckets: prometheus.LinearBuckets(1, 1, 15),
		}),
		FEANonConverge: promauto.NewCounter(prometheus.CounterOpts{
			Name: "aqueduct_fea_nonconverge_total",
			Help: "FEA evaluations that did not converge (used fallback)",
		}),
		SensorValue: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aqueduct_sensor_value",
			Help: "Latest sensor value by segment and type",
		}, []string{"segment_id", "sensor_type"}),
	}
}

func (m *Metrics) ObserveEval(duration time.Duration, safetyLevel string, aqueductID string) {
	m.EvalDuration.Observe(duration.Seconds())
	m.EvalProcessed.Inc()
	if safetyLevel != "" {
		m.EvalBySafetyLevel.WithLabelValues(safetyLevel, aqueductID).Inc()
	}
}

func (m *Metrics) ObserveAlert(severity string, alertType string) {
	m.AlertsPublished.Inc()
	m.AlertsBySeverity.WithLabelValues(severity, alertType).Inc()
}

func (m *Metrics) ObserveHTTP(method, path, status string, duration time.Duration) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}

func (m *Metrics) ObserveDTU(aqueductID, sensorType string) {
	m.DTUReceived.Inc()
	m.DTUByAqueduct.WithLabelValues(aqueductID, sensorType).Inc()
}

func (m *Metrics) SetQueueSize(stage string, size int) {
	m.PipelineQueueSize.WithLabelValues(stage).Set(float64(size))
}

func (m *Metrics) SetSensorValue(segmentID, sensorType string, value float64) {
	m.SensorValue.WithLabelValues(segmentID, sensorType).Set(value)
}
