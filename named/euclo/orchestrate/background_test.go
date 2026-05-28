package orchestrate

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

type recordingSubmitter struct {
	mu   sync.Mutex
	spec jobs.JobSpec
	job  *jobs.Job
}

func (r *recordingSubmitter) Submit(_ context.Context, spec jobs.JobSpec) (*jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spec = spec
	if r.job != nil {
		return r.job, nil
	}
	job := &jobs.Job{
		ID:    "job-1",
		Spec:  spec,
		State: jobs.JobStateQueued,
	}
	r.job = job
	return job, nil
}

type recordingTelemetry struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *recordingTelemetry) Emit(event core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func TestBackgroundJobNodeSubmitsJobAndInvokesCompletionHook(t *testing.T) {
	submitter := &recordingSubmitter{}
	node := NewBackgroundJobNode("background1").
		WithSubmitter(submitter).
		WithDefaultQueue("background").
		WithDefaultKind("euclo.background.build")

	env := contextdata.NewEnvelope("task-1", "session-1")
	contextdata.SetTyped(env, "euclo.background.payload", map[string]any{"action": "build"})

	var hookCalled bool
	node.WithCompletionHook(func(_ context.Context, job jobs.Job, data map[string]any) {
		hookCalled = true
		if job.ID != "job-1" {
			t.Fatalf("unexpected job passed to completion hook: %s", job.ID)
		}
		if data["job_id"] != "job-1" {
			t.Fatalf("unexpected completion data: %#v", data)
		}
	})

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if !hookCalled {
		t.Fatal("expected completion hook to be called")
	}
	if got, ok := core.ResultField(result.Data, "job_started"); !ok || got != true {
		t.Fatalf("expected job_started true, got %v", got)
	}
	if got, ok := core.ResultField(result.Data, "job_id"); !ok || got != "job-1" {
		t.Fatalf("unexpected job id: %v", got)
	}
	if got, ok := core.ResultField(result.Data, "job_completed"); !ok || got != true {
		t.Fatalf("expected job_completed true, got %v", got)
	}
	if submitter.spec.Kind != "euclo.background.build" {
		t.Fatalf("unexpected kind: %s", submitter.spec.Kind)
	}
	if submitter.spec.Queue != "background" {
		t.Fatalf("unexpected queue: %s", submitter.spec.Queue)
	}
	if !state.GetBackgroundJobSubmitted(env) {
		t.Fatal("expected submission marker in envelope")
	}
	if !state.GetBackgroundJobCompleted(env) {
		t.Fatal("expected completion marker in envelope")
	}
}

func TestBackgroundJobNodeEmitsTelemetry(t *testing.T) {
	submitter := &recordingSubmitter{}
	telemetry := &recordingTelemetry{}
	node := NewBackgroundJobNode("background2").
		WithSubmitter(submitter).
		WithTelemetry(reporting.NewEucloTelemetry(telemetry))

	env := contextdata.NewEnvelope("task-2", "session-2")
	state.SetBackgroundJobKind(env, "euclo.background.test")
	contextdata.SetTyped(env, "euclo.background.payload", map[string]any{"target": "test"})

	_, err := node.Execute(core.WithTelemetry(context.Background(), telemetry), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	if len(telemetry.events) < 2 {
		t.Fatalf("expected submit and complete telemetry events, got %d", len(telemetry.events))
	}
	if telemetry.events[0].Type != core.EventType(reporting.EventTypeJobSubmitted) {
		t.Fatalf("unexpected first event type: %s", telemetry.events[0].Type)
	}
	if telemetry.events[1].Type != core.EventType(reporting.EventTypeJobCompleted) {
		t.Fatalf("unexpected second event type: %s", telemetry.events[1].Type)
	}
}
