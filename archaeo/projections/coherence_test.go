package projections_test

import (
	"context"
	"testing"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	projections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestCoherenceEmptyWorkflowID(t *testing.T) {
	f := testscenario.NewFixture(t)
	proj, err := f.ProjectionService().Coherence(context.Background(), "")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if proj != nil {
		t.Fatalf("expected nil projection for empty workflow ID, got %+v", proj)
	}
}

func TestCoherenceNilStore(t *testing.T) {
	svc := &projections.Service{}
	proj, err := svc.Coherence(context.Background(), "wf-none")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if proj != nil {
		t.Fatalf("expected nil projection for nil store, got %+v", proj)
	}
}

func TestCoherenceBasic(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-coherence-basic", "basic coherence test")

	session, snapshot := f.SeedExploration("wf-coherence-basic", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-coherence-basic",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline",
	})
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-coherence-basic",
		WorkflowID: "wf-coherence-basic",
		Title:      "Test Plan",
		Version:    1,
		Steps:      map[string]*frameworkplan.PlanStep{},
		StepOrder:  []string{},
		CreatedAt:  f.Now(),
		UpdatedAt:  f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-coherence-basic",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})
	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      "wf-coherence-basic",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		SourceRef:       "gap:basic",
		Kind:            "semantic_gap",
		Description:     "test tension",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-1",
	})
	learning := f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:      "wf-coherence-basic",
		ExplorationID:   session.ID,
		Kind:            archaeolearning.InteractionIntentRefinement,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       session.ID,
		Title:           "Test learning",
		Blocking:        false,
		BasedOnRevision: "rev-1",
	})

	proj, err := f.ProjectionService().Coherence(context.Background(), "wf-coherence-basic")
	if err != nil {
		t.Fatalf("build coherence projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected coherence projection")
	}
	if proj.WorkflowID != "wf-coherence-basic" {
		t.Errorf("workflow id mismatch: %s", proj.WorkflowID)
	}
	if proj.ActivePlanVersion == nil || proj.ActivePlanVersion.Version != active.Version {
		t.Errorf("expected active plan version %d, got %+v", active.Version, proj.ActivePlanVersion)
	}
	if len(proj.ActiveTensions) != 1 || proj.ActiveTensions[0].ID != tension.ID {
		t.Errorf("expected active tension %s, got %+v", tension.ID, proj.ActiveTensions)
	}
	if len(proj.PendingLearning) != 1 || proj.PendingLearning[0].ID != learning.ID {
		t.Errorf("expected pending learning %s, got %+v", learning.ID, proj.PendingLearning)
	}
}
