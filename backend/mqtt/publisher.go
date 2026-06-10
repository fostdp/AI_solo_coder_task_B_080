package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "eclipse.org/paho.mqtt.golang"
	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type AlertPublisher struct {
	client mqtt.Client
	cfg    *config.MQTTConfig
	repo   *repository.Repository
}

type MQTTAlertPayload struct {
	ID             string                 `json:"alert_id"`
	AqueductID     string                 `json:"aqueduct_id"`
	SegmentID      string                 `json:"segment_id,omitempty"`
	SensorID       string                 `json:"sensor_id,omitempty"`
	AlertType      string                 `json:"alert_type"`
	Severity       string                 `json:"severity"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	ThresholdValue float64                `json:"threshold_value,omitempty"`
	MeasuredValue  float64                `json:"measured_value,omitempty"`
	Unit           string                 `json:"unit,omitempty"`
	TriggeredAt    time.Time              `json:"triggered_at"`
	PublishedAt    time.Time              `json:"published_at"`
	Source         string                 `json:"source"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

var publishCounter uint64

func NewAlertPublisher(cfg *config.MQTTConfig, repo *repository.Repository) (*AlertPublisher, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "_" + uuid.New().String()[:8])
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetWriteTimeout(30 * time.Second)
	opts.SetMaxReconnectInterval(2 * time.Minute)

	opts.OnConnect = func(c mqtt.Client) {
		log.Printf("✓ MQTT connected to %s", cfg.Broker)
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		log.Printf("⚠ MQTT connection lost: %v. Reconnecting...", err)
	}

	opts.OnReconnecting = func(c mqtt.Client, opts *mqtt.ClientOptions) {
		log.Printf("↻ MQTT reconnecting to %s...", cfg.Broker)
	}

	client := mqtt.NewClient(opts)

	maxAttempts := 5
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("Connecting MQTT (attempt %d/%d)...", attempt, maxAttempts)
		token := client.Connect()
		if token.WaitTimeout(10*time.Second) && token.Error() == nil {
			lastErr = nil
			break
		}
		lastErr = token.Error()
		if attempt < maxAttempts {
			log.Printf("MQTT connect attempt %d failed: %v. Waiting...", attempt, lastErr)
			time.Sleep(3 * time.Second)
		}
	}

	if lastErr != nil {
		log.Printf("⚠ Warning: Could not connect to MQTT broker %s: %v. Alerts will be stored locally only.", cfg.Broker, lastErr)
	}

	return &AlertPublisher{
		client: client,
		cfg:    cfg,
		repo:   repo,
	}, nil
}

func (p *AlertPublisher) PublishAlert(ctx context.Context, alert *models.Alert) error {
	if alert == nil {
		return fmt.Errorf("nil alert")
	}

	segmentID := ""
	if alert.SegmentID != nil {
		segmentID = alert.SegmentID.String()
	}
	sensorID := ""
	if alert.SensorID != nil {
		sensorID = alert.SensorID.String()
	}

	payload := MQTTAlertPayload{
		ID:             alert.ID.String(),
		AqueductID:     alert.AqueductID.String(),
		SegmentID:      segmentID,
		SensorID:       sensorID,
		AlertType:      alert.AlertType,
		Severity:       alert.Severity,
		Title:          alert.Title,
		Description:    alert.Description,
		ThresholdValue: alert.ThresholdValue,
		MeasuredValue:  alert.MeasuredValue,
		Unit:           alert.Unit,
		TriggeredAt:    alert.TriggeredAt,
		PublishedAt:    time.Now().UTC(),
		Source:         "aqueduct_monitor_backend",
		Metadata: map[string]interface{}{
			"aqueduct_name": alert.AqueductName,
		},
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MQTT payload: %w", err)
	}

	topic := p.buildTopic(alert)

	published := false
	messageID := ""

	if p.client != nil && p.client.IsConnectionOpen() {
		qos := byte(1)
		retained := alert.Severity == "CRITICAL" || alert.Severity == "EMERGENCY"

		publishCounter++
		messageID = fmt.Sprintf("msg-%d-%d", publishCounter, time.Now().UnixNano())

		token := p.client.Publish(topic, qos, retained, data)
		if token.WaitTimeout(15 * time.Second) {
			if token.Error() == nil {
				published = true
				log.Printf("✓ MQTT published alert [%s] %s to topic %s (qos=%d, retained=%v)",
					alert.Severity, alert.Title, topic, qos, retained)
			} else {
				log.Printf("✗ MQTT publish failed for alert %s: %v", alert.ID, token.Error())
			}
		} else {
			log.Printf("✗ MQTT publish timeout for alert %s", alert.ID)
		}
	} else {
		log.Printf("⚠ MQTT not connected, alert %s stored in DB only", alert.ID)
	}

	if published || p.repo != nil {
		updateErr := p.repo.UpdateAlertMQTT(ctx, alert.ID, published, messageID)
		if updateErr != nil {
			log.Printf("Warning: Failed to update MQTT status for alert %s: %v", alert.ID, updateErr)
		}
	}

	if !published {
		return fmt.Errorf("MQTT publish failed for alert %s", alert.ID)
	}

	return nil
}

func (p *AlertPublisher) buildTopic(alert *models.Alert) string {
	base := p.cfg.AlertTopic
	severity := "info"
	switch alert.Severity {
	case "EMERGENCY":
		severity = "emergency"
	case "CRITICAL":
		severity = "critical"
	case "WARNING":
		severity = "warning"
	}

	shortID := ""
	if len(alert.AqueductID.String()) >= 8 {
		shortID = alert.AqueductID.String()[:8]
	}

	return fmt.Sprintf("%s/%s/%s/%s", base, severity, shortID, alert.AlertType)
}

func (p *AlertPublisher) RepublishPendingAlerts(ctx context.Context) (int, error) {
	if p.repo == nil {
		return 0, fmt.Errorf("repository not available")
	}

	alerts, err := p.repo.GetActiveAlerts(ctx, nil, 500)
	if err != nil {
		return 0, fmt.Errorf("failed to load pending alerts: %w", err)
	}

	published := 0
	for i := range alerts {
		if !alerts[i].MQTTPublished {
			a := alerts[i]
			err := p.PublishAlert(ctx, &a)
			if err == nil {
				published++
			}
		}
	}

	if published > 0 {
		log.Printf("✓ Re-published %d pending alerts via MQTT", published)
	}

	return published, nil
}

func (p *AlertPublisher) Close() {
	if p.client != nil {
		p.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}
