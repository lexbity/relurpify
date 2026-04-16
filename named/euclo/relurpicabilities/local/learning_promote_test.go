package local

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestLearningPromoteRoutineExecutePromotesPendingInteraction(t *testing.T) {
	store := newPromoteWorkflowStore(t)
	service := archaeolearning.Service{
		Store: store,
		NewID: func(prefix string) string { return prefix + "-1" },
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-promote")
	state.Set("euclo.active_exploration_id", "explore-promote")
	state.Set("euclo.learning_promote_input", eucloruntime.LearningPromoteInput{
		Title:       "Remember the pattern",
		Description: "Keep this for future sessions",
		Kind:        string(archaeolearning.InteractionPatternProposal),
		SubjectID:   "pattern-1",
		SubjectType: string(archaeolearning.SubjectPattern),
		Blocking:    true,
	})

	artifacts, err := (LearningPromoteRoutine{
		LearningService: service,
		WorkflowResolver: func(state *core.Context) (string, string) {
			return state.GetString("euclo.workflow_id"), state.GetString("euclo.active_exploration_id")
		},
	}).Execute(context.Background(), euclorelurpic.RoutineInput{State: state})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != euclotypes.ArtifactKindLearningPromotion {
		t.Fatalf("unexpected artifact kind: %s", artifacts[0].Kind)
	}

	raw, ok := state.Get("euclo.promoted_learning_interaction")
	if !ok {
		t.Fatal("expected promoted interaction in state")
	}
	interaction, ok := raw.(archaeolearning.Interaction)
	if !ok {
		t.Fatalf("unexpected interaction type: %T", raw)
	}
	if interaction.Status != archaeolearning.StatusPending {
		t.Fatalf("expected pending status, got %s", interaction.Status)
	}
	if interaction.Kind != archaeolearning.InteractionPatternProposal {
		t.Fatalf("unexpected interaction kind: %s", interaction.Kind)
	}
	if interaction.SubjectType != archaeolearning.SubjectPattern {
		t.Fatalf("unexpected subject type: %s", interaction.SubjectType)
	}

	records, err := service.ListByWorkflow(context.Background(), "wf-promote")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 stored interaction, got %d", len(records))
	}
	if records[0].Kind != archaeolearning.InteractionPatternProposal {
		t.Fatalf("unexpected stored interaction kind: %s", records[0].Kind)
	}
}

func TestLearningPromoteRoutineMapsKnowledgeProposalKind(t *testing.T) {
	store := newPromoteWorkflowStore(t)
	service := archaeolearning.Service{
		Store: store,
		NewID: func(prefix string) string { return prefix + "-1" },
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-promote")
	state.Set("euclo.active_exploration_id", "explore-promote")
	state.Set("euclo.learning_promote_input", eucloruntime.LearningPromoteInput{
		Title:       "Remember this",
		Description: "Keep this for future sessions",
		Kind:        string(archaeolearning.InteractionKnowledgeProposal),
	})

	_, err := (LearningPromoteRoutine{
		LearningService: service,
		WorkflowResolver: func(state *core.Context) (string, string) {
			return state.GetString("euclo.workflow_id"), state.GetString("euclo.active_exploration_id")
		},
	}).Execute(context.Background(), euclorelurpic.RoutineInput{State: state})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	records, err := service.ListByWorkflow(context.Background(), "wf-promote")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 stored interaction, got %d", len(records))
	}
	if records[0].Kind != archaeolearning.InteractionKnowledgeProposal {
		t.Fatalf("unexpected stored interaction kind: %s", records[0].Kind)
	}
	if records[0].SubjectType != archaeolearning.SubjectExploration {
		t.Fatalf("unexpected subject type: %s", records[0].SubjectType)
	}
}

func TestLearningPromoteRoutineRejectsMissingWorkflow(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.learning_promote_input", eucloruntime.LearningPromoteInput{
		Title:       "Remember this",
		Description: "Keep this for later",
		Kind:        string(archaeolearning.InteractionKnowledgeProposal),
	})

	_, err := (LearningPromoteRoutine{}).Execute(context.Background(), euclorelurpic.RoutineInput{State: state})
	if err == nil || !strings.Contains(err.Error(), "no active workflow") {
		t.Fatalf("expected no active workflow error, got %v", err)
	}
}

func TestLearningPromoteRoutinePropagatesCreateError(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-promote")
	state.Set("euclo.active_exploration_id", "explore-promote")
	state.Set("euclo.learning_promote_input", eucloruntime.LearningPromoteInput{
		Title:       "Remember this",
		Description: "Keep this for later",
		Kind:        string(archaeolearning.InteractionKnowledgeProposal),
	})

	_, err := (LearningPromoteRoutine{
		LearningService: archaeolearning.Service{},
		WorkflowResolver: func(state *core.Context) (string, string) {
			return state.GetString("euclo.workflow_id"), state.GetString("euclo.active_exploration_id")
		},
	}).Execute(context.Background(), euclorelurpic.RoutineInput{State: state})
	if err == nil || !strings.Contains(err.Error(), "workflow state store required") {
		t.Fatalf("expected service error to propagate, got %v", err)
	}
}

func TestExtractEvidenceFromStateUsesTraceAndSemanticInputs(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", eucloexec.Trace{
		PrimaryCapabilityID: "euclo:chat.inspect",
		Path:                "unit_of_work_behavior",
		ExecutorFamily:      "react",
	})
	state.Set("euclo.semantic_inputs", eucloruntime.SemanticInputBundle{
		PatternRefs:           []string{"pattern-1"},
		RequestProvenanceRefs: []string{"symbol-1"},
	})

	evidence := extractEvidenceFromState(state, eucloruntime.LearningPromoteInput{})
	if len(evidence) == 0 {
		t.Fatal("expected evidence from trace-backed state")
	}
	foundPattern := false
	foundSymbol := false
	for _, item := range evidence {
		switch item.Kind {
		case "pattern_ref":
			foundPattern = true
		case "touched_symbol":
			foundSymbol = true
		}
	}
	if !foundPattern || !foundSymbol {
		t.Fatalf("expected pattern and symbol evidence, got %#v", evidence)
	}
}

func TestExtractEvidenceFromStateEmptyStateReturnsNil(t *testing.T) {
	if got := extractEvidenceFromState(core.NewContext(), eucloruntime.LearningPromoteInput{}); got != nil {
		t.Fatalf("expected nil evidence, got %#v", got)
	}
}

func newPromoteWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-promote",
		TaskID:      "task-promote",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "promote learning",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Unix(0, 0).UTC(),
		UpdatedAt:   time.Unix(0, 0).UTC(),
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	return store
}
