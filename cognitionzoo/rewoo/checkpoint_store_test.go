package rewoo

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	capports "codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

type recordingLifecycleRepo struct {
	artifacts []agentlifecycle.WorkflowArtifactRecord
}

func (r *recordingLifecycleRepo) CreateWorkflow(context.Context, agentlifecycle.WorkflowRecord) error {
	return nil
}

func (r *recordingLifecycleRepo) GetWorkflow(context.Context, string) (*agentlifecycle.WorkflowRecord, error) {
	var workflow *agentlifecycle.WorkflowRecord
	return workflow, nil
}

func (r *recordingLifecycleRepo) ListWorkflows(context.Context) ([]agentlifecycle.WorkflowRecord, error) {
	var workflows []agentlifecycle.WorkflowRecord
	return workflows, nil
}

func (r *recordingLifecycleRepo) CreateRun(context.Context, agentlifecycle.WorkflowRunRecord) error {
	return nil
}

func (r *recordingLifecycleRepo) GetRun(context.Context, string) (*agentlifecycle.WorkflowRunRecord, error) {
	var run *agentlifecycle.WorkflowRunRecord
	return run, nil
}

func (r *recordingLifecycleRepo) ListRuns(context.Context, string) ([]agentlifecycle.WorkflowRunRecord, error) {
	var runs []agentlifecycle.WorkflowRunRecord
	return runs, nil
}

func (r *recordingLifecycleRepo) UpdateRunStatus(context.Context, string, string) error {
	return nil
}

func (r *recordingLifecycleRepo) UpsertDelegation(context.Context, agentlifecycle.DelegationEntry) error {
	return nil
}

func (r *recordingLifecycleRepo) GetDelegation(context.Context, string) (*agentlifecycle.DelegationEntry, error) {
	var delegation *agentlifecycle.DelegationEntry
	return delegation, nil
}

func (r *recordingLifecycleRepo) ListDelegations(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	var delegations []agentlifecycle.DelegationEntry
	return delegations, nil
}

func (r *recordingLifecycleRepo) ListDelegationsByRun(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	var delegations []agentlifecycle.DelegationEntry
	return delegations, nil
}

func (r *recordingLifecycleRepo) AppendDelegationTransition(context.Context, agentlifecycle.DelegationTransitionEntry) error {
	return nil
}

func (r *recordingLifecycleRepo) ListDelegationTransitions(context.Context, string) ([]agentlifecycle.DelegationTransitionEntry, error) {
	var transitions []agentlifecycle.DelegationTransitionEntry
	return transitions, nil
}

func (r *recordingLifecycleRepo) AppendEvent(context.Context, agentlifecycle.WorkflowEventRecord) error {
	return nil
}

func (r *recordingLifecycleRepo) ListEvents(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	var events []agentlifecycle.WorkflowEventRecord
	return events, nil
}

func (r *recordingLifecycleRepo) ListEventsByRun(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	var events []agentlifecycle.WorkflowEventRecord
	return events, nil
}

func (r *recordingLifecycleRepo) UpsertArtifact(_ context.Context, artifact agentlifecycle.WorkflowArtifactRecord) error {
	r.artifacts = append(r.artifacts, artifact)
	return nil
}

func (r *recordingLifecycleRepo) GetArtifact(context.Context, string) (*agentlifecycle.WorkflowArtifactRecord, error) {
	var artifact *agentlifecycle.WorkflowArtifactRecord
	return artifact, nil
}

func (r *recordingLifecycleRepo) ListArtifacts(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	return append([]agentlifecycle.WorkflowArtifactRecord(nil), r.artifacts...), nil
}

func (r *recordingLifecycleRepo) ListArtifactsByRun(_ context.Context, runID string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	out := make([]agentlifecycle.WorkflowArtifactRecord, 0, len(r.artifacts))
	for _, artifact := range r.artifacts {
		if artifact.RunID == runID {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func (r *recordingLifecycleRepo) UpsertLineageBinding(context.Context, agentlifecycle.LineageBindingRecord) error {
	return nil
}

func (r *recordingLifecycleRepo) GetLineageBinding(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	var binding *agentlifecycle.LineageBindingRecord
	return binding, nil
}

func (r *recordingLifecycleRepo) FindLineageBindingByWorkflow(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	var bindings []agentlifecycle.LineageBindingRecord
	return bindings, nil
}

func (r *recordingLifecycleRepo) FindLineageBindingByRun(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	var bindings []agentlifecycle.LineageBindingRecord
	return bindings, nil
}

func (r *recordingLifecycleRepo) FindLineageBindingByLineageID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	var binding *agentlifecycle.LineageBindingRecord
	return binding, nil
}

func (r *recordingLifecycleRepo) FindLineageBindingByAttemptID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	var binding *agentlifecycle.LineageBindingRecord
	return binding, nil
}

func (r *recordingLifecycleRepo) Close() error { return nil }

type captureSessionCapability struct {
	seenEnv *contextdata.Envelope
}

var _ handler.InvocableCapabilityHandler = (*captureSessionCapability)(nil)

func (h *captureSessionCapability) Descriptor(context.Context, capports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{ID: "rewoo:test.capture", Availability: descriptor.AvailabilitySpec{Available: true}}
}

func (h *captureSessionCapability) Invoke(_ context.Context, env capports.State, _ map[string]any) (*capports.ToolResult, error) {
	h.seenEnv = contextdata.EnvelopeFromState(env)
	return &capports.ToolResult{
		Success: true,
		Data:    map[string]any{"seen": h.seenEnv != nil},
	}, nil
}

func TestExecutePlanPassesEnvelopeStateToSessionCapability(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	reg := capability.NewRegistry()
	handler := &captureSessionCapability{}
	if err := capability.RegisterSessionCapability(env.State(), "rewoo:test.capture", handler); err != nil {
		t.Fatalf("RegisterSessionCapability: %v", err)
	}

	plan := &RewooPlan{
		Steps: []RewooStep{{
			ID:   "step-1",
			Tool: "rewoo:test.capture",
			Params: map[string]any{
				"hello": "world",
			},
		}},
	}

	results, err := ExecutePlan(context.Background(), reg, plan, env, RewooOptions{})
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if got := len(results); got != 1 {
		t.Fatalf("result count = %d, want 1", got)
	}
	if !results[0].Success {
		t.Fatalf("unexpected failure result: %+v", results[0])
	}
	if handler.seenEnv != env {
		t.Fatalf("session handler saw env %#v, want %#v", handler.seenEnv, env)
	}
}

func TestCheckpointArtifactsReturnReferences(t *testing.T) {
	repo := &recordingLifecycleRepo{}
	store := NewRewooCheckpointStore(repo, nil)
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("rewoo.workflow_id", "workflow-1", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.run_id", "run-1", contextdata.MemoryClassTask)

	plan := &RewooPlan{
		Goal: "demo",
		Steps: []RewooStep{{
			ID:   "step-1",
			Tool: "tool:one",
		}},
	}
	results := []RewooStepResult{{
		StepID:  "step-1",
		Tool:    "tool:one",
		Success: true,
	}}
	env.SetWorkingValueWithClass("rewoo.plan", plan, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.tool_results", results, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.synthesis", "done", contextdata.MemoryClassTask)

	planRef := store.persistPlanArtifact(context.Background(), "checkpoint-1", "workflow-1", "run-1", plan)
	if planRef == nil {
		t.Fatal("persistPlanArtifact returned nil")
	}
	if planRef.ArtifactID != "checkpoint-1.plan" {
		t.Fatalf("plan artifact id = %q, want %q", planRef.ArtifactID, "checkpoint-1.plan")
	}

	resultsRef := store.persistToolResultsArtifact(context.Background(), "checkpoint-1", "workflow-1", "run-1", results)
	if resultsRef == nil {
		t.Fatal("persistToolResultsArtifact returned nil")
	}
	if resultsRef.ArtifactID != "checkpoint-1.tool_results" {
		t.Fatalf("tool results artifact id = %q, want %q", resultsRef.ArtifactID, "checkpoint-1.tool_results")
	}

	synthesisRef := store.persistSynthesisArtifact(context.Background(), "checkpoint-1", "workflow-1", "run-1", "done", results)
	if synthesisRef == nil {
		t.Fatal("persistSynthesisArtifact returned nil")
	}
	if synthesisRef.ArtifactID != "checkpoint-1.synthesis" {
		t.Fatalf("synthesis artifact id = %q, want %q", synthesisRef.ArtifactID, "checkpoint-1.synthesis")
	}

	store.ensureCheckpointArtifactRefs(context.Background(), "checkpoint-1", env)
	if got, ok := contextdata.GetTyped[relurpctx.ArtifactReference](env, "rewoo.plan_ref"); !ok || got.ArtifactID != "checkpoint-1.plan" {
		t.Fatalf("plan ref = %#v ok=%v", got, ok)
	}
	if got, ok := contextdata.GetTyped[relurpctx.ArtifactReference](env, "rewoo.tool_results_ref"); !ok || got.ArtifactID != "checkpoint-1.tool_results" {
		t.Fatalf("tool results ref = %#v ok=%v", got, ok)
	}
	if got, ok := contextdata.GetTyped[relurpctx.ArtifactReference](env, "rewoo.synthesis_ref"); !ok || got.ArtifactID != "checkpoint-1.synthesis" {
		t.Fatalf("synthesis ref = %#v ok=%v", got, ok)
	}
	if got := len(repo.artifacts); got != 6 {
		t.Fatalf("artifact writes = %d, want 6", got)
	}
}
