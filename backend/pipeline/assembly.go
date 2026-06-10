package pipeline

import (
	"context"
	"log"
	"sync"
	"time"

	"aqueduct-monitor/alarm_publisher"
	"aqueduct-monitor/config"
	"aqueduct-monitor/dtu_receiver"
	"aqueduct-monitor/mqtt"
	"aqueduct-monitor/repair_advisor"
	"aqueduct-monitor/repository"
	"aqueduct-monitor/structural_evaluator"
)

type Pipeline struct {
	cfg        *config.Config
	repo       *repository.Repository
	mqttPub    *mqtt.AlertPublisher

	DTUReceiver         *dtu_receiver.DTUReceiver
	StructuralEvaluator *structural_evaluator.StructuralEvaluator
	RepairAdvisor       *repair_advisor.RepairAdvisor
	AlarmPublisher      *alarm_publisher.AlarmPublisher

	sensorChan    chan SensorReadingMsg
	evalOutChan   chan EvalResult
	repairReqChan chan RepairRequest
	repairOutChan chan RepairResult
	alertChan     chan AlertMsg

	stats   PipelineStats
	mu      sync.Mutex

	workerWg sync.WaitGroup
	running  bool
	muRun    sync.Mutex
}

func NewPipeline(cfg *config.Config, repo *repository.Repository, mqttPub *mqtt.AlertPublisher) *Pipeline {
	bufSize := cfg.Pipeline.BufferSize
	if bufSize <= 0 {
		bufSize = 200
	}

	sensorChan := make(chan SensorReadingMsg, bufSize)
	evalOutChan := make(chan EvalResult, bufSize)
	repairReqChan := make(chan RepairRequest, bufSize)
	repairOutChan := make(chan RepairResult, bufSize/2)
	alertChan := make(chan AlertMsg, bufSize)

	dtuRecv := dtu_receiver.NewDTUReceiver(repo, bufSize)
	eval := structural_evaluator.NewStructuralEvaluator(repo, cfg.FEA, cfg.Threshold, bufSize)
	repair := repair_advisor.NewRepairAdvisor(repo, cfg.MADM, bufSize)
	alarm := alarm_publisher.NewAlarmPublisher(mqttPub, repo, bufSize)

	return &Pipeline{
		cfg:                 cfg,
		repo:                repo,
		mqttPub:             mqttPub,
		DTUReceiver:         dtuRecv,
		StructuralEvaluator: eval,
		RepairAdvisor:       repair,
		AlarmPublisher:      alarm,
		sensorChan:          sensorChan,
		evalOutChan:         evalOutChan,
		repairReqChan:       repairReqChan,
		repairOutChan:       repairOutChan,
		alertChan:           alertChan,
	}
}

func (p *Pipeline) Start(ctx context.Context) error {
	p.muRun.Lock()
	if p.running {
		p.muRun.Unlock()
		return nil
	}
	p.running = true
	p.muRun.Unlock()

	log.Println("➤ Starting processing pipeline...")

	p.DTUReceiver.SetOutputChannel(p.sensorChan)
	p.StructuralEvaluator.SetInputChannel(p.sensorChan)
	p.StructuralEvaluator.SetOutputChannels(p.evalOutChan, p.repairReqChan)
	p.RepairAdvisor.SetInputChannel(p.repairReqChan)
	p.RepairAdvisor.SetOutputChannel(p.repairOutChan)
	p.AlarmPublisher.SetInputChannel(p.alertChan)

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerEval := p.cfg.Pipeline.WorkerCountEval
	if workerEval <= 0 {
		workerEval = 4
	}
	workerRepair := p.cfg.Pipeline.WorkerCountRepair
	if workerRepair <= 0 {
		workerRepair = 2
	}
	workerAlarm := p.cfg.Pipeline.WorkerCountAlarm
	if workerAlarm <= 0 {
		workerAlarm = 2
	}

	p.workerWg.Add(4)

	go func() {
		defer p.workerWg.Done()
		if err := p.StructuralEvaluator.Run(workerCtx); err != nil && err != context.Canceled {
			log.Printf("⚠ StructuralEvaluator Run error: %v", err)
		}
	}()

	go func() {
		defer p.workerWg.Done()
		p.StructuralEvaluator.RunWorkers(workerCtx, workerEval)
	}()

	go func() {
		defer p.workerWg.Done()
		p.RepairAdvisor.RunWorkers(workerCtx, workerRepair)
	}()

	go func() {
		defer p.workerWg.Done()
		p.AlarmPublisher.RunWorkers(workerCtx, workerAlarm)
	}()

	go p.bridgeEvalToAlerts(workerCtx)
	go p.bridgeRepairToAlerts(workerCtx)
	go p.statsAggregator(workerCtx)

	log.Printf("✓ Pipeline started with %d eval workers, %d repair workers, %d alarm workers",
		workerEval, workerRepair, workerAlarm)

	<-ctx.Done()
	log.Println("⏻ Pipeline shutting down...")

	p.DTUReceiver.Close()
	p.StructuralEvaluator.Close()
	p.RepairAdvisor.Close()
	p.AlarmPublisher.Close()

	p.workerWg.Wait()

	close(p.sensorChan)
	close(p.evalOutChan)
	close(p.repairReqChan)
	close(p.repairOutChan)
	close(p.alertChan)

	log.Println("✓ Pipeline stopped cleanly")
	return nil
}

func (p *Pipeline) bridgeEvalToAlerts(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-p.evalOutChan:
			if !ok {
				return
			}
			for i := range result.Alerts {
				alert := result.Alerts[i]
				priority := 0
				switch alert.Severity {
				case "EMERGENCY":
					priority = 4
				case "CRITICAL":
					priority = 3
				case "WARNING":
					priority = 2
				case "INFO":
					priority = 1
				}
				msg := AlertMsg{
					ID:        alert.ID.String(),
					Alert:     &alert,
					Source:    "structural_evaluator",
					Timestamp: result.ProcessedAt,
					Priority:  priority,
				}
				select {
				case p.alertChan <- msg:
				default:
					log.Printf("⚠ Alert channel full, dropped alert: %s", alert.ID)
				}
			}
		}
	}
}

func (p *Pipeline) bridgeRepairToAlerts(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-p.repairOutChan:
			if !ok {
				return
			}
			if result.Error != "" {
				log.Printf("⚠ Repair recommendation error for segment %s: %s", result.SegmentID, result.Error)
			}
		}
	}
}

func (p *Pipeline) statsAggregator(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dtuStats := p.DTUReceiver.GetStats()
			evalStats := p.StructuralEvaluator.GetStats()
			repairStats := p.RepairAdvisor.GetStats()
			alarmStats := p.AlarmPublisher.GetStats()

			p.mu.Lock()
			p.stats.DTUReceived = dtuStats.DTUReceived
			p.stats.EvalProcessed = evalStats.EvalProcessed
			p.stats.EvalErrors = evalStats.EvalErrors
			p.stats.RepairProcessed = repairStats.RepairProcessed
			p.stats.AlertsPublished = alarmStats.AlertsPublished
			p.stats.AlertsBuffered = alarmStats.AlertsBuffered
			p.stats.QueueSizeDTU = len(p.sensorChan)
			p.stats.QueueSizeEval = evalStats.QueueSizeEval
			p.stats.QueueSizeRepair = repairStats.QueueSizeRepair
			p.stats.QueueSizeAlert = alarmStats.QueueSizeAlert
			p.mu.Unlock()
		}
	}
}

func (p *Pipeline) GetStats() PipelineStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

func (p *Pipeline) Receiver() *dtu_receiver.DTUReceiver {
	return p.DTUReceiver
}

func (p *Pipeline) Evaluator() *structural_evaluator.StructuralEvaluator {
	return p.StructuralEvaluator
}

func (p *Pipeline) RepairAdviser() *repair_advisor.RepairAdvisor {
	return p.RepairAdvisor
}

func (p *Pipeline) Alarm() *alarm_publisher.AlarmPublisher {
	return p.AlarmPublisher
}
