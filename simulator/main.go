package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"
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
	DtuID     string          `json:"dtu_id"`
	RSSI      float64         `json:"rssi"`
	Timestamp string          `json:"timestamp"`
	Readings  []SensorReading `json:"readings"`
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

type AqueductConfig struct {
	AqueductID        string
	Name              string
	BaseStress        float64
	BaseDisplacement  float64
	BaseWeathering    float64
	BaseSettlement    float64
	AnomalyProbability float64
	Sensors           []SensorInfo
	DailyBias         map[string]float64
	BaseValues        map[string]float64
	CumulativeDrift   map[string]float64
	LastSettlementStep map[string]int
}

type SensorSim struct {
	client            *http.Client
	allAqueducts      []AqueductInfo
	allSensors        []SensorInfo
	aqueductConfigs   map[string]*AqueductConfig
	selectedAqueducts []string
	config            *SimConfig
}

type SimConfig struct {
	AqueductIDs      string
	Interval         int
	BackfillDays     int
	InjectWeathering float64
	InjectDeformation float64
	Seed             int
	APIBase          string
	Realtime         bool
	ListAqueducts    bool
}

func main() {
	log.Println("=")
	log.Println("  传感器数据模拟器 - Sensor Data Simulator")
	log.Println("  模拟古罗马水道11条水道的传感器定期上报")
	log.Println("=")
	log.Println()

	cfg := parseFlags()

	rand.Seed(int64(cfg.Seed))

	sim := &SensorSim{
		client:          &http.Client{Timeout: 60 * time.Second},
		aqueductConfigs: make(map[string]*AqueductConfig),
		config:          cfg,
	}

	log.Println("→ 等待后端服务启动...")
	waitForBackend(sim.config.APIBase, 30)

	log.Println("→ 加载水道列表...")
	if err := sim.loadAqueducts(); err != nil {
		log.Fatalf("加载水道列表失败: %v", err)
	}
	log.Printf("  ✓ 加载 %d 条水道", len(sim.allAqueducts))

	if sim.config.ListAqueducts {
		sim.printAqueductList()
		return
	}

	sim.filterAqueducts()

	log.Println("→ 加载传感器列表...")
	if err := sim.loadSensors(); err != nil {
		log.Fatalf("加载传感器列表失败: %v", err)
	}
	log.Printf("  ✓ 加载 %d 个传感器", len(sim.allSensors))

	sim.initAqueductConfigs()
	sim.printConfigSummary()

	if sim.config.BackfillDays > 0 {
		log.Printf("\n→ 开始回填过去 %d 天的历史数据 (每 %d 秒一个采样点)...",
			sim.config.BackfillDays, sim.config.Interval)
		sim.backfillHistorical(sim.config.BackfillDays, sim.config.Interval)
	}

	if sim.config.Realtime {
		log.Printf("\n→ 启动实时模拟模式，每 %d 秒模拟一次数据上报...", sim.config.Interval)
		log.Println("  (Ctrl+C 退出)")
		sim.startRealtime()
	}
}

func parseFlags() *SimConfig {
	cfg := &SimConfig{}

	flag.StringVar(&cfg.AqueductIDs, "aqueducts", "all", "要模拟的水道ID列表，逗号分隔")
	flag.IntVar(&cfg.Interval, "interval", 3600, "上报间隔秒数")
	flag.IntVar(&cfg.BackfillDays, "backfill-days", 365, "历史回填天数")
	flag.Float64Var(&cfg.InjectWeathering, "inject-weathering", 1.0, "全局风化速率倍率注入")
	flag.Float64Var(&cfg.InjectDeformation, "inject-deformation", 1.0, "全局变形倍率注入")
	flag.IntVar(&cfg.Seed, "seed", 42, "随机种子")
	flag.StringVar(&cfg.APIBase, "api-base", "http://backend:8080/api", "后端API地址")
	flag.BoolVar(&cfg.Realtime, "realtime", true, "是否启动实时模拟")
	flag.BoolVar(&cfg.ListAqueducts, "list-aqueducts", false, "列出所有可用水道后退出")

	flag.Parse()

	return cfg
}

func waitForBackend(apiBase string, maxSeconds int) {
	for i := 0; i < maxSeconds; i++ {
		resp, err := http.Get(apiBase + "/health")
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
	resp, err := s.client.Get(s.config.APIBase + "/aqueducts")
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
	return json.Unmarshal(dataBytes, &s.allAqueducts)
}

func (s *SensorSim) loadSensors() error {
	for _, aqID := range s.selectedAqueducts {
		resp, err := s.client.Get(fmt.Sprintf("%s/aqueducts/%s", s.config.APIBase, aqID))
		if err != nil {
			log.Printf("  警告: 加载水道 %s 传感器失败: %v", aqID, err)
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
					s.allSensors = append(s.allSensors, si)
				}
			}
		}
	}
	return nil
}

func (s *SensorSim) filterAqueducts() {
	if s.config.AqueductIDs == "all" {
		for _, aq := range s.allAqueducts {
			s.selectedAqueducts = append(s.selectedAqueducts, aq.ID)
		}
	} else {
		idList := strings.Split(s.config.AqueductIDs, ",")
		for _, id := range idList {
			id = strings.TrimSpace(id)
			for _, aq := range s.allAqueducts {
				if aq.ID == id {
					s.selectedAqueducts = append(s.selectedAqueducts, id)
					break
				}
			}
		}
	}
}

func (s *SensorSim) initAqueductConfigs() {
	aqNameMap := make(map[string]string)
	for _, aq := range s.allAqueducts {
		aqNameMap[aq.ID] = aq.Name
	}

	aqSensors := make(map[string][]SensorInfo)
	for _, sensor := range s.allSensors {
		aqSensors[sensor.AqueductID] = append(aqSensors[sensor.AqueductID], sensor)
	}

	for i, aqID := range s.selectedAqueducts {
		cfg := &AqueductConfig{
			AqueductID:          aqID,
			Name:                aqNameMap[aqID],
			BaseStress:          4.0 + float64(i)*0.3 + rand.Float64()*2.0,
			BaseDisplacement:    0.5 + float64(i)*0.1 + rand.Float64()*0.5,
			BaseWeathering:      3.0 + float64(i)*0.5 + rand.Float64()*3.0,
			BaseSettlement:      2.0 + float64(i)*0.4 + rand.Float64()*2.0,
			AnomalyProbability:  0.001 + float64(i)*0.0002,
			Sensors:             aqSensors[aqID],
			DailyBias:           make(map[string]float64),
			BaseValues:          make(map[string]float64),
			CumulativeDrift:     make(map[string]float64),
			LastSettlementStep:  make(map[string]int),
		}

		for _, sensor := range cfg.Sensors {
			switch sensor.SensorType {
			case "stress":
				cfg.BaseValues[sensor.ID] = cfg.BaseStress + rand.Float64()*5.0
				cfg.DailyBias[sensor.ID] = (rand.Float64() - 0.5) * 0.005
			case "displacement":
				cfg.BaseValues[sensor.ID] = cfg.BaseDisplacement + rand.Float64()*1.0
				cfg.DailyBias[sensor.ID] = 0.0001 + rand.Float64()*0.0002
				cfg.CumulativeDrift[sensor.ID] = 0
			case "weathering":
				cfg.BaseValues[sensor.ID] = cfg.BaseWeathering + rand.Float64()*8.0
				cfg.DailyBias[sensor.ID] = (0.005 + rand.Float64()*0.008) * s.config.InjectWeathering
			case "settlement":
				cfg.BaseValues[sensor.ID] = cfg.BaseSettlement + rand.Float64()*5.0
				cfg.DailyBias[sensor.ID] = (0.003 + rand.Float64()*0.006) * s.config.InjectDeformation
				cfg.LastSettlementStep[sensor.ID] = 0
			case "mortar_strength":
				cfg.BaseValues[sensor.ID] = 25.0 + rand.Float64()*15.0
				cfg.DailyBias[sensor.ID] = -0.0005 - rand.Float64()*0.0003
			case "tilt":
				cfg.BaseValues[sensor.ID] = 0.05 + rand.Float64()*0.15
				cfg.DailyBias[sensor.ID] = (rand.Float64() - 0.5) * 0.0002
			case "temperature":
				cfg.BaseValues[sensor.ID] = 18.0
				cfg.DailyBias[sensor.ID] = 0
			case "humidity":
				cfg.BaseValues[sensor.ID] = 65.0
				cfg.DailyBias[sensor.ID] = 0
			}
		}

		s.aqueductConfigs[aqID] = cfg
	}
}

func (s *SensorSim) printConfigSummary() {
	totalSensors := 0
	for _, aqID := range s.selectedAqueducts {
		cfg := s.aqueductConfigs[aqID]
		totalSensors += len(cfg.Sensors)
	}

	log.Println()
	log.Println("=== 配置摘要 ===")
	log.Printf("  选中水道数: %d", len(s.selectedAqueducts))
	log.Printf("  总监测点数: %d", totalSensors)
	log.Printf("  上报间隔: %d 秒", s.config.Interval)
	log.Printf("  回填天数: %d 天", s.config.BackfillDays)
	log.Printf("  全局风化倍率: %.2f", s.config.InjectWeathering)
	log.Printf("  全局变形倍率: %.2f", s.config.InjectDeformation)
	log.Printf("  随机种子: %d", s.config.Seed)
	log.Printf("  API地址: %s", s.config.APIBase)
	log.Printf("  实时模拟: %t", s.config.Realtime)
	log.Println("================")
	log.Println()

	for _, aqID := range s.selectedAqueducts {
		cfg := s.aqueductConfigs[aqID]
		log.Printf("  水道 [%s] %s:", aqID, cfg.Name)
		log.Printf("    监测点: %d 个", len(cfg.Sensors))
		log.Printf("    基准应力: %.2f MPa, 基准位移: %.3f mm", cfg.BaseStress, cfg.BaseDisplacement)
		log.Printf("    基准风化: %.2f mm, 基准沉降: %.2f mm", cfg.BaseWeathering, cfg.BaseSettlement)
		log.Printf("    异常概率: %.4f", cfg.AnomalyProbability)
	}
}

func (s *SensorSim) printAqueductList() {
	log.Println()
	log.Println("=== 可用水道列表 ===")
	for _, aq := range s.allAqueducts {
		sensorCount := s.getAqueductSensorCount(aq.ID)
		log.Printf("  ID: %-12s  名称: %-20s  监测点数: %d", aq.ID, aq.Name, sensorCount)
	}
	log.Println("====================")
	log.Println()
}

func (s *SensorSim) getAqueductSensorCount(aqID string) int {
	resp, err := s.client.Get(fmt.Sprintf("%s/aqueducts/%s", s.config.APIBase, aqID))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if data, ok := result["data"].(map[string]interface{}); ok {
		if sensorArr, ok := data["sensors"].([]interface{}); ok {
			return len(sensorArr)
		}
	}
	return 0
}

func (s *SensorSim) generateValue(aqID string, sensor SensorInfo, intervalsElapsed int, current time.Time) (float64, string) {
	aqCfg := s.aqueductConfigs[aqID]
	base := aqCfg.BaseValues[sensor.ID]
	bias := aqCfg.DailyBias[sensor.ID]
	totalHours := float64(intervalsElapsed) * float64(s.config.Interval) / 3600.0
	days := totalHours / 24.0

	seasonalPhase := 2 * math.Pi * (float64(current.YearDay()) / 365.0)
	diurnalPhase := 2 * math.Pi * (float64(current.Hour()) / 24.0)

	anomalyMultiplier := 1.0
	if rand.Float64() < aqCfg.AnomalyProbability {
		anomalyMultiplier = 1.5 + rand.Float64()*1.0
		log.Printf("  ⚠ 异常事件触发: 水道 %s, 传感器 %s (%s), 倍率 ×%.2f @ %s",
			aqID, sensor.SensorCode, sensor.SensorType, anomalyMultiplier,
			current.Format("2006-01-02 15:04"))
	}

	switch sensor.SensorType {
	case "stress":
		trend := base + bias*days*365*1.3
		seasonal := 0.8 * math.Sin(seasonalPhase)
		diurnal := 0.15 * math.Sin(diurnalPhase)
		noise := (rand.Float64() - 0.5) * 0.3
		value := trend + seasonal + diurnal + noise
		return math.Max(0.5, value*anomalyMultiplier), "MPa"

	case "displacement":
		aqCfg.CumulativeDrift[sensor.ID] += bias * s.config.InjectDeformation
		drift := aqCfg.CumulativeDrift[sensor.ID] * days * 365
		vibration := (rand.Float64() - 0.5) * 0.002
		value := base + drift + vibration
		return math.Max(0.0, value*anomalyMultiplier), "mm"

	case "weathering":
		trend := base + bias*days*365*8.0*s.config.InjectWeathering
		seasonal := 0.3 * math.Sin(seasonalPhase+math.Pi/2)
		noise := (rand.Float64() - 0.5) * 0.15
		value := trend + seasonal + noise
		return math.Max(0.0, value*anomalyMultiplier), "mm"

	case "settlement":
		stepInterval := 24 * 30
		currentStep := intervalsElapsed / stepInterval
		if currentStep > aqCfg.LastSettlementStep[sensor.ID] {
			aqCfg.LastSettlementStep[sensor.ID] = currentStep
		}
		stepValue := float64(aqCfg.LastSettlementStep[sensor.ID]) * 0.08 * s.config.InjectDeformation
		seasonal := 0.4 * math.Sin(seasonalPhase-math.Pi/2)
		noise := (rand.Float64() - 0.5) * 0.2
		value := base + stepValue + seasonal + noise
		return math.Max(0.0, value*anomalyMultiplier), "mm"

	case "mortar_strength":
		trend := base + bias*days*365*5.0
		noise := (rand.Float64() - 0.5) * 0.3
		value := trend + noise
		return math.Max(5.0, value*anomalyMultiplier), "MPa"

	case "tilt":
		trend := base + bias*days*365*15.0
		seasonal := 0.03 * math.Sin(seasonalPhase)
		noise := (rand.Float64() - 0.5) * 0.01
		value := trend + seasonal + noise
		return math.Max(0.0, math.Abs(value*anomalyMultiplier)), "deg"

	case "temperature":
		trend := base
		seasonal := 12.0 * math.Sin(seasonalPhase-math.Pi/2)
		diurnal := 6.0 * math.Sin(diurnalPhase-math.Pi/2)
		noise := (rand.Float64() - 0.5) * 0.5
		return (trend + seasonal + diurnal + noise) * anomalyMultiplier, "°C"

	case "humidity":
		trend := base
		seasonal := -15.0 * math.Sin(seasonalPhase-math.Pi/2)
		diurnal := 10.0 * math.Sin(diurnalPhase+math.Pi/2)
		noise := (rand.Float64() - 0.5) * 2.0
		value := trend + seasonal + diurnal + noise
		return math.Max(10, math.Min(100, value*anomalyMultiplier)), "%"

	default:
		return base * anomalyMultiplier, ""
	}
}

func (s *SensorSim) backfillHistorical(days int, intervalSeconds int) {
	type batchRecord struct {
		dtuID    string
		rssi     float64
		readings []SensorReading
		time     time.Time
	}

	var batches []batchRecord
	totalIntervals := days * 24 * 3600 / intervalSeconds

	start := time.Now().UTC().AddDate(0, 0, -days)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	processed := 0
	totalReadings := 0

	for i := 0; i < totalIntervals; i++ {
		currentTime := start.Add(time.Duration(i) * time.Duration(intervalSeconds) * time.Second)

		dtuGroups := make(map[string]*batchRecord)

		for _, sensor := range s.allSensors {
			dtuID := fmt.Sprintf("DTU-%s", sensor.AqueductID[:6])

			val, unit := s.generateValue(sensor.AqueductID, sensor, i, currentTime)

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

		if processed%(24*3600/intervalSeconds) == 0 {
			dayNum := processed * intervalSeconds / (24 * 3600)
			pct := int(float64(processed) / float64(totalIntervals) * 100)
			log.Printf("  回填进度: 第 %d/%d 天完成 (%d%%)", dayNum, days, pct)
		}
	}

	log.Printf("  生成 %d 个时间点数据，共 %d 个DTU批次，%d 条传感器读数",
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
	ticker := time.NewTicker(time.Duration(s.config.Interval) * time.Second)
	defer ticker.Stop()

	intervalsSimulated := 0
	backfillIntervals := s.config.BackfillDays * 24 * 3600 / s.config.Interval

	for range ticker.C {
		intervalsSimulated++
		totalIntervals := backfillIntervals + intervalsSimulated
		currentSimTime := time.Now().UTC().Add(time.Duration(intervalsSimulated) * time.Duration(s.config.Interval) * time.Second)

		dtuGroups := make(map[string][]SensorReading)
		dtuRssi := make(map[string]float64)

		for _, sensor := range s.allSensors {
			dtuID := fmt.Sprintf("DTU-%s", sensor.AqueductID[:6])

			val, unit := s.generateValue(sensor.AqueductID, sensor, totalIntervals, currentSimTime)

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

		hoursSimulated := float64(intervalsSimulated) * float64(s.config.Interval) / 3600.0
		log.Printf("  [模拟时间 %s] 已模拟 %.1f 小时: 提交 %d DTU批次 / %d 读数, 成功 %d",
			currentSimTime.Format("2006-01-02 15:04"),
			hoursSimulated,
			len(dtuGroups), totalReadings, successBatches)
	}
}

func (s *SensorSim) submitBatch(req SubmitRequest) error {
	body, _ := json.Marshal(req)
	resp, err := s.client.Post(
		s.config.APIBase+"/dtu/submit",
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
