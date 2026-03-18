package euclotest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAgentExecutePersistsArtifactsToWorkflowStore(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-persist", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-persist",
		Instruction: "summarize current status",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	workflowID := state.GetString("euclo.workflow_id")
	runID := state.GetString("euclo.run_id")
	require.NotEmpty(t, workflowID)
	require.NotEmpty(t, runID)

	artifacts, listErr := workflowStore.ListWorkflowArtifacts(ctx, workflowID, runID)
	require.NoError(t, listErr)
	require.NotEmpty(t, artifacts)
	require.Contains(t, artifacts[0].Kind, "euclo.")
}

func TestLoadPersistedArtifactsRestoresStateAndFinalReport(t *testing.T) {
	ctx := context.Background()
	writer := &workflowArtifactWriterStub{}
	artifacts := []euclotypes.Artifact{
		{ID: "euclo_intake", Kind: euclotypes.ArtifactKindIntake, Summary: "task", Payload: map[string]any{"task_id": "task-1"}},
		{ID: "euclo_mode", Kind: euclotypes.ArtifactKindModeResolution, Summary: "code", Payload: map[string]any{"mode_id": "code"}},
		{ID: "euclo_verify", Kind: euclotypes.ArtifactKindVerification, Summary: "tests passed", Payload: map[string]any{"status": "pass"}},
		{ID: "euclo_gate", Kind: euclotypes.ArtifactKindSuccessGate, Summary: "accepted", Payload: map[string]any{"allowed": true}},
	}
	require.NoError(t, euclotypes.PersistWorkflowArtifacts(ctx, writer, "wf-1", "run-1", artifacts))

	reader := &workflowArtifactReaderStub{records: writer.records}
	loaded, err := euclotypes.LoadPersistedArtifacts(ctx, reader, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, loaded, 4)

	state := core.NewContext()
	euclotypes.RestoreStateFromArtifacts(state, loaded)
	require.NotEmpty(t, state.GetString("pipeline.verify"))

	report := euclotypes.AssembleFinalReport(loaded)
	require.Equal(t, 4, report["artifacts"])
	require.NotNil(t, report["mode"])
	require.NotNil(t, report["verification"])
	require.NotNil(t, report["success_gate"])
}
