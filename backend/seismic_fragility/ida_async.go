package seismic_fragility

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

type IDATaskStatus string

const (
	TaskStatusPending   IDATaskStatus = "PENDING"
	TaskStatusRunning   IDATaskStatus = "RUNNING"
	TaskStatusCompleted IDATaskStatus = "COMPLETED"
	TaskStatusFailed    IDATaskStatus = "FAILED"
)

type IDATask struct {
	TaskID      uuid.UUID
	SegmentID   uuid.UUID
	Status      IDATaskStatus
	Results     []models.SeismicVulnerability
	Error       error
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	segment     *models.StructureSegment
	cfg         *config.SeismicConfig
}

type IDATaskManager struct {
	cfg          *config.SeismicConfig
	tasks        map[uuid.UUID]*IDATask
	taskQueue    chan *IDATask
	workerCount  int
	mu           sync.RWMutex
	wg           sync.WaitGroup
	shutdownChan chan struct{}
	running      bool
}

func NewIDATaskManager(cfg *config.SeismicConfig, workerCount int) *IDATaskManager {
	if workerCount <= 0 {
		workerCount = 2
	}
	return &IDATaskManager{
		cfg:          cfg,
		tasks:        make(map[uuid.UUID]*IDATask),
		taskQueue:    make(chan *IDATask, 100),
		workerCount:  workerCount,
		shutdownChan: make(chan struct{}),
	}
}

func (tm *IDATaskManager) Start() {
	tm.mu.Lock()
	if tm.running {
		tm.mu.Unlock()
		return
	}
	tm.running = true
	tm.mu.Unlock()

	for i := 0; i < tm.workerCount; i++ {
		tm.wg.Add(1)
		go tm.worker()
	}
}

func (tm *IDATaskManager) Stop() {
	tm.mu.Lock()
	if !tm.running {
		tm.mu.Unlock()
		return
	}
	tm.running = false
	close(tm.shutdownChan)
	tm.mu.Unlock()

	tm.wg.Wait()
}

func (tm *IDATaskManager) worker() {
	defer tm.wg.Done()
	for {
		select {
		case <-tm.shutdownChan:
			return
		case task, ok := <-tm.taskQueue:
			if !ok {
				return
			}
			tm.executeTask(task)
		}
	}
}

func (tm *IDATaskManager) executeTask(task *IDATask) {
	now := time.Now().UTC()
	tm.mu.Lock()
	task.Status = TaskStatusRunning
	task.StartedAt = &now
	tm.mu.Unlock()

	results, err := ComputeIDA(task.segment, task.cfg)

	now = time.Now().UTC()
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if err != nil {
		task.Status = TaskStatusFailed
		task.Error = err
	} else {
		task.Status = TaskStatusCompleted
		task.Results = results
	}
	task.CompletedAt = &now
}

func (tm *IDATaskManager) SubmitTask(seg *models.StructureSegment) (uuid.UUID, error) {
	taskID := uuid.New()
	task := &IDATask{
		TaskID:    taskID,
		SegmentID: seg.ID,
		Status:    TaskStatusPending,
		CreatedAt: time.Now().UTC(),
		segment:   seg,
		cfg:       tm.cfg,
	}

	tm.mu.Lock()
	tm.tasks[taskID] = task

	if !tm.running {
		tm.mu.Unlock()
		return taskID, nil
	}

	tm.mu.Unlock()

	select {
	case tm.taskQueue <- task:
	default:
		return taskID, nil
	}

	return taskID, nil
}

func (tm *IDATaskManager) GetTask(taskID uuid.UUID) (*IDATask, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[taskID]
	return task, ok
}

func (tm *IDATaskManager) GetTaskStatus(taskID uuid.UUID) (IDATaskStatus, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[taskID]
	if !ok {
		return "", false
	}
	return task.Status, true
}

func (tm *IDATaskManager) GetTaskResult(taskID uuid.UUID) ([]models.SeismicVulnerability, error, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[taskID]
	if !ok {
		return nil, nil, false
	}
	if task.Status != TaskStatusCompleted && task.Status != TaskStatusFailed {
		return nil, nil, false
	}
	return task.Results, task.Error, true
}

func (tm *IDATaskManager) ListTasks() []*IDATask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]*IDATask, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		result = append(result, task)
	}
	return result
}

func (tm *IDATaskManager) CleanupCompleted(olderThan time.Duration) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	cutoff := time.Now().UTC().Add(-olderThan)
	count := 0
	for id, task := range tm.tasks {
		if (task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed) &&
			task.CompletedAt != nil && task.CompletedAt.Before(cutoff) {
			delete(tm.tasks, id)
			count++
		}
	}
	return count
}

func ComputeIDAGoroutine(seg *models.StructureSegment, cfg *config.SeismicConfig) (<-chan IDAResult, error) {
	resultChan := make(chan IDAResult, 1)

	go func() {
		defer close(resultChan)
		results, err := ComputeIDA(seg, cfg)
		resultChan <- IDAResult{Results: results, Err: err}
	}()

	return resultChan, nil
}

type BatchIDATask struct {
	BatchID   uuid.UUID
	TaskIDs   []uuid.UUID
	CreatedAt time.Time
}

func (tm *IDATaskManager) SubmitBatchTasks(segments []models.StructureSegment) (uuid.UUID, error) {
	batchID := uuid.New()
	taskIDs := make([]uuid.UUID, 0, len(segments))

	for i := range segments {
		taskID, err := tm.SubmitTask(&segments[i])
		if err != nil {
			continue
		}
		taskIDs = append(taskIDs, taskID)
	}

	return batchID, nil
}

func (tm *IDATaskManager) BatchProgress(taskIDs []uuid.UUID) (completed int, total int, failed int) {
	total = len(taskIDs)
	for _, tid := range taskIDs {
		status, ok := tm.GetTaskStatus(tid)
		if !ok {
			continue
		}
		switch status {
		case TaskStatusCompleted:
			completed++
		case TaskStatusFailed:
			failed++
		}
	}
	return
}
