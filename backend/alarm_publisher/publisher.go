package alarm_publisher

import (
	"context"
	"fmt"
	"log"
	"sync"

	"aqueduct-monitor/mqtt"
	"aqueduct-monitor/pipeline"
	"aqueduct-monitor/repository"
)

// AlarmPublisher 告警MQTT推送服务
// 封装 mqtt.AlertPublisher，提供管道式的告警推送能力
type AlarmPublisher struct {
	InChan        <-chan pipeline.AlertMsg // 输入通道
	mqttPublisher *mqtt.AlertPublisher     // 内部MQTT发布器
	repo          *repository.Repository   // 数据仓库
	mu            sync.Mutex               // 互斥锁
	stats         *pipeline.PipelineStats  // 统计信息
	closed        bool                     // 是否已关闭
}

// NewAlarmPublisher 创建告警推送器
// mqttPublisher: MQTT告警发布器
// repo: 数据仓库
// bufferSize: 预留参数（当前由调用方传入InChan时决定缓冲大小）
func NewAlarmPublisher(mqttPublisher *mqtt.AlertPublisher, repo *repository.Repository, bufferSize int) *AlarmPublisher {
	return &AlarmPublisher{
		mqttPublisher: mqttPublisher,
		repo:          repo,
		stats:         &pipeline.PipelineStats{},
		closed:        false,
	}
}

// SetInChan 设置输入通道
// 用于在创建后绑定输入通道
func (p *AlarmPublisher) SetInChan(in <-chan pipeline.AlertMsg) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.InChan = in
}

func (p *AlarmPublisher) SetInputChannel(in <-chan pipeline.AlertMsg) {
	p.SetInChan(in)
}

// Run 主循环，从输入通道读取告警消息并推送
// ctx: 上下文，用于取消
func (p *AlarmPublisher) Run(ctx context.Context) error {
	if p.InChan == nil {
		return fmt.Errorf("input channel not set")
	}

	log.Println("[AlarmPublisher] 启动告警推送服务")

	for {
		select {
		case <-ctx.Done():
			log.Println("[AlarmPublisher] 收到停止信号，正在退出")
			return ctx.Err()

		case alertMsg, ok := <-p.InChan:
			if !ok {
				log.Println("[AlarmPublisher] 输入通道已关闭")
				return nil
			}

			p.mu.Lock()
			p.stats.AlertsBuffered = int64(len(p.InChan))
			p.mu.Unlock()

			err := p.publishAlertInternal(ctx, &alertMsg)
			if err != nil {
				log.Printf("[AlarmPublisher] 告警推送处理: %v", err)
			}

			p.mu.Lock()
			p.stats.AlertsPublished++
			p.stats.AlertsBuffered = int64(len(p.InChan))
			p.stats.QueueSizeAlert = len(p.InChan)
			p.mu.Unlock()
		}
	}
}

func (p *AlarmPublisher) RunWorkers(ctx context.Context, count int) {
	if count <= 0 {
		count = 1
	}
	log.Printf("[AlarmPublisher] 启动 %d 个告警推送 worker", count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			p.workerLoop(ctx, id)
		}(i)
	}
	wg.Wait()
	log.Println("[AlarmPublisher] 所有 worker 已退出")
}

func (p *AlarmPublisher) workerLoop(ctx context.Context, workerID int) {
	log.Printf("[AlarmPublisher] Worker-%d 启动", workerID)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[AlarmPublisher] Worker-%d 收到退出信号", workerID)
			return
		case alertMsg, ok := <-p.InChan:
			if !ok {
				log.Printf("[AlarmPublisher] Worker-%d 输入通道已关闭", workerID)
				return
			}
			err := p.publishAlertInternal(ctx, &alertMsg)
			if err != nil {
				log.Printf("[AlarmPublisher] Worker-%d 推送失败: %v", workerID, err)
			}
			p.mu.Lock()
			p.stats.AlertsPublished++
			p.stats.QueueSizeAlert = len(p.InChan)
			p.mu.Unlock()
		}
	}
}

// publishAlertInternal 内部发布告警消息
// 封装 mqttPublisher.PublishAlert 调用
func (p *AlarmPublisher) publishAlertInternal(ctx context.Context, msg *pipeline.AlertMsg) error {
	if msg == nil || msg.Alert == nil {
		return fmt.Errorf("nil alert message")
	}

	err := p.mqttPublisher.PublishAlert(ctx, msg.Alert)
	if err != nil {
		// MQTT包内部已处理离线队列，这里只记录日志
		log.Printf("[AlarmPublisher] 告警 %s 推送失败，已进入离线队列: %v", msg.ID, err)
	}

	return nil
}

// PublishAlert 同步发送单个告警的便利方法
// ctx: 上下文
// alert: 告警消息
func (p *AlarmPublisher) PublishAlert(ctx context.Context, alert *pipeline.AlertMsg) error {
	if alert == nil {
		return fmt.Errorf("nil alert")
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("publisher is closed")
	}
	p.mu.Unlock()

	return p.publishAlertInternal(ctx, alert)
}

// FlushOffline 强制刷新离线队列
// 立即尝试发送所有离线队列中的消息
// 返回成功发送的消息数量
func (p *AlarmPublisher) FlushOffline() error {
	if p.mqttPublisher == nil {
		return fmt.Errorf("mqtt publisher not available")
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("publisher is closed")
	}
	p.mu.Unlock()

	sent := p.mqttPublisher.FlushOfflineQueue()
	if sent > 0 {
		log.Printf("[AlarmPublisher] 离线队列刷新完成，成功发送 %d 条消息", sent)

		p.mu.Lock()
		p.stats.AlertsPublished += int64(sent)
		p.mu.Unlock()
	}

	return nil
}

// QueueStats 获取队列状态
// 返回离线队列大小、连接状态等信息
func (p *AlarmPublisher) QueueStats() map[string]interface{} {
	if p.mqttPublisher == nil {
		return map[string]interface{}{
			"offline_size": 0,
			"is_connected": false,
		}
	}

	mqttStats := p.mqttPublisher.QueueStats()

	p.mu.Lock()
	defer p.mu.Unlock()

	result := make(map[string]interface{})
	for k, v := range mqttStats {
		result[k] = v
	}
	result["input_buffer_size"] = len(p.InChan)
	result["alerts_published"] = p.stats.AlertsPublished

	return result
}

// GetStats 获取管道统计信息
func (p *AlarmPublisher) GetStats() pipeline.PipelineStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := *p.stats
	stats.QueueSizeAlert = len(p.InChan)

	return stats
}

// Close 关闭告警推送器
// 保存离线队列状态
func (p *AlarmPublisher) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	log.Println("[AlarmPublisher] 正在关闭告警推送服务")

	if p.mqttPublisher != nil {
		p.mqttPublisher.Close()
	}

	log.Println("[AlarmPublisher] 告警推送服务已关闭")
}
