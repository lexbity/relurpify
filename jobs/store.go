package jobs

import (
	"context"
	"errors"
)

var (
	ErrNotFound     = errors.New("job not found")
	ErrExists       = errors.New("job already exists")
	ErrCkptNotFound = errors.New("checkpoint not found")
)

type Query struct {
	Queue string `json:"queue,omitempty"`
	State State  `json:"state,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type Store interface {
	Create(ctx context.Context, job Job) error
	Update(ctx context.Context, job Job) error
	Load(ctx context.Context, id string) (*Job, error)
	List(ctx context.Context, q Query) ([]Job, error)

	// Claim atomically transitions up to limit due queued jobs
	// (state == queued and NextAttemptAt due) to running for the named
	// worker. Queues are honored in the order given; within a queue,
	// priority descending, then FIFO by CreatedAt (Q14). Exactly-once
	// claims: a job claimed by one worker is never returned to another.
	Claim(ctx context.Context, worker string, queues []string, limit int) ([]Job, error)

	AppendEvent(ctx context.Context, e Event) error
	Events(ctx context.Context, jobID string) ([]Event, error)

	SaveCheckpoint(ctx context.Context, c Checkpoint) error
	LoadCheckpoint(ctx context.Context, jobID string) (*Checkpoint, error)
}
