package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/dgraph-io/badger/v4"
	"codeburg.org/lexbit/relurpify/jobs"
)

type config struct {
	path     string
	inMemory bool
}

type Option func(*config)

func WithPath(path string) Option {
	return func(c *config) {
		c.path = path
	}
}

func WithInMemory(inMemory bool) Option {
	return func(c *config) {
		c.inMemory = inMemory
	}
}

func Open(options ...Option) (*Store, error) {
	var cfg config
	for _, o := range options {
		o(&cfg)
	}

	bopts := badger.DefaultOptions(cfg.path)
	if cfg.inMemory || cfg.path == "" {
		bopts = badger.DefaultOptions("").WithInMemory(true)
	}
	// Reduce logging noise from badger in tests/runtime.
	bopts = bopts.WithLogger(nil)

	db, err := badger.Open(bopts)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}

	return &Store{db: db}, nil
}

type Store struct {
	db *badger.DB
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ────────────────────────────────────────────────────────────────────
// Job Mutations & Queries
// ────────────────────────────────────────────────────────────────────

func (s *Store) Create(ctx context.Context, job jobs.Job) error {
	if err := job.Valid(); err != nil {
		return err
	}

	key := []byte("job:" + job.ID)
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			return jobs.ErrExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return txn.Set(key, data)
	})
}

func (s *Store) Update(ctx context.Context, job jobs.Job) error {
	if err := job.Valid(); err != nil {
		return err
	}

	key := []byte("job:" + job.ID)
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return jobs.ErrNotFound
			}
			return err
		}
		return txn.Set(key, data)
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) Load(ctx context.Context, id string) (*jobs.Job, error) {
	key := []byte("job:" + id)
	var data []byte

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return jobs.ErrNotFound
			}
			return err
		}
		data, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, err
	}

	var job jobs.Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("unmarshal job: %w", err)
	}
	return &job, nil
}

func (s *Store) List(ctx context.Context, q jobs.Query) ([]jobs.Job, error) {
	var out []jobs.Job
	prefix := []byte("job:")

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var j jobs.Job
			if err := json.Unmarshal(val, &j); err != nil {
				return err
			}

			// Filter
			if q.Queue != "" && j.Spec.Queue != q.Queue {
				continue
			}
			if q.State != "" && j.State != q.State {
				continue
			}
			out = append(out, j)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort: priority DESC, created_at ASC
	sort.Slice(out, func(i, j int) bool {
		if out[i].Spec.Priority != out[j].Spec.Priority {
			return out[i].Spec.Priority > out[j].Spec.Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})

	// Limit
	if q.Limit > 0 && q.Limit < len(out) {
		out = out[:q.Limit]
	}

	return out, nil
}

// ────────────────────────────────────────────────────────────────────
// Event Mutations & Queries
// ────────────────────────────────────────────────────────────────────

func (s *Store) AppendEvent(ctx context.Context, e jobs.Event) error {
	if err := e.Valid(); err != nil {
		return err
	}

	// Check if job exists
	jobKey := []byte("job:" + e.JobID)
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(jobKey)
		return err
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("job not found: %s", e.JobID)
		}
		return err
	}

	// Key: event:{job_id}:{occurred_unix_nano}:{event_id}
	key := []byte(fmt.Sprintf("event:%s:%020d:%s", e.JobID, e.Occurred.UnixNano(), e.ID))
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

func (s *Store) Events(ctx context.Context, jobID string) ([]jobs.Event, error) {
	var out []jobs.Event
	prefix := []byte("event:" + jobID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var e jobs.Event
			if err := json.Unmarshal(val, &e); err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by occurred ASC
	sort.Slice(out, func(i, j int) bool {
		return out[i].Occurred.Before(out[j].Occurred)
	})

	return out, nil
}

// ────────────────────────────────────────────────────────────────────
// Checkpoint Mutations & Queries
// ────────────────────────────────────────────────────────────────────

func (s *Store) SaveCheckpoint(ctx context.Context, c jobs.Checkpoint) error {
	if err := c.Valid(); err != nil {
		return err
	}

	// Check if job exists
	jobKey := []byte("job:" + c.JobID)
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(jobKey)
		return err
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("job not found: %s", c.JobID)
		}
		return err
	}

	// Key: checkpoint:{job_id}:{created_unix_nano}:{checkpoint_id}
	key := []byte(fmt.Sprintf("checkpoint:%s:%020d:%s", c.JobID, c.Created.UnixNano(), c.ID))
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

func (s *Store) LoadCheckpoint(ctx context.Context, jobID string) (*jobs.Checkpoint, error) {
	prefix := []byte("checkpoint:" + jobID + ":")
	var out []jobs.Checkpoint

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var c jobs.Checkpoint
			if err := json.Unmarshal(val, &c); err != nil {
				return err
			}
			out = append(out, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, jobs.ErrCkptNotFound
	}

	// Sort by created DESC
	sort.Slice(out, func(i, j int) bool {
		return out[i].Created.After(out[j].Created)
	})

	return &out[0], nil
}
