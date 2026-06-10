package dtu_receiver

import (
	"context"
	"sync"
	"time"

	"aqueduct-monitor/metrics"
	"aqueduct-monitor/models"
	"aqueduct-monitor/pipeline"
	"aqueduct-monitor/repository"
)

type DTUSensorReading struct {
	SensorCode string    `json:"sensor_code"`
	Value      float64   `json:"value"`
	Timestamp  time.Time `json:"timestamp"`
}

type DTUReceiver struct {
	OutChan chan<- pipeline.SensorReadingMsg
	repo    *repository.Repository
	metrics *metrics.Metrics
	stats   *pipeline.PipelineStats
	mu      sync.Mutex
}

func NewDTUReceiver(repo *repository.Repository, bufferSize int) *DTUReceiver {
	ch := make(chan pipeline.SensorReadingMsg, bufferSize)
	return &DTUReceiver{
		OutChan: ch,
		repo:    repo,
		stats:   &pipeline.PipelineStats{},
	}
}

func (r *DTUReceiver) SetOutputChannel(ch chan<- pipeline.SensorReadingMsg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.OutChan != nil {
		close(r.OutChan)
	}
	r.OutChan = ch
}

func (r *DTUReceiver) SubmitReadings(ctx context.Context, dtuID string, rssi float64, readings []DTUSensorReading) ([]pipeline.SensorReadingMsg, error) {
	var validData []models.SensorData
	var validMsgs []pipeline.SensorReadingMsg

	for _, reading := range readings {
		sensor, err := r.repo.GetSensorByCode(ctx, reading.SensorCode)
		if err != nil {
			continue
		}

		readingTime := reading.Timestamp
		if readingTime.IsZero() {
			readingTime = time.Now().UTC()
		}

		sensorData := models.SensorData{
			SensorID:   sensor.ID,
			AqueductID: sensor.AqueductID,
			SensorType: sensor.SensorType,
			SegmentID:  sensor.SegmentID,
			Timestamp:  readingTime,
			Value:      reading.Value,
			Quality:    1,
			DtuID:      dtuID,
			RSSI:       rssi,
		}
		validData = append(validData, sensorData)

		msg := pipeline.SensorReadingMsg{
			SegmentID:  sensor.SegmentID,
			AqueductID: sensor.AqueductID,
			SensorID:   sensor.SensorCode,
			SensorType: sensor.SensorType,
			Value:      reading.Value,
			Timestamp:  readingTime,
			DTUID:      dtuID,
			RSSI:       rssi,
			Stored:     false,
		}
		validMsgs = append(validMsgs, msg)
	}

	if len(validData) == 0 {
		return validMsgs, nil
	}

	if err := r.ProcessBatch(ctx, validData); err != nil {
		return nil, err
	}

	r.mu.Lock()
	for i := range validMsgs {
		validMsgs[i].Stored = true
		if r.metrics != nil {
			r.metrics.ObserveDTU(validMsgs[i].AqueductID, validMsgs[i].SensorType)
			r.metrics.SetSensorValue(validMsgs[i].SegmentID, validMsgs[i].SensorType, validMsgs[i].Value)
		}
		select {
		case r.OutChan <- validMsgs[i]:
		default:
		}
	}
	r.stats.DTUReceived += int64(len(validMsgs))
	r.stats.QueueSizeDTU = len(r.OutChan)
	r.mu.Unlock()

	return validMsgs, nil
}

func (r *DTUReceiver) ProcessBatch(ctx context.Context, data []models.SensorData) error {
	if len(data) == 0 {
		return nil
	}
	return r.repo.InsertSensorData(ctx, data)
}

func (r *DTUReceiver) GetStats() pipeline.PipelineStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	stats := *r.stats
	stats.QueueSizeDTU = len(r.OutChan)
	return stats
}

func (r *DTUReceiver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.OutChan != nil {
		close(r.OutChan)
		r.OutChan = nil
	}
}

func (r *DTUReceiver) SetMetrics(m *metrics.Metrics) {
	r.metrics = m
}

func (r *DTUReceiver) Output() <-chan pipeline.SensorReadingMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	return (<-chan pipeline.SensorReadingMsg)(r.OutChan)
}
