package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

// BackgroundJobNode submits work to the framework job boundary and records
// completion metadata for downstream orchestration.
type BackgroundJobNode struct {
	id             string
	submitter      jobs.Submitter
	telemetry      *reporting.EucloTelemetry
	completionHook func(context.Context, jobs.Job, map[string]any)
	defaultQueue   string
	defaultKind    string
	defaultPayload any
}

// NewBackgroundJobNode creates a new background job node.
func NewBackgroundJobNode(id string) *BackgroundJobNode {
	return &BackgroundJobNode{
		id:           id,
		defaultQueue: "background",
		defaultKind:  "euclo.background",
	}
}

// WithSubmitter sets the job submitter used for background dispatch.
func (n *BackgroundJobNode) WithSubmitter(submitter jobs.Submitter) *BackgroundJobNode {
	if n != nil {
		n.submitter = submitter
	}
	return n
}

// WithTelemetry sets the telemetry wrapper used to emit job lifecycle events.
func (n *BackgroundJobNode) WithTelemetry(t *reporting.EucloTelemetry) *BackgroundJobNode {
	if n != nil {
		n.telemetry = t
	}
	return n
}

// WithCompletionHook sets a callback that runs after submission succeeds.
func (n *BackgroundJobNode) WithCompletionHook(fn func(context.Context, jobs.Job, map[string]any)) *BackgroundJobNode {
	if n != nil {
		n.completionHook = fn
	}
	return n
}

// WithDefaultQueue sets the queue used when the envelope does not override it.
func (n *BackgroundJobNode) WithDefaultQueue(queue string) *BackgroundJobNode {
	if n != nil && strings.TrimSpace(queue) != "" {
		n.defaultQueue = strings.TrimSpace(queue)
	}
	return n
}

// WithDefaultKind sets the job kind used when the envelope does not override it.
func (n *BackgroundJobNode) WithDefaultKind(kind string) *BackgroundJobNode {
	if n != nil && strings.TrimSpace(kind) != "" {
		n.defaultKind = strings.TrimSpace(kind)
	}
	return n
}

// WithDefaultPayload sets the payload used when the envelope does not override it.
func (n *BackgroundJobNode) WithDefaultPayload(payload any) *BackgroundJobNode {
	if n != nil {
		n.defaultPayload = payload
	}
	return n
}

// ID returns the node ID.
func (n *BackgroundJobNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *BackgroundJobNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeSystem
}

// Execute submits a background job and records submission/completion metadata.
func (n *BackgroundJobNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if env == nil {
		return nil, fmt.Errorf("background job node %q missing envelope", n.id)
	}
	if n.submitter == nil {
		return nil, fmt.Errorf("background job node %q missing submitter", n.id)
	}

	spec, err := n.buildJobSpec(env)
	if err != nil {
		return nil, err
	}
	job, err := n.submitter.Submit(ctx, spec)
	if err != nil {
		return nil, err
	}

	submittedAt := time.Now().UTC()
	env.SetWorkingValue("euclo.background.job_id", job.ID, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.job_queue", spec.Queue, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.job_kind", spec.Kind, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.job_submitted", true, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.job_state", string(job.State), contextdata.MemoryClassTask)

	tel := n.telemetry
	if tel == nil {
		tel = reporting.NewEucloTelemetry(core.TelemetryFromContext(ctx))
	}
	if tel != nil {
		tel.EmitJobSubmitted(ctx, reporting.EventJobSubmitted{
			EventHeader: reporting.EventHeader{
				TaskID:     env.TaskID,
				SessionID:  env.SessionID,
				Seq:        0,
				OccurredAt: submittedAt,
			},
			JobID:         job.ID,
			RouteID:       spec.Kind,
			ExecutionMode: "background",
		})
	}

	completionData := map[string]any{
		"job_id":    job.ID,
		"queue":     spec.Queue,
		"kind":      spec.Kind,
		"state":     string(job.State),
		"submitted": true,
	}
	if n.completionHook != nil {
		n.completionHook(ctx, *job, completionData)
	}
	env.SetWorkingValue("euclo.background.job_completed", true, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.job_completion", completionData, contextdata.MemoryClassTask)

	if tel != nil {
		tel.EmitJobCompleted(ctx, reporting.EventJobCompleted{
			EventHeader: reporting.EventHeader{
				TaskID:     env.TaskID,
				SessionID:  env.SessionID,
				Seq:        1,
				OccurredAt: time.Now().UTC(),
			},
			JobID:      job.ID,
			Status:     string(job.State),
			DurationMs: 0,
		})
	}

	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"job_started":   true,
			"job_submitted": true,
			"job_id":        job.ID,
			"job_queue":     spec.Queue,
			"job_kind":      spec.Kind,
			"job_state":     string(job.State),
			"job_completed": true,
		},
	}, nil
}

func (n *BackgroundJobNode) buildJobSpec(env *contextdata.Envelope) (jobs.JobSpec, error) {
	if env == nil {
		return jobs.JobSpec{}, fmt.Errorf("background job node %q missing envelope", n.id)
	}
	if value, ok := env.GetWorkingValue("euclo.background.job_spec"); ok {
		switch spec := value.(type) {
		case jobs.JobSpec:
			if err := spec.Validate(); err != nil {
				return jobs.JobSpec{}, err
			}
			return spec, nil
		case *jobs.JobSpec:
			if spec == nil {
				break
			}
			if err := spec.Validate(); err != nil {
				return jobs.JobSpec{}, err
			}
			return *spec, nil
		}
	}

	queue := n.defaultQueue
	if value, ok := env.GetWorkingValue("euclo.background.queue"); ok {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			queue = strings.TrimSpace(s)
		}
	}
	kind := n.defaultKind
	if value, ok := env.GetWorkingValue("euclo.background.kind"); ok {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			kind = strings.TrimSpace(s)
		}
	}
	payload := n.defaultPayload
	if value, ok := env.GetWorkingValue("euclo.background.payload"); ok {
		payload = value
	}
	if payload == nil {
		if task, ok := env.GetWorkingValue("task.input"); ok {
			payload = task
		}
	}
	if payload == nil {
		payload = map[string]any{
			"task_id":    env.TaskID,
			"session_id": env.SessionID,
		}
	}

	spec := jobs.JobSpec{
		Kind:     kind,
		Payload:  payload,
		Queue:    queue,
		Priority: 0,
		WorkerSelector: jobs.WorkerSelector{
			Labels: map[string]string{"euclo.background": "true"},
		},
		RetryPolicy: jobs.RetryPolicy{
			MaxAttempts: 0,
			Backoff: jobs.BackoffPolicy{
				Strategy:   jobs.BackoffStrategyFixed,
				FixedDelay: time.Second,
			},
		},
		CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
		ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeDisabled},
		TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
		LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
	}
	if err := spec.Validate(); err != nil {
		return jobs.JobSpec{}, err
	}
	return spec, nil
}

// JobRecord tracks a background job.
type JobRecord struct {
	JobID       string
	JobType     string
	StartedAt   int64
	Status      string
	Output      string
	CompletedAt *int64
}
