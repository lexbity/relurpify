package pretask

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
)

type countingPatternStore struct {
	results []patterns.PatternRecord
	calls   int
}

func (s *countingPatternStore) Save(context.Context, patterns.PatternRecord) error { return nil }
func (s *countingPatternStore) Load(context.Context, string) (*patterns.PatternRecord, error) {
	return nil, nil
}
func (s *countingPatternStore) ListByStatus(_ context.Context, _ patterns.PatternStatus, _ string) ([]patterns.PatternRecord, error) {
	s.calls++
	return append([]patterns.PatternRecord(nil), s.results...), nil
}
func (s *countingPatternStore) ListByKind(context.Context, patterns.PatternKind, string) ([]patterns.PatternRecord, error) {
	return nil, nil
}
func (s *countingPatternStore) UpdateStatus(context.Context, string, patterns.PatternStatus, string) error {
	return nil
}
func (s *countingPatternStore) Supersede(context.Context, string, patterns.PatternRecord) error {
	return nil
}

func TestLearningSyncStepRunNoOpWithoutWorkflowID(t *testing.T) {
	state := core.NewContext()
	step := LearningSyncStep{}
	if err := step.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := state.Get("euclo.pending_learning_interactions"); ok {
		t.Fatal("expected no pending learning interactions")
	}
}

func TestLearningSyncStepRunSyncsAndSeedsState(t *testing.T) {
	ctx := context.Background()
	store := newLearningWorkflowStore(t)
	requireWorkflow(t, store, "wf-learning")
	patternStore := &countingPatternStore{results: []patterns.PatternRecord{
		{
			ID:          "pattern-1",
			Kind:        patterns.PatternKindSemantic,
			Title:       "Confirm pattern",
			Description: "pattern requires review",
			Status:      patterns.PatternStatusProposed,
			CorpusScope: "workspace",
		},
	}}
	step := LearningSyncStep{
		LearningService: archaeolearning.Service{
			Store:        store,
			PatternStore: patternStore,
			Now:          func() time.Time { return testNow() },
			NewID:        func(prefix string) string { return prefix + "-1" },
		},
		WorkflowResolver: func(state *core.Context) string {
			return state.GetString("euclo.workflow_id")
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-learning")
	state.Set("euclo.active_exploration_id", "explore-1")
	state.Set("corpus_scope", "workspace")
	state.Set("euclo.code_revision", "rev-1")
	state.Set("euclo.pending_learning_interactions", []archaeolearning.Interaction{
		{
			ID:            "learn-existing",
			WorkflowID:    "wf-learning",
			ExplorationID: "explore-1",
			Kind:          archaeolearning.InteractionKnowledgeProposal,
			SubjectType:   archaeolearning.SubjectExploration,
			Title:         "Existing pending learning",
			Description:   "seeded before sync",
			Status:        archaeolearning.StatusPending,
			CreatedAt:     testNow(),
			UpdatedAt:     testNow(),
		},
	})

	if err := step.Run(ctx, state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if patternStore.calls != 1 {
		t.Fatalf("expected one pattern-store sync call, got %d", patternStore.calls)
	}
	rawPending, ok := state.Get("euclo.pending_learning_interactions")
	if !ok {
		t.Fatal("expected pending learning interactions")
	}
	pending, ok := rawPending.([]archaeolearning.Interaction)
	if !ok {
		t.Fatalf("unexpected pending type: %#v", rawPending)
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending interactions, got %#v", pending)
	}
	if got, ok := state.Get("euclo.has_blocking_learning"); !ok || got != true {
		t.Fatalf("expected blocking learning flag, got %#v", got)
	}
	rawItems, ok := state.Get("context.knowledge_items")
	if !ok {
		t.Fatal("expected context knowledge items")
	}
	items, ok := rawItems.([]any)
	if !ok {
		t.Fatalf("unexpected knowledge item type: %#v", rawItems)
	}
	foundLearning := 0
	for _, raw := range items {
		switch typed := raw.(type) {
		case ContextKnowledgeItem:
			if typed.Source == "learning_interaction" {
				foundLearning++
			}
		}
	}
	if foundLearning != 2 {
		t.Fatalf("expected two learning context items, got %d from %#v", foundLearning, items)
	}
}

func TestLearningSyncStepRunNilServiceDoesNotError(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	step := LearningSyncStep{
		LearningService: archaeolearning.Service{},
		WorkflowResolver: func(state *core.Context) string {
			return state.GetString("euclo.workflow_id")
		},
	}
	if err := step.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, ok := state.Get("euclo.has_blocking_learning"); !ok || got != false {
		t.Fatalf("expected blocking learning flag to be false, got %#v", got)
	}
	rawPending, ok := state.Get("euclo.pending_learning_interactions")
	if !ok {
		t.Fatal("expected pending learning interactions key")
	}
	pending, ok := rawPending.([]archaeolearning.Interaction)
	if !ok {
		t.Fatalf("unexpected pending type: %#v", rawPending)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending interactions, got %#v", pending)
	}
}

func TestLearningInteractionToKnowledgeItemCoversKinds(t *testing.T) {
	kinds := []archaeolearning.InteractionKind{
		archaeolearning.InteractionPatternProposal,
		archaeolearning.InteractionAnchorProposal,
		archaeolearning.InteractionTensionReview,
		archaeolearning.InteractionIntentRefinement,
		archaeolearning.InteractionKnowledgeProposal,
	}
	for _, kind := range kinds {
		item := learningInteractionToKnowledgeItem(archaeolearning.Interaction{
			Kind:        kind,
			SubjectType: archaeolearning.SubjectExploration,
			Title:       "Title",
			Description: "Description",
		})
		if item.Content == "" {
			t.Fatalf("expected content for kind %s", kind)
		}
		if item.Priority != 10 {
			t.Fatalf("expected default priority for non-blocking interaction, got %d", item.Priority)
		}
	}
	blocking := learningInteractionToKnowledgeItem(archaeolearning.Interaction{Blocking: true, Title: "Block", Description: "Desc"})
	if blocking.Priority <= 10 {
		t.Fatalf("expected elevated priority for blocking interaction, got %d", blocking.Priority)
	}
}

func newLearningWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func requireWorkflow(t *testing.T, store *memorydb.SQLiteWorkflowStateStore, workflowID string) {
	t.Helper()
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      workflowID + "-task",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "sync learning",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
}

func testNow() time.Time {
	return time.Unix(1713220800, 0).UTC()
}
