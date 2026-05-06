package testsuite

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
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

func TestBackgroundJobNodeSubmitsJobAndInvokesCompletionHook(t *testing.T) {
	submitter := &recordingSubmitter{}
	node := orchestrate.NewBackgroundJobNode("background1").
		WithSubmitter(submitter).
		WithDefaultQueue("background").
		WithDefaultKind("euclo.background.build")

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.background.payload", map[string]any{"action": "build"}, contextdata.MemoryClassTask)

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
	if got := result.Data["job_started"]; got != true {
		t.Fatalf("expected job_started true, got %v", got)
	}
	if got := result.Data["job_id"]; got != "job-1" {
		t.Fatalf("unexpected job id: %v", got)
	}
	if got := result.Data["job_completed"]; got != true {
		t.Fatalf("expected job_completed true, got %v", got)
	}
	if submitter.spec.Kind != "euclo.background.build" {
		t.Fatalf("unexpected kind: %s", submitter.spec.Kind)
	}
	if submitter.spec.Queue != "background" {
		t.Fatalf("unexpected queue: %s", submitter.spec.Queue)
	}
	if _, ok := env.GetWorkingValue("euclo.background.job_submitted"); !ok {
		t.Fatal("expected submission marker in envelope")
	}
	if _, ok := env.GetWorkingValue("euclo.background.job_completed"); !ok {
		t.Fatal("expected completion marker in envelope")
	}
}

func TestBackgroundJobNodeEmitsTelemetry(t *testing.T) {
	submitter := &recordingSubmitter{}
	telemetry := &recordingTelemetry{}
	node := orchestrate.NewBackgroundJobNode("background2").
		WithSubmitter(submitter).
		WithTelemetry(reporting.NewEucloTelemetry(telemetry))

	env := contextdata.NewEnvelope("task-2", "session-2")
	env.SetWorkingValue("euclo.background.kind", "euclo.background.test", contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.payload", map[string]any{"target": "test"}, contextdata.MemoryClassTask)

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
