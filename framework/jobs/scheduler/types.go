package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/jobs"
)

type WorkerDescriptor struct {
	ID              string            `json:"id" yaml:"id"`
	Kind            string            `json:"kind,omitempty" yaml:"kind,omitempty"`
	Labels          map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	QueueAffinities []string          `json:"queue_affinities,omitempty" yaml:"queue_affinities,omitempty"`
	Locality        string            `json:"locality,omitempty" yaml:"locality,omitempty"`
	ResourceClass   string            `json:"resource_class,omitempty" yaml:"resource_class,omitempty"`
	TrustDomain     string            `json:"trust_domain,omitempty" yaml:"trust_domain,omitempty"`
	Load            int               `json:"load,omitempty" yaml:"load,omitempty"`
	Eligible        bool              `json:"eligible,omitempty" yaml:"eligible,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (w WorkerDescriptor) Validate() error {
	if strings.TrimSpace(w.ID) == "" {
		return errors.New("worker id required")
	}
	if w.Load < 0 {
		return errors.New("load must be >= 0")
	}
	for key, value := range w.Labels {
		if strings.TrimSpace(key) == "" {
			return errors.New("worker labels contains empty key")
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("worker label %s contains empty value", key)
		}
	}
	return nil
}

type WorkerSelection struct {
	WorkerID    string         `json:"worker_id" yaml:"worker_id"`
	Reason      string         `json:"reason" yaml:"reason"`
	EligibleIDs []string       `json:"eligible_ids,omitempty" yaml:"eligible_ids,omitempty"`
	Queue       string         `json:"queue" yaml:"queue"`
	SelectedAt  time.Time      `json:"selected_at" yaml:"selected_at"`
	Metadata    map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type DispatchResult struct {
	Job       jobs.Job         `json:"job" yaml:"job"`
	Worker    WorkerDescriptor `json:"worker" yaml:"worker"`
	Selection WorkerSelection  `json:"selection" yaml:"selection"`
	Lease     jobs.JobLease    `json:"lease" yaml:"lease"`
}

type Scheduler interface {
	Admit(ctx context.Context, jobID string) (*jobs.Job, error)
	Dispatch(ctx context.Context) (*DispatchResult, error)
	Reschedule(ctx context.Context, jobID string) (*jobs.Job, error)
	Cancel(ctx context.Context, jobID, reason string) (*jobs.Job, error)
	Resume(ctx context.Context, jobID string) (*jobs.Job, error)
	RenewLease(ctx context.Context, jobID, leaseID string) (*jobs.JobLease, error)
	ReleaseLease(ctx context.Context, jobID, leaseID, reason string) (*jobs.JobLease, error)
	ExpireLease(ctx context.Context, jobID, leaseID string) (*jobs.JobLease, error)
	Complete(ctx context.Context, jobID, leaseID string) (*jobs.Job, error)
	Fail(ctx context.Context, jobID, leaseID, failureClass, reason string) (*jobs.Job, error)
}

type MemoryScheduler struct {
	store   jobs.JobStore
	clock   func() time.Time
	workers map[string]WorkerDescriptor
}

func NewMemoryScheduler(store jobs.JobStore, workers []WorkerDescriptor) *MemoryScheduler {
	registry := make(map[string]WorkerDescriptor, len(workers))
	for _, worker := range workers {
		registry[worker.ID] = cloneWorker(worker)
	}
	return &MemoryScheduler{
		store:   store,
		clock:   time.Now,
		workers: registry,
	}
}

func (s *MemoryScheduler) Admit(ctx context.Context, jobID string) (*jobs.Job, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.State != jobs.JobStateQueued {
		return nil, fmt.Errorf("job %s is not queued", jobID)
	}
	job.State = jobs.JobStateAdmitted
	job.UpdatedAt = s.now()
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "admitted", job.AttemptCount),
		JobID:      job.ID,
		Type:       jobs.JobEventTypeAdmitted,
		State:      jobs.JobStateAdmitted,
		OccurredAt: s.now(),
	}); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) Dispatch(ctx context.Context) (*DispatchResult, error) {
	candidates, err := s.readyJobs(ctx)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, jobs.ErrJobNotFound
	}
	now := s.now()
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Spec.Priority != candidates[j].Spec.Priority {
			return candidates[i].Spec.Priority > candidates[j].Spec.Priority
		}
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	for _, candidate := range candidates {
		if !candidate.AvailableAt.IsZero() && candidate.AvailableAt.After(now) {
			continue
		}
		worker, selection, err := s.selectWorker(&candidate)
		if err != nil {
			continue
		}
		job := candidate
		job.State = jobs.JobStateLeased
		job.AttemptCount++
		job.UpdatedAt = s.now()
		job.NextRetryAt = time.Time{}
		lease := jobs.JobLease{
			ID:         leaseID(job.ID, job.AttemptCount),
			JobID:      job.ID,
			AttemptID:  attemptID(job.ID, job.AttemptCount),
			WorkerID:   worker.ID,
			Sequence:   job.AttemptCount,
			AcquiredAt: s.now(),
			ExpiresAt:  s.now().Add(job.Spec.LeasePolicy.Duration),
		}
		job.CurrentLease = &lease
		job.CurrentAttempt = &jobs.JobAttempt{
			ID:            attemptID(job.ID, job.AttemptCount),
			JobID:         job.ID,
			AttemptNumber: job.AttemptCount,
			WorkerID:      worker.ID,
			LeaseID:       lease.ID,
			State:         jobs.JobStateLeased,
			StartedAt:     s.now(),
		}
		if err := s.store.StoreLease(ctx, lease); err != nil {
			return nil, err
		}
		if err := s.store.UpdateJob(ctx, job); err != nil {
			return nil, err
		}
		if err := s.store.AppendEvent(ctx, jobs.JobEvent{
			ID:         eventID(job.ID, "leased", job.AttemptCount),
			JobID:      job.ID,
			AttemptID:  lease.AttemptID,
			Type:       jobs.JobEventTypeLeased,
			State:      jobs.JobStateLeased,
			WorkerID:   worker.ID,
			LeaseID:    lease.ID,
			OccurredAt: s.now(),
			Metadata: map[string]any{
				"selection_reason": selection.Reason,
				"eligible_workers": selection.EligibleIDs,
			},
		}); err != nil {
			return nil, err
		}
		return &DispatchResult{
			Job:       job,
			Worker:    worker,
			Selection: selection,
			Lease:     lease,
		}, nil
	}
	return nil, fmt.Errorf("no eligible workers for queued jobs")
}

func (s *MemoryScheduler) Reschedule(ctx context.Context, jobID string) (*jobs.Job, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	job.State = jobs.JobStateRetrying
	job.CurrentLease = nil
	job.CurrentAttempt = nil
	job.AvailableAt = s.now()
	job.UpdatedAt = s.now()
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) Cancel(ctx context.Context, jobID, reason string) (*jobs.Job, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	job.State = jobs.JobStateCancelled
	job.LastError = strings.TrimSpace(reason)
	job.UpdatedAt = s.now()
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "cancelled", job.AttemptCount),
		JobID:      job.ID,
		Type:       jobs.JobEventTypeCancelled,
		State:      jobs.JobStateCancelled,
		OccurredAt: s.now(),
		Message:    reason,
	}); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) Resume(ctx context.Context, jobID string) (*jobs.Job, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	checkpoint, err := s.latestCheckpointForJob(ctx, job)
	if err != nil {
		return nil, err
	}
	if err := job.Spec.ResumePolicy.CanResume(checkpoint); err != nil {
		return nil, err
	}
	job.State = jobs.JobStateResumed
	job.ResumeCheckpointID = ""
	if checkpoint != nil {
		job.ResumeCheckpointID = checkpoint.ID
	}
	job.CurrentLease = nil
	job.CurrentAttempt = nil
	job.AvailableAt = s.now()
	job.UpdatedAt = s.now()
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "resumed", job.AttemptCount),
		JobID:      job.ID,
		Type:       jobs.JobEventTypeResumed,
		State:      jobs.JobStateResumed,
		OccurredAt: s.now(),
		Metadata: map[string]any{
			"checkpoint_id": job.ResumeCheckpointID,
		},
	}); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) RenewLease(ctx context.Context, jobID, leaseID string) (*jobs.JobLease, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.CurrentLease == nil || !strings.EqualFold(job.CurrentLease.ID, leaseID) {
		return nil, fmt.Errorf("lease %s does not match current job lease", leaseID)
	}
	now := s.now()
	if !job.CurrentLease.ExpiresAt.IsZero() && now.After(job.CurrentLease.ExpiresAt) {
		return nil, fmt.Errorf("lease %s already expired", leaseID)
	}
	renewed := cloneLease(*job.CurrentLease)
	renewed.RenewedAt = now
	renewed.HeartbeatAt = now
	renewed.ExpiresAt = now.Add(job.Spec.LeasePolicy.Duration)
	if err := s.store.UpdateLease(ctx, renewed); err != nil {
		return nil, err
	}
	job.CurrentLease = &renewed
	job.State = jobs.JobStateLeased
	job.UpdatedAt = now
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "heartbeat", job.AttemptCount),
		JobID:      job.ID,
		AttemptID:  renewed.AttemptID,
		Type:       jobs.JobEventTypeHeartbeatRenewed,
		State:      jobs.JobStateLeased,
		LeaseID:    renewed.ID,
		WorkerID:   renewed.WorkerID,
		OccurredAt: now,
	}); err != nil {
		return nil, err
	}
	return &renewed, nil
}

func (s *MemoryScheduler) ReleaseLease(ctx context.Context, jobID, leaseID, reason string) (*jobs.JobLease, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.CurrentLease == nil || !strings.EqualFold(job.CurrentLease.ID, leaseID) {
		return nil, fmt.Errorf("lease %s does not match current job lease", leaseID)
	}
	now := s.now()
	released := cloneLease(*job.CurrentLease)
	released.RevokedAt = now
	released.Reason = reason
	if err := s.store.UpdateLease(ctx, released); err != nil {
		return nil, err
	}
	job.CurrentLease = nil
	job.CurrentAttempt = nil
	job.State = jobs.JobStateQueued
	job.AvailableAt = now
	job.NextRetryAt = time.Time{}
	job.UpdatedAt = now
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "released", job.AttemptCount),
		JobID:      job.ID,
		AttemptID:  released.AttemptID,
		Type:       jobs.JobEventTypeQueued,
		State:      jobs.JobStateQueued,
		LeaseID:    released.ID,
		WorkerID:   released.WorkerID,
		OccurredAt: now,
		Message:    reason,
	}); err != nil {
		return nil, err
	}
	return &released, nil
}

func (s *MemoryScheduler) ExpireLease(ctx context.Context, jobID, leaseID string) (*jobs.JobLease, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.CurrentLease == nil || !strings.EqualFold(job.CurrentLease.ID, leaseID) {
		return nil, fmt.Errorf("lease %s does not match current job lease", leaseID)
	}
	now := s.now()
	expired := cloneLease(*job.CurrentLease)
	expired.RevokedAt = now
	expired.Reason = "expired"
	if err := s.store.UpdateLease(ctx, expired); err != nil {
		return nil, err
	}
	job.CurrentLease = nil
	job.CurrentAttempt = nil
	job.State = jobs.JobStateExpired
	job.AvailableAt = now
	job.NextRetryAt = now
	job.UpdatedAt = now
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "expired", job.AttemptCount),
		JobID:      job.ID,
		AttemptID:  expired.AttemptID,
		Type:       jobs.JobEventTypeExpired,
		State:      jobs.JobStateExpired,
		LeaseID:    expired.ID,
		WorkerID:   expired.WorkerID,
		OccurredAt: now,
	}); err != nil {
		return nil, err
	}
	return &expired, nil
}

func (s *MemoryScheduler) Complete(ctx context.Context, jobID, leaseID string) (*jobs.Job, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.State == jobs.JobStateCompleted && (job.CurrentLease == nil || strings.EqualFold(job.CurrentLease.ID, leaseID)) {
		return job, nil
	}
	if job.CurrentLease == nil || !strings.EqualFold(job.CurrentLease.ID, leaseID) {
		return nil, fmt.Errorf("lease %s does not match current job lease", leaseID)
	}
	now := s.now()
	closed := cloneLease(*job.CurrentLease)
	closed.RevokedAt = now
	closed.Reason = "completed"
	if err := s.store.UpdateLease(ctx, closed); err != nil {
		return nil, err
	}
	job.State = jobs.JobStateCompleted
	job.CompletedAt = now
	job.CurrentLease = nil
	job.CurrentAttempt = nil
	job.AvailableAt = time.Time{}
	job.NextRetryAt = time.Time{}
	job.UpdatedAt = now
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "completed", job.AttemptCount),
		JobID:      job.ID,
		AttemptID:  closed.AttemptID,
		Type:       jobs.JobEventTypeCompleted,
		State:      jobs.JobStateCompleted,
		LeaseID:    closed.ID,
		WorkerID:   closed.WorkerID,
		OccurredAt: now,
	}); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) Fail(ctx context.Context, jobID, leaseID, failureClass, reason string) (*jobs.Job, error) {
	job, err := s.loadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.CurrentLease == nil || !strings.EqualFold(job.CurrentLease.ID, leaseID) {
		if job.State == jobs.JobStateFailed {
			return job, nil
		}
		return nil, fmt.Errorf("lease %s does not match current job lease", leaseID)
	}
	now := s.now()
	failedLease := cloneLease(*job.CurrentLease)
	failedLease.RevokedAt = now
	failedLease.Reason = reason
	if err := s.store.UpdateLease(ctx, failedLease); err != nil {
		return nil, err
	}
	job.LastError = strings.TrimSpace(reason)
	job.CurrentLease = nil
	job.CurrentAttempt = nil
	job.UpdatedAt = now
	shouldRetry := job.Spec.RetryPolicy.CanRetry(job.AttemptCount, failureClass, false)
	if shouldRetry {
		nextAttempt := job.AttemptCount + 1
		nextAt, ok := job.Spec.RetryPolicy.NextScheduledAt(now, nextAttempt, job.ID)
		if ok {
			job.State = jobs.JobStateRetrying
			job.AvailableAt = nextAt
			job.NextRetryAt = nextAt
			if err := s.store.UpdateJob(ctx, *job); err != nil {
				return nil, err
			}
			if err := s.store.AppendEvent(ctx, jobs.JobEvent{
				ID:         eventID(job.ID, "retry", job.AttemptCount),
				JobID:      job.ID,
				AttemptID:  failedLease.AttemptID,
				Type:       jobs.JobEventTypeRetryScheduled,
				State:      jobs.JobStateRetrying,
				LeaseID:    failedLease.ID,
				WorkerID:   failedLease.WorkerID,
				OccurredAt: now,
				Metadata: map[string]any{
					"next_attempt":  nextAttempt,
					"next_retry_at": nextAt,
					"failure_class": failureClass,
				},
			}); err != nil {
				return nil, err
			}
			return job, nil
		}
	}
	job.State = jobs.JobStateFailed
	job.CompletedAt = now
	job.AvailableAt = time.Time{}
	job.NextRetryAt = time.Time{}
	if err := s.store.UpdateJob(ctx, *job); err != nil {
		return nil, err
	}
	if err := s.store.AppendEvent(ctx, jobs.JobEvent{
		ID:         eventID(job.ID, "failed", job.AttemptCount),
		JobID:      job.ID,
		AttemptID:  failedLease.AttemptID,
		Type:       jobs.JobEventTypeFailed,
		State:      jobs.JobStateFailed,
		LeaseID:    failedLease.ID,
		WorkerID:   failedLease.WorkerID,
		OccurredAt: now,
		Message:    reason,
		Metadata: map[string]any{
			"failure_class": failureClass,
		},
	}); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) selectWorker(job *jobs.Job) (WorkerDescriptor, WorkerSelection, error) {
	if job == nil {
		return WorkerDescriptor{}, WorkerSelection{}, errors.New("job required")
	}
	eligible := make([]WorkerDescriptor, 0, len(s.workers))
	for _, worker := range s.workers {
		if workerEligibleForJob(worker, *job) {
			eligible = append(eligible, cloneWorker(worker))
		}
	}
	if len(eligible) == 0 {
		return WorkerDescriptor{}, WorkerSelection{}, fmt.Errorf("no eligible workers for job %s", job.ID)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Load != eligible[j].Load {
			return eligible[i].Load < eligible[j].Load
		}
		return eligible[i].ID < eligible[j].ID
	})
	selected := eligible[0]
	reason := "lowest-load eligible worker"
	if len(job.Spec.WorkerSelector.WorkerIDs) > 0 {
		for _, preferredID := range job.Spec.WorkerSelector.WorkerIDs {
			for _, worker := range eligible {
				if strings.EqualFold(worker.ID, preferredID) {
					selected = worker
					reason = "explicit worker id preference"
					goto done
				}
			}
		}
	}
done:
	eligibleIDs := make([]string, 0, len(eligible))
	for _, worker := range eligible {
		eligibleIDs = append(eligibleIDs, worker.ID)
	}
	return selected, WorkerSelection{
		WorkerID:    selected.ID,
		Reason:      reason,
		EligibleIDs: eligibleIDs,
		Queue:       job.Spec.Queue,
		SelectedAt:  s.now(),
	}, nil
}

func (s *MemoryScheduler) readyJobs(ctx context.Context) ([]jobs.Job, error) {
	states := []jobs.JobState{
		jobs.JobStateQueued,
		jobs.JobStateAdmitted,
		jobs.JobStateRetrying,
		jobs.JobStateExpired,
		jobs.JobStateResumed,
	}
	out := make([]jobs.Job, 0)
	for _, state := range states {
		jobsByState, err := s.store.ListJobs(ctx, jobs.JobQuery{State: state})
		if err != nil {
			return nil, err
		}
		out = append(out, jobsByState...)
	}
	return out, nil
}

func (s *MemoryScheduler) latestCheckpointForJob(ctx context.Context, job *jobs.Job) (*jobs.JobCheckpoint, error) {
	if job == nil {
		return nil, fmt.Errorf("job required")
	}
	if strings.TrimSpace(job.ResumeCheckpointID) != "" {
		checkpoint, err := s.store.LoadCheckpoint(ctx, job.ID, job.ResumeCheckpointID)
		if err != nil {
			return nil, err
		}
		return checkpoint, nil
	}
	checkpoints, err := s.store.ListCheckpoints(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	if len(checkpoints) == 0 {
		return nil, nil
	}
	latest := checkpoints[len(checkpoints)-1]
	return &latest, nil
}

func workerEligibleForJob(worker WorkerDescriptor, job jobs.Job) bool {
	if strings.TrimSpace(worker.ID) == "" {
		return false
	}
	if strings.TrimSpace(job.Spec.Queue) == "" {
		return false
	}
	if len(job.Spec.WorkerSelector.WorkerIDs) > 0 && !containsString(job.Spec.WorkerSelector.WorkerIDs, worker.ID) {
		return false
	}
	if len(job.Spec.WorkerSelector.WorkerKinds) > 0 && !containsString(job.Spec.WorkerSelector.WorkerKinds, worker.Kind) {
		return false
	}
	if len(job.Spec.WorkerSelector.Capabilities) > 0 && !containsAllStrings(worker.Capabilities, job.Spec.WorkerSelector.Capabilities) {
		return false
	}
	if len(job.Spec.WorkerSelector.QueueAffinities) > 0 && !containsString(job.Spec.WorkerSelector.QueueAffinities, job.Spec.Queue) {
		return false
	}
	if len(worker.QueueAffinities) > 0 && !containsString(worker.QueueAffinities, job.Spec.Queue) {
		return false
	}
	if len(job.Spec.WorkerSelector.Localities) > 0 && !containsString(job.Spec.WorkerSelector.Localities, worker.Locality) {
		return false
	}
	if strings.TrimSpace(job.Spec.WorkerSelector.ResourceClass) != "" && !strings.EqualFold(job.Spec.WorkerSelector.ResourceClass, worker.ResourceClass) {
		return false
	}
	if strings.TrimSpace(job.Spec.WorkerSelector.TrustDomain) != "" && !strings.EqualFold(job.Spec.WorkerSelector.TrustDomain, worker.TrustDomain) {
		return false
	}
	for key, value := range job.Spec.WorkerSelector.Labels {
		if workerValue, ok := worker.Labels[key]; !ok || !strings.EqualFold(workerValue, value) {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func containsAllStrings(values []string, wants []string) bool {
	for _, want := range wants {
		if !containsString(values, want) {
			return false
		}
	}
	return true
}

func (s *MemoryScheduler) loadJob(ctx context.Context, jobID string) (*jobs.Job, error) {
	job, err := s.store.LoadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *MemoryScheduler) now() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

func cloneWorker(worker WorkerDescriptor) WorkerDescriptor {
	worker.Labels = cloneStringMap(worker.Labels)
	worker.Capabilities = append([]string{}, worker.Capabilities...)
	worker.QueueAffinities = append([]string{}, worker.QueueAffinities...)
	worker.Metadata = cloneAnyMap(worker.Metadata)
	return worker
}

func cloneLease(lease jobs.JobLease) jobs.JobLease {
	lease.Metadata = cloneAnyMap(lease.Metadata)
	return lease
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

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func eventID(jobID, kind string, attempt int) string {
	return fmt.Sprintf("%s.%s.%d", jobID, kind, attempt)
}

func attemptID(jobID string, attempt int) string {
	return fmt.Sprintf("%s.attempt.%d", jobID, attempt)
}

func leaseID(jobID string, attempt int) string {
	return fmt.Sprintf("%s.lease.%d", jobID, attempt)
}
