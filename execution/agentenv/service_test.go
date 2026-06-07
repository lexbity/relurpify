package agentenv

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type mockService struct {
	startCount atomic.Int32
	stopCount  atomic.Int32
	blockStart bool
	startErr   error
	stopErr    error
	startCh    chan struct{}
}

func (m *mockService) Start(ctx context.Context) error {
	m.startCount.Add(1)
	if m.blockStart {
		<-ctx.Done()
		return ctx.Err()
	}
	if m.startCh != nil {
		close(m.startCh)
	}
	return m.startErr
}

func (m *mockService) Stop() error {
	m.stopCount.Add(1)
	return m.stopErr
}

func TestServiceManager_RegisterAndGet(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	sm.Register("test", svc)
	if got := sm.Get("test"); got != svc {
		t.Errorf("Get returned %v, want %v", got, svc)
	}
	if got := sm.Get("missing"); got != nil {
		t.Errorf("Get missing returned %v, want nil", got)
	}
}

func TestServiceManager_StartStopAndSnapshot(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	sm.RegisterWithInfo("svc", svc, ServiceRegistrationInfo{Source: "test/source", Owner: "workspace", Notes: []string{"note one", "note two"}})

	if err := sm.Start("svc", context.Background()); err != nil {
		t.Fatalf("Start returned %v", err)
	}
	if svc.startCount.Load() != 1 {
		t.Fatalf("startCount = %d, want 1", svc.startCount.Load())
	}
	snapshots := sm.Snapshot()
	if len(snapshots) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshots))
	}
	if snapshots[0].Status != serviceStatusRunning {
		t.Fatalf("Snapshot status = %q, want %q", snapshots[0].Status, serviceStatusRunning)
	}
	if snapshots[0].Source != "test/source" {
		t.Fatalf("Snapshot source = %q, want test/source", snapshots[0].Source)
	}
	if snapshots[0].Owner != "workspace" {
		t.Fatalf("Snapshot owner = %q, want workspace", snapshots[0].Owner)
	}
	if len(snapshots[0].Notes) != 2 {
		t.Fatalf("Snapshot notes len = %d, want 2", len(snapshots[0].Notes))
	}

	if err := sm.Stop("svc"); err != nil {
		t.Fatalf("Stop returned %v", err)
	}
	if svc.stopCount.Load() != 1 {
		t.Fatalf("stopCount = %d, want 1", svc.stopCount.Load())
	}
	snapshots = sm.Snapshot()
	if len(snapshots) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshots))
	}
	if snapshots[0].Status != serviceStatusStopped {
		t.Fatalf("Snapshot status = %q, want %q", snapshots[0].Status, serviceStatusStopped)
	}
}

func TestServiceManager_Deregister(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	sm.Register("test", svc)
	if !sm.Has("test") {
		t.Error("Has should return true after register")
	}
	sm.Deregister("test")
	if sm.Has("test") {
		t.Error("Has should return false after deregister")
	}
	// Deregister again should be safe
	sm.Deregister("test")
}

func TestServiceManager_DeregisterStopError(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{stopErr: context.Canceled}
	sm.Register("test", svc)
	sm.Deregister("test")
	if svc.stopCount.Load() != 1 {
		t.Fatalf("stopCount = %d, want 1", svc.stopCount.Load())
	}
	if sm.Has("test") {
		t.Fatal("service still registered after deregister")
	}
}

func TestServiceManager_StartAll(t *testing.T) {
	sm := NewServiceManager()
	svc1 := &mockService{startCh: make(chan struct{})}
	svc2 := &mockService{}
	sm.Register("svc1", svc1)
	sm.Register("svc2", svc2)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := sm.StartAll(ctx)
	if err != nil {
		t.Errorf("StartAll returned unexpected error: %v", err)
	}
	// Wait for start to be called at least once.
	select {
	case <-svc1.startCh:
	case <-ctx.Done():
		t.Fatal("svc1 start not called")
	}
	if svc1.startCount.Load() != 1 {
		t.Errorf("svc1 startCount = %d, want 1", svc1.startCount.Load())
	}
	if svc2.startCount.Load() != 1 {
		t.Errorf("svc2 startCount = %d, want 1", svc2.startCount.Load())
	}
}

func TestServiceManager_StartAllEmpty(t *testing.T) {
	sm := NewServiceManager()
	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll(empty) returned %v", err)
	}
}

func TestServiceManager_StartAllWithError(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("svc", &mockService{startErr: context.Canceled})
	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll returned unexpected error: %v", err)
	}
}

func TestServiceScheduler_StartAndStop(t *testing.T) {
	scheduler := NewServiceScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{}, 1)
	scheduler.Register(ScheduledJob{
		ID:       "job-1",
		Interval: time.Minute,
		Action: func(context.Context) error {
			started <- struct{}{}
			return nil
		},
	})

	startErr := make(chan error, 1)
	go func() {
		startErr <- scheduler.Start(ctx)
	}()

	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Start returned %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("scheduler Start blocked")
	}

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("scheduler job did not run on start")
	}

	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop returned %v", err)
	}
}

func TestServiceManager_StopAll(t *testing.T) {
	sm := NewServiceManager()
	svc1 := &mockService{}
	svc2 := &mockService{}
	sm.Register("svc1", svc1)
	sm.Register("svc2", svc2)
	// Start to have them running (not required for stop test)
	ctx := context.Background()
	_ = sm.StartAll(ctx)
	// Stop
	err := sm.StopAll()
	if err != nil {
		t.Errorf("StopAll returned unexpected error: %v", err)
	}
	if svc1.stopCount.Load() != 1 {
		t.Errorf("svc1 stopCount = %d, want 1", svc1.stopCount.Load())
	}
	if svc2.stopCount.Load() != 1 {
		t.Errorf("svc2 stopCount = %d, want 1", svc2.stopCount.Load())
	}
	// Registry should still be intact (StopAll does not clear)
	if !sm.Has("svc1") || !sm.Has("svc2") {
		t.Error("Registry cleared unexpectedly after StopAll")
	}
}

func TestServiceManager_StopAllWithErrors(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("svc1", &mockService{stopErr: context.Canceled})
	sm.Register("svc2", &mockService{stopErr: context.DeadlineExceeded})
	if err := sm.StopAll(); err == nil {
		t.Fatal("StopAll should return error when services fail to stop")
	}
}

func TestServiceManager_Clear(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	sm.Register("svc", svc)
	ctx := context.Background()
	_ = sm.StartAll(ctx)
	err := sm.Clear()
	if err != nil {
		t.Errorf("Clear returned unexpected error: %v", err)
	}
	if svc.stopCount.Load() != 1 {
		t.Errorf("stopCount = %d, want 1", svc.stopCount.Load())
	}
	if sm.Has("svc") {
		t.Error("Registry not cleared after Clear")
	}
}

func TestServiceManager_ClearWithErrors(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("svc", &mockService{stopErr: context.Canceled})
	if err := sm.Clear(); err == nil {
		t.Fatal("Clear should return stop error")
	}
	if sm.Count() != 0 {
		t.Fatalf("Count = %d, want 0 after Clear", sm.Count())
	}
}

func TestServiceManager_Count(t *testing.T) {
	sm := NewServiceManager()
	if sm.Count() != 0 {
		t.Errorf("Count = %d, want 0", sm.Count())
	}
	sm.Register("a", &mockService{})
	if sm.Count() != 1 {
		t.Errorf("Count = %d, want 1", sm.Count())
	}
	sm.Register("b", &mockService{})
	if sm.Count() != 2 {
		t.Errorf("Count = %d, want 2", sm.Count())
	}
	sm.Deregister("a")
	if sm.Count() != 1 {
		t.Errorf("Count = %d, want 1", sm.Count())
	}
}
