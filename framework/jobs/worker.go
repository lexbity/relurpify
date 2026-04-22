package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JobExecutionEnvelope is the job-shaped runtime input passed to workers.
// It preserves routing metadata and execution policy without importing agent
// taxonomy into the jobs package.
type JobExecutionEnvelope struct {
	JobID                string            `json:"job_id" yaml:"job_id"`
	AttemptID            string            `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty"`
	LeaseID              string            `json:"lease_id,omitempty" yaml:"lease_id,omitempty"`
	WorkerID             string            `json:"worker_id,omitempty" yaml:"worker_id,omitempty"`
	Queue                string            `json:"queue" yaml:"queue"`
	Kind                 string            `json:"kind" yaml:"kind"`
	Priority             int               `json:"priority" yaml:"priority"`
	Payload              any               `json:"payload" yaml:"payload"`
	Labels               map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Tags                 []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	RequiredCapabilities []string          `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty"`
	WorkerSelector       WorkerSelector    `json:"worker_selector" yaml:"worker_selector"`
	SelectionReason      string            `json:"selection_reason,omitempty" yaml:"selection_reason,omitempty"`
	RouteMetadata        map[string]any    `json:"route_metadata,omitempty" yaml:"route_metadata,omitempty"`
	AttemptNumber        int               `json:"attempt_number,omitempty" yaml:"attempt_number,omitempty"`
	CreatedAt            time.Time         `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	AvailableAt          time.Time         `json:"available_at,omitempty" yaml:"available_at,omitempty"`
	NextRetryAt          time.Time         `json:"next_retry_at,omitempty" yaml:"next_retry_at,omitempty"`
}

func (e JobExecutionEnvelope) Validate() error {
	if strings.TrimSpace(e.JobID) == "" {
		return errors.New("job id required")
	}
	if strings.TrimSpace(e.Queue) == "" {
		return errors.New("queue required")
	}
	if strings.TrimSpace(e.Kind) == "" {
		return errors.New("kind required")
	}
	if e.Payload == nil {
		return errors.New("payload required")
	}
	if err := validateStringMap("labels", e.Labels); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("tags", e.Tags); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("required_capabilities", e.RequiredCapabilities); err != nil {
		return err
	}
	if err := e.WorkerSelector.Validate(); err != nil {
		return fmt.Errorf("worker selector invalid: %w", err)
	}
	return nil
}

func (e JobExecutionEnvelope) RoutingMetadata() map[string]any {
	out := make(map[string]any, 12)
	out["job_id"] = e.JobID
	out["attempt_id"] = e.AttemptID
	out["lease_id"] = e.LeaseID
	out["worker_id"] = e.WorkerID
	out["queue"] = e.Queue
	out["kind"] = e.Kind
	out["priority"] = e.Priority
	out["selection_reason"] = e.SelectionReason
	out["attempt_number"] = e.AttemptNumber
	out["required_capabilities"] = append([]string{}, e.RequiredCapabilities...)
	out["worker_selector"] = e.WorkerSelector
	for key, value := range e.RouteMetadata {
		out[key] = value
	}
	return out
}

func NewJobExecutionEnvelope(job Job) JobExecutionEnvelope {
	env := JobExecutionEnvelope{
		JobID:                job.ID,
		Queue:                job.Spec.Queue,
		Kind:                 job.Spec.Kind,
		Priority:             job.Spec.Priority,
		Payload:              job.Spec.Payload,
		Labels:               cloneJobStringMap(job.Labels),
		Tags:                 append([]string{}, job.Tags...),
		RequiredCapabilities: append([]string{}, job.Spec.RequiredCapabilities...),
		WorkerSelector:       job.Spec.WorkerSelector,
		AttemptNumber:        job.AttemptCount,
		CreatedAt:            job.CreatedAt,
		AvailableAt:          job.AvailableAt,
		NextRetryAt:          job.NextRetryAt,
	}
	if job.CurrentAttempt != nil {
		env.AttemptID = job.CurrentAttempt.ID
	}
	if job.CurrentLease != nil {
		env.LeaseID = job.CurrentLease.ID
		env.WorkerID = job.CurrentLease.WorkerID
		if env.AttemptID == "" {
			env.AttemptID = job.CurrentLease.AttemptID
		}
	}
	env.RouteMetadata = map[string]any{
		"job_parent_id":     job.ParentJobID,
		"root_workflow_id":  job.RootWorkflowID,
		"trace_id":          job.TraceID,
		"idempotency_key":   job.IdempotencyKey,
		"source":            job.Source,
		"resume_checkpoint": job.ResumeCheckpointID,
	}
	return env
}

// Worker is the job-layer execution contract. Concrete workers may be agents,
// but the jobs package does not define agent taxonomy.
type Worker interface {
	Accept(ctx context.Context, envelope JobExecutionEnvelope) error
	Start(ctx context.Context, envelope JobExecutionEnvelope) error
	Heartbeat(ctx context.Context, envelope JobExecutionEnvelope) error
	Checkpoint(ctx context.Context, envelope JobExecutionEnvelope, checkpoint JobCheckpoint) error
	Complete(ctx context.Context, envelope JobExecutionEnvelope) error
	Fail(ctx context.Context, envelope JobExecutionEnvelope, failureClass, reason string) error
	Cancel(ctx context.Context, envelope JobExecutionEnvelope, reason string) error
}

// JobExecutor is a minimal adapter contract for concrete agent entry points.
// It keeps the jobs package independent from task and capability policy types.
type JobExecutor interface {
	ExecuteJob(ctx context.Context, envelope JobExecutionEnvelope) (map[string]any, error)
}

type WorkerAdapter struct {
	Executor JobExecutor
}

func (a WorkerAdapter) Execute(ctx context.Context, job Job) (map[string]any, error) {
	if a.Executor == nil {
		return nil, errors.New("job executor required")
	}
	env := NewJobExecutionEnvelope(job)
	if err := env.Validate(); err != nil {
		return nil, err
	}
	return a.Executor.ExecuteJob(ctx, env)
}

func cloneJobStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
