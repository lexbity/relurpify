package bkc

import (
	"context"
	"path/filepath"
	"testing"

	archaeobkc "github.com/lexcodex/relurpify/archaeo/bkc"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestCompileCapabilityQueuesCandidate(t *testing.T) {
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	if err := workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-bkc",
		TaskID:      "task-bkc",
		TaskType:    core.TaskTypePlanning,
		Instruction: "phase 7 compile test",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	graph, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(t.TempDir(), "graphdb")))
	if err != nil {
		t.Fatalf("graphdb: %v", err)
	}
	defer graph.Close()

	cap := NewCompileCapability(agentenv.AgentEnvironment{
		Model:        testutil.StubModel{},
		Registry:     capability.NewRegistry(),
		IndexManager: &ast.IndexManager{GraphDB: graph},
	})
	state := core.NewContext()
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "task-bkc-compile",
			Instruction: "Compile this semantic seam into BKC.",
			Context: map[string]any{
				"workflow_id":    "wf-bkc",
				"exploration_id": "exp-bkc",
				"workspace_id":   "ws-bkc",
			},
		},
		State:         state,
		WorkflowStore: workflowStore,
		WorkflowID:    "wf-bkc",
	})
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if _, ok := state.Get("euclo.semantic_compile"); !ok {
		t.Fatal("expected semantic compile payload in state")
	}
}

func TestStreamCapabilityStreamsChunkContext(t *testing.T) {
	graph, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(t.TempDir(), "graphdb")))
	if err != nil {
		t.Fatalf("graphdb: %v", err)
	}
	defer graph.Close()
	store := &archaeobkc.ChunkStore{Graph: graph}
	if _, err := store.Save(archaeobkc.KnowledgeChunk{
		ID:            "chunk:seed",
		WorkspaceID:   "ws-bkc",
		ContentHash:   "hash",
		TokenEstimate: 12,
		Provenance:    archaeobkc.ChunkProvenance{WorkflowID: "wf-bkc", CompiledBy: archaeobkc.CompilerDeterministic},
		Freshness:     archaeobkc.FreshnessValid,
		Body:          archaeobkc.ChunkBody{Raw: "seed context", Fields: map[string]any{"file_path": "service.go"}},
	}); err != nil {
		t.Fatalf("save chunk: %v", err)
	}

	cap := NewStreamCapability(agentenv.AgentEnvironment{
		Model:        testutil.StubModel{},
		Registry:     capability.NewRegistry(),
		IndexManager: &ast.IndexManager{GraphDB: graph},
	})
	state := core.NewContext()
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "task-bkc-stream",
			Instruction: "Stream semantic context.",
			Context: map[string]any{
				"files": []string{"service.go"},
			},
		},
		State: state,
	})
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if _, ok := state.Get("euclo.semantic_context"); !ok {
		t.Fatal("expected semantic context payload in state")
	}
}
