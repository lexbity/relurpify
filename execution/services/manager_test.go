package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type mgrService struct {
	startErr error
	stopErr  error
	starts   atomic.Int32
	stops    atomic.Int32
	block    time.Duration
}

func (s *mgrService) Start(ctx context.Context) error {
	s.starts.Add(1)
	if s.block > 0 {
		time.Sleep(s.block)
	}
	return s.startErr
}

func (s *mgrService) Stop() error {
	s.stops.Add(1)
	return s.stopErr
}

func TestServiceManager_RegisterAndSnapshot(t *testing.T) {
	sm := NewServiceManager()
	sm.RegisterWithInfo("svc", &mgrService{}, ServiceRegistrationInfo{
		Source: "test", Owner: "workspace", Notes: []string{"  note  ", "", "second"},
	})
	if !sm.Has("svc") {
		t.Fatal("service must be registered")
	}
	if got := sm.Count(); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
	if sm.Get("svc") == nil {
		t.Fatal("Get must return the registered service")
	}
	if sm.Get("missing") != nil {
		t.Fatal("Get of a missing id must return nil")
	}
	snaps := sm.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	s := snaps[0]
	if s.Status != "stopped" {
		t.Errorf("initial status = %q, want stopped", s.Status)
	}
	if s.Source != "test" || s.Owner != "workspace" {
		t.Errorf("provenance = %q/%q", s.Source, s.Owner)
	}
	// normalize: blank notes dropped, notes trimmed.
	if len(s.Notes) != 2 || s.Notes[0] != "note" || s.Notes[1] != "second" {
		t.Errorf("normalized notes = %v", s.Notes)
	}
	if ids := sm.ListIDs(); len(ids) != 1 || ids[0] != "svc" {
		t.Errorf("ListIDs = %v", ids)
	}
}

func TestServiceManager_StartStopLifecycle(t *testing.T) {
	sm := NewServiceManager()
	svc := &mgrService{}
	sm.Register("lifecycle", svc)

	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	if svc.starts.Load() != 1 {
		t.Fatalf("starts = %d, want 1", svc.starts.Load())
	}
	if snap := sm.Snapshot(); snap[0].Status != "running" {
		t.Fatalf("status after start = %q, want running", snap[0].Status)
	}

	if err := sm.StopAll(); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	if svc.stops.Load() != 1 {
		t.Fatalf("stops = %d, want 1", svc.stops.Load())
	}
	if snap := sm.Snapshot(); snap[0].Status != "stopped" {
		t.Fatalf("status after stop = %q, want stopped", snap[0].Status)
	}
}

func TestServiceManager_StartFailureMarksError(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("broken", &mgrService{startErr: errors.New("boom")})
	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("a failed start must not fail StartAll (logged, others proceed): %v", err)
	}
	if snap := sm.Snapshot(); snap[0].Status != "error" {
		t.Fatalf("status after failed start = %q, want error", snap[0].Status)
	}
}

func TestServiceManager_StopErrorSurfaces(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("stubborn", &mgrService{stopErr: errors.New("no")})
	if err := sm.StopAll(); err == nil {
		t.Fatal("a stop error must surface from StopAll")
	}
}

func TestServiceManager_StartStopRestartSingle(t *testing.T) {
	sm := NewServiceManager()
	svc := &mgrService{}
	sm.Register("one", svc)
	if err := sm.Start(context.Background(), "one"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := sm.Restart(context.Background(), "one"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if svc.starts.Load() != 2 || svc.stops.Load() != 1 {
		t.Fatalf("starts/stops = %d/%d, want 2/1", svc.starts.Load(), svc.stops.Load())
	}
	if err := sm.Start(context.Background(), "ghost"); err == nil {
		t.Fatal("starting a missing service must error")
	}
	if err := sm.Stop("ghost"); err == nil {
		t.Fatal("stopping a missing service must error")
	}
}

func TestServiceManager_DeregisterStops(t *testing.T) {
	sm := NewServiceManager()
	svc := &mgrService{}
	sm.Register("gone", svc)
	sm.StartAll(context.Background())
	sm.Deregister("gone")
	if svc.stops.Load() != 1 {
		t.Fatalf("deregister must stop the service, stops = %d", svc.stops.Load())
	}
	if sm.Has("gone") {
		t.Fatal("deregistered service must be gone")
	}
	sm.Deregister("gone") // idempotent for missing ids
}

func TestServiceManager_OverwriteStopsPrevious(t *testing.T) {
	sm := NewServiceManager()
	first := &mgrService{}
	second := &mgrService{}
	sm.Register("dup", first)
	sm.StartAll(context.Background())
	sm.Register("dup", second) // overwrite
	if first.stops.Load() != 1 {
		t.Fatalf("overwritten service must be stopped, stops = %d", first.stops.Load())
	}
	if sm.Count() != 1 {
		t.Fatalf("count after overwrite = %d, want 1", sm.Count())
	}
}

func TestServiceManager_Clear(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("a", &mgrService{})
	sm.Register("b", &mgrService{})
	sm.StartAll(context.Background())
	if err := sm.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if sm.Count() != 0 {
		t.Fatalf("count after clear = %d, want 0", sm.Count())
	}
}

func TestServiceManager_EmptyStartAll(t *testing.T) {
	sm := NewServiceManager()
	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll with no services must be a no-op: %v", err)
	}
	if err := NewServiceManager().StopAll(); err != nil {
		t.Fatalf("StopAll on empty manager must be nil: %v", err)
	}
}
