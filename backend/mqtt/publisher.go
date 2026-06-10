package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	mqtt "eclipse.org/paho.mqtt.golang"
	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
	"aqueduct-monitor/repository"
)

type AlertPublisher struct {
	client        mqtt.Client
	cfg           *config.MQTTConfig
	repo          *repository.Repository
	offlineQueue  *OfflineQueue
	reconnectDone chan struct{}
	closed        bool
	mu            sync.Mutex
}

type QueuedMessage struct {
	ID          string          `json:"id"`
	Topic       string          `json:"topic"`
	QoS         byte            `json:"qos"`
	Retained    bool            `json:"retained"`
	Payload     json.RawMessage `json:"payload"`
	AlertID     string          `json:"alert_id"`
	CreatedAt   time.Time       `json:"created_at"`
	RetryCount  int             `json:"retry_count"`
	NextRetryAt time.Time       `json:"next_retry_at"`
	AlertJSON   json.RawMessage `json:"alert_json,omitempty"`
}

type OfflineQueue struct {
	messages []QueuedMessage
	path     string
	mu       sync.Mutex
	maxSize  int
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

const (
	OFFLINE_QUEUE_MAX     = 5000
	OFFLINE_FLUSH_INTERVAL = 5 * time.Second
	RETRY_BASE_DELAY      = 2 * time.Second
	RETRY_MAX_DELAY       = 10 * time.Minute
	RETRY_MAX_ATTEMPTS    = 50
	PUBLISH_TIMEOUT       = 15 * time.Second
)

var publishCounter uint64

func NewOfflineQueue(dataDir string) (*OfflineQueue, error) {
	if dataDir == "" {
		dataDir = "./mqtt_queue"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create queue dir: %w", err)
	}
	path := filepath.Join(dataDir, "offline_messages.json")

	q := &OfflineQueue{
		messages: make([]QueuedMessage, 0),
		path:     path,
		maxSize:  OFFLINE_QUEUE_MAX,
	}

	if err := q.load(); err != nil {
		log.Printf("Warning: failed to load offline queue: %v (starting empty)", err)
	}
	return q, nil
}

func (q *OfflineQueue) load() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, &q.messages)
}

func (q *OfflineQueue) save() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := json.MarshalIndent(q.messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.path, data, 0644)
}

func (q *OfflineQueue) Enqueue(msg QueuedMessage) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.messages) >= q.maxSize {
		dropCount := len(q.messages) - q.maxSize + 1
		q.messages = q.messages[dropCount:]
		log.Printf("⚠ Offline queue full, dropped %d oldest messages", dropCount)
	}

	q.messages = append(q.messages, msg)
	return len(q.messages), nil
}

func (q *OfflineQueue) DequeueEligible(now time.Time, maxCount int) []QueuedMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	eligible := make([]QueuedMessage, 0, maxCount)
	remaining := make([]QueuedMessage, 0, len(q.messages))

	for _, msg := range q.messages {
		if len(eligible) < maxCount && now.After(msg.NextRetryAt) {
			eligible = append(eligible, msg)
		} else {
			remaining = append(remaining, msg)
		}
	}
	q.messages = remaining
	return eligible
}

func (q *OfflineQueue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	found := false
	newList := make([]QueuedMessage, 0, len(q.messages))
	for _, msg := range q.messages {
		if msg.ID == id {
			found = true
			continue
		}
		newList = append(newList, msg)
	}
	q.messages = newList
	return found
}

func (q *OfflineQueue) Retry(msg QueuedMessage) (bool, error) {
	msg.RetryCount++
	if msg.RetryCount >= RETRY_MAX_ATTEMPTS {
		log.Printf("⚠ Dropping message %s after %d retries", msg.ID, msg.RetryCount)
		return false, nil
	}
	delay := RETRY_BASE_DELAY * time.Duration(1<<uint(msg.RetryCount-1))
	if delay > RETRY_MAX_DELAY {
		delay = RETRY_MAX_DELAY
	}
	msg.NextRetryAt = time.Now().UTC().Add(delay)

	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, msg)
	return true, nil
}

func (q *OfflineQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages)
}

func NewAlertPublisher(cfg *config.MQTTConfig, repo *repository.Repository) (*AlertPublisher, error) {
	dataDir := "./mqtt_queue"
	if os.Getenv("MQTT_QUEUE_DIR") != "" {
		dataDir = os.Getenv("MQTT_QUEUE_DIR")
	}
	offlineQueue, err := NewOfflineQueue(dataDir)
	if err != nil {
		return nil, fmt.Errorf("init offline queue: %w", err)
	}
	if sz := offlineQueue.Size(); sz > 0 {
		log.Printf("ℹ Loaded %d messages from offline queue", sz)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "_" + uuid.New().String()[:8])
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetCleanSession(false)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetWriteTimeout(30 * time.Second)
	opts.SetMaxReconnectInterval(2 * time.Minute)
	opts.SetOrderMatters(false)
	opts.SetResumeSubs(true)

	p := &AlertPublisher{
		cfg:          cfg,
		repo:         repo,
		offlineQueue: offlineQueue,
		reconnectDone: make(chan struct{}, 1),
	}

	opts.OnConnect = func(c mqtt.Client) {
		log.Printf("✓ MQTT connected to %s", cfg.Broker)
		select {
		case p.reconnectDone <- struct{}{}:
		default:
		}
		go p.flushOfflineQueue()
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		log.Printf("⚠ MQTT connection lost: %v. Offline queue active (size: %d)",
			err, p.offlineQueue.Size())
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
		log.Printf("⚠ Warning: Could not connect to MQTT broker %s: %v. All alerts will be buffered offline (queue size: %d).",
			cfg.Broker, lastErr, p.offlineQueue.Size())
	}

	p.client = client

	go p.flushLoop()
	go p.autoSaveLoop()

	return p, nil
}

func (p *AlertPublisher) isConnected() bool {
	if p == nil {
		return false
	}
	return p.client != nil && p.client.IsConnectionOpen()
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

	payloadBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MQTT payload: %w", err)
	}

	alertBytes, _ := json.Marshal(alert)

	topic := p.buildTopic(alert)

	qos := byte(1)
	retained := alert.Severity == "CRITICAL" || alert.Severity == "EMERGENCY"

	publishCounter++
	messageID := fmt.Sprintf("msg-%d-%d", publishCounter, time.Now().UnixNano())

	queued := QueuedMessage{
		ID:          messageID,
		Topic:       topic,
		QoS:         qos,
		Retained:    retained,
		Payload:     payloadBytes,
		AlertID:     alert.ID.String(),
		CreatedAt:   time.Now().UTC(),
		RetryCount:  0,
		NextRetryAt: time.Now().UTC(),
		AlertJSON:   alertBytes,
	}

	published := false
	if p.isConnected() {
		published = p.attemptPublish(queued)
	}

	if !published {
		queueSize, err := p.offlineQueue.Enqueue(queued)
		if err != nil {
			log.Printf("✗ Failed to enqueue alert %s: %v", alert.ID, err)
		} else {
			log.Printf("⚠ Alert %s buffered offline (queue: %d). Topic: %s",
				alert.ID, queueSize, topic)
		}
	}

	if p.repo != nil {
		updateErr := p.repo.UpdateAlertMQTT(ctx, alert.ID, published, messageID)
		if updateErr != nil {
			log.Printf("Warning: Failed to update MQTT status for alert %s: %v", alert.ID, updateErr)
		}
	}

	if !published {
		_ = p.offlineQueue.save()
		return fmt.Errorf("MQTT publish buffered offline for alert %s", alert.ID)
	}

	return nil
}

func (p *AlertPublisher) attemptPublish(msg QueuedMessage) bool {
	if !p.isConnected() {
		return false
	}

	token := p.client.Publish(msg.Topic, msg.QoS, msg.Retained, msg.Payload)
	if !token.WaitTimeout(PUBLISH_TIMEOUT) {
		log.Printf("✗ MQTT publish timeout for alert %s (msg %s)", msg.AlertID, msg.ID)
		return false
	}
	if token.Error() != nil {
		log.Printf("✗ MQTT publish failed for alert %s: %v", msg.AlertID, token.Error())
		return false
	}

	log.Printf("✓ MQTT published alert [%s] to %s (qos=%d, retained=%v, attempt=%d)",
		msg.AlertID[:8], msg.Topic, msg.QoS, msg.Retained, msg.RetryCount+1)
	return true
}

func (p *AlertPublisher) flushOfflineQueue() {
	time.Sleep(100 * time.Millisecond)

	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return
	}

	batchSize := 50
	flushCount := 0
	failCount := 0

	for {
		msgs := p.offlineQueue.DequeueEligible(time.Now().UTC(), batchSize)
		if len(msgs) == 0 {
			break
		}
		if !p.isConnected() {
			for _, m := range msgs {
				_, _ = p.offlineQueue.Retry(m)
			}
			break
		}
		for _, msg := range msgs {
			if p.attemptPublish(msg) {
				p.offlineQueue.Remove(msg.ID)
				flushCount++
				if p.repo != nil {
					alertID, err := uuid.Parse(msg.AlertID)
					if err == nil {
						_ = p.repo.UpdateAlertMQTT(context.Background(), alertID, true, msg.ID)
					}
				}
			} else {
				reenqueued, _ := p.offlineQueue.Retry(msg)
				if !reenqueued {
					failCount++
				}
				if !p.isConnected() {
					break
				}
			}
		}
	}

	if flushCount > 0 || failCount > 0 {
		log.Printf("↻ Offline flush: sent %d, dropped %d, remaining %d",
			flushCount, failCount, p.offlineQueue.Size())
		if flushCount > 0 {
			_ = p.offlineQueue.save()
		}
	}
}

func (p *AlertPublisher) flushLoop() {
	ticker := time.NewTicker(OFFLINE_FLUSH_INTERVAL)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return
		}
		if p.offlineQueue.Size() > 0 && p.isConnected() {
			p.flushOfflineQueue()
		}
	}
}

func (p *AlertPublisher) autoSaveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return
		}
		if p.offlineQueue.Size() > 0 {
			if err := p.offlineQueue.save(); err != nil {
				log.Printf("Warning: failed to save offline queue: %v", err)
			}
		}
	}
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

	if p.offlineQueue != nil {
		if sz := p.offlineQueue.Size(); sz > 0 {
			log.Printf("ℹ Offline queue size after DB republish: %d", sz)
			if p.isConnected() {
				p.flushOfflineQueue()
			}
		}
	}

	if published > 0 {
		log.Printf("✓ Re-published %d pending alerts via MQTT", published)
	}

	return published, nil
}

func (p *AlertPublisher) FlushOfflineQueue() int {
	if p.offlineQueue.Size() == 0 {
		return 0
	}

	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return 0
	}

	beforeSize := p.offlineQueue.Size()
	p.flushOfflineQueue()
	afterSize := p.offlineQueue.Size()

	return beforeSize - afterSize
}

func (p *AlertPublisher) QueueStats() map[string]interface{} {
	return map[string]interface{}{
		"offline_size":   p.offlineQueue.Size(),
		"is_connected":   p.isConnected(),
		"max_queue_size": OFFLINE_QUEUE_MAX,
	}
}

func (p *AlertPublisher) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	if p.offlineQueue != nil && p.offlineQueue.Size() > 0 {
		if err := p.offlineQueue.save(); err != nil {
			log.Printf("Warning: failed to save offline queue on close: %v", err)
		} else {
			log.Printf("✓ Saved %d offline messages on shutdown", p.offlineQueue.Size())
		}
	}

	if p.client != nil {
		p.client.Disconnect(1000)
		log.Println("MQTT client disconnected")
	}
}
