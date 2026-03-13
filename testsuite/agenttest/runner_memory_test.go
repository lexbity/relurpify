package agenttest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
)

func TestResolveMemorySpecDefaultsToHybrid(t *testing.T) {
	spec := resolveMemorySpec(nil, CaseSpec{})
	if spec.Backend != "hybrid" {
		t.Fatalf("expected hybrid backend, got %q", spec.Backend)
	}
}

func TestResolveMemorySpecAppliesCaseOverride(t *testing.T) {
	suite := &Suite{Spec: SuiteSpec{Memory: MemorySpec{Backend: "hybrid"}}}
	spec := resolveMemorySpec(suite, CaseSpec{
		Overrides: CaseOverrideSpec{
			Memory: &MemorySpec{Backend: "sqlite_runtime"},
		},
	})
	if spec.Backend != "sqlite_runtime" {
		t.Fatalf("expected sqlite_runtime backend, got %q", spec.Backend)
	}
}

func TestPrepareCaseMemoryBuildsSQLiteRuntimeStore(t *testing.T) {
	workspace := t.TempDir()
	suite := &Suite{Spec: SuiteSpec{Memory: MemorySpec{Backend: "sqlite_runtime"}}}

	prepared, err := prepareCaseMemory(workspace, suite, CaseSpec{}, nil)
	if err != nil {
		t.Fatalf("prepareCaseMemory: %v", err)
	}
	defer prepared.Close()

	if prepared.Backend != "sqlite_runtime" {
		t.Fatalf("expected sqlite_runtime backend, got %q", prepared.Backend)
	}
	if _, ok := prepared.Store.(*memorydb.SQLiteRuntimeMemoryStore); !ok {
		t.Fatalf("expected SQLiteRuntimeMemoryStore, got %T", prepared.Store)
	}
}

func TestSeedCaseStateWritesRuntimeMemoryAndWorkflowStore(t *testing.T) {
	workspace := t.TempDir()
	suite := &Suite{Spec: SuiteSpec{Memory: MemorySpec{Backend: "sqlite_runtime"}}}
	prepared, err := prepareCaseMemory(workspace, suite, CaseSpec{}, nil)
	if err != nil {
		t.Fatalf("prepareCaseMemory: %v", err)
	}
	defer prepared.Close()

	setup := SetupSpec{
		Memory: MemorySeedSpec{
			Declarative: []DeclarativeMemorySeedSpec{{
				RecordID: "fact-1",
				Scope:    string(memory.MemoryScopeProject),
				Kind:     string(memory.DeclarativeMemoryKindProjectKnowledge),
				Summary:  "retrieval-backed memory",
				Content:  "runtime store mirrors declarative memory into retrieval",
			}},
		},
		Workflows: []WorkflowSeedSpec{{
			Workflow: WorkflowRecordSeedSpec{
				WorkflowID:  "wf-1",
				Instruction: "retrieval workflow",
			},
			Runs: []WorkflowRunSeedSpec{{
				RunID: "run-1",
			}},
			Knowledge: []WorkflowKnowledgeSeedSpec{{
				RecordID: "knowledge-1",
				Kind:     string(memory.KnowledgeKindFact),
				Content:  "workflow retrieval seeded",
			}},
		}},
	}

	if err := seedCaseState(context.Background(), workspace, prepared.Store, setup); err != nil {
		t.Fatalf("seedCaseState: %v", err)
	}

	runtimeStore := prepared.Store.(*memorydb.SQLiteRuntimeMemoryStore)
	decl, err := runtimeStore.SearchDeclarative(context.Background(), memory.DeclarativeMemoryQuery{
		Query: "retrieval-backed",
		Scope: memory.MemoryScopeProject,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchDeclarative: %v", err)
	}
	if len(decl) != 1 || decl[0].RecordID != "fact-1" {
		t.Fatalf("unexpected declarative results: %#v", decl)
	}

	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(config.New(workspace).WorkflowStateFile())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	defer workflowStore.Close()

	if _, ok, err := workflowStore.GetWorkflow(context.Background(), "wf-1"); err != nil || !ok {
		t.Fatalf("expected seeded workflow, ok=%v err=%v", ok, err)
	}
	knowledge, err := workflowStore.ListKnowledge(context.Background(), "wf-1", "", false)
	if err != nil {
		t.Fatalf("ListKnowledge: %v", err)
	}
	if len(knowledge) != 1 || knowledge[0].RecordID != "knowledge-1" {
		t.Fatalf("unexpected knowledge: %#v", knowledge)
	}
}

func TestSeedCaseStateWritesWorkflowCheckpoints(t *testing.T) {
	workspace := t.TempDir()
	suite := &Suite{Spec: SuiteSpec{Memory: MemorySpec{Backend: "sqlite_runtime"}}}
	prepared, err := prepareCaseMemory(workspace, suite, CaseSpec{}, nil)
	if err != nil {
		t.Fatalf("prepareCaseMemory: %v", err)
	}
	defer prepared.Close()

	setup := SetupSpec{
		Workflows: []WorkflowSeedSpec{{
			Workflow: WorkflowRecordSeedSpec{
				WorkflowID: "wf-checkpoint",
			},
			Runs: []WorkflowRunSeedSpec{{
				RunID: "run-checkpoint",
			}},
			Checkpoints: []WorkflowCheckpointSeedSpec{{
				CheckpointID: "cp-1",
				TaskID:       "task-1",
				StageName:    "explain.explore",
				StageIndex:   0,
				ContextState: map[string]any{
					"plan.completed_steps": []string{"explain.explore"},
				},
			}},
		}},
	}

	if err := seedCaseState(context.Background(), workspace, prepared.Store, setup); err != nil {
		t.Fatalf("seedCaseState: %v", err)
	}

	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(config.New(workspace).WorkflowStateFile())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	defer workflowStore.Close()

	record, ok, err := workflowStore.LoadPipelineCheckpoint(context.Background(), "task-1", "cp-1")
	if err != nil {
		t.Fatalf("LoadPipelineCheckpoint: %v", err)
	}
	if !ok || record == nil {
		t.Fatal("expected seeded checkpoint")
	}
	if record.StageName != "explain.explore" {
		t.Fatalf("unexpected checkpoint: %#v", record)
	}
}

func TestEvaluateExpectationsChecksStateKeys(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"pipeline.workflow_retrieval": map[string]any{"summary": "seeded"},
		},
	}

	if err := evaluateExpectations(ExpectSpec{
		StateKeysMustExist: []string{"pipeline.workflow_retrieval"},
	}, t.TempDir(), "", nil, nil, nil, TokenUsageReport{}, MemoryOutcomeReport{}, snapshot); err != nil {
		t.Fatalf("evaluateExpectations: %v", err)
	}
}

func TestEvaluateExpectationsChecksMemoryAndWorkflowOutcome(t *testing.T) {
	err := evaluateExpectations(ExpectSpec{
		MemoryRecordsCreated: 2,
		WorkflowStateUpdated: true,
	}, t.TempDir(), "", nil, nil, nil, TokenUsageReport{}, MemoryOutcomeReport{
		MemoryRecordsCreated: 2,
		WorkflowStateUpdated: true,
	}, &core.ContextSnapshot{})
	if err != nil {
		t.Fatalf("evaluateExpectations: %v", err)
	}
}

func TestEvaluateExpectationsChecksFileContentAndTelemetry(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "out.txt"), []byte("hello final world"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	events := []core.Event{
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "file_read"}},
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "file_write"}},
		{Type: core.EventLLMResponse},
		{Type: core.EventLLMResponse},
		{Type: core.EventLLMResponse},
	}

	err := evaluateExpectations(ExpectSpec{
		FilesContain: []FileContentExpectation{{
			Path:     "out.txt",
			Contains: []string{"final"},
		}},
		ToolCallsInOrder: []string{"file_read", "file_write"},
		LLMCalls:         3,
	}, workspace, "", nil, map[string]int{"file_read": 1, "file_write": 1}, events, TokenUsageReport{}, MemoryOutcomeReport{}, &core.ContextSnapshot{})
	if err != nil {
		t.Fatalf("evaluateExpectations: %v", err)
	}
}
