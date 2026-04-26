package bkc

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
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
	store := &knowledge.ChunkStore{Graph: graph}
	if _, err := store.Save(knowledge.KnowledgeChunk{
		ID:            "chunk:seed",
		WorkspaceID:   "ws-bkc",
		ContentHash:   "hash",
		TokenEstimate: 12,
		Provenance:    knowledge.ChunkProvenance{WorkflowID: "wf-bkc", CompiledBy: knowledge.CompilerDeterministic},
		Freshness:     knowledge.FreshnessValid,
		Body:          knowledge.ChunkBody{Raw: "seed context", Fields: map[string]any{"file_path": "service.go"}},
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

func TestStreamCapabilityReportsStaleChunkGaps(t *testing.T) {
	graph, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(t.TempDir(), "graphdb")))
	if err != nil {
		t.Fatalf("graphdb: %v", err)
	}
	defer graph.Close()
	store := &knowledge.ChunkStore{Graph: graph}
	if _, err := store.Save(knowledge.KnowledgeChunk{
		ID:            "chunk:stale",
		WorkspaceID:   "ws-bkc",
		ContentHash:   "hash",
		TokenEstimate: 12,
		Provenance:    knowledge.ChunkProvenance{WorkflowID: "wf-bkc", CompiledBy: knowledge.CompilerDeterministic},
		Freshness:     knowledge.FreshnessStale,
		Body:          knowledge.ChunkBody{Raw: "stale context", Fields: map[string]any{"file_path": "service.go"}},
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
	if staleIDs, ok := state.Get("euclo.bkc.stale_chunk_ids"); !ok || staleIDs == nil {
		t.Fatal("expected stale chunk ids in state")
	}
	if messages, ok := state.Get("euclo.bkc.stale_gap_messages"); !ok || messages == nil {
		t.Fatal("expected stale gap messages in state")
	}
	if len(result.Artifacts) != 2 || result.Artifacts[1].Kind != euclotypes.ArtifactKindTension || result.Artifacts[1].Status != "gap" {
		t.Fatalf("expected gap artifact, got %+v", result.Artifacts)
	}
}
