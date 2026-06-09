package testsuite

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type checkpointArtifactRepo struct {
	artifact agentlifecycle.WorkflowArtifactRecord
}

func (r *checkpointArtifactRepo) CreateWorkflow(context.Context, agentlifecycle.WorkflowRecord) error {
	return nil
}

func (r *checkpointArtifactRepo) GetWorkflow(context.Context, string) (*agentlifecycle.WorkflowRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) ListWorkflows(context.Context) ([]agentlifecycle.WorkflowRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) CreateRun(context.Context, agentlifecycle.WorkflowRunRecord) error {
	return nil
}

func (r *checkpointArtifactRepo) GetRun(context.Context, string) (*agentlifecycle.WorkflowRunRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) ListRuns(context.Context, string) ([]agentlifecycle.WorkflowRunRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) UpdateRunStatus(context.Context, string, string) error {
	return nil
}

func (r *checkpointArtifactRepo) UpsertDelegation(context.Context, agentlifecycle.DelegationEntry) error {
	return nil
}

func (r *checkpointArtifactRepo) GetDelegation(context.Context, string) (*agentlifecycle.DelegationEntry, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) ListDelegations(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) ListDelegationsByRun(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) AppendDelegationTransition(context.Context, agentlifecycle.DelegationTransitionEntry) error {
	return nil
}

func (r *checkpointArtifactRepo) ListDelegationTransitions(context.Context, string) ([]agentlifecycle.DelegationTransitionEntry, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) AppendEvent(context.Context, agentlifecycle.WorkflowEventRecord) error {
	return nil
}

func (r *checkpointArtifactRepo) ListEvents(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) ListEventsByRun(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) UpsertArtifact(_ context.Context, artifact agentlifecycle.WorkflowArtifactRecord) error {
	r.artifact = artifact
	return nil
}

func (r *checkpointArtifactRepo) GetArtifact(context.Context, string) (*agentlifecycle.WorkflowArtifactRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) ListArtifacts(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) ListArtifactsByRun(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	if r.artifact.ArtifactID == "" {
		return nil, nil
	}
	return []agentlifecycle.WorkflowArtifactRecord{r.artifact}, nil
}

func (r *checkpointArtifactRepo) UpsertLineageBinding(context.Context, agentlifecycle.LineageBindingRecord) error {
	return nil
}

func (r *checkpointArtifactRepo) GetLineageBinding(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) FindLineageBindingByWorkflow(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) FindLineageBindingByRun(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}

func (r *checkpointArtifactRepo) FindLineageBindingByLineageID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) FindLineageBindingByAttemptID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *checkpointArtifactRepo) Close() error { return nil }

func TestEndToEndCheckpointMaterialization(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "checkpoint.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	repo := &checkpointArtifactRepo{}
	writer := newPersistenceWriter(t)
	deps := rootGraphDeps(caps)
	deps.Checkpoints = repo
	deps.Persistence = writer
	graph, err := orchestrate.NewRootGraph(deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-checkpoint", "session-checkpoint")
	seedTask(env, "add a cache to the handler", "checkpoint.go")
	runPreIngestion(t, env, dir, []string{"checkpoint.go"})
	env.RequestCheckpoint("materialize after streaming", 9, true)
	euclostate.SetStreamResult(env, &contextstream.Result{
		Request: contextstream.Request{
			ID:   "stream-checkpoint",
			Mode: contextstream.ModeBlocking,
		},
		Trim: contextstream.TrimMetadata{
			ShortfallTokens: 2,
		},
		StartedAt:   time.Now().Add(-1 * time.Minute),
		CompletedAt: time.Now(),
	})

	rec := &recordingTelemetry{}
	if err := graph.Execute(telemetry.WithTelemetry(context.Background(), rec), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if repo.artifact.ArtifactID == "" {
		t.Fatal("expected checkpoint artifact to be persisted")
	}
	if got := mustWorkingValue(t, env, "checkpoint.materialized"); got != true {
		t.Fatalf("expected checkpoint.materialized true, got %v", got)
	}
	if got := mustStringValue(t, env, "checkpoint.id"); got != repo.artifact.ArtifactID {
		t.Fatalf("checkpoint id = %q, want %q", got, repo.artifact.ArtifactID)
	}
	if env.CheckpointRequest != nil {
		t.Fatal("expected checkpoint request to be cleared")
	}
	if len(env.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint reference, got %d", len(env.References.Checkpoints))
	}
	if got := env.References.Checkpoints[0].CheckpointID; got != repo.artifact.ArtifactID {
		t.Fatalf("checkpoint reference id = %q, want %q", got, repo.artifact.ArtifactID)
	}
	if got := len(writer.GetAuditLog()); got == 0 {
		t.Fatal("expected mirrored persistence write to record an audit entry")
	}
	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
}

func mustWorkingValue(t *testing.T, env *contextdata.Envelope, key string) any {
	t.Helper()
	value, ok := contextdata.GetTyped[any](env, key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	return value
}
