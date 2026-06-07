package jobs

import (
	"testing"
	"time"
)

func TestStateValid(t *testing.T) {
	tests := []struct {
		s  State
		ok bool
	}{
		{StateQueued, true},
		{StateRunning, true},
		{StateCompleted, true},
		{StateFailed, true},
		{StateCancelled, true},
		{State("unknown"), false},
		{State(""), false},
	}
	for _, tt := range tests {
		if got := tt.s.Valid(); got != tt.ok {
			t.Errorf("State(%q).Valid() = %v, want %v", tt.s, got, tt.ok)
		}
	}
}

func TestEventTypeValid(t *testing.T) {
	tests := []struct {
		et EventType
		ok bool
	}{
		{EventCreated, true},
		{EventStarted, true},
		{EventCheckpoint, true},
		{EventCompleted, true},
		{EventFailed, true},
		{EventCancelled, true},
		{EventRetried, true},
		{EventType("unknown"), false},
	}
	for _, tt := range tests {
		if got := tt.et.Valid(); got != tt.ok {
			t.Errorf("EventType(%q).Valid() = %v, want %v", tt.et, got, tt.ok)
		}
	}
}

func TestSpecValid(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		spec Spec
		err  bool
	}{
		{"valid minimal", Spec{Kind: "test", Payload: "data", Queue: "q"}, false},
		{"missing kind", Spec{Payload: "data", Queue: "q"}, true},
		{"missing payload", Spec{Kind: "test", Queue: "q"}, true},
		{"missing queue", Spec{Kind: "test", Payload: "data"}, true},
		{"with labels", Spec{Kind: "test", Payload: "x", Queue: "q",
			Labels: map[string]string{"env": "test"}}, false},
		{"empty label key", Spec{Kind: "test", Payload: "x", Queue: "q",
			Labels: map[string]string{"": "v"}}, true},
		{"with tags", Spec{Kind: "test", Payload: "x", Queue: "q",
			Tags: []string{"fast"}}, false},
		{"empty tag", Spec{Kind: "test", Payload: "x", Queue: "q",
			Tags: []string{""}}, true},
		{"with timeout", Spec{Kind: "test", Payload: "x", Queue: "q",
			Timeout: time.Second}, false},
		{"negative attempt", Spec{Kind: "test", Payload: "x", Queue: "q",
			MaxAttempts: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Valid()
			if tt.err && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.err && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	_ = now
}

func TestJobValid(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		job  Job
		err  bool
	}{
		{"valid", Job{ID: "j1", Spec: Spec{Kind: "k", Payload: "p", Queue: "q"}, State: StateQueued, CreatedAt: now}, false},
		{"missing id", Job{Spec: Spec{Kind: "k", Payload: "p", Queue: "q"}, State: StateQueued, CreatedAt: now}, true},
		{"invalid state", Job{ID: "j1", Spec: Spec{Kind: "k", Payload: "p", Queue: "q"}, State: State("bad"), CreatedAt: now}, true},
		{"zero created", Job{ID: "j1", Spec: Spec{Kind: "k", Payload: "p", Queue: "q"}, State: StateQueued}, true},
		{"updated before created", Job{ID: "j1", Spec: Spec{Kind: "k", Payload: "p", Queue: "q"}, State: StateQueued, CreatedAt: now, UpdatedAt: now.Add(-time.Hour)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Valid()
			if tt.err && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.err && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestEventValid(t *testing.T) {
	now := time.Now()
	e := Event{ID: "e1", JobID: "j1", Type: EventCreated, Occurred: now}
	if err := e.Valid(); err != nil {
		t.Errorf("valid event: %v", err)
	}
	e2 := Event{ID: "e1", Type: EventCreated, Occurred: now}
	if err := e2.Valid(); err == nil {
		t.Error("expected error for missing job_id")
	}
}

func TestCheckpointValid(t *testing.T) {
	now := time.Now()
	c := Checkpoint{ID: "c1", JobID: "j1", State: "progress", Created: now}
	if err := c.Valid(); err != nil {
		t.Errorf("valid checkpoint: %v", err)
	}
	c2 := Checkpoint{ID: "c1", JobID: "j1", Created: now}
	if err := c2.Valid(); err == nil {
		t.Error("expected error for nil state")
	}
}

func TestNoopSubmitter(t *testing.T) {
	s := NoopSubmitter{}
	job, err := s.Submit(nil, Spec{Kind: "k", Payload: "p", Queue: "q"})
	if err != nil {
		t.Fatalf("noop submit: %v", err)
	}
	if job.State != StateQueued {
		t.Errorf("noop submitter state = %s, want queued", job.State)
	}
}
