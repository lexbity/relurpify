package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/jobs"
)

type MemoryJobStore struct {
	mu          sync.RWMutex
	jobs        map[string]jobs.Job
	events      map[string][]jobs.JobEvent
	checkpoints map[string]jobs.JobCheckpoint
	leases      map[string]jobs.JobLease
}

func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{
		jobs:        make(map[string]jobs.Job),
		events:      make(map[string][]jobs.JobEvent),
		checkpoints: make(map[string]jobs.JobCheckpoint),
		leases:      make(map[string]jobs.JobLease),
	}
}

func (s *MemoryJobStore) CreateJob(_ context.Context, job jobs.Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return jobs.ErrJobAlreadyExists
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryJobStore) UpdateJob(_ context.Context, job jobs.Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return jobs.ErrJobNotFound
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryJobStore) LoadJob(_ context.Context, jobID string) (*jobs.Job, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job id required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, jobs.ErrJobNotFound
	}
	cloned := cloneJob(job)
	return &cloned, nil
}

func (s *MemoryJobStore) ListJobs(_ context.Context, query jobs.JobQuery) ([]jobs.Job, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]jobs.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		if query.Queue != "" && job.Spec.Queue != query.Queue {
			continue
		}
		if query.State != "" && job.State != query.State {
			continue
		}
		out = append(out, cloneJob(job))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out, nil
}

func (s *MemoryJobStore) AppendEvent(_ context.Context, event jobs.JobEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[event.JobID]; !ok {
		return jobs.ErrJobNotFound
	}
	events := s.events[event.JobID]
	for _, existing := range events {
		if existing.ID == event.ID {
			return jobs.ErrEventAlreadyExists
		}
	}
	s.events[event.JobID] = append(events, cloneEvent(event))
	return nil
}

func (s *MemoryJobStore) LoadEvents(_ context.Context, jobID string) ([]jobs.JobEvent, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job id required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.events[jobID]
	out := make([]jobs.JobEvent, 0, len(events))
	for _, event := range events {
		out = append(out, cloneEvent(event))
	}
	return out, nil
}

func (s *MemoryJobStore) StoreCheckpoint(_ context.Context, checkpoint jobs.JobCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[checkpoint.JobID]; !ok {
		return jobs.ErrJobNotFound
	}
	if _, exists := s.checkpoints[checkpoint.ID]; exists {
		return jobs.ErrCheckpointExists
	}
	s.checkpoints[checkpoint.ID] = cloneCheckpoint(checkpoint)
	return nil
}

func (s *MemoryJobStore) LoadCheckpoint(_ context.Context, jobID, checkpointID string) (*jobs.JobCheckpoint, error) {
	jobID = strings.TrimSpace(jobID)
	checkpointID = strings.TrimSpace(checkpointID)
	if jobID == "" {
		return nil, fmt.Errorf("job id required")
	}
	if checkpointID == "" {
		return nil, fmt.Errorf("checkpoint id required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	checkpoint, ok := s.checkpoints[checkpointID]
	if !ok || checkpoint.JobID != jobID {
		return nil, jobs.ErrCheckpointNotFound
	}
	cloned := cloneCheckpoint(checkpoint)
	return &cloned, nil
}

func (s *MemoryJobStore) ListCheckpoints(_ context.Context, jobID string) ([]jobs.JobCheckpoint, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job id required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]jobs.JobCheckpoint, 0)
	for _, checkpoint := range s.checkpoints {
		if checkpoint.JobID == jobID {
			out = append(out, cloneCheckpoint(checkpoint))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out, nil
}

func (s *MemoryJobStore) StoreLease(_ context.Context, lease jobs.JobLease) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[lease.JobID]; !ok {
		return jobs.ErrJobNotFound
	}
	if _, exists := s.leases[lease.ID]; exists {
		return jobs.ErrLeaseAlreadyExists
	}
	s.leases[lease.ID] = cloneLease(lease)
	return nil
}

func (s *MemoryJobStore) UpdateLease(_ context.Context, lease jobs.JobLease) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[lease.JobID]; !ok {
		return jobs.ErrJobNotFound
	}
	if _, exists := s.leases[lease.ID]; !exists {
		return jobs.ErrLeaseNotFound
	}
	s.leases[lease.ID] = cloneLease(lease)
	return nil
}

func (s *MemoryJobStore) LoadLease(_ context.Context, jobID, leaseID string) (*jobs.JobLease, error) {
	jobID = strings.TrimSpace(jobID)
	leaseID = strings.TrimSpace(leaseID)
	if jobID == "" {
		return nil, fmt.Errorf("job id required")
	}
	if leaseID == "" {
		return nil, fmt.Errorf("lease id required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	lease, ok := s.leases[leaseID]
	if !ok || lease.JobID != jobID {
		return nil, jobs.ErrLeaseNotFound
	}
	cloned := cloneLease(lease)
	return &cloned, nil
}

func (s *MemoryJobStore) ListLeases(_ context.Context, jobID string) ([]jobs.JobLease, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job id required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]jobs.JobLease, 0)
	for _, lease := range s.leases {
		if lease.JobID == jobID {
			out = append(out, cloneLease(lease))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out, nil
}

func cloneJob(job jobs.Job) jobs.Job {
	job.Labels = cloneStringMap(job.Labels)
	job.Tags = cloneStrings(job.Tags)
	job.Metadata = cloneAnyMap(job.Metadata)
	if job.CurrentAttempt != nil {
		attempt := cloneAttempt(*job.CurrentAttempt)
		job.CurrentAttempt = &attempt
	}
	if job.CurrentLease != nil {
		lease := cloneLease(*job.CurrentLease)
		job.CurrentLease = &lease
	}
	return job
}

func cloneAttempt(attempt jobs.JobAttempt) jobs.JobAttempt {
	attempt.CheckpointIDs = cloneStrings(attempt.CheckpointIDs)
	attempt.Metadata = cloneAnyMap(attempt.Metadata)
	return attempt
}

func cloneLease(lease jobs.JobLease) jobs.JobLease {
	lease.Metadata = cloneAnyMap(lease.Metadata)
	return lease
}

func cloneCheckpoint(checkpoint jobs.JobCheckpoint) jobs.JobCheckpoint {
	checkpoint.Metadata = cloneAnyMap(checkpoint.Metadata)
	checkpoint.State = cloneAny(checkpoint.State)
	return checkpoint
}

func cloneEvent(event jobs.JobEvent) jobs.JobEvent {
	event.Metadata = cloneAnyMap(event.Metadata)
	return event
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(input any) any {
	switch typed := input.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case map[string]string:
		return cloneStringMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = cloneAny(value)
		}
		return out
	case []string:
		return cloneStrings(typed)
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for i, value := range typed {
			out[i] = cloneAnyMap(value)
		}
		return out
	default:
		return input
	}
}
