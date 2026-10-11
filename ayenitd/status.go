// status.go is the status edge (Q13): status.json, rewritten atomically
// (temp + rename) every heartbeat interval (5 s ± 1 s, NFR-5). It carries
// IDs, kinds, states, and counts — never prompt content (§5.7). Doctor
// treats a heartbeat age ≥ 15 s as runner-down (FR-22).
package ayenitd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/jobs"
)

// RunnerStatus is the on-disk schema of <state>/jobs/status.json (schema v2,
// owner ruling R-8): the `jobs` field is a bounded sample of up to 20 jobs in
// JOB-ID order — it is not a recency window; a renamed field must not carry a
// lie. Consumers: the doctor and human operators.
type RunnerStatus struct {
	RunnerID    string                 `json:"runner_id"`
	PID         int                    `json:"pid"`
	StartedAt   time.Time              `json:"started_at"`
	HeartbeatAt time.Time              `json:"heartbeat_at"`
	Draining    bool                   `json:"draining"`
	StoreDir    string                 `json:"store_dir"`
	Queues      map[string]QueueTotals `json:"queues"`
	Totals      Totals                 `json:"totals"`
	Jobs        []RecentJob            `json:"jobs"`
}

// QueueTotals counts a single queue's live jobs.
type QueueTotals struct {
	Queued  int `json:"queued"`
	Running int `json:"running"`
}

// Totals aggregates global counters.
type Totals struct {
	Queued       int `json:"queued"`
	Running      int `json:"running"`
	Completed24h int `json:"completed_24h"`
	Failed24h    int `json:"failed_24h"`
}

// RecentJob is one entry of the job-ID-order bounded sample.
type RecentJob struct {
	ID          string    `json:"id"`
	CorrelateID string    `json:"correlate_id,omitempty"`
	Kind        string    `json:"kind"`
	State       string    `json:"state"`
	Attempt     int       `json:"attempt"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// statusWriter periodically rewrites status.json atomically.
type statusWriter struct {
	stateDir  string
	storeDir  string
	runnerID  string
	pid       int
	store     jobs.Store
	every     time.Duration
	startedAt time.Time

	mu      sync.Mutex
	drained bool

	cancel context.CancelFunc
	done   chan struct{}
}

func newStatusWriter(stateDir, storeDir, runnerID string, store jobs.Store, every time.Duration) *statusWriter {
	if every <= 0 {
		every = 5 * time.Second
	}
	return &statusWriter{
		stateDir: stateDir, storeDir: storeDir, runnerID: runnerID,
		pid: os.Getpid(), store: store, every: every, startedAt: time.Now().UTC(),
	}
}

func (w *statusWriter) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	go w.loop(runCtx)
	return nil
}

// Stop cancels the ticker loop and JOINS it before the final drain flush
// (D7): writeOnce is single-writer by construction — the join guarantees the
// ticker can never race the final flush onto the same temp file, so
// status.json cannot tear.
func (w *statusWriter) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	if w.done != nil {
		<-w.done
	}
	// Final drain flush with draining=true (Q16 shutdown contract); under
	// the LIFO stop order this runs after the executor has drained, so the
	// last status.json shows post-drain counts.
	w.mu.Lock()
	w.drained = true
	w.mu.Unlock()
	_ = w.writeOnce()
	return nil
}

func (w *statusWriter) loop(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.writeOnce()
		}
	}
}

// writeOnce builds the status snapshot from the store and rewrites
// status.json atomically.
func (w *statusWriter) writeOnce() error {
	ctx := context.Background()
	status := RunnerStatus{
		RunnerID:    w.runnerID,
		PID:         w.pid,
		StartedAt:   w.startedAt,
		HeartbeatAt: time.Now().UTC(),
		StoreDir:    w.storeDir,
		Queues:      map[string]QueueTotals{},
	}
	w.mu.Lock()
	status.Draining = w.drained
	w.mu.Unlock()

	all, err := w.store.List(ctx, jobs.Query{})
	if err == nil {
		dayAgo := time.Now().UTC().Add(-24 * time.Hour)
		for _, j := range all {
			q := status.Queues[j.Spec.Queue]
			switch j.State {
			case jobs.StateQueued:
				q.Queued++
				status.Totals.Queued++
			case jobs.StateRunning:
				q.Running++
				status.Totals.Running++
			case jobs.StateCompleted:
				if j.CompletedAt.After(dayAgo) {
					status.Totals.Completed24h++
				}
			case jobs.StateFailed:
				if j.CompletedAt.After(dayAgo) {
					status.Totals.Failed24h++
				}
			}
			status.Queues[j.Spec.Queue] = q
		}
		recent := all
		if len(recent) > 20 {
			recent = recent[len(recent)-20:]
		}
		for _, j := range recent {
			status.Jobs = append(status.Jobs, RecentJob{
				ID: j.ID, CorrelateID: j.Spec.CorrelateID, Kind: j.Spec.Kind,
				State: string(j.State), Attempt: j.Attempt, UpdatedAt: j.UpdatedAt,
			})
		}
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("status: marshal: %w", err)
	}
	final := filepath.Join(w.stateDir, "jobs", "status.json")
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return fmt.Errorf("status: mkdir: %w", err)
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("status: write: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("status: rename: %w", err)
	}
	return nil
}
