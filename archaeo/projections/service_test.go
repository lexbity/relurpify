package projections_test

import (
	"context"
	"testing"

	projections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestWorkflowEmptyID(t *testing.T) {
	f := testscenario.NewFixture(t)
	_, err := f.ProjectionService().Workflow(context.Background(), "")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWorkflowNilStore(t *testing.T) {
	svc := &projections.Service{}
	model, err := svc.Workflow(context.Background(), "wf-none")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if model != nil {
		t.Fatalf("expected nil model for nil store, got %+v", model)
	}
}

func TestWorkflowBasic(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-workflow-basic", "basic workflow test")

	session, snapshot := f.SeedExploration("wf-workflow-basic", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-workflow-basic",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline",
	})
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-workflow-basic",
		WorkflowID: "wf-workflow-basic",
		Title:      "Test Plan",
		Version:    1,
		Steps:      map[string]*frameworkplan.PlanStep{},
		StepOrder:  []string{},
		CreatedAt:  f.Now(),
		UpdatedAt:  f.Now(),
	}
	f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-workflow-basic",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})

	model, err := f.ProjectionService().Workflow(context.Background(), "wf-workflow-basic")
	if err != nil {
		t.Fatalf("build workflow projection: %v", err)
	}
	if model == nil {
		t.Fatal("expected workflow model")
	}
	if model.WorkflowID != "wf-workflow-basic" {
		t.Errorf("workflow id mismatch: %s", model.WorkflowID)
	}
	if model.ActiveExploration == nil || model.ActiveExploration.ID != session.ID {
		t.Errorf("expected active exploration %s, got %+v", session.ID, model.ActiveExploration)
	}
	if len(model.ExplorationSnapshots) != 1 || model.ExplorationSnapshots[0].ID != snapshot.ID {
		t.Errorf("expected snapshot %s, got %+v", snapshot.ID, model.ExplorationSnapshots)
	}
	if model.ActivePlanVersion == nil || model.ActivePlanVersion.Version != 1 {
		t.Errorf("expected active plan version 1, got %+v", model.ActivePlanVersion)
	}
}

func TestExplorationProjection(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-exploration", "exploration test")

	session, snapshot := f.SeedExploration("wf-exploration", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-exploration",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline",
	})

	proj, err := f.ProjectionService().Exploration(context.Background(), "wf-exploration")
	if err != nil {
		t.Fatalf("build exploration projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected exploration projection")
	}
	if proj.WorkflowID != "wf-exploration" {
		t.Errorf("workflow id mismatch: %s", proj.WorkflowID)
	}
	if proj.ActiveExploration == nil || proj.ActiveExploration.ID != session.ID {
		t.Errorf("expected active exploration %s, got %+v", session.ID, proj.ActiveExploration)
	}
	if len(proj.ExplorationSnapshots) != 1 || proj.ExplorationSnapshots[0].ID != snapshot.ID {
		t.Errorf("expected snapshot %s, got %+v", snapshot.ID, proj.ExplorationSnapshots)
	}
}

func TestLearningQueueBasic(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-learning", "learning queue test")

	session, _ := f.SeedExploration("wf-learning", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-learning",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline",
	})
	learning := f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:      "wf-learning",
		ExplorationID:   session.ID,
		Kind:            archaeolearning.InteractionIntentRefinement,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       session.ID,
		Title:           "Test learning",
		Blocking:        true,
		BasedOnRevision: "rev-1",
	})

	proj, err := f.ProjectionService().LearningQueue(context.Background(), "wf-learning")
	if err != nil {
		t.Fatalf("build learning queue projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected learning queue projection")
	}
	if proj.WorkflowID != "wf-learning" {
		t.Errorf("workflow id mismatch: %s", proj.WorkflowID)
	}
	if len(proj.PendingLearning) != 1 || proj.PendingLearning[0].ID != learning.ID {
		t.Errorf("expected learning %s, got %+v", learning.ID, proj.PendingLearning)
	}
	if len(proj.BlockingLearning) != 1 || proj.BlockingLearning[0] != learning.ID {
		t.Errorf("expected blocking learning ID %s, got %+v", learning.ID, proj.BlockingLearning)
	}
}
