package contextstream

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Job represents an asynchronous context-stream request.
type Job struct {
	ID          string
	Request     Request
	StartedAt   time.Time
	CompletedAt time.Time

	mu     sync.Mutex
	result *Result
	err    error
	done   chan struct{}
}

// NewJob creates a job placeholder for the given request.
func NewJob(req Request) *Job {
	return &Job{
		ID:      req.ID,
		Request: req,
		done:    make(chan struct{}),
	}
}

// Done returns a channel closed when the job finishes.
func (j *Job) Done() <-chan struct{} {
	if j == nil {
		return nil
	}
	return j.done
}

// Wait blocks until the job completes or the context ends.
func (j *Job) Wait(ctx context.Context) (*Result, error) {
	if j == nil {
		return nil, errors.New("contextstream: nil job")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-j.done:
		j.mu.Lock()
		defer j.mu.Unlock()
		return j.result, j.err
	}
}

func (j *Job) complete(result *Result, err error) {
	if j == nil {
		return
	}
	j.mu.Lock()
	j.result = result
	j.err = err
	j.CompletedAt = time.Now().UTC()
	close(j.done)
	j.mu.Unlock()
}
