package repair_advisor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"aqueduct-monitor/config"
	"aqueduct-monitor/pipeline"
	"aqueduct-monitor/recommendation"
	"aqueduct-monitor/repository"
)

// RepairAdvisor 修复材料多属性决策推荐服务
// 从输入通道接收修复请求，调用推荐算法生成推荐结果，输出到结果通道
type RepairAdvisor struct {
	InChan       <-chan pipeline.RepairRequest  // 输入通道：修复请求
	OutChan      chan<- pipeline.RepairResult   // 输出通道：推荐结果
	AlertOutChan chan<- pipeline.AlertMsg       // 告警输出通道（可选）
	repo         *repository.Repository         // 数据仓库
	madmConfig   config.MADMConfig              // 多属性决策配置
	mu           sync.Mutex                     // 互斥锁，保护统计数据
	stats        *pipeline.PipelineStats        // 管道统计信息
	recommender  *recommendation.RepairRecommender // 修复推荐器
	closed       bool                           // 是否已关闭
}

// NewRepairAdvisor 创建修复推荐服务实例
// repo: 数据仓库
// madmCfg: 多属性决策配置
// bufferSize: 输出通道缓冲区大小
func NewRepairAdvisor(repo *repository.Repository, madmCfg config.MADMConfig, bufferSize int) *RepairAdvisor {
	return &RepairAdvisor{
		repo:        repo,
		madmConfig:  madmCfg,
		recommender: recommendation.NewRepairRecommender(repo),
		stats:       &pipeline.PipelineStats{},
		OutChan:     make(chan pipeline.RepairResult, bufferSize),
		closed:      false,
	}
}

func (ra *RepairAdvisor) SetInputChannel(in <-chan pipeline.RepairRequest) {
	ra.InChan = in
}

func (ra *RepairAdvisor) SetOutputChannel(out chan<- pipeline.RepairResult) {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	if ra.OutChan != nil {
		close(ra.OutChan)
	}
	ra.OutChan = out
}

// SetAlertChannel 设置告警输出通道（可选）
func (ra *RepairAdvisor) SetAlertChannel(alertOut chan<- pipeline.AlertMsg) {
	ra.AlertOutChan = alertOut
}

// Run 启动主循环，从输入通道读取请求并生成推荐结果
// 单 worker 模式运行
func (ra *RepairAdvisor) Run(ctx context.Context) error {
	if ra.InChan == nil {
		return fmt.Errorf("input channel not set")
	}

	log.Println("[RepairAdvisor] 启动修复推荐服务（单 worker 模式）")

	for {
		select {
		case <-ctx.Done():
			log.Println("[RepairAdvisor] 收到退出信号，停止主循环")
			return ctx.Err()
		case req, ok := <-ra.InChan:
			if !ok {
				log.Println("[RepairAdvisor] 输入通道已关闭，停止主循环")
				return nil
			}

			result, err := ra.RecommendForSegment(ctx, req)
			if err != nil {
				log.Printf("[RepairAdvisor] 推荐失败: segment=%s, error=%v", req.SegmentID, err)
				result.Error = err.Error()
			}

			ra.mu.Lock()
			ra.stats.RepairProcessed++
			if err != nil {
				ra.stats.EvalErrors++
			}
			ra.mu.Unlock()

			select {
			case ra.OutChan <- result:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// RecommendForSegment 对单个结构段生成修复推荐
// 从数据库获取结构段数据，调用推荐算法生成推荐结果
func (ra *RepairAdvisor) RecommendForSegment(ctx context.Context, req pipeline.RepairRequest) (pipeline.RepairResult, error) {
	result := pipeline.RepairResult{
		RequestID:   req.RequestID,
		SegmentID:   req.SegmentID,
		AqueductID:  req.AqueductID,
		ProcessedAt: time.Now().UTC(),
	}

	// 从数据库获取结构段数据（含状态信息）
	segment, err := ra.repo.GetSegmentByIDWithStatus(ctx, req.SegmentID)
	if err != nil {
		return result, fmt.Errorf("获取结构段数据失败: %w", err)
	}
	if segment == nil {
		return result, fmt.Errorf("结构段不存在: %s", req.SegmentID)
	}

	// 调用推荐器生成推荐
	rec, err := ra.recommender.RecommendForSegment(ctx, segment)
	if err != nil {
		return result, fmt.Errorf("生成推荐失败: %w", err)
	}

	result.Recommendation = rec

	// 如果置信度低且设置了告警通道，发送告警升级
	if ra.AlertOutChan != nil {
		if scores, ok := rec.DecisionScores["sensitivity_analysis"].(map[string]interface{}); ok {
			if confidence, ok := scores["confidence_score"].(float64); ok && confidence < 0.6 {
				alert := pipeline.AlertMsg{
					ID:        "low_confidence_" + req.RequestID,
					Source:    "repair_advisor",
					Timestamp: time.Now().UTC(),
					Priority:  2,
				}
				select {
				case ra.AlertOutChan <- alert:
				default:
					log.Printf("[RepairAdvisor] 告警通道已满，跳过低置信度告警")
				}
			}
		}
	}

	return result, nil
}

// RunWorkers 启动多个 worker 并行处理推荐请求
// count: worker 数量
func (ra *RepairAdvisor) RunWorkers(ctx context.Context, count int) {
	if count <= 0 {
		count = 1
	}

	log.Printf("[RepairAdvisor] 启动 %d 个推荐 worker", count)

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ra.workerLoop(ctx, workerID)
		}(i)
	}

	wg.Wait()
	log.Println("[RepairAdvisor] 所有 worker 已退出")
}

// workerLoop 单个 worker 的处理循环
func (ra *RepairAdvisor) workerLoop(ctx context.Context, workerID int) {
	log.Printf("[RepairAdvisor] Worker-%d 启动", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[RepairAdvisor] Worker-%d 收到退出信号", workerID)
			return
		case req, ok := <-ra.InChan:
			if !ok {
				log.Printf("[RepairAdvisor] Worker-%d 输入通道已关闭", workerID)
				return
			}

			result, err := ra.RecommendForSegment(ctx, req)
			if err != nil {
				log.Printf("[RepairAdvisor] Worker-%d 推荐失败: segment=%s, error=%v",
					workerID, req.SegmentID, err)
				result.Error = err.Error()
			}

			ra.mu.Lock()
			ra.stats.RepairProcessed++
			if err != nil {
				ra.stats.EvalErrors++
			}
			ra.mu.Unlock()

			select {
			case ra.OutChan <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

// GetStats 获取管道统计信息
func (ra *RepairAdvisor) GetStats() pipeline.PipelineStats {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	stats := *ra.stats

	// 计算队列大小
	stats.QueueSizeRepair = len(ra.OutChan)

	return stats
}

// Close 关闭推荐服务，关闭输出通道
func (ra *RepairAdvisor) Close() {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	if ra.closed {
		return
	}

	ra.closed = true
	close(ra.OutChan)

	if ra.AlertOutChan != nil {
		// 注意：不关闭告警通道，因为可能有其他生产者
	}

	log.Println("[RepairAdvisor] 服务已关闭")
}
