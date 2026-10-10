package jobs

import (
	"context"
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
	job, err := s.Submit(context.TODO(), Spec{Kind: "k", Payload: "p", Queue: "q"})
	if err != nil {
		t.Fatalf("noop submit: %v", err)
	}
	if job.State != StateQueued {
		t.Errorf("noop submitter state = %s, want queued", job.State)
	}
}

func TestNextBackoff(t *testing.T) {
	base := Spec{}
	cases := []struct {
		name    string
		spec    Spec
		attempt int
		want    time.Duration
	}{
		{"default 30s base", Spec{}, 1, 30 * time.Second},
		{"second attempt doubles", Spec{}, 2, 60 * time.Second},
		{"third attempt quadruples", Spec{}, 3, 120 * time.Second},
		{"both caps: 32x base is 16m, 5m cap wins", Spec{}, 99, 5 * time.Minute},
		{"attempt 4 is 8x base = 4m, under 5m", Spec{}, 4, 4 * time.Minute},
		{"attempt 5 would be 16x base, clamped to 5m", Spec{}, 5, 5 * time.Minute},
		{"explicit base honored", Spec{Backoff: time.Second}, 1, time.Second},
		{"explicit base doubles", Spec{Backoff: time.Second}, 2, 2 * time.Second},
		{"explicit base caps at 32x", Spec{Backoff: time.Second}, 42, 32 * time.Second},
		{"zero attempt treated as first", Spec{}, 0, 30 * time.Second},
		{"negative attempt treated as first", Spec{}, -3, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextBackoff(tc.spec, tc.attempt); got != tc.want {
				t.Fatalf("NextBackoff(%+v, %d) = %v, want %v", tc.spec, tc.attempt, got, tc.want)
			}
		})
	}
	_ = base
}

func TestSpecValid_NextAttemptSemantics(t *testing.T) {
	// NextAttemptAt lives on Job, not Spec; Spec.Valid is unchanged in its
	// requirements but the zero-time semantics are pinned here via Job.
	j := Job{
		ID:        "j1",
		Spec:      Spec{Kind: "k", Payload: map[string]any{}, Queue: "q"},
		State:     StateQueued,
		CreatedAt: time.Now(),
	}
	// Zero NextAttemptAt = due now; Valid must accept it.
	if err := j.Valid(); err != nil {
		t.Fatalf("zero NextAttemptAt must be valid: %v", err)
	}
	j.NextAttemptAt = time.Now().Add(time.Minute)
	if err := j.Valid(); err != nil {
		t.Fatalf("future NextAttemptAt must be valid: %v", err)
	}
}
