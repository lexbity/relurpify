package agentgraph

import (
	"context"
	"errors"
	"testing"
	"time"

	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type checkpointRepoStub struct {
	artifact agentlifecycle.WorkflowArtifactRecord
}

func (r *checkpointRepoStub) CreateWorkflow(context.Context, agentlifecycle.WorkflowRecord) error {
	return nil
}
func (r *checkpointRepoStub) GetWorkflow(context.Context, string) (*agentlifecycle.WorkflowRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) ListWorkflows(context.Context) ([]agentlifecycle.WorkflowRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) CreateRun(context.Context, agentlifecycle.WorkflowRunRecord) error {
	return nil
}
func (r *checkpointRepoStub) GetRun(context.Context, string) (*agentlifecycle.WorkflowRunRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) ListRuns(context.Context, string) ([]agentlifecycle.WorkflowRunRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) UpdateRunStatus(context.Context, string, string) error { return nil }
func (r *checkpointRepoStub) UpsertDelegation(context.Context, agentlifecycle.DelegationEntry) error {
	return nil
}
func (r *checkpointRepoStub) GetDelegation(context.Context, string) (*agentlifecycle.DelegationEntry, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) ListDelegations(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListDelegationsByRun(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) AppendDelegationTransition(context.Context, agentlifecycle.DelegationTransitionEntry) error {
	return nil
}
func (r *checkpointRepoStub) ListDelegationTransitions(context.Context, string) ([]agentlifecycle.DelegationTransitionEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) AppendEvent(context.Context, agentlifecycle.WorkflowEventRecord) error {
	return nil
}
func (r *checkpointRepoStub) ListEvents(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListEventsByRun(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) UpsertArtifact(_ context.Context, artifact agentlifecycle.WorkflowArtifactRecord) error {
	r.artifact = artifact
	return nil
}
func (r *checkpointRepoStub) GetArtifact(context.Context, string) (*agentlifecycle.WorkflowArtifactRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) ListArtifacts(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListArtifactsByRun(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) UpsertLineageBinding(context.Context, agentlifecycle.LineageBindingRecord) error {
	return nil
}
func (r *checkpointRepoStub) GetLineageBinding(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) FindLineageBindingByWorkflow(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) FindLineageBindingByRun(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) FindLineageBindingByLineageID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) FindLineageBindingByAttemptID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, errors.New("not implemented")
}
func (r *checkpointRepoStub) Close() error { return nil }

func TestCheckpointNodeMaterializesCheckpointFromStreamHook(t *testing.T) {
	repo := &checkpointRepoStub{}
	node := NewCheckpointNode("checkpoint-1").WithRepository(repo)

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.RequestCheckpoint("materialize after stream", 7, true)
	env.SetWorkingValue("contextstream.result", &contextstream.Result{
		Request: contextstream.Request{
			ID:   "stream-1",
			Mode: contextstream.ModeBlocking,
		},
		Trim: contextstream.TrimMetadata{
			ShortfallTokens: 3,
		},
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful checkpoint materialization, got %#v", result)
	}
	if repo.artifact.ArtifactID == "" {
		t.Fatal("expected checkpoint artifact to be persisted")
	}
	if repo.artifact.Kind != "checkpoint" {
		t.Fatalf("unexpected artifact kind: %q", repo.artifact.Kind)
	}
	if repo.artifact.WorkflowID != "task-1" || repo.artifact.RunID != "session-1" {
		t.Fatalf("unexpected workflow/run ids: %+v", repo.artifact)
	}
	if env.CheckpointRequest != nil {
		t.Fatal("expected checkpoint request to be cleared")
	}
	if got := mustWorkingValue(t, env, "checkpoint.materialized"); got != true {
		t.Fatalf("expected checkpoint.materialized true, got %v", got)
	}
	if len(env.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint reference, got %d", len(env.References.Checkpoints))
	}
	if env.References.Checkpoints[0].CheckpointID != repo.artifact.ArtifactID {
		t.Fatalf("unexpected checkpoint reference: %+v", env.References.Checkpoints[0])
	}
}

func mustWorkingValue(t *testing.T, env *contextdata.Envelope, key string) any {
	t.Helper()
	got, ok := env.GetWorkingValue(key)
	if !ok {
		t.Fatalf("missing working value %q", key)
	}
	return got
}

func TestCheckpointNodeSkipsWhenNoRequestOrStream(t *testing.T) {
	node := NewCheckpointNode("checkpoint-2")
	env := contextdata.NewEnvelope("task-2", "session-2")

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected skipped success result, got %#v", result)
	}
	if got, _ := execution.ResultField(result.Data, "checkpoint_created"); got != false {
		t.Fatalf("expected checkpoint_created false, got %v", got)
	}
}

func TestCheckpointNodeRequiresRepositoryWhenMaterializing(t *testing.T) {
	node := NewCheckpointNode("checkpoint-3")
	env := contextdata.NewEnvelope("task-3", "session-3")
	env.RequestCheckpoint("please checkpoint", 1, false)

	_, err := node.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("expected error when repository is missing")
	}
}

func TestCheckpointNodeContract(t *testing.T) {
	node := NewCheckpointNode("checkpoint-4")
	contract := node.Contract()
	if contract.CheckpointPolicy != CheckpointPolicyPreferred {
		t.Fatalf("unexpected checkpoint policy: %q", contract.CheckpointPolicy)
	}
	if contract.SideEffectClass != SideEffectContext {
		t.Fatalf("unexpected side effect class: %q", contract.SideEffectClass)
	}
	if contract.Idempotency != IdempotencyReplaySafe {
		t.Fatalf("unexpected idempotency class: %q", contract.Idempotency)
	}
}

func TestCheckpointNodeMirrorsStreamResultToEnvelope(t *testing.T) {
	repo := &checkpointRepoStub{}
	node := NewCheckpointNode("checkpoint-5").WithRepository(repo)
	env := contextdata.NewEnvelope("task-5", "session-5")
	env.RequestCheckpoint("stream checkpoint", 1, false)
	env.SetWorkingValue("contextstream.result", &contextstream.Result{
		Request: contextstream.Request{
			ID:   "stream-2",
			Mode: contextstream.ModeBackground,
		},
		StartedAt:   time.Now().Add(-1 * time.Minute),
		CompletedAt: time.Now(),
		Trim: contextstream.TrimMetadata{
			ShortfallTokens: 0,
		},
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if got, ok := env.GetWorkingValue("checkpoint.snapshot"); !ok || got == nil {
		t.Fatal("expected checkpoint snapshot to be written")
	}
}
