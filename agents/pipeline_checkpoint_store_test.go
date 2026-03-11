package agents

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

func TestSQLitePipelineCheckpointStoreRoundTripsCheckpoint(t *testing.T) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	requireNoError(t, err)
	defer store.Close()

	adapter := NewSQLitePipelineCheckpointStore(store, "wf-1", "run-1")
	ctx := core.NewContext()
	ctx.Set("stage.output", map[string]any{"files": 1})
	checkpoint := &pipeline.Checkpoint{
		CheckpointID: "cp-1",
		TaskID:       "task-1",
		StageName:    "explore",
		StageIndex:   0,
		CreatedAt:    time.Now().UTC(),
		Context:      ctx,
		Result: pipeline.StageResult{
			StageName:       "explore",
			ContractName:    "explore-contract",
			ContractVersion: "v1",
			DecodedOutput:   map[string]any{"files": 1},
			ValidationOK:    true,
			Transition:      pipeline.StageTransition{Kind: pipeline.TransitionNext},
		},
	}

	requireNoError(t, adapter.Save(checkpoint))

	loaded, err := adapter.Load("task-1", "cp-1")
	requireNoError(t, err)
	if loaded.StageIndex != 0 || loaded.StageName != "explore" {
		t.Fatalf("unexpected checkpoint metadata: %+v", loaded)
	}
	if loaded.Context.GetString("stage.output") == "" {
		t.Fatalf("expected restored state key in checkpoint context")
	}
}

func TestSQLitePipelineCheckpointStoreMissingLoadReturnsNotFound(t *testing.T) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	requireNoError(t, err)
	defer store.Close()

	adapter := NewSQLitePipelineCheckpointStore(store, "wf-1", "run-1")
	_, err = adapter.Load("task-1", "missing")
	if err == nil || err != ErrPipelineCheckpointNotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestPipelineAgentResumesFromSQLiteCheckpoint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflow.db")
	stage1 := makePipelineStage("explore", "in", "stage1.out", map[string]any{"files": 1})
	stage1.applyFn = func(ctx *core.Context, output any) { ctx.Set("stage1.out", output) }
	stage2 := makePipelineStage("analyze", "stage1.out", "stage2.out", map[string]any{"issues": 2})
	stage2.applyFn = func(ctx *core.Context, output any) { ctx.Set("stage2.out", output) }

	first := &PipelineAgent{
		Model:             &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}, {Text: "{}"}}},
		Stages:            []pipeline.Stage{stage1, stage2},
		WorkflowStatePath: dbPath,
	}
	requireNoError(t, first.Initialize(&core.Config{Model: "test-model"}))

	state := core.NewContext()
	_, err := first.Execute(context.Background(), &core.Task{ID: "task-resume", Instruction: "checkpoint pipeline"}, state)
	requireNoError(t, err)

	store, err := db.NewSQLiteWorkflowStateStore(dbPath)
	requireNoError(t, err)
	ids, err := store.ListPipelineCheckpoints(context.Background(), "task-resume")
	requireNoError(t, err)
	_ = store.Close()
	if len(ids) == 0 {
		t.Fatal("expected persisted pipeline checkpoints")
	}

	resume := &PipelineAgent{
		Model:              &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}},
		Stages:             []pipeline.Stage{stage1, stage2},
		WorkflowStatePath:  dbPath,
		ResumeCheckpointID: ids[0],
	}
	requireNoError(t, resume.Initialize(&core.Config{Model: "test-model"}))

	resumeState := core.NewContext()
	_, err = resume.Execute(context.Background(), &core.Task{ID: "task-resume", Instruction: "resume pipeline"}, resumeState)
	requireNoError(t, err)
	if got := resumeState.GetString("stage2.out"); got == "" {
		t.Fatalf("expected resumed stage to apply downstream output")
	}
}
