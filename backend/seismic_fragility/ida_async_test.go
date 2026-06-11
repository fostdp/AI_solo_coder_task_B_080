package seismic_fragility

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"aqueduct-monitor/config"
	"aqueduct-monitor/models"
)

func makeTestSegmentForAsync(name string) models.StructureSegment {
	return models.StructureSegment{
		ID:                 uuid.New(),
		AqueductID:         uuid.New(),
		SegmentType:        "arch",
		SegmentIndex:       1,
		DesignStrength:     25.0,
		DesignLoadCapacity: 150.0,
		OriginalMaterial:   "roman_concrete",
		WeatheringDepth:    15.0,
		CurrentStress:      80.0,
		SettlementMM:       2.0,
	}
}

func TestNewIDATaskManager(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("default_worker_count", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 0)
		if tm == nil {
			t.Fatal("task manager should not be nil")
		}
		if tm.workerCount != 2 {
			t.Errorf("expected default worker count 2, got %d", tm.workerCount)
		}
	})

	t.Run("custom_worker_count", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 5)
		if tm.workerCount != 5 {
			t.Errorf("expected worker count 5, got %d", tm.workerCount)
		}
	})

	t.Run("initial_state", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		if len(tm.tasks) != 0 {
			t.Error("initial task map should be empty")
		}
		if tm.running {
			t.Error("manager should not be running initially")
		}
	})
}

func TestIDATaskManager_StartStop(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("start_stop_single", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		if !tm.running {
			t.Error("manager should be running after Start()")
		}
		tm.Stop()
		if tm.running {
			t.Error("manager should not be running after Stop()")
		}
	})

	t.Run("double_start_no_panic", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		tm.Start()
		tm.Stop()
	})

	t.Run("double_stop_no_panic", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		tm.Stop()
		tm.Stop()
	})
}

func TestIDATaskManager_SubmitTask(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("submit_not_running", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		seg := makeTestSegmentForAsync("test")
		taskID, err := tm.SubmitTask(&seg)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if taskID == uuid.Nil {
			t.Error("task ID should not be nil")
		}

		task, ok := tm.GetTask(taskID)
		if !ok {
			t.Fatal("task should exist")
		}
		if task.Status != TaskStatusPending {
			t.Errorf("expected status PENDING, got %s", task.Status)
		}
	})

	t.Run("submit_then_start", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		seg := makeTestSegmentForAsync("test")
		taskID, _ := tm.SubmitTask(&seg)

		tm.Start()
		defer tm.Stop()

		time.Sleep(100 * time.Millisecond)

		status, ok := tm.GetTaskStatus(taskID)
		if !ok {
			t.Fatal("task should exist")
		}
		if status != TaskStatusCompleted && status != TaskStatusRunning {
			t.Errorf("expected status COMPLETED or RUNNING, got %s", status)
		}
	})
}

func TestIDATaskManager_GetTask(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("existing_task", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		seg := makeTestSegmentForAsync("test")
		taskID, _ := tm.SubmitTask(&seg)

		task, ok := tm.GetTask(taskID)
		if !ok {
			t.Fatal("task should be found")
		}
		if task.TaskID != taskID {
			t.Error("task ID mismatch")
		}
	})

	t.Run("nonexistent_task", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		_, ok := tm.GetTask(uuid.New())
		if ok {
			t.Error("non-existent task should not be found")
		}
	})
}

func TestIDATaskManager_GetTaskStatus(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("pending_status", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		seg := makeTestSegmentForAsync("test")
		taskID, _ := tm.SubmitTask(&seg)

		status, ok := tm.GetTaskStatus(taskID)
		if !ok {
			t.Fatal("task should be found")
		}
		if status != TaskStatusPending {
			t.Errorf("expected PENDING, got %s", status)
		}
	})

	t.Run("completed_status", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		defer tm.Stop()

		seg := makeTestSegmentForAsync("test")
		taskID, _ := tm.SubmitTask(&seg)

		time.Sleep(100 * time.Millisecond)

		status, ok := tm.GetTaskStatus(taskID)
		if !ok {
			t.Fatal("task should be found")
		}
		if status != TaskStatusCompleted {
			t.Errorf("expected COMPLETED, got %s", status)
		}
	})
}

func TestIDATaskManager_GetTaskResult(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("completed_task_has_results", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		defer tm.Stop()

		seg := makeTestSegmentForAsync("test")
		taskID, _ := tm.SubmitTask(&seg)

		time.Sleep(100 * time.Millisecond)

		results, err, ready := tm.GetTaskResult(taskID)
		if !ready {
			t.Fatal("task result should be ready")
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(results) == 0 {
			t.Error("results should not be empty")
		}
	})

	t.Run("pending_task_not_ready", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		seg := makeTestSegmentForAsync("test")
		taskID, _ := tm.SubmitTask(&seg)

		_, _, ready := tm.GetTaskResult(taskID)
		if ready {
			t.Error("pending task should not be ready")
		}
	})

	t.Run("nonexistent_task", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		_, _, ready := tm.GetTaskResult(uuid.New())
		if ready {
			t.Error("non-existent task should not be ready")
		}
	})
}

func TestIDATaskManager_ListTasks(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("empty_list", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tasks := tm.ListTasks()
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("multiple_tasks", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		for i := 0; i < 5; i++ {
			seg := makeTestSegmentForAsync("test")
			tm.SubmitTask(&seg)
		}

		tasks := tm.ListTasks()
		if len(tasks) != 5 {
			t.Errorf("expected 5 tasks, got %d", len(tasks))
		}
	})
}

func TestIDATaskManager_CleanupCompleted(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("no_cleanup_needed", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		defer tm.Stop()

		seg := makeTestSegmentForAsync("test")
		tm.SubmitTask(&seg)
		time.Sleep(50 * time.Millisecond)

		removed := tm.CleanupCompleted(time.Hour)
		if removed != 0 {
			t.Errorf("expected 0 removed tasks, got %d", removed)
		}
	})

	t.Run("cleanup_old_tasks", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()

		seg := makeTestSegmentForAsync("old")
		taskID, _ := tm.SubmitTask(&seg)
		time.Sleep(50 * time.Millisecond)

		tm.Stop()

		task, _ := tm.GetTask(taskID)
		oldTime := time.Now().UTC().Add(-2 * time.Hour)
		task.CompletedAt = &oldTime

		removed := tm.CleanupCompleted(time.Hour)
		if removed < 1 {
			t.Errorf("expected at least 1 removed task, got %d", removed)
		}
	})
}

func TestComputeIDAGoroutine(t *testing.T) {
	cfg := defaultSeismicConfig()
	seg := makeTestSegmentForAsync("test")

	resultChan, err := ComputeIDAGoroutine(&seg, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := <-resultChan
	if !ok {
		t.Fatal("channel should have result")
	}

	if result.Err != nil {
		t.Errorf("unexpected error in result: %v", result.Err)
	}
	if len(result.Results) == 0 {
		t.Error("results should not be empty")
	}
}

func TestComputeIDAGoroutine_Concurrent(t *testing.T) {
	cfg := defaultSeismicConfig()
	count := 10

	var wg sync.WaitGroup
	results := make([]<-chan IDAResult, count)

	for i := 0; i < count; i++ {
		seg := makeTestSegmentForAsync("test")
		ch, err := ComputeIDAGoroutine(&seg, cfg)
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
		results[i] = ch
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-results[idx]
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent IDA computations")
	}
}

func TestIDATaskManager_BatchTasks(t *testing.T) {
	cfg := defaultSeismicConfig()

	t.Run("submit_batch", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		segments := make([]models.StructureSegment, 5)
		for i := 0; i < 5; i++ {
			segments[i] = makeTestSegmentForAsync("test")
		}

		batchID, err := tm.SubmitBatchTasks(segments)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if batchID == uuid.Nil {
			t.Error("batch ID should not be nil")
		}

		tasks := tm.ListTasks()
		if len(tasks) != 5 {
			t.Errorf("expected 5 tasks, got %d", len(tasks))
		}
	})

	t.Run("batch_progress", func(t *testing.T) {
		tm := NewIDATaskManager(cfg, 2)
		tm.Start()
		defer tm.Stop()

		segments := make([]models.StructureSegment, 3)
		taskIDs := make([]uuid.UUID, 3)
		for i := 0; i < 3; i++ {
			segments[i] = makeTestSegmentForAsync("test")
			tid, _ := tm.SubmitTask(&segments[i])
			taskIDs[i] = tid
		}

		time.Sleep(100 * time.Millisecond)

		completed, total, failed := tm.BatchProgress(taskIDs)
		if total != 3 {
			t.Errorf("expected total 3, got %d", total)
		}
		if completed != 3 {
			t.Errorf("expected 3 completed, got %d", completed)
		}
		if failed != 0 {
			t.Errorf("expected 0 failed, got %d", failed)
		}
	})
}

func TestIDATaskStatus_Values(t *testing.T) {
	testCases := []struct {
		status IDATaskStatus
		desc   string
	}{
		{TaskStatusPending, "PENDING"},
		{TaskStatusRunning, "RUNNING"},
		{TaskStatusCompleted, "COMPLETED"},
		{TaskStatusFailed, "FAILED"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if string(tc.status) != tc.desc {
				t.Errorf("expected %s, got %s", tc.desc, string(tc.status))
			}
		})
	}
}

func TestIDATask_Structure(t *testing.T) {
	taskID := uuid.New()
	segID := uuid.New()
	now := time.Now().UTC()

	task := &IDATask{
		TaskID:      taskID,
		SegmentID:   segID,
		Status:      TaskStatusPending,
		Results:     nil,
		Error:       nil,
		CreatedAt:   now,
		StartedAt:   nil,
		CompletedAt: nil,
	}

	if task.TaskID != taskID {
		t.Error("TaskID mismatch")
	}
	if task.SegmentID != segID {
		t.Error("SegmentID mismatch")
	}
	if task.Status != TaskStatusPending {
		t.Error("Status mismatch")
	}
	if !task.CreatedAt.Equal(now) {
		t.Error("CreatedAt mismatch")
	}
}

func TestBatchIDATask_Structure(t *testing.T) {
	batchID := uuid.New()
	taskIDs := []uuid.UUID{uuid.New(), uuid.New()}
	now := time.Now().UTC()

	batch := &BatchIDATask{
		BatchID:   batchID,
		TaskIDs:   taskIDs,
		CreatedAt: now,
	}

	if batch.BatchID != batchID {
		t.Error("BatchID mismatch")
	}
	if len(batch.TaskIDs) != 2 {
		t.Error("TaskIDs count mismatch")
	}
}
