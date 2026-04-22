package jobs

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

type JobState string

const (
	JobStateQueued       JobState = "queued"
	JobStateAdmitted     JobState = "admitted"
	JobStateLeased       JobState = "leased"
	JobStateRunning      JobState = "running"
	JobStatePaused       JobState = "paused"
	JobStateRetrying     JobState = "retrying"
	JobStateCancelled    JobState = "cancelled"
	JobStateFailed       JobState = "failed"
	JobStateCompleted    JobState = "completed"
	JobStateExpired      JobState = "expired"
	JobStateResumed      JobState = "resumed"
	JobStateDeadLettered JobState = "dead-lettered"
)

func (s JobState) Validate() error {
	switch s {
	case JobStateQueued, JobStateAdmitted, JobStateLeased, JobStateRunning, JobStatePaused, JobStateRetrying, JobStateCancelled, JobStateFailed, JobStateCompleted, JobStateExpired, JobStateResumed, JobStateDeadLettered:
		return nil
	default:
		return fmt.Errorf("job state %s invalid", s)
	}
}

type JobEventType string

const (
	JobEventTypeCreated          JobEventType = "created"
	JobEventTypeQueued           JobEventType = "queued"
	JobEventTypeAdmitted         JobEventType = "admitted"
	JobEventTypeLeased           JobEventType = "leased"
	JobEventTypeStarted          JobEventType = "started"
	JobEventTypeCheckpointed     JobEventType = "checkpointed"
	JobEventTypeHeartbeatRenewed JobEventType = "heartbeat-renewed"
	JobEventTypeRetryScheduled   JobEventType = "retry-scheduled"
	JobEventTypeCancelled        JobEventType = "cancelled"
	JobEventTypeResumed          JobEventType = "resumed"
	JobEventTypeCompleted        JobEventType = "completed"
	JobEventTypeFailed           JobEventType = "failed"
	JobEventTypeExpired          JobEventType = "expired"
	JobEventTypeDeadLettered     JobEventType = "dead-lettered"
)

func (t JobEventType) Validate() error {
	switch t {
	case JobEventTypeCreated, JobEventTypeQueued, JobEventTypeAdmitted, JobEventTypeLeased, JobEventTypeStarted, JobEventTypeCheckpointed, JobEventTypeHeartbeatRenewed, JobEventTypeRetryScheduled, JobEventTypeCancelled, JobEventTypeResumed, JobEventTypeCompleted, JobEventTypeFailed, JobEventTypeExpired, JobEventTypeDeadLettered:
		return nil
	default:
		return fmt.Errorf("job event type %s invalid", t)
	}
}

type WorkerSelector struct {
	WorkerIDs           []string          `json:"worker_ids,omitempty" yaml:"worker_ids,omitempty"`
	WorkerKinds         []string          `json:"worker_kinds,omitempty" yaml:"worker_kinds,omitempty"`
	Labels              map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Capabilities        []string          `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	QueueAffinities     []string          `json:"queue_affinities,omitempty" yaml:"queue_affinities,omitempty"`
	Localities          []string          `json:"localities,omitempty" yaml:"localities,omitempty"`
	ResourceClass       string            `json:"resource_class,omitempty" yaml:"resource_class,omitempty"`
	TrustDomain         string            `json:"trust_domain,omitempty" yaml:"trust_domain,omitempty"`
	RequireExclusiveUse bool              `json:"require_exclusive_use,omitempty" yaml:"require_exclusive_use,omitempty"`
}

func (s WorkerSelector) Validate() error {
	nonEmpty := 0
	for _, value := range append(append(append(append(append([]string{}, s.WorkerIDs...), s.WorkerKinds...), s.Capabilities...), s.QueueAffinities...), s.Localities...) {
		if strings.TrimSpace(value) == "" {
			return errors.New("worker selector contains empty value")
		}
		if strings.TrimSpace(value) != "" {
			nonEmpty++
		}
	}
	for key, value := range s.Labels {
		if strings.TrimSpace(key) == "" {
			return errors.New("worker selector labels contains empty key")
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("worker selector label %s contains empty value", key)
		}
		nonEmpty++
	}
	if strings.TrimSpace(s.ResourceClass) != "" {
		nonEmpty++
	}
	if strings.TrimSpace(s.TrustDomain) != "" {
		nonEmpty++
	}
	if nonEmpty == 0 {
		return errors.New("worker selector required")
	}
	return nil
}

type BackoffStrategy string

const (
	BackoffStrategyFixed       BackoffStrategy = "fixed"
	BackoffStrategyExponential BackoffStrategy = "exponential"
)

func (s BackoffStrategy) Validate() error {
	switch s {
	case BackoffStrategyFixed, BackoffStrategyExponential:
		return nil
	default:
		return fmt.Errorf("backoff strategy %s invalid", s)
	}
}

type BackoffPolicy struct {
	Strategy     BackoffStrategy `json:"strategy" yaml:"strategy"`
	InitialDelay time.Duration   `json:"initial_delay,omitempty" yaml:"initial_delay,omitempty"`
	FixedDelay   time.Duration   `json:"fixed_delay,omitempty" yaml:"fixed_delay,omitempty"`
	MaxDelay     time.Duration   `json:"max_delay,omitempty" yaml:"max_delay,omitempty"`
	Multiplier   float64         `json:"multiplier,omitempty" yaml:"multiplier,omitempty"`
	Jitter       float64         `json:"jitter,omitempty" yaml:"jitter,omitempty"`
}

func (p BackoffPolicy) Validate() error {
	if err := p.Strategy.Validate(); err != nil {
		return err
	}
	if p.InitialDelay < 0 {
		return errors.New("initial_delay must be >= 0")
	}
	if p.FixedDelay < 0 {
		return errors.New("fixed_delay must be >= 0")
	}
	if p.MaxDelay < 0 {
		return errors.New("max_delay must be >= 0")
	}
	if p.Multiplier < 0 {
		return errors.New("multiplier must be >= 0")
	}
	if p.Jitter < 0 || p.Jitter >= 1 {
		return errors.New("jitter must be in [0,1)")
	}
	switch p.Strategy {
	case BackoffStrategyFixed:
		if p.FixedDelay <= 0 {
			return errors.New("fixed_delay must be > 0 for fixed backoff")
		}
	case BackoffStrategyExponential:
		if p.InitialDelay <= 0 {
			return errors.New("initial_delay must be > 0 for exponential backoff")
		}
		if p.Multiplier < 1 {
			return errors.New("multiplier must be >= 1 for exponential backoff")
		}
		if p.MaxDelay > 0 && p.MaxDelay < p.InitialDelay {
			return errors.New("max_delay must be >= initial_delay")
		}
	}
	return nil
}

type RetryPolicy struct {
	MaxAttempts               int           `json:"max_attempts" yaml:"max_attempts"`
	Backoff                   BackoffPolicy `json:"backoff,omitempty" yaml:"backoff,omitempty"`
	RetryableErrors           []string      `json:"retryable_errors,omitempty" yaml:"retryable_errors,omitempty"`
	TerminalErrors            []string      `json:"terminal_errors,omitempty" yaml:"terminal_errors,omitempty"`
	RetryAfterLeaseExpiration bool          `json:"retry_after_lease_expiration,omitempty" yaml:"retry_after_lease_expiration,omitempty"`
	RetryBudgetCap            int           `json:"retry_budget_cap,omitempty" yaml:"retry_budget_cap,omitempty"`
}

func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 0 {
		return errors.New("max_attempts must be >= 0")
	}
	if p.RetryBudgetCap < 0 {
		return errors.New("retry_budget_cap must be >= 0")
	}
	if err := p.Backoff.Validate(); err != nil {
		return fmt.Errorf("backoff invalid: %w", err)
	}
	if err := validateNonEmptyStrings("retryable_errors", p.RetryableErrors); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("terminal_errors", p.TerminalErrors); err != nil {
		return err
	}
	return nil
}

func (p RetryPolicy) CanRetry(attemptNumber int, failureClass string, leaseExpired bool) bool {
	if attemptNumber < 0 {
		return false
	}
	if p.MaxAttempts <= 0 {
		return false
	}
	if leaseExpired && !p.RetryAfterLeaseExpiration {
		return false
	}
	if attemptNumber > p.MaxAttempts {
		return false
	}
	class := strings.ToLower(strings.TrimSpace(failureClass))
	for _, terminal := range p.TerminalErrors {
		if strings.EqualFold(strings.TrimSpace(terminal), class) {
			return false
		}
	}
	if len(p.RetryableErrors) > 0 {
		for _, retryable := range p.RetryableErrors {
			if strings.EqualFold(strings.TrimSpace(retryable), class) {
				return true
			}
		}
		return false
	}
	return true
}

func (p RetryPolicy) NextDelay(attemptNumber int, key string) time.Duration {
	if attemptNumber < 1 {
		attemptNumber = 1
	}
	base := p.Backoff.FixedDelay
	switch p.Backoff.Strategy {
	case BackoffStrategyFixed:
		if base <= 0 {
			base = p.Backoff.InitialDelay
		}
	case BackoffStrategyExponential:
		base = p.Backoff.InitialDelay
		for i := 1; i < attemptNumber; i++ {
			base = time.Duration(float64(base) * p.Backoff.Multiplier)
		}
		if p.Backoff.MaxDelay > 0 && base > p.Backoff.MaxDelay {
			base = p.Backoff.MaxDelay
		}
	default:
		return 0
	}
	if p.Backoff.Jitter <= 0 || base <= 0 {
		return base
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(key)))
	_, _ = h.Write([]byte{byte(attemptNumber & 0xff), byte((attemptNumber >> 8) & 0xff)})
	ratio := float64(h.Sum64()%10000) / 9999.0
	delta := (ratio*2 - 1) * p.Backoff.Jitter * float64(base)
	out := time.Duration(float64(base) + delta)
	if out < 0 {
		return 0
	}
	return out
}

func (p RetryPolicy) NextScheduledAt(now time.Time, attemptNumber int, key string) (time.Time, bool) {
	if !p.CanRetry(attemptNumber, "", false) {
		return time.Time{}, false
	}
	delay := p.NextDelay(attemptNumber, key)
	return now.Add(delay), true
}

type CancelMode string

const (
	CancelModeBestEffort  CancelMode = "best-effort"
	CancelModeCooperative CancelMode = "cooperative"
	CancelModeHard        CancelMode = "hard"
)

func (m CancelMode) Validate() error {
	switch m {
	case CancelModeBestEffort, CancelModeCooperative, CancelModeHard:
		return nil
	default:
		return fmt.Errorf("cancel mode %s invalid", m)
	}
}

type CancelPolicy struct {
	Mode           CancelMode    `json:"mode" yaml:"mode"`
	GracePeriod    time.Duration `json:"grace_period,omitempty" yaml:"grace_period,omitempty"`
	ReasonRequired bool          `json:"reason_required,omitempty" yaml:"reason_required,omitempty"`
}

func (p CancelPolicy) Validate() error {
	if err := p.Mode.Validate(); err != nil {
		return err
	}
	if p.GracePeriod < 0 {
		return errors.New("grace_period must be >= 0")
	}
	return nil
}

type ResumeMode string

const (
	ResumeModeDisabled    ResumeMode = "disabled"
	ResumeModeCheckpoint  ResumeMode = "checkpoint"
	ResumeModeRestartable ResumeMode = "restartable"
)

func (m ResumeMode) Validate() error {
	switch m {
	case ResumeModeDisabled, ResumeModeCheckpoint, ResumeModeRestartable:
		return nil
	default:
		return fmt.Errorf("resume mode %s invalid", m)
	}
}

type ResumePolicy struct {
	Mode                 ResumeMode `json:"mode" yaml:"mode"`
	RequiresCheckpoint   bool       `json:"requires_checkpoint,omitempty" yaml:"requires_checkpoint,omitempty"`
	CheckpointKey        string     `json:"checkpoint_key,omitempty" yaml:"checkpoint_key,omitempty"`
	AllowAfterFailure    bool       `json:"allow_after_failure,omitempty" yaml:"allow_after_failure,omitempty"`
	AllowAfterExpiration bool       `json:"allow_after_expiration,omitempty" yaml:"allow_after_expiration,omitempty"`
}

func (p ResumePolicy) Validate() error {
	if err := p.Mode.Validate(); err != nil {
		return err
	}
	if p.Mode == ResumeModeDisabled {
		if p.RequiresCheckpoint || strings.TrimSpace(p.CheckpointKey) != "" || p.AllowAfterFailure || p.AllowAfterExpiration {
			return errors.New("resume policy disabled cannot declare checkpoint or resume allowances")
		}
		return nil
	}
	if p.RequiresCheckpoint && strings.TrimSpace(p.CheckpointKey) == "" {
		return errors.New("checkpoint_key required when requires_checkpoint=true")
	}
	if p.Mode == ResumeModeCheckpoint && strings.TrimSpace(p.CheckpointKey) == "" {
		return errors.New("checkpoint_key required for checkpoint resume mode")
	}
	return nil
}

func (p ResumePolicy) CanResume(checkpoint *JobCheckpoint) error {
	if p.Mode == ResumeModeDisabled {
		return errors.New("resume disabled")
	}
	if p.RequiresCheckpoint && checkpoint == nil {
		return errors.New("checkpoint required for resume")
	}
	if checkpoint == nil {
		return nil
	}
	if strings.TrimSpace(p.CheckpointKey) != "" {
		if !strings.EqualFold(strings.TrimSpace(p.CheckpointKey), strings.TrimSpace(checkpoint.ID)) &&
			!strings.EqualFold(strings.TrimSpace(p.CheckpointKey), strings.TrimSpace(checkpoint.ResumeToken)) {
			return fmt.Errorf("checkpoint %s does not match resume checkpoint_key", checkpoint.ID)
		}
	}
	return nil
}

type LeasePolicy struct {
	Duration          time.Duration `json:"duration" yaml:"duration"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval,omitempty" yaml:"heartbeat_interval,omitempty"`
	MaxRenewals       int           `json:"max_renewals,omitempty" yaml:"max_renewals,omitempty"`
}

func (p LeasePolicy) Validate() error {
	if p.Duration <= 0 {
		return errors.New("duration must be > 0")
	}
	if p.HeartbeatInterval < 0 {
		return errors.New("heartbeat_interval must be >= 0")
	}
	if p.HeartbeatInterval > 0 && p.HeartbeatInterval > p.Duration {
		return errors.New("heartbeat_interval must be <= duration")
	}
	if p.MaxRenewals < 0 {
		return errors.New("max_renewals must be >= 0")
	}
	return nil
}

type TimeoutPolicy struct {
	Execution   time.Duration `json:"execution,omitempty" yaml:"execution,omitempty"`
	QueueWait   time.Duration `json:"queue_wait,omitempty" yaml:"queue_wait,omitempty"`
	GracePeriod time.Duration `json:"grace_period,omitempty" yaml:"grace_period,omitempty"`
}

func (p TimeoutPolicy) Validate() error {
	if p.Execution < 0 {
		return errors.New("execution must be >= 0")
	}
	if p.QueueWait < 0 {
		return errors.New("queue_wait must be >= 0")
	}
	if p.GracePeriod < 0 {
		return errors.New("grace_period must be >= 0")
	}
	return nil
}

type JobSpec struct {
	Kind                 string            `json:"kind" yaml:"kind"`
	Payload              any               `json:"payload" yaml:"payload"`
	Queue                string            `json:"queue" yaml:"queue"`
	Priority             int               `json:"priority" yaml:"priority"`
	WorkerSelector       WorkerSelector    `json:"worker_selector" yaml:"worker_selector"`
	RetryPolicy          RetryPolicy       `json:"retry_policy" yaml:"retry_policy"`
	CancelPolicy         CancelPolicy      `json:"cancel_policy" yaml:"cancel_policy"`
	ResumePolicy         ResumePolicy      `json:"resume_policy" yaml:"resume_policy"`
	TimeoutPolicy        TimeoutPolicy     `json:"timeout_policy" yaml:"timeout_policy"`
	LeasePolicy          LeasePolicy       `json:"lease_policy" yaml:"lease_policy"`
	Labels               map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Tags                 []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	CorrelationID        string            `json:"correlation_id,omitempty" yaml:"correlation_id,omitempty"`
	ParentJobID          string            `json:"parent_job_id,omitempty" yaml:"parent_job_id,omitempty"`
	PreferredWorkers     []string          `json:"preferred_workers,omitempty" yaml:"preferred_workers,omitempty"`
	RequiredCapabilities []string          `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty"`
	Notes                string            `json:"notes,omitempty" yaml:"notes,omitempty"`
}

func (s JobSpec) Validate() error {
	if strings.TrimSpace(s.Kind) == "" {
		return errors.New("kind required")
	}
	if s.Payload == nil {
		return errors.New("payload required")
	}
	if strings.TrimSpace(s.Queue) == "" {
		return errors.New("queue required")
	}
	if err := s.WorkerSelector.Validate(); err != nil {
		return fmt.Errorf("worker selector invalid: %w", err)
	}
	if err := s.RetryPolicy.Validate(); err != nil {
		return fmt.Errorf("retry policy invalid: %w", err)
	}
	if err := s.CancelPolicy.Validate(); err != nil {
		return fmt.Errorf("cancel policy invalid: %w", err)
	}
	if err := s.ResumePolicy.Validate(); err != nil {
		return fmt.Errorf("resume policy invalid: %w", err)
	}
	if err := s.TimeoutPolicy.Validate(); err != nil {
		return fmt.Errorf("timeout policy invalid: %w", err)
	}
	if err := s.LeasePolicy.Validate(); err != nil {
		return fmt.Errorf("lease policy invalid: %w", err)
	}
	if err := validateStringMap("labels", s.Labels); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("tags", s.Tags); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("preferred_workers", s.PreferredWorkers); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("required_capabilities", s.RequiredCapabilities); err != nil {
		return err
	}
	if strings.TrimSpace(s.CorrelationID) == "" && s.CorrelationID != "" {
		return errors.New("correlation_id must not be whitespace")
	}
	if strings.TrimSpace(s.ParentJobID) == "" && s.ParentJobID != "" {
		return errors.New("parent_job_id must not be whitespace")
	}
	return nil
}

type Job struct {
	ID                 string            `json:"id" yaml:"id"`
	ParentJobID        string            `json:"parent_job_id,omitempty" yaml:"parent_job_id,omitempty"`
	RootWorkflowID     string            `json:"root_workflow_id,omitempty" yaml:"root_workflow_id,omitempty"`
	TraceID            string            `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
	IdempotencyKey     string            `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
	Source             string            `json:"source,omitempty" yaml:"source,omitempty"`
	Spec               JobSpec           `json:"spec" yaml:"spec"`
	State              JobState          `json:"state" yaml:"state"`
	AttemptCount       int               `json:"attempt_count,omitempty" yaml:"attempt_count,omitempty"`
	CreatedAt          time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	AvailableAt        time.Time         `json:"available_at,omitempty" yaml:"available_at,omitempty"`
	NextRetryAt        time.Time         `json:"next_retry_at,omitempty" yaml:"next_retry_at,omitempty"`
	CompletedAt        time.Time         `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	ResumeCheckpointID string            `json:"resume_checkpoint_id,omitempty" yaml:"resume_checkpoint_id,omitempty"`
	LastError          string            `json:"last_error,omitempty" yaml:"last_error,omitempty"`
	Labels             map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Tags               []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Metadata           map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CurrentAttempt     *JobAttempt       `json:"current_attempt,omitempty" yaml:"current_attempt,omitempty"`
	CurrentLease       *JobLease         `json:"current_lease,omitempty" yaml:"current_lease,omitempty"`
}

func (j Job) Validate() error {
	if strings.TrimSpace(j.ID) == "" {
		return errors.New("job id required")
	}
	if err := j.Spec.Validate(); err != nil {
		return fmt.Errorf("spec invalid: %w", err)
	}
	if err := j.State.Validate(); err != nil {
		return err
	}
	if j.AttemptCount < 0 {
		return errors.New("attempt_count must be >= 0")
	}
	if err := validateStringMap("labels", j.Labels); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("tags", j.Tags); err != nil {
		return err
	}
	if j.CreatedAt.IsZero() {
		return errors.New("created_at required")
	}
	if !j.UpdatedAt.IsZero() && j.UpdatedAt.Before(j.CreatedAt) {
		return errors.New("updated_at must be after created_at")
	}
	if !j.AvailableAt.IsZero() && j.AvailableAt.Before(j.CreatedAt) {
		return errors.New("available_at must be after created_at")
	}
	if !j.NextRetryAt.IsZero() && j.NextRetryAt.Before(j.CreatedAt) {
		return errors.New("next_retry_at must be after created_at")
	}
	if !j.CompletedAt.IsZero() && j.CompletedAt.Before(j.CreatedAt) {
		return errors.New("completed_at must be after created_at")
	}
	if j.CurrentAttempt != nil {
		if err := j.CurrentAttempt.Validate(); err != nil {
			return fmt.Errorf("current attempt invalid: %w", err)
		}
	}
	if j.CurrentLease != nil {
		if err := j.CurrentLease.Validate(); err != nil {
			return fmt.Errorf("current lease invalid: %w", err)
		}
	}
	return nil
}

type JobAttempt struct {
	ID              string         `json:"id" yaml:"id"`
	JobID           string         `json:"job_id" yaml:"job_id"`
	AttemptNumber   int            `json:"attempt_number" yaml:"attempt_number"`
	WorkerID        string         `json:"worker_id,omitempty" yaml:"worker_id,omitempty"`
	RunID           string         `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	LeaseID         string         `json:"lease_id,omitempty" yaml:"lease_id,omitempty"`
	State           JobState       `json:"state" yaml:"state"`
	StartedAt       time.Time      `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	FinishedAt      time.Time      `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	LastHeartbeatAt time.Time      `json:"last_heartbeat_at,omitempty" yaml:"last_heartbeat_at,omitempty"`
	Error           string         `json:"error,omitempty" yaml:"error,omitempty"`
	CheckpointIDs   []string       `json:"checkpoint_ids,omitempty" yaml:"checkpoint_ids,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (a JobAttempt) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("attempt id required")
	}
	if strings.TrimSpace(a.JobID) == "" {
		return errors.New("job id required")
	}
	if a.AttemptNumber < 0 {
		return errors.New("attempt_number must be >= 0")
	}
	if err := a.State.Validate(); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("checkpoint_ids", a.CheckpointIDs); err != nil {
		return err
	}
	if !a.FinishedAt.IsZero() && !a.StartedAt.IsZero() && a.FinishedAt.Before(a.StartedAt) {
		return errors.New("finished_at must be after started_at")
	}
	if !a.LastHeartbeatAt.IsZero() && !a.StartedAt.IsZero() && a.LastHeartbeatAt.Before(a.StartedAt) {
		return errors.New("last_heartbeat_at must be after started_at")
	}
	return nil
}

type JobLease struct {
	ID          string         `json:"id" yaml:"id"`
	JobID       string         `json:"job_id" yaml:"job_id"`
	AttemptID   string         `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty"`
	WorkerID    string         `json:"worker_id" yaml:"worker_id"`
	Sequence    int            `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	AcquiredAt  time.Time      `json:"acquired_at" yaml:"acquired_at"`
	ExpiresAt   time.Time      `json:"expires_at" yaml:"expires_at"`
	RenewedAt   time.Time      `json:"renewed_at,omitempty" yaml:"renewed_at,omitempty"`
	HeartbeatAt time.Time      `json:"heartbeat_at,omitempty" yaml:"heartbeat_at,omitempty"`
	RevokedAt   time.Time      `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
	Reason      string         `json:"reason,omitempty" yaml:"reason,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (l JobLease) Validate() error {
	if strings.TrimSpace(l.ID) == "" {
		return errors.New("lease id required")
	}
	if strings.TrimSpace(l.JobID) == "" {
		return errors.New("job id required")
	}
	if strings.TrimSpace(l.WorkerID) == "" {
		return errors.New("worker id required")
	}
	if l.AcquiredAt.IsZero() {
		return errors.New("acquired_at required")
	}
	if l.ExpiresAt.IsZero() {
		return errors.New("expires_at required")
	}
	if l.ExpiresAt.Before(l.AcquiredAt) {
		return errors.New("expires_at must be after acquired_at")
	}
	if !l.RenewedAt.IsZero() && l.RenewedAt.Before(l.AcquiredAt) {
		return errors.New("renewed_at must be after acquired_at")
	}
	if !l.HeartbeatAt.IsZero() && l.HeartbeatAt.Before(l.AcquiredAt) {
		return errors.New("heartbeat_at must be after acquired_at")
	}
	return nil
}

type JobCheckpoint struct {
	ID            string         `json:"id" yaml:"id"`
	JobID         string         `json:"job_id" yaml:"job_id"`
	AttemptNumber int            `json:"attempt_number" yaml:"attempt_number"`
	Sequence      int            `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	ResumeToken   string         `json:"resume_token,omitempty" yaml:"resume_token,omitempty"`
	State         any            `json:"state" yaml:"state"`
	CreatedAt     time.Time      `json:"created_at" yaml:"created_at"`
	Metadata      map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (c JobCheckpoint) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("checkpoint id required")
	}
	if strings.TrimSpace(c.JobID) == "" {
		return errors.New("job id required")
	}
	if c.AttemptNumber < 0 {
		return errors.New("attempt_number must be >= 0")
	}
	if c.State == nil {
		return errors.New("state required")
	}
	if c.CreatedAt.IsZero() {
		return errors.New("created_at required")
	}
	return nil
}

type JobEvent struct {
	ID         string         `json:"id" yaml:"id"`
	JobID      string         `json:"job_id" yaml:"job_id"`
	AttemptID  string         `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty"`
	Type       JobEventType   `json:"type" yaml:"type"`
	State      JobState       `json:"state,omitempty" yaml:"state,omitempty"`
	Sequence   int            `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	OccurredAt time.Time      `json:"occurred_at" yaml:"occurred_at"`
	WorkerID   string         `json:"worker_id,omitempty" yaml:"worker_id,omitempty"`
	LeaseID    string         `json:"lease_id,omitempty" yaml:"lease_id,omitempty"`
	Message    string         `json:"message,omitempty" yaml:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (e JobEvent) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("event id required")
	}
	if strings.TrimSpace(e.JobID) == "" {
		return errors.New("job id required")
	}
	if err := e.Type.Validate(); err != nil {
		return err
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at required")
	}
	if e.State != "" {
		if err := e.State.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateNonEmptyStrings(name string, values []string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains empty value", name)
		}
	}
	return nil
}

func validateStringMap(name string, values map[string]string) error {
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%s contains empty key", name)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s key %s contains empty value", name, key)
		}
	}
	return nil
}
