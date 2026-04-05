package ayenitd

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
