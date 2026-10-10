// Package jobsstore is the durable Badger-backed implementation of the
// jobs.Store contract. It runs inside the single runner process: Badger's
// per-directory file lock is the mutual-exclusion primitive, so exactly one
// process can ever open a store directory (FR-17) — a second opener fails
// at Open and the app treats that as "runner already present".
//
// Key families (graphdb's keyPrefix style, deliberately not sharing its
// graph-shaped backend interface):
//
//	j:{jobID}                                            canonical Job JSON
//	q:{queue}:{pri%06d=999999-p}:{createdNano%020d}:{jobID}  ready index (claim order)
//	w:{worker}:{jobID}                                   running index (crash scan)
//	e:{jobID}:{seq%020d}                                 event log
//	x:{correlateID} -> jobID                             idempotent-ingest index
//	c:{jobID}                                            latest checkpoint
//	n:{jobID}                                            per-job event sequence counter
//
// All multi-key writes (claim, state transition, event append, index moves)
// happen in one badger read-write transaction; badger.ErrConflict retries
// bounded (≤50). Open runs crash recovery (Q14): every job found in a
// running index is failed with "interrupted by restart" and re-queued when
// attempts remain.
package jobsstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dgraph-io/badger/v4"

	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/jobs"
)

// Options configures Open. Dir opens (or creates) a durable Badger store;
// InMemory opens an ephemeral store for tests.
type Options struct {
	Dir      string
	InMemory bool
}

type store struct {
	db *badger.DB
}

// Open creates or opens the jobs store. When the directory is locked by
// another process, badger.Open fails and the caller sees the error — the
// runner exits nonzero (FR-17).
func Open(opts Options) (jobs.Store, error) {
	var bopts badger.Options
	switch {
	case opts.InMemory:
		bopts = badger.DefaultOptions("").WithInMemory(true)
	case opts.Dir != "":
		if err := fs.MkdirAllSecure(opts.Dir); err != nil {
			return nil, fmt.Errorf("jobsstore: create dir: %w", err)
		}
		bopts = badger.DefaultOptions(opts.Dir).WithValueDir(opts.Dir)
	default:
		return nil, fmt.Errorf("jobsstore: Dir required unless InMemory")
	}
	bopts = bopts.WithLogger(nil)
	db, err := badger.Open(bopts)
	if err != nil {
		return nil, fmt.Errorf("jobsstore: open badger: %w", err)
	}
	s := &store{db: db}
	if !opts.InMemory {
		if err := s.recover(); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("jobsstore: recovery: %w", err)
		}
	}
	return s, nil
}

// Close flushes and closes the store.
func (s *store) Close() error { return s.db.Close() }

// ── key encoding ────────────────────────────────────────────────────

func jobKey(id string) []byte { return []byte("j:" + id) }

// readyIndexKey encodes the claim order: queue, priority descending
// (999999-p, clamped to [0,999999]), then FIFO by CreatedAt, then job ID.
func readyIndexKey(j jobs.Job) []byte {
	pri := j.Spec.Priority
	if pri < 0 {
		pri = 0
	}
	if pri > 999999 {
		pri = 999999
	}
	return []byte(fmt.Sprintf("q:%s:%06d:%020d:%s", j.Spec.Queue, 999999-pri, j.CreatedAt.UnixNano(), j.ID))
}

func readyIndexPrefix(queue string) []byte { return []byte("q:" + queue + ":") }

func runningIndexKey(worker, jobID string) []byte {
	return []byte("w:" + worker + ":" + jobID)
}

func runningIndexPrefix() []byte { return []byte("w:") }

func eventKey(jobID string, seq uint64) []byte {
	return []byte(fmt.Sprintf("e:%s:%020d", jobID, seq))
}

func eventPrefix(jobID string) []byte { return []byte("e:" + jobID + ":") }

func eventSeqKey(jobID string) []byte { return []byte("n:" + jobID) }

func correlateKey(correlateID string) []byte { return []byte("x:" + correlateID) }

func checkpointKey(jobID string) []byte { return []byte("c:" + jobID) }

// maxConflictRetries bounds optimistic-concurrency retries per transaction.
const maxConflictRetries = 50

// ── Store implementation ────────────────────────────────────────────

func (s *store) Create(ctx context.Context, job jobs.Job) error {
	if err := job.Valid(); err != nil {
		return fmt.Errorf("jobsstore: create: %w", err)
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("jobsstore: marshal: %w", err)
	}
	return s.retryTxn(func() error {
		return s.db.Update(func(txn *badger.Txn) error {
			// Job-ID uniqueness: Create never overwrites.
			if _, err := txn.Get(jobKey(job.ID)); err == nil {
				return jobs.ErrExists
			} else if !errIsNotFound(err) {
				return err
			}
			// CorrelateID uniqueness: the idempotent-ingest index maps a
			// correlate ID to at most one job (FR-16).
			if job.Spec.CorrelateID != "" {
				if _, err := txn.Get(correlateKey(job.Spec.CorrelateID)); err == nil {
					return jobs.ErrExists
				} else if !errIsNotFound(err) {
					return err
				}
				if err := txn.Set(correlateKey(job.Spec.CorrelateID), []byte(job.ID)); err != nil {
					return err
				}
			}
			if err := txn.Set(jobKey(job.ID), raw); err != nil {
				return err
			}
			return txn.Set(readyIndexKey(job), []byte(job.ID))
		})
	})
}

func (s *store) Update(ctx context.Context, job jobs.Job) error {
	if err := job.Valid(); err != nil {
		return fmt.Errorf("jobsstore: update: %w", err)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("jobsstore: marshal: %w", err)
	}
	return s.retryTxn(func() error {
		return s.db.Update(func(txn *badger.Txn) error {
			old, err := s.loadTxn(txn, job.ID)
			if err != nil {
				return err
			}
			if err := txn.Set(jobKey(job.ID), raw); err != nil {
				return err
			}
			// Keep the ready index in sync with queue/priority/state moves.
			if old.State == jobs.StateQueued {
				if err := txn.Delete(readyIndexKey(*old)); err != nil {
					return err
				}
			}
			if job.State == jobs.StateQueued {
				if err := txn.Set(readyIndexKey(job), []byte(job.ID)); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (s *store) Load(ctx context.Context, id string) (*jobs.Job, error) {
	var out *jobs.Job
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		out, err = s.loadTxn(txn, id)
		return err
	})
	return out, err
}

func (s *store) loadTxn(txn *badger.Txn, id string) (*jobs.Job, error) {
	item, err := txn.Get(jobKey(id))
	if err != nil {
		if errIsNotFound(err) {
			return nil, jobs.ErrNotFound
		}
		return nil, err
	}
	var job jobs.Job
	if err := item.Value(func(v []byte) error { return json.Unmarshal(v, &job) }); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *store) List(ctx context.Context, q jobs.Query) ([]jobs.Job, error) {
	var out []jobs.Job
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		it.Seek([]byte("j:"))
		for ; it.ValidForPrefix([]byte("j:")); it.Next() {
			item := it.Item()
			var job jobs.Job
			if err := item.Value(func(v []byte) error { return json.Unmarshal(v, &job) }); err != nil {
				return err
			}
			if q.Queue != "" && job.Spec.Queue != q.Queue {
				continue
			}
			if q.State != "" && job.State != q.State {
				continue
			}
			out = append(out, job)
			if q.Limit > 0 && len(out) >= q.Limit {
				return nil
			}
		}
		return nil
	})
	return out, err
}

// Claim is the atomic transition (Q14): one read-write transaction per
// claim batch, iterating the per-queue ready indexes in config order,
// re-checking state and NextAttemptAt from the canonical record, flipping
// to running, and moving the index entries.
func (s *store) Claim(ctx context.Context, worker string, queues []string, limit int) ([]jobs.Job, error) {
	if worker == "" {
		return nil, fmt.Errorf("jobsstore: claim: worker required")
	}
	if limit <= 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	var claimed []jobs.Job
	err := s.retryTxn(func() error {
		claimed = nil
		return s.db.Update(func(txn *badger.Txn) error {
			for _, queue := range queues {
				if len(claimed) >= limit {
					return nil
				}
				if err := s.claimFromQueue(txn, worker, queue, now, limit, &claimed); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *store) claimFromQueue(txn *badger.Txn, worker, queue string, now time.Time, limit int, claimed *[]jobs.Job) error {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	it := txn.NewIterator(opts)
	defer it.Close()
	prefix := readyIndexPrefix(queue)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		if len(*claimed) >= limit {
			return nil
		}
		item := it.Item()
		idb, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		id := string(idb)
		// Re-check the canonical record: the index may be stale relative to
		// a concurrent transition (cancel-while-queued, retry reschedule).
		canonical, err := s.loadTxn(txn, id)
		if err != nil {
			if errIsNotFound(err) {
				if err := txn.Delete(item.KeyCopy(nil)); err != nil {
					return err
				}
				continue
			}
			return err
		}
		if canonical.State != jobs.StateQueued {
			if err := txn.Delete(item.KeyCopy(nil)); err != nil {
				return err
			}
			continue
		}
		if !canonical.NextAttemptAt.IsZero() && canonical.NextAttemptAt.After(now) {
			continue // not due yet
		}
		// Flip to running: attempt++, timestamps, worker index entry.
		canonical.State = jobs.StateRunning
		canonical.Attempt++
		canonical.UpdatedAt = now
		raw, err := json.Marshal(*canonical)
		if err != nil {
			return err
		}
		if err := txn.Set(jobKey(canonical.ID), raw); err != nil {
			return err
		}
		if err := txn.Delete(item.KeyCopy(nil)); err != nil {
			return err
		}
		if err := txn.Set(runningIndexKey(worker, canonical.ID), []byte(canonical.ID)); err != nil {
			return err
		}
		*claimed = append(*claimed, *canonical)
	}
	return nil
}

func (s *store) AppendEvent(ctx context.Context, e jobs.Event) error {
	if err := e.Valid(); err != nil {
		return fmt.Errorf("jobsstore: append event: %w", err)
	}
	return s.retryTxn(func() error {
		return s.db.Update(func(txn *badger.Txn) error {
			var seq uint64
			item, err := txn.Get(eventSeqKey(e.JobID))
			switch {
			case err == nil:
				if err := item.Value(func(v []byte) error {
					parsed, parseErr := strconv.ParseUint(string(v), 10, 64)
					seq = parsed
					return parseErr
				}); err != nil {
					return err
				}
			case errIsNotFound(err):
				// first event for this job
			default:
				return err
			}
			seq++
			raw, err := json.Marshal(e)
			if err != nil {
				return err
			}
			if err := txn.Set(eventKey(e.JobID, seq), raw); err != nil {
				return err
			}
			return txn.Set(eventSeqKey(e.JobID), []byte(strconv.FormatUint(seq, 10)))
		})
	})
}

func (s *store) Events(ctx context.Context, jobID string) ([]jobs.Event, error) {
	var out []jobs.Event
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := eventPrefix(jobID)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var e jobs.Event
			if err := item.Value(func(v []byte) error { return json.Unmarshal(v, &e) }); err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

func (s *store) SaveCheckpoint(ctx context.Context, c jobs.Checkpoint) error {
	if err := c.Valid(); err != nil {
		return fmt.Errorf("jobsstore: save checkpoint: %w", err)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("jobsstore: marshal: %w", err)
	}
	return s.retryTxn(func() error {
		return s.db.Update(func(txn *badger.Txn) error {
			// Latest checkpoint per job: c:{jobID} holds exactly one.
			if _, err := txn.Get(checkpointKey(c.JobID)); err == nil {
				if err := txn.Delete(checkpointKey(c.JobID)); err != nil {
					return err
				}
			} else if !errIsNotFound(err) {
				return err
			}
			return txn.Set(checkpointKey(c.JobID), raw)
		})
	})
}

func (s *store) LoadCheckpoint(ctx context.Context, jobID string) (*jobs.Checkpoint, error) {
	var out *jobs.Checkpoint
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(checkpointKey(jobID))
		if err != nil {
			if errIsNotFound(err) {
				return jobs.ErrCkptNotFound
			}
			return err
		}
		var c jobs.Checkpoint
		if err := item.Value(func(v []byte) error { return json.Unmarshal(v, &c) }); err != nil {
			return err
		}
		out = &c
		return nil
	})
	return out, err
}

// ── recovery (Q14) ──────────────────────────────────────────────────

// recover scans the running indexes once at Open: every found job is failed
// with "interrupted by restart" and re-queued when attempts remain, so a
// crash mid-attempt never loses or orphans work. Every recovery decision
// appends an event (FR-19).
func (s *store) recover() error {
	now := time.Now().UTC()
	type interrupted struct {
		worker string
		job    jobs.Job
	}
	var found []interrupted
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		it.Seek(runningIndexPrefix())
		for ; it.ValidForPrefix(runningIndexPrefix()); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			var jobID string
			if err := item.Value(func(v []byte) error { jobID = string(v); return nil }); err != nil {
				return err
			}
			// key is w:{worker}:{jobID}
			worker := string(key[len("w:"):])
			if i := len(worker) - len(jobID) - 1; i >= 0 {
				worker = worker[:i]
			}
			job, err := s.loadTxn(txn, jobID)
			if err != nil {
				if errIsNotFound(err) {
					continue // stale index entry; cleaned below
				}
				return err
			}
			found = append(found, interrupted{worker: worker, job: *job})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, f := range found {
		if err := s.recoverOne(f.worker, f.job, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) recoverOne(worker string, job jobs.Job, now time.Time) error {
	return s.retryTxn(func() error {
		return s.db.Update(func(txn *badger.Txn) error {
			canonical, err := s.loadTxn(txn, job.ID)
			if err != nil {
				return err
			}
			// Drop the running index entry for this worker.
			if err := txn.Delete(runningIndexKey(worker, job.ID)); err != nil {
				return err
			}
			// The canonical record is the truth: a stale running index
			// entry for a job that already reached a terminal state (the
			// executor completes/fails without the worker key) is index
			// debris, not an interrupted attempt. Only a job that is still
			// running is recoverable.
			if canonical.State != jobs.StateRunning {
				return nil
			}
			// 1. interrupted → failed with an event.
			canonical.State = jobs.StateFailed
			canonical.LastError = "interrupted by restart"
			canonical.UpdatedAt = now
			if err := appendEventTxn(txn, jobs.Event{
				ID:       newEventID(canonical.ID, "interrupted"),
				JobID:    canonical.ID,
				Type:     jobs.EventFailed,
				State:    jobs.StateFailed,
				Occurred: now,
				Message:  "interrupted by restart",
			}); err != nil {
				return err
			}
			raw, err := json.Marshal(*canonical)
			if err != nil {
				return err
			}
			if err := txn.Set(jobKey(canonical.ID), raw); err != nil {
				return err
			}
			// 2. Re-queue when attempts remain (at-least-once delivery).
			if canonical.Attempt < max(maxAttempts(*canonical), 1) {
				canonical.State = jobs.StateQueued
				canonical.NextAttemptAt = now.Add(jobs.NextBackoff(canonical.Spec, canonical.Attempt))
				canonical.UpdatedAt = now
				raw, err = json.Marshal(*canonical)
				if err != nil {
					return err
				}
				if err := txn.Set(jobKey(canonical.ID), raw); err != nil {
					return err
				}
				if err := txn.Set(readyIndexKey(*canonical), []byte(canonical.ID)); err != nil {
					return err
				}
				if err := appendEventTxn(txn, jobs.Event{
					ID:       newEventID(canonical.ID, "requeued"),
					JobID:    canonical.ID,
					Type:     jobs.EventRetried,
					State:    jobs.StateQueued,
					Occurred: now,
					Message:  "requeued after interruption",
				}); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

// maxAttempts resolves the effective attempt budget: MaxAttempts 0 means 1.
func maxAttempts(j jobs.Job) int {
	if j.Spec.MaxAttempts <= 0 {
		return 1
	}
	return j.Spec.MaxAttempts
}

func appendEventTxn(txn *badger.Txn, e jobs.Event) error {
	var seq uint64
	item, err := txn.Get(eventSeqKey(e.JobID))
	switch {
	case err == nil:
		if err := item.Value(func(v []byte) error {
			parsed, parseErr := strconv.ParseUint(string(v), 10, 64)
			seq = parsed
			return parseErr
		}); err != nil {
			return err
		}
	case errIsNotFound(err):
	default:
		return err
	}
	seq++
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := txn.Set(eventKey(e.JobID, seq), raw); err != nil {
		return err
	}
	return txn.Set(eventSeqKey(e.JobID), []byte(strconv.FormatUint(seq, 10)))
}

func newEventID(jobID, tag string) string {
	return fmt.Sprintf("%s:%s:%020d", jobID, tag, time.Now().UnixNano())
}

// errIsNotFound reports whether err is any "not found" in the contract:
// badger's raw key miss or the jobs.Store sentinel errors.
func errIsNotFound(err error) bool {
	return errors.Is(err, badger.ErrKeyNotFound) ||
		errors.Is(err, jobs.ErrNotFound) ||
		errors.Is(err, jobs.ErrCkptNotFound)
}

func (s *store) retryTxn(op func() error) error {
	for i := 0; i < maxConflictRetries; i++ {
		err := op()
		if err == badger.ErrConflict {
			continue
		}
		return err
	}
	return fmt.Errorf("jobsstore: transaction retried %d times, still conflicting", maxConflictRetries)
}
