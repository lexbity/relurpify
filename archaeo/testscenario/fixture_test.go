package testscenario

import (
	"testing"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestNewFixtureSeedsStoresAndServices(t *testing.T) {
	start := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	f := NewFixture(t, WithClock(fixedSequenceClock(start)))

	f.SeedWorkflow("wf-1", "test workflow")
	_, snapshot := f.SeedExploration("wf-1", "", "rev-1", SnapshotInput("wf-1", "rev-1"))
	if snapshot == nil {
		t.Fatal("expected exploration snapshot")
	}

	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		Title:      "Investigate",
		Steps:      map[string]*frameworkplan.PlanStep{},
		CreatedAt:  f.Now(),
		UpdatedAt:  f.Now(),
	}
	active := f.SeedActivePlan(plan, DraftInput("wf-1", "rev-1"))
	if active == nil || active.Version != 1 {
		t.Fatalf("expected active version 1, got %#v", active)
	}
}

func TestFixtureSeedingAndAssertions(t *testing.T) {
	f := NewFixture(t)
	f.SeedWorkflow("wf-1", "test workflow")

	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:  "wf-1",
		SourceRef:   "gap-1",
		Kind:        "intent_gap",
		Description: "Boundary mismatch",
		Status:      archaeodomain.TensionUnresolved,
	})
	f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		Kind:          archaeolearning.InteractionTensionReview,
		SubjectType:   archaeolearning.SubjectTension,
		SubjectID:     tension.ID,
		Title:         "Review tension",
		Blocking:      true,
	})

	RequireActiveTensionIDs(t, f.TensionService(), "wf-1", tension.ID)
	pending, err := f.LearningService().Pending(f.Context(), "wf-1")
	if err != nil {
		t.Fatalf("pending learning: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending interaction, got %d", len(pending))
	}
}

func SnapshotInput(workflowID, rev string) archaeoarch.SnapshotInput {
	return archaeoarch.SnapshotInput{
		WorkflowID:      workflowID,
		BasedOnRevision: rev,
		WorkspaceID:     "",
		Summary:         "prepared snapshot",
	}
}

func DraftInput(workflowID, rev string) archaeoplans.DraftVersionInput {
	return archaeoplans.DraftVersionInput{
		WorkflowID:      workflowID,
		BasedOnRevision: rev,
	}
}
