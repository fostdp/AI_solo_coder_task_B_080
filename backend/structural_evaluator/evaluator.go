package structural_evaluator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/evaluation"
	"aqueduct-monitor/metrics"
	"aqueduct-monitor/models"
	"aqueduct-monitor/pipeline"
	"aqueduct-monitor/repository"
)

const (
	// 默认去抖时间窗口：同段500ms内多次数据只评估一次
	defaultDebounceWindow = 500 * time.Millisecond
)

// StructuralEvaluator 结构评估器
// 负责从输入通道读取传感器消息，按段聚合去抖后执行有限元分析和结构安全评估
type StructuralEvaluator struct {
	// InChan 输入通道，接收传感器读数消息
	InChan <-chan pipeline.SensorReadingMsg

	// OutChan 输出通道，评估结果发送到告警模块
	OutChan chan<- pipeline.EvalResult

	// RepairOutChan 修复推荐输出通道，需要修复时发送请求
	RepairOutChan chan<- pipeline.RepairRequest

	// repo 数据仓库
	repo *repository.Repository

	// feaParams 有限元分析参数
	feaParams config.FEAConfig

	// thresholds 阈值配置
	thresholds config.ThresholdConfig

	// metrics Prometheus指标
	metrics *metrics.Metrics

	// mu 互斥锁，保护共享状态
	mu sync.Mutex

	// stats 管道统计信息
	stats *pipeline.PipelineStats

	// internalEvaluator 内部评估器，复用 evaluation 包的FEA逻辑
	internalEvaluator *evaluation.StructuralEvaluator

	// taskChan 内部任务队列，workers 从中消费
	taskChan chan uuid.UUID

	// lastEvalTime 记录每个段上次评估时间，用于去抖
	lastEvalTime map[uuid.UUID]time.Time

	// debounceWindow 去抖时间窗口
	debounceWindow time.Duration

	// closed 标记是否已关闭
	closed bool

	// workerWg worker 等待组
	workerWg sync.WaitGroup
}

// NewStructuralEvaluator 创建结构评估器
// repo: 数据仓库
// feaCfg: FEA配置
// thresholdCfg: 阈值配置
// bufferSize: 内部缓冲大小
func NewStructuralEvaluator(
	repo *repository.Repository,
	feaCfg config.FEAConfig,
	thresholdCfg config.ThresholdConfig,
	bufferSize int,
) *StructuralEvaluator {
	internalCfg := &config.Config{
		FEA:       feaCfg,
		Threshold: thresholdCfg,
	}

	return &StructuralEvaluator{
		repo:              repo,
		feaParams:         feaCfg,
		thresholds:        thresholdCfg,
		stats:             &pipeline.PipelineStats{},
		internalEvaluator: evaluation.NewStructuralEvaluator(repo, internalCfg),
		taskChan:          make(chan uuid.UUID, bufferSize),
		lastEvalTime:      make(map[uuid.UUID]time.Time),
		debounceWindow:    defaultDebounceWindow,
		closed:            false,
	}
}

func (e *StructuralEvaluator) SetInputChannel(in <-chan pipeline.SensorReadingMsg) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.InChan = in
}

func (e *StructuralEvaluator) SetOutputChannels(
	evalOut chan<- pipeline.EvalResult,
	repairOut chan<- pipeline.RepairRequest,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OutChan = evalOut
	e.RepairOutChan = repairOut
}

// Run 启动主循环
// 从 InChan 读取传感器消息，按 segment_id 聚合并发去重后触发评估
func (e *StructuralEvaluator) Run(ctx context.Context) error {
	if e.InChan == nil {
		return fmt.Errorf("input channel is nil")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-e.InChan:
			if !ok {
				return fmt.Errorf("input channel closed")
			}

			// 检查是否需要去抖
			if e.shouldDebounce(msg.SegmentID) {
				continue
			}

			// 更新最后评估时间
			e.mu.Lock()
			e.lastEvalTime[msg.SegmentID] = time.Now()
			e.mu.Unlock()

			// 发送到任务队列
			select {
			case e.taskChan <- msg.SegmentID:
				// 成功入队
			default:
				// 队列满，跳过并记录错误统计
				e.mu.Lock()
				e.stats.EvalErrors++
				e.mu.Unlock()
			}
		}
	}
}

// shouldDebounce 判断是否需要去抖
// 同段在去抖窗口内的多次数据只评估一次
func (e *StructuralEvaluator) shouldDebounce(segmentID uuid.UUID) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	lastTime, exists := e.lastEvalTime[segmentID]
	if !exists {
		return false
	}

	return time.Since(lastTime) < e.debounceWindow
}

// evaluateSegmentInternal 内部评估方法
// 调用 evaluation 包的逻辑执行评估，生成 EvalResult
func (e *StructuralEvaluator) evaluateSegmentInternal(
	ctx context.Context,
	segmentID uuid.UUID,
) (pipeline.EvalResult, error) {
	result := pipeline.EvalResult{
		RequestID:   uuid.New().String(),
		SegmentID:   segmentID,
		ProcessedAt: time.Now().UTC(),
	}

	// 调用内部评估器执行FEA和安全评估
	alerts, err := e.internalEvaluator.EvaluateSegment(ctx, segmentID)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("evaluate segment failed: %w", err)
	}

	// 获取段信息（包含最新评估状态）
	segment, err := e.repo.GetSegmentByIDWithStatus(ctx, segmentID)
	if err == nil && segment != nil {
		result.AqueductID = segment.AqueductID
		result.SafetyLevel = segment.SafetyLevel
		result.ResidualRatio = segment.CapacityRatio
		result.MaxStress = segment.CurrentStress
		result.SettlementMM = segment.SettlementMM
		result.WeatheringRate = segment.WeatheringDepth
	}

	// 获取最新传感器数据补充信息
	sensorVals, err := e.repo.GetSegmentLatestSensorValues(ctx, segmentID)
	if err == nil {
		if result.SettlementMM == 0 {
			result.SettlementMM = sensorVals["settlement"]
		}
		if result.WeatheringRate == 0 {
			result.WeatheringRate = sensorVals["weathering"]
		}
		if stress, ok := sensorVals["stress"]; ok && result.MaxStress == 0 {
			result.MaxStress = stress
		}
		if displacement, ok := sensorVals["displacement"]; ok {
			result.MaxDisplacement = displacement
		}
	}

	// 检查风化速率是否加速
	result.WeatheringAccel = e.checkWeatheringAcceleration(ctx, segmentID)

	// 填充告警信息
	result.Alerts = make([]models.Alert, 0, len(alerts))
	for _, a := range alerts {
		if a != nil {
			result.Alerts = append(result.Alerts, *a)
		}
	}

	// 判断是否需要修复推荐
	result.NeedsRepair = e.needsRepair(result.SafetyLevel, len(alerts) > 0)

	if result.NeedsRepair {
		result.DamageType = e.determineDamageType(result)
		result.DamageSeverity = e.calculateDamageSeverity(result)
	}

	// FEA 收敛状态（默认假设收敛，实际可从评估结果中获取）
	result.FEAConverged = true
	result.FEAIterations = 0

	return result, nil
}

// checkWeatheringAcceleration 检查风化速率是否加速
func (e *StructuralEvaluator) checkWeatheringAcceleration(
	ctx context.Context,
	segmentID uuid.UUID,
) bool {
	recentRate, err := e.repo.GetWeatheringRate(ctx, segmentID, 30)
	if err != nil {
		return false
	}

	baselineRate, err := e.repo.GetWeatheringRate(ctx, segmentID, 365)
	if err != nil || baselineRate <= 0 {
		return false
	}

	return recentRate > baselineRate*e.thresholds.WeatheringAccelRatio
}

// needsRepair 判断是否需要修复推荐
func (e *StructuralEvaluator) needsRepair(safetyLevel string, hasAlerts bool) bool {
	switch safetyLevel {
	case "CRITICAL", "DANGER":
		return true
	case "WARNING":
		return hasAlerts
	default:
		return false
	}
}

// determineDamageType 确定损伤类型
func (e *StructuralEvaluator) determineDamageType(result pipeline.EvalResult) string {
	if result.ResidualRatio < e.thresholds.LoadCapacityThreshold {
		return "LOAD_CAPACITY_DEFICIT"
	}
	if result.SettlementMM > e.thresholds.SettlementLimitMM {
		return "SETTLEMENT_DAMAGE"
	}
	if result.WeatheringAccel {
		return "WEATHERING_DAMAGE"
	}
	if result.MaxStress > e.thresholds.StressLimitMpa {
		return "STRESS_DAMAGE"
	}
	return "STRUCTURAL_DEGRADATION"
}

// calculateDamageSeverity 计算损伤严重程度
func (e *StructuralEvaluator) calculateDamageSeverity(result pipeline.EvalResult) float64 {
	switch result.SafetyLevel {
	case "CRITICAL":
		return 0.9
	case "DANGER":
		return 0.7
	case "WARNING":
		return 0.5
	default:
		return 0.2
	}
}

// RunWorkers 启动多个 worker goroutine 并行评估
// count: worker 数量
func (e *StructuralEvaluator) RunWorkers(ctx context.Context, count int) {
	for i := 0; i < count; i++ {
		e.workerWg.Add(1)
		go e.worker(ctx, i)
	}
}

func (e *StructuralEvaluator) SetMetrics(m *metrics.Metrics) {
	e.metrics = m
}

// worker 单个 worker 协程
// 从 taskChan 读取段ID，执行评估，将结果发送到输出通道
func (e *StructuralEvaluator) worker(ctx context.Context, id int) {
	defer e.workerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case segmentID, ok := <-e.taskChan:
			if !ok {
				return
			}

			start := time.Now()

			// 执行评估
			result, err := e.evaluateSegmentInternal(ctx, segmentID)

			duration := time.Since(start)

			if e.metrics != nil {
				if err != nil {
					e.metrics.EvalErrors.Inc()
				} else {
					e.metrics.ObserveEval(duration, result.SafetyLevel, result.AqueductID)
					if !result.FEAConverged {
						e.metrics.FEANonConverge.Inc()
					}
					if result.FEAIterations > 0 {
						e.metrics.FEAIterations.Observe(float64(result.FEAIterations))
					}
				}
			}

			// 更新统计
			e.mu.Lock()
			if err != nil {
				e.stats.EvalErrors++
			} else {
				e.stats.EvalProcessed++
			}
			e.mu.Unlock()

			// 发送评估结果到告警通道
			if e.OutChan != nil {
				select {
				case e.OutChan <- result:
				default:
					e.mu.Lock()
					e.stats.EvalErrors++
					e.mu.Unlock()
				}
			}

			// 如果需要修复，发送修复请求
			if result.NeedsRepair && e.RepairOutChan != nil {
				repairReq := pipeline.RepairRequest{
					RequestID:      uuid.New().String(),
					SegmentID:      result.SegmentID,
					AqueductID:     result.AqueductID,
					DamageType:     result.DamageType,
					DamageSeverity: result.DamageSeverity,
					SafetyLevel:    result.SafetyLevel,
					TriggerType:    "auto_evaluation",
					Timestamp:      time.Now().UTC(),
				}

				select {
				case e.RepairOutChan <- repairReq:
				default:
					// 队列满，跳过
				}
			}
		}
	}
}

// GetStats 获取统计信息
func (e *StructuralEvaluator) GetStats() pipeline.PipelineStats {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 复制一份返回
	stats := *e.stats

	// 填充队列大小
	stats.QueueSizeEval = len(e.taskChan)

	return stats
}

// Close 关闭评估器
// 等待所有 worker 完成后关闭内部通道
func (e *StructuralEvaluator) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()

	// 关闭任务队列，让 workers 自然退出
	close(e.taskChan)

	// 等待所有 worker 完成
	e.workerWg.Wait()
}
