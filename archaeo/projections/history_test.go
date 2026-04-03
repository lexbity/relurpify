package projections_test

import (
	"context"
	"testing"

	projections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestMutationHistoryEmpty(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-mutation-empty", "mutation history empty")
	proj, err := f.ProjectionService().MutationHistory(context.Background(), "wf-mutation-empty")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if proj == nil {
		t.Fatal("expected non‑nil projection even when empty")
	}
	if proj.WorkflowID != "wf-mutation-empty" {
		t.Errorf("workflow id mismatch: %s", proj.WorkflowID)
	}
	if len(proj.Mutations) != 0 {
		t.Errorf("expected zero mutations, got %d", len(proj.Mutations))
	}
}

func TestRequestHistoryBasic(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-request-history", "request history test")

	session, snapshot := f.SeedExploration("wf-request-history", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-request-history",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline",
	})
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-request-history",
		WorkflowID: "wf-request-history",
		Title:      "Test Plan",
		Version:    1,
		Steps:      map[string]*frameworkplan.PlanStep{},
		StepOrder:  []string{},
		CreatedAt:  f.Now(),
		UpdatedAt:  f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-request-history",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})

	proj, err := f.ProjectionService().RequestHistory(context.Background(), "wf-request-history")
	if err != nil {
		t.Fatalf("build request history projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected request history projection")
	}
	if proj.WorkflowID != "wf-request-history" {
		t.Errorf("workflow id mismatch: %s", proj.WorkflowID)
	}
	// No requests seeded, counts should be zero
	if proj.Pending != 0 || proj.Completed != 0 || proj.Failed != 0 {
		t.Errorf("expected zero counts, got pending=%d completed=%d failed=%d", proj.Pending, proj.Completed, proj.Failed)
	}
}

func TestPlanLineageBasic(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-lineage", "plan lineage test")

	session, snapshot := f.SeedExploration("wf-lineage", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-lineage",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline",
	})
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-lineage",
		WorkflowID: "wf-lineage",
		Title:      "Test Plan",
		Version:    1,
		Steps:      map[string]*frameworkplan.PlanStep{},
		StepOrder:  []string{},
		CreatedAt:  f.Now(),
		UpdatedAt:  f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-lineage",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})
	draft := f.SeedPlanVersion(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-lineage",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-2",
		SemanticSnapshotRef:    snapshot.ID,
	})

	proj, err := f.ProjectionService().PlanLineage(context.Background(), "wf-lineage")
	if err != nil {
		t.Fatalf("build plan lineage projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected plan lineage projection")
	}
	if proj.WorkflowID != "wf-lineage" {
		t.Errorf("workflow id mismatch: %s", proj.WorkflowID)
	}
	if proj.ActiveVersion == nil || proj.ActiveVersion.Version != active.Version {
		t.Errorf("expected active version %d, got %+v", active.Version, proj.ActiveVersion)
	}
	if len(proj.DraftVersions) != 1 || proj.DraftVersions[0].Version != draft.Version {
		t.Errorf("expected draft version %d, got %+v", draft.Version, proj.DraftVersions)
	}
}

func TestDeferredDraftsWorkspace(t *testing.T) {
	f := testscenario.NewFixture(t)
	proj, err := f.ProjectionService().DeferredDrafts(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("build deferred drafts projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected deferred drafts projection")
	}
	if proj.WorkspaceID != f.Workspace {
		t.Errorf("workspace id mismatch: expected %s, got %s", f.Workspace, proj.WorkspaceID)
	}
	// Initially zero drafts
	if proj.OpenCount != 0 || proj.FormedCount != 0 {
		t.Errorf("expected zero counts, got open=%d formed=%d", proj.OpenCount, proj.FormedCount)
	}
}

func TestDecisionTrailWorkspace(t *testing.T) {
	f := testscenario.NewFixture(t)
	proj, err := f.ProjectionService().DecisionTrail(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("build decision trail projection: %v", err)
	}
	if proj == nil {
		t.Fatal("expected decision trail projection")
	}
	if proj.WorkspaceID != f.Workspace {
		t.Errorf("workspace id mismatch: expected %s, got %s", f.Workspace, proj.WorkspaceID)
	}
	// Initially zero decisions
	if proj.OpenCount != 0 || proj.Resolved != 0 {
		t.Errorf("expected zero counts, got open=%d resolved=%d", proj.OpenCount, proj.Resolved)
	}
}
