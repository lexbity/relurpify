package jobs

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

func (s State) Valid() bool {
	switch s {
	case StateQueued, StateRunning, StateCompleted, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

type EventType string

const (
	EventCreated    EventType = "created"
	EventStarted    EventType = "started"
	EventCheckpoint EventType = "checkpoint"
	EventCompleted  EventType = "completed"
	EventFailed     EventType = "failed"
	EventCancelled  EventType = "cancelled"
	EventRetried    EventType = "retried"
)

func (t EventType) Valid() bool {
	switch t {
	case EventCreated, EventStarted, EventCheckpoint, EventCompleted, EventFailed, EventCancelled, EventRetried:
		return true
	default:
		return false
	}
}

type Spec struct {
	Kind        string            `json:"kind"`
	Payload     any               `json:"payload"`
	Queue       string            `json:"queue"`
	Priority    int               `json:"priority,omitempty"`
	MaxAttempts int               `json:"max_attempts,omitempty"`
	Backoff     time.Duration     `json:"backoff,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	CorrelateID string            `json:"correlate_id,omitempty"`
}

func (s Spec) Valid() error {
	if strings.TrimSpace(s.Kind) == "" {
		return errors.New("kind required")
	}
	if s.Payload == nil {
		return errors.New("payload required")
	}
	if strings.TrimSpace(s.Queue) == "" {
		return errors.New("queue required")
	}
	if s.MaxAttempts < 0 {
		return errors.New("max_attempts must be >= 0")
	}
	if s.Backoff < 0 {
		return errors.New("backoff must be >= 0")
	}
	if s.Timeout < 0 {
		return errors.New("timeout must be >= 0")
	}
	for k, v := range s.Labels {
		if strings.TrimSpace(k) == "" {
			return errors.New("label key required")
		}
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("label %q value required", k)
		}
	}
	for _, t := range s.Tags {
		if strings.TrimSpace(t) == "" {
			return errors.New("tag must not be empty")
		}
	}
	return nil
}

type Job struct {
	ID          string            `json:"id"`
	Spec        Spec              `json:"spec"`
	State       State             `json:"state"`
	Attempt     int               `json:"attempt,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	ResumeToken string            `json:"resume_token,omitempty"`
}

func (j Job) Valid() error {
	if strings.TrimSpace(j.ID) == "" {
		return errors.New("id required")
	}
	if err := j.Spec.Valid(); err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	if !j.State.Valid() {
		return fmt.Errorf("state %q invalid", j.State)
	}
	if j.Attempt < 0 {
		return errors.New("attempt must be >= 0")
	}
	if j.CreatedAt.IsZero() {
		return errors.New("created_at required")
	}
	if !j.UpdatedAt.IsZero() && j.UpdatedAt.Before(j.CreatedAt) {
		return errors.New("updated_at must be after created_at")
	}
	if !j.CompletedAt.IsZero() && j.CompletedAt.Before(j.CreatedAt) {
		return errors.New("completed_at must be after created_at")
	}
	for k, v := range j.Labels {
		if strings.TrimSpace(k) == "" {
			return errors.New("label key required")
		}
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("label %q value required", k)
		}
	}
	for _, t := range j.Tags {
		if strings.TrimSpace(t) == "" {
			return errors.New("tag must not be empty")
		}
	}
	return nil
}

type Event struct {
	ID       string         `json:"id"`
	JobID    string         `json:"job_id"`
	Type     EventType      `json:"type"`
	State    State          `json:"state,omitempty"`
	Occurred time.Time      `json:"occurred"`
	Message  string         `json:"message,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (e Event) Valid() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("event id required")
	}
	if strings.TrimSpace(e.JobID) == "" {
		return errors.New("job id required")
	}
	if !e.Type.Valid() {
		return fmt.Errorf("event type %q invalid", e.Type)
	}
	if e.Occurred.IsZero() {
		return errors.New("occurred required")
	}
	return nil
}

type Checkpoint struct {
	ID      string    `json:"id"`
	JobID   string    `json:"job_id"`
	State   any       `json:"state"`
	Token   string    `json:"token,omitempty"`
	Created time.Time `json:"created"`
}

func (c Checkpoint) Valid() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("checkpoint id required")
	}
	if strings.TrimSpace(c.JobID) == "" {
		return errors.New("job id required")
	}
	if c.State == nil {
		return errors.New("state required")
	}
	if c.Created.IsZero() {
		return errors.New("created required")
	}
	return nil
}
