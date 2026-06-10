package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/jobs"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	tmpDir := t.TempDir()
	s, err := Open(WithPath(tmpDir))
	if err != nil {
		t.Fatalf("badger init failed: %v", err)
	}
	return s, tmpDir
}

func closeStore(s *Store, _ string) {
	if s != nil {
		s.Close()
	}
}

func TestCreateLoad(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "j1",
		Spec:      jobs.Spec{Kind: "test", Payload: "payload-data", Queue: "default"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}

	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}

	loaded, err := s.Load(ctx, "j1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != "j1" {
		t.Errorf("loaded id = %q, want j1", loaded.ID)
	}
	if loaded.State != jobs.StateQueued {
		t.Errorf("loaded state = %s, want queued", loaded.State)
	}
	if loaded.Spec.Kind != "test" {
		t.Errorf("loaded kind = %s, want test", loaded.Spec.Kind)
	}
}

func TestCreateDuplicate(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "dup",
		Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}
	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := s.Create(ctx, job); !errors.Is(err, jobs.ErrExists) {
		t.Fatalf("duplicate create: want ErrExists, got %v", err)
	}
}

func TestUpdateLoad(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "upd",
		Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}
	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}
	job.State = jobs.StateRunning
	job.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, job); err != nil {
		t.Fatalf("update: %v", err)
	}
	loaded, err := s.Load(ctx, "upd")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.State != jobs.StateRunning {
		t.Errorf("state after update = %s, want running", loaded.State)
	}
}

func TestUpdateNotFound(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	err := s.Update(context.Background(), jobs.Job{
		ID:        "nonexistent",
		Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("update nonexistent: want ErrNotFound, got %v", err)
	}
}

func TestListByState(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	for i, state := range []jobs.State{jobs.StateQueued, jobs.StateQueued, jobs.StateCompleted} {
		job := jobs.Job{
			ID:        fmt.Sprintf("l%d", i),
			Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
			State:     state,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		if err := s.Create(ctx, job); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	results, err := s.List(ctx, jobs.Query{State: jobs.StateQueued})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("list queued: got %d, want 2", len(results))
	}
}

func TestEvents(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "evt",
		Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}
	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}

	e := jobs.Event{
		ID:       "e1",
		JobID:    "evt",
		Type:     jobs.EventStarted,
		State:    jobs.StateRunning,
		Occurred: now.Add(time.Second),
	}
	if err := s.AppendEvent(ctx, e); err != nil {
		t.Fatalf("append event: %v", err)
	}

	events, err := s.Events(ctx, "evt")
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events count: got %d, want 1", len(events))
	}
	if events[0].Type != jobs.EventStarted {
		t.Errorf("event type = %s, want started", events[0].Type)
	}
}

func TestCheckpoints(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "ckpt",
		Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}
	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}

	c := jobs.Checkpoint{
		ID:      "c1",
		JobID:   "ckpt",
		State:   map[string]int{"progress": 50},
		Created: now.Add(time.Second),
	}
	if err := s.SaveCheckpoint(ctx, c); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	loaded, err := s.LoadCheckpoint(ctx, "ckpt")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if loaded.ID != "c1" {
		t.Errorf("checkpoint id = %s, want c1", loaded.ID)
	}
}

func TestDurabilityAcrossRestart(t *testing.T) {
	s, path := newStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "durable",
		Spec:      jobs.Spec{Kind: "t", Payload: "data", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}
	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}
	s.Close()

	s2, err := Open(WithPath(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer closeStore(s2, path)

	loaded, err := s2.Load(ctx, "durable")
	if err != nil {
		t.Fatalf("load after restart: %v", err)
	}
	if loaded.ID != "durable" {
		t.Errorf("id after restart = %q", loaded.ID)
	}
	if loaded.State != jobs.StateQueued {
		t.Errorf("state after restart = %s, want queued", loaded.State)
	}

	loaded.State = jobs.StateCompleted
	loaded.CompletedAt = time.Now().UTC()
	if err := s2.Update(ctx, *loaded); err != nil {
		t.Fatalf("update after restart: %v", err)
	}
}

func TestLifecycle(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	ctx := context.Background()

	now := time.Now().UTC()
	job := jobs.Job{
		ID:        "life",
		Spec:      jobs.Spec{Kind: "t", Payload: "p", Queue: "q"},
		State:     jobs.StateQueued,
		CreatedAt: now,
	}
	if err := s.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}

	s.AppendEvent(ctx, jobs.Event{ID: "le1", JobID: "life", Type: jobs.EventCreated, Occurred: now})

	job.State = jobs.StateRunning
	s.Update(ctx, job)
	s.AppendEvent(ctx, jobs.Event{ID: "le2", JobID: "life", Type: jobs.EventStarted, Occurred: now.Add(time.Second)})

	s.SaveCheckpoint(ctx, jobs.Checkpoint{ID: "lc1", JobID: "life", State: "mid", Created: now.Add(2 * time.Second)})

	job.State = jobs.StateCompleted
	job.CompletedAt = now.Add(3 * time.Second)
	s.Update(ctx, job)
	s.AppendEvent(ctx, jobs.Event{ID: "le3", JobID: "life", Type: jobs.EventCompleted, Occurred: now.Add(3 * time.Second)})

	loaded, err := s.Load(ctx, "life")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.State != jobs.StateCompleted {
		t.Errorf("final state = %s, want completed", loaded.State)
	}

	events, _ := s.Events(ctx, "life")
	if len(events) != 3 {
		t.Errorf("events count = %d, want 3", len(events))
	}

	ckpt, err := s.LoadCheckpoint(ctx, "life")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if ckpt.ID != "lc1" {
		t.Errorf("checkpoint id = %s, want lc1", ckpt.ID)
	}
}

func TestListEmpty(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	results, err := s.List(context.Background(), jobs.Query{})
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("list empty: got %d, want 0", len(results))
	}
}

func TestLoadNotFound(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	_, err := s.Load(context.Background(), "nope")
	if !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("load not found: want ErrNotFound, got %v", err)
	}
}

func TestCheckpointNotFound(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	_, err := s.LoadCheckpoint(context.Background(), "nonexistent")
	if !errors.Is(err, jobs.ErrCkptNotFound) {
		t.Fatalf("checkpoint not found: want ErrCkptNotFound, got %v", err)
	}
}

func TestEventValidFails(t *testing.T) {
	s, path := newStore(t)
	defer closeStore(s, path)
	err := s.AppendEvent(context.Background(), jobs.Event{ID: "", JobID: "j1", Type: jobs.EventStarted, Occurred: time.Now()})
	if err == nil {
		t.Error("expected error for invalid event")
	}
}
