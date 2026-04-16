package pretask

import (
	"context"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
)

type fakeLearningDeltaService struct {
	interactions []archaeolearning.Interaction
	calls        int
}

func (f *fakeLearningDeltaService) ListByWorkflow(context.Context, string) ([]archaeolearning.Interaction, error) {
	f.calls++
	return append([]archaeolearning.Interaction(nil), f.interactions...), nil
}

func TestLearningDeltaStepRunNoOpWithoutWorkflowID(t *testing.T) {
	state := core.NewContext()
	step := LearningDeltaStep{
		LearningService: &fakeLearningDeltaService{},
	}

	if err := step.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := state.Get("euclo.learning_delta"); ok {
		t.Fatal("expected no learning delta state")
	}
}

func TestLearningDeltaStepRunCountsResolvedInteractionsSinceLastSession(t *testing.T) {
	since := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	service := &fakeLearningDeltaService{
		interactions: []archaeolearning.Interaction{
			{
				ID:          "learn-1",
				WorkflowID:  "wf-1",
				Kind:        archaeolearning.InteractionKnowledgeProposal,
				SubjectType: archaeolearning.SubjectPattern,
				SubjectID:   "pattern-1",
				Status:      archaeolearning.StatusResolved,
				Resolution:  &archaeolearning.Resolution{Kind: archaeolearning.ResolutionConfirm},
				UpdatedAt:   since.Add(time.Minute),
			},
			{
				ID:          "learn-2",
				WorkflowID:  "wf-1",
				Kind:        archaeolearning.InteractionKnowledgeProposal,
				SubjectType: archaeolearning.SubjectTension,
				SubjectID:   "tension-1",
				Status:      archaeolearning.StatusResolved,
				Resolution:  &archaeolearning.Resolution{Kind: archaeolearning.ResolutionConfirm},
				UpdatedAt:   since.Add(2 * time.Minute),
			},
			{
				ID:          "learn-3",
				WorkflowID:  "wf-1",
				Kind:        archaeolearning.InteractionKnowledgeProposal,
				SubjectType: archaeolearning.SubjectAnchor,
				SubjectID:   "anchor-1",
				Status:      archaeolearning.StatusResolved,
				Resolution:  &archaeolearning.Resolution{Kind: archaeolearning.ResolutionRefine},
				UpdatedAt:   since.Add(-time.Minute),
			},
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	state.Set("euclo.last_session_time", since)
	step := LearningDeltaStep{
		LearningService: service,
		WorkflowResolver: func(state *core.Context) string {
			return state.GetString("euclo.workflow_id")
		},
		SessionResolver: func(state *core.Context) string {
			return "rev-2"
		},
	}

	if err := step.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if service.calls != 1 {
		t.Fatalf("expected one learning service call, got %d", service.calls)
	}
	raw, ok := state.Get("euclo.learning_delta")
	if !ok {
		t.Fatal("expected learning delta state")
	}
	delta, ok := raw.(LearningDeltaSummary)
	if !ok {
		t.Fatalf("unexpected delta type: %T", raw)
	}
	if delta.TotalResolved != 2 {
		t.Fatalf("TotalResolved = %d, want 2", delta.TotalResolved)
	}
	if delta.ByKind["pattern_confirmed"] != 1 {
		t.Fatalf("pattern_confirmed = %d, want 1", delta.ByKind["pattern_confirmed"])
	}
	if delta.ByKind["tension_resolved"] != 1 {
		t.Fatalf("tension_resolved = %d, want 1", delta.ByKind["tension_resolved"])
	}
	if len(delta.ConfirmedPatterns) != 1 || delta.ConfirmedPatterns[0] != "pattern-1" {
		t.Fatalf("ConfirmedPatterns = %#v, want [pattern-1]", delta.ConfirmedPatterns)
	}
	if len(delta.ResolvedTensions) != 1 || delta.ResolvedTensions[0] != "tension-1" {
		t.Fatalf("ResolvedTensions = %#v, want [tension-1]", delta.ResolvedTensions)
	}
	if delta.SinceSummary != "Since your last session: 1 pattern confirmed, 1 tension resolved." {
		t.Fatalf("SinceSummary = %q", delta.SinceSummary)
	}
	if got := state.GetString("euclo.last_session_revision"); got != "rev-2" {
		t.Fatalf("last_session_revision = %q, want rev-2", got)
	}

	rawItems, ok := state.Get("context.knowledge_items")
	if !ok {
		t.Fatal("expected knowledge items")
	}
	items, ok := rawItems.([]any)
	if !ok {
		t.Fatalf("unexpected knowledge items type: %T", rawItems)
	}
	found := false
	for _, item := range items {
		switch typed := item.(type) {
		case ContextKnowledgeItem:
			if typed.Source == "learning_delta" {
				found = true
				if typed.Content != delta.SinceSummary {
					t.Fatalf("knowledge item content = %q, want %q", typed.Content, delta.SinceSummary)
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected learning_delta knowledge item in %#v", items)
	}
}

func TestLearningDeltaStepRunZeroSinceCountsAllResolved(t *testing.T) {
	service := &fakeLearningDeltaService{
		interactions: []archaeolearning.Interaction{
			{
				ID:          "learn-1",
				WorkflowID:  "wf-1",
				Kind:        archaeolearning.InteractionKnowledgeProposal,
				SubjectType: archaeolearning.SubjectPattern,
				SubjectID:   "pattern-1",
				Status:      archaeolearning.StatusResolved,
				Resolution:  &archaeolearning.Resolution{Kind: archaeolearning.ResolutionReject},
				UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				ID:          "learn-2",
				WorkflowID:  "wf-1",
				Kind:        archaeolearning.InteractionKnowledgeProposal,
				SubjectType: archaeolearning.SubjectTension,
				SubjectID:   "tension-1",
				Status:      archaeolearning.StatusResolved,
				Resolution:  &archaeolearning.Resolution{Kind: archaeolearning.ResolutionConfirm},
				UpdatedAt:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	step := LearningDeltaStep{
		LearningService: service,
		WorkflowResolver: func(state *core.Context) string {
			return state.GetString("euclo.workflow_id")
		},
	}

	if err := step.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, ok := state.Get("euclo.learning_delta")
	if !ok {
		t.Fatal("expected learning delta state")
	}
	delta, ok := raw.(LearningDeltaSummary)
	if !ok {
		t.Fatalf("unexpected delta type: %T", raw)
	}
	if delta.TotalResolved != 2 {
		t.Fatalf("TotalResolved = %d, want 2", delta.TotalResolved)
	}
	if delta.SinceSummary != "Since your last session: 1 pattern rejected, 1 tension resolved." {
		t.Fatalf("SinceSummary = %q", delta.SinceSummary)
	}
}

func TestLearningDeltaStepRunNoStateWriteWhenNoResolvedSinceSession(t *testing.T) {
	service := &fakeLearningDeltaService{
		interactions: []archaeolearning.Interaction{
			{
				ID:          "learn-1",
				WorkflowID:  "wf-1",
				Kind:        archaeolearning.InteractionKnowledgeProposal,
				SubjectType: archaeolearning.SubjectPattern,
				SubjectID:   "pattern-1",
				Status:      archaeolearning.StatusResolved,
				Resolution:  &archaeolearning.Resolution{Kind: archaeolearning.ResolutionConfirm},
				UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	state.Set("euclo.last_session_time", time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))
	step := LearningDeltaStep{
		LearningService: service,
		WorkflowResolver: func(state *core.Context) string {
			return state.GetString("euclo.workflow_id")
		},
	}

	if err := step.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if service.calls != 1 {
		t.Fatalf("expected one learning service call, got %d", service.calls)
	}
	if _, ok := state.Get("euclo.learning_delta"); ok {
		t.Fatal("expected no learning delta state when nothing resolved since last session")
	}
	if raw, ok := state.Get("context.knowledge_items"); ok && raw != nil {
		if items, ok := raw.([]any); ok && len(items) != 0 {
			t.Fatalf("expected no knowledge items, got %#v", items)
		}
	}
}

func TestSummarizeLearningDeltaLine(t *testing.T) {
	got := summarizeLearningDeltaLine(LearningDeltaSummary{
		TotalResolved: 3,
		ByKind: map[string]int{
			"pattern_confirmed": 2,
			"tension_resolved":  1,
		},
	})
	want := "Since your last session: 2 patterns confirmed, 1 tension resolved."
	if got != want {
		t.Fatalf("summarizeLearningDeltaLine() = %q, want %q", got, want)
	}

	if empty := summarizeLearningDeltaLine(LearningDeltaSummary{}); empty != "" {
		t.Fatalf("summarizeLearningDeltaLine(empty) = %q, want empty", empty)
	}
}
