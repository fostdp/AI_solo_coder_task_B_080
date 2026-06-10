package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"
)

const (
	API_BASE      = "http://localhost:8080/api"
	BACKFILL_DAYS = 365
	HOURLY_STEP   = 1
)

type SensorInfo struct {
	ID         string `json:"id"`
	SensorCode string `json:"sensor_code"`
	SegmentID  string `json:"segment_id"`
	AqueductID string `json:"aqueduct_id"`
	SensorType string `json:"sensor_type"`
}

type AqueductInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SubmitRequest struct {
	DtuID     string           `json:"dtu_id"`
	RSSI      float64          `json:"rssi"`
	Timestamp string           `json:"timestamp"`
	Readings  []SensorReading  `json:"readings"`
}

type SensorReading struct {
	SensorCode string  `json:"sensor_code"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Timestamp  string  `json:"timestamp"`
}

type APIResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
	Count  int         `json:"count"`
}

type SensorSim struct {
	client     *http.Client
	sensors    []SensorInfo
	aqueducts  []AqueductInfo
	dailyBias  map[string]float64
	baseValues map[string]float64
}

func main() {
	log.Println("=")
	log.Println("  传感器数据模拟器 - Sensor Data Simulator")
	log.Println("  模拟古罗马水道11条水道的传感器定期上报")
	log.Println("=")
	log.Println()

	sim := &SensorSim{
		client:     &http.Client{Timeout: 60 * time.Second},
		dailyBias:  make(map[string]float64),
		baseValues: make(map[string]float64),
	}

	log.Println("→ 等待后端服务启动...")
	waitForBackend(30)

	log.Println("→ 加载水道列表...")
	if err := sim.loadAqueducts(); err != nil {
		log.Fatalf("Failed to load aqueducts: %v", err)
	}
	log.Printf("  ✓ 加载 %d 条水道", len(sim.aqueducts))

	log.Println("→ 加载传感器列表...")
	if err := sim.loadSensors(); err != nil {
		log.Fatalf("Failed to load sensors: %v", err)
	}
	log.Printf("  ✓ 加载 %d 个传感器", len(sim.sensors))

	sim.initBaseValues()

	log.Printf("\n→ 开始回填过去 %d 天的历史数据 (每1小时一个采样点)...", BACKFILL_DAYS)
	sim.backfillHistorical(BACKFILL_DAYS)

	log.Println("\n→ 启动实时模拟模式，每 10 秒模拟一次 1 小时的数据上报...")
	log.Println("  (Ctrl+C 退出)")
	sim.startRealtime()
}

func waitForBackend(maxSeconds int) {
	for i := 0; i < maxSeconds; i++ {
		resp, err := http.Get(API_BASE + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				log.Println("  ✓ 后端服务已就绪")
				return
			}
		}
		if i%5 == 0 && i > 0 {
			log.Printf("  等待中... %ds / %ds", i, maxSeconds)
		}
		time.Sleep(1 * time.Second)
	}
	log.Println("  ⚠ 后端连接超时，继续尝试...")
}

func (s *SensorSim) loadAqueducts() error {
	resp, err := s.client.Get(API_BASE + "/aqueducts")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result APIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	dataBytes, _ := json.Marshal(result.Data)
	return json.Unmarshal(dataBytes, &s.aqueducts)
}

func (s *SensorSim) loadSensors() error {
	for _, aq := range s.aqueducts {
		resp, err := s.client.Get(fmt.Sprintf("%s/aqueducts/%s", API_BASE, aq.ID))
		if err != nil {
			log.Printf("  警告: 加载水道 %s 传感器失败: %v", aq.Name, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]interface{}
		json.Unmarshal(body, &result)

		if data, ok := result["data"].(map[string]interface{}); ok {
			if sensorArr, ok := data["sensors"].([]interface{}); ok {
				for _, sItem := range sensorArr {
					b, _ := json.Marshal(sItem)
					var si SensorInfo
					json.Unmarshal(b, &si)
					s.sensors = append(s.sensors, si)
				}
			}
		}
	}
	return nil
}

func (s *SensorSim) initBaseValues() {
	for _, sensor := range s.sensors {
		switch sensor.SensorType {
		case "stress":
			s.baseValues[sensor.ID] = 4.0 + rand.Float64()*5.0
			s.dailyBias[sensor.ID] = (rand.Float64() - 0.5) * 0.005
		case "weathering":
			s.baseValues[sensor.ID] = 3.0 + rand.Float64()*8.0
			s.dailyBias[sensor.ID] = 0.005 + rand.Float64()*0.008
		case "settlement":
			s.baseValues[sensor.ID] = 2.0 + rand.Float64()*5.0
			s.dailyBias[sensor.ID] = 0.003 + rand.Float64()*0.006
		case "tilt":
			s.baseValues[sensor.ID] = 0.05 + rand.Float64()*0.15
			s.dailyBias[sensor.ID] = (rand.Float64() - 0.5) * 0.0002
		case "temperature":
			s.baseValues[sensor.ID] = 18.0
			s.dailyBias[sensor.ID] = 0
		case "humidity":
			s.baseValues[sensor.ID] = 65.0
			s.dailyBias[sensor.ID] = 0
		}
	}
}

func (s *SensorSim) generateValue(sensor SensorInfo, hoursElapsed int, current time.Time) (float64, string) {
	base := s.baseValues[sensor.ID]
	bias := s.dailyBias[sensor.ID]
	totalHours := float64(hoursElapsed)
	days := totalHours / 24.0

	seasonalPhase := 2 * math.Pi * (float64(current.YearDay()) / 365.0)
	diurnalPhase := 2 * math.Pi * (float64(current.Hour()) / 24.0)

	anomaly := 0.0
	if rand.Float64() < 0.0005 {
		anomaly = (rand.Float64() - 0.3) * 15.0
		log.Printf("  ⚠ 模拟异常事件: %s=%+.2f @ %s", sensor.SensorType, anomaly, current.Format("2006-01-02 15:04"))
	}

	switch sensor.SensorType {
	case "stress":
		trend := base + bias*days*365*1.3
		seasonal := 0.8 * math.Sin(seasonalPhase)
		diurnal := 0.15 * math.Sin(diurnalPhase)
		noise := (rand.Float64() - 0.5) * 0.3
		return math.Max(0.5, trend+seasonal+diurnal+noise+anomaly), "MPa"

	case "weathering":
		trend := base + bias*days*365*8.0
		seasonal := 0.3 * math.Sin(seasonalPhase+math.Pi/2)
		noise := (rand.Float64() - 0.5) * 0.15
		return math.Max(0.0, trend+seasonal+noise+math.Abs(anomaly)), "mm"

	case "settlement":
		trend := base + bias*days*365*2.5
		settlePhase := 1 - math.Exp(-days/100.0)
		trend = base + (20.0-base)*settlePhase*0.6
		seasonal := 0.4 * math.Sin(seasonalPhase-math.Pi/2)
		noise := (rand.Float64() - 0.5) * 0.2
		return math.Max(0.0, trend+seasonal+noise+anomaly), "mm"

	case "tilt":
		trend := base + bias*days*365*15.0
		seasonal := 0.03 * math.Sin(seasonalPhase)
		noise := (rand.Float64() - 0.5) * 0.01
		return math.Max(0.0, math.Abs(trend+seasonal+noise+anomaly)), "deg"

	case "temperature":
		trend := base
		seasonal := 12.0 * math.Sin(seasonalPhase-math.Pi/2)
		diurnal := 6.0 * math.Sin(diurnalPhase-math.Pi/2)
		noise := (rand.Float64() - 0.5) * 0.5
		return trend + seasonal + diurnal + noise, "°C"

	case "humidity":
		trend := base
		seasonal := -15.0 * math.Sin(seasonalPhase-math.Pi/2)
		diurnal := 10.0 * math.Sin(diurnalPhase+math.Pi/2)
		noise := (rand.Float64() - 0.5) * 2.0
		return math.Max(10, math.Min(100, trend+seasonal+diurnal+noise)), "%"

	default:
		return base, ""
	}
}

func (s *SensorSim) backfillHistorical(days int) {
	type batchRecord struct {
		dtuID    string
		rssi     float64
		readings []SensorReading
		time     time.Time
	}

	var batches []batchRecord
	totalHours := days * 24

	start := time.Now().UTC().AddDate(0, 0, -days)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	processed := 0
	totalReadings := 0

	for h := 0; h < totalHours; h += HOURLY_STEP {
		currentTime := start.Add(time.Duration(h) * time.Hour)

		dtuGroups := make(map[string]*batchRecord)

		for _, sensor := range s.sensors {
			dtuID := fmt.Sprintf("DTU-%s", sensor.AqueductID[:6])

			val, unit := s.generateValue(sensor, h, currentTime)

			if _, ok := dtuGroups[dtuID]; !ok {
				dtuGroups[dtuID] = &batchRecord{
					dtuID:    dtuID,
					rssi:     -65 - rand.Float64()*35,
					time:     currentTime,
					readings: []SensorReading{},
				}
			}

			dtuGroups[dtuID].readings = append(dtuGroups[dtuID].readings, SensorReading{
				SensorCode: sensor.SensorCode,
				Value:      roundTo(val, 4),
				Unit:       unit,
				Timestamp:  currentTime.Format(time.RFC3339),
			})
		}

		for _, batch := range dtuGroups {
			batches = append(batches, *batch)
			totalReadings += len(batch.readings)
		}
		processed++
	}

	log.Printf("  生成 %d 小时数据点，共 %d 个DTU批次，%d 条传感器读数",
		processed, len(batches), totalReadings)

	log.Println("  开始提交数据至后端...")
	success := 0
	failed := 0

	progressInterval := len(batches) / 20
	if progressInterval < 1 {
		progressInterval = 1
	}

	for i, batch := range batches {
		req := SubmitRequest{
			DtuID:     batch.dtuID,
			RSSI:      batch.rssi,
			Timestamp: batch.time.Format(time.RFC3339),
			Readings:  batch.readings,
		}

		if err := s.submitBatch(req); err != nil {
			failed++
			if failed < 5 {
				log.Printf("  ✗ 提交失败 (批次 %d): %v", i, err)
			}
		} else {
			success++
		}

		if (i+1)%progressInterval == 0 {
			pct := int(float64(i+1) / float64(len(batches)) * 100)
			bar := ""
			for p := 0; p < 50; p++ {
				if p < pct/2 {
					bar += "█"
				} else {
					bar += "░"
				}
			}
			fmt.Printf("\r  %s %3d%%  |  成功 %d / 失败 %d", bar, pct, success, failed)
		}
	}

	fmt.Println()
	log.Printf("  ✓ 历史数据回填完成: 成功 %d, 失败 %d", success, failed)
}

func (s *SensorSim) startRealtime() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	hoursSimulated := 0

	for range ticker.C {
		hoursSimulated++
		currentSimTime := time.Now().UTC().Add(time.Duration(hoursSimulated) * time.Hour)

		dtuGroups := make(map[string][]SensorReading)
		dtuRssi := make(map[string]float64)

		for _, sensor := range s.sensors {
			dtuID := fmt.Sprintf("DTU-%s", sensor.AqueductID[:6])

			val, unit := s.generateValue(sensor, 365*24+hoursSimulated, currentSimTime)

			dtuGroups[dtuID] = append(dtuGroups[dtuID], SensorReading{
				SensorCode: sensor.SensorCode,
				Value:      roundTo(val, 4),
				Unit:       unit,
				Timestamp:  currentSimTime.Format(time.RFC3339),
			})
			if _, ok := dtuRssi[dtuID]; !ok {
				dtuRssi[dtuID] = -65 - rand.Float64()*35
			}
		}

		totalReadings := 0
		successBatches := 0

		for dtuID, readings := range dtuGroups {
			req := SubmitRequest{
				DtuID:     dtuID,
				RSSI:      dtuRssi[dtuID],
				Timestamp: currentSimTime.Format(time.RFC3339),
				Readings:  readings,
			}
			totalReadings += len(readings)

			if err := s.submitBatch(req); err == nil {
				successBatches++
			}
		}

		log.Printf("  [模拟时间 %s] 模拟 +1h: 提交 %d DTU批次 / %d 读数, 成功 %d",
			currentSimTime.Format("2006-01-02 15:04"),
			len(dtuGroups), totalReadings, successBatches)
	}
}

func (s *SensorSim) submitBatch(req SubmitRequest) error {
	body, _ := json.Marshal(req)
	resp, err := s.client.Post(
		API_BASE+"/dtu/submit",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func roundTo(v float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(v*pow) / pow
}
