package euclobindings_test

import (
	"context"
	"testing"
	"time"

	euclobindings "github.com/lexcodex/relurpify/archaeo/bindings/euclo"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestRuntimeArchaeologyServicePersistsPhaseState(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-binding-phase", "prepare archaeology state through Euclo binding")
	plan := bindingPlan("plan-binding-phase", "wf-binding-phase")
	f.SeedActivePlan(plan, draftInput("wf-binding-phase", "rev-1"))

	runtime := bindingRuntime(f)
	svc := runtime.ArchaeologyService(euclobindings.ArchaeologyConfig{
		PersistPhase: f.PersistPhaseFunc(),
		EvaluateGate: allowBindingPreflight,
	})
	state := f.NewState()
	task := f.Task("wf-binding-phase", "prepare archaeology state through Euclo binding", map[string]any{
		"based_on_revision": "rev-1",
	})

	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-binding-phase")
	if result.Err != nil {
		t.Fatalf("prepare living plan: %v", result.Err)
	}
	testscenario.RequirePhase(t, f.PhaseService(), "wf-binding-phase", archaeodomain.PhaseSurfacing)
	testscenario.RequireExplorationStatus(t, svc, state.GetString("euclo.active_exploration_id"), archaeodomain.ExplorationStatusActive)

	view, err := runtime.ActiveExploration(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("active exploration: %v", err)
	}
	if view == nil || view.Session == nil {
		t.Fatalf("expected active exploration view")
	}
}

func TestRuntimeArchaeologyServiceUsesProviderBackedRequests(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-binding-providers", "exercise provider-backed archaeology refresh")
	plan := bindingPlan("plan-binding-providers", "wf-binding-providers")
	f.SeedActivePlan(plan, draftInput("wf-binding-providers", "rev-1"))

	runtime := bindingRuntime(f)
	svc := runtime.ArchaeologyService(euclobindings.ArchaeologyConfig{
		PersistPhase: f.PersistPhaseFunc(),
		EvaluateGate: allowBindingPreflight,
	})
	state := f.NewState()
	task := f.Task("wf-binding-providers", "exercise provider-backed archaeology refresh", map[string]any{
		"based_on_revision": "rev-1",
		"symbol_scope":      "pkg/service.go",
	})

	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-binding-providers")
	if result.Err != nil {
		t.Fatalf("prepare living plan: %v", result.Err)
	}

	requests, err := runtime.RequestService().Pending(context.Background(), "wf-binding-providers")
	if err != nil {
		t.Fatalf("pending requests: %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("expected 3 pending provider requests, got %d", len(requests))
	}
	kinds := map[archaeodomain.RequestKind]bool{}
	for _, record := range requests {
		kinds[record.Kind] = true
	}
	for _, kind := range []archaeodomain.RequestKind{
		archaeodomain.RequestPatternSurfacing,
		archaeodomain.RequestTensionAnalysis,
		archaeodomain.RequestProspectiveAnalysis,
	} {
		if !kinds[kind] {
			t.Fatalf("expected pending request kind %s, got %+v", kind, kinds)
		}
	}
}

func bindingRuntime(f *testscenario.Fixture) euclobindings.Runtime {
	return euclobindings.Runtime{
		WorkflowStore: f.WorkflowStore,
		PlanStore:     f.PlanStore,
		PatternStore:  f.PatternStore,
		CommentStore:  f.CommentStore,
		Retrieval:     f.Retrieval,
		Providers:     f.ProviderBundle(),
		Now:           f.Now,
		NewID:         f.NewID,
	}
}

func bindingPlan(planID, workflowID string) *frameworkplan.LivingPlan {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return &frameworkplan.LivingPlan{
		ID:         planID,
		WorkflowID: workflowID,
		Title:      "Binding plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"inspect": {
				ID:              "inspect",
				Description:     "Inspect current implementation",
				Scope:           []string{"symbol.present"},
				ConfidenceScore: 0.9,
				Status:          frameworkplan.PlanStepPending,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		StepOrder: []string{"inspect"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func draftInput(workflowID, revision string) archaeoplans.DraftVersionInput {
	return archaeoplans.DraftVersionInput{
		WorkflowID:      workflowID,
		BasedOnRevision: revision,
	}
}

func allowBindingPreflight(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
	return archaeoexec.PreflightOutcome{}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tensions round-trip via Runtime binding
// ─────────────────────────────────────────────────────────────────────────────

// TestRuntimeTensions_CreateAndQueryByWorkflow verifies that a tension seeded
// through the fixture is queryable via the Runtime binding's TensionsByWorkflow,
// and that UpdateTensionStatus transitions persist and are visible immediately.
func TestRuntimeTensions_CreateAndQueryByWorkflow(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-tensions-rt", "tension round-trip via runtime binding")

	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:  "wf-tensions-rt",
		Kind:        "boundary_mismatch",
		Description: "Transport layer leaks domain types",
		Severity:    "significant",
		Status:      archaeodomain.TensionUnresolved,
	})
	if tension == nil {
		t.Fatal("SeedTension returned nil")
	}

	runtime := bindingRuntime(f)

	tensions, err := runtime.TensionsByWorkflow(context.Background(), "wf-tensions-rt")
	if err != nil {
		t.Fatalf("TensionsByWorkflow: %v", err)
	}
	if len(tensions) != 1 {
		t.Fatalf("expected 1 tension, got %d", len(tensions))
	}
	if tensions[0].ID != tension.ID {
		t.Fatalf("tension ID mismatch: got %q, want %q", tensions[0].ID, tension.ID)
	}
	if tensions[0].Status != archaeodomain.TensionUnresolved {
		t.Fatalf("expected unresolved, got %q", tensions[0].Status)
	}

	// Update status and verify the change is visible via the binding.
	updated, err := runtime.UpdateTensionStatus(
		context.Background(),
		"wf-tensions-rt",
		tension.ID,
		archaeodomain.TensionResolved,
		nil,
	)
	if err != nil {
		t.Fatalf("UpdateTensionStatus: %v", err)
	}
	if updated == nil || updated.Status != archaeodomain.TensionResolved {
		t.Fatalf("expected resolved status after update, got %+v", updated)
	}

	testscenario.RequireTensionStatus(t, f.TensionService(), "wf-tensions-rt", tension.ID, archaeodomain.TensionResolved)
}

// TestRuntimeTensions_SummaryReflectsSeededCount confirms that TensionSummaryByWorkflow
// counts are consistent with what was seeded through the fixture.
func TestRuntimeTensions_SummaryReflectsSeededCount(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-tension-summary", "tension summary count test")

	for i := 0; i < 3; i++ {
		f.SeedTension(archaeotensions.CreateInput{
			WorkflowID:  "wf-tension-summary",
			Kind:        "boundary_mismatch",
			SourceRef:   "src-" + string(rune('a'+i)),
			Description: "tension " + string(rune('a'+i)),
			Severity:    "minor",
			Status:      archaeodomain.TensionUnresolved,
		})
	}

	runtime := bindingRuntime(f)
	summary, err := runtime.TensionSummaryByWorkflow(context.Background(), "wf-tension-summary")
	if err != nil {
		t.Fatalf("TensionSummaryByWorkflow: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil tension summary")
	}
	if summary.Total < 3 {
		t.Fatalf("expected at least 3 tensions in summary, got total=%d", summary.Total)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Convergence records via Runtime binding
// ─────────────────────────────────────────────────────────────────────────────

// TestRuntimeConvergenceRecord_CreateAndResolve verifies the full lifecycle:
// create → status is open → resolve → status transitions to resolved.
// Uses the Runtime binding (not the service directly) to confirm that
// CreateConvergenceRecord and ResolveConvergenceRecord both persist correctly.
func TestRuntimeConvergenceRecord_CreateAndResolve(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-conv-rt", "convergence record lifecycle via runtime")

	runtime := bindingRuntime(f)

	record, err := runtime.CreateConvergenceRecord(context.Background(), archaeoconvergence.CreateInput{
		WorkspaceID: f.Workspace,
		WorkflowID:  "wf-conv-rt",
		Title:       "Evaluate transport boundary split",
		Question:    "Should we split the transport and domain layers?",
	})
	if err != nil {
		t.Fatalf("CreateConvergenceRecord: %v", err)
	}
	if record == nil {
		t.Fatal("expected non-nil convergence record")
	}
	if record.Status != archaeodomain.ConvergenceResolutionOpen {
		t.Fatalf("expected open status, got %q", record.Status)
	}

	testscenario.RequireConvergenceState(t, f.ConvergenceService(), f.Workspace, archaeodomain.ConvergenceResolutionOpen)

	resolved, err := runtime.ResolveConvergenceRecord(context.Background(), archaeoconvergence.ResolveInput{
		WorkflowID: "wf-conv-rt",
		RecordID:   record.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:      archaeodomain.ConvergenceResolutionResolved,
			ChosenOption: "split transport and domain into separate packages",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConvergenceRecord: %v", err)
	}
	if resolved == nil || resolved.Status != archaeodomain.ConvergenceResolutionResolved {
		t.Fatalf("expected resolved status, got %+v", resolved)
	}

	testscenario.RequireConvergenceState(t, f.ConvergenceService(), f.Workspace, archaeodomain.ConvergenceResolutionResolved)
}

// TestRuntimeConvergenceRecord_WithTensionRefs confirms that tension references
// provided at creation are stored and retrievable via the convergence projection.
func TestRuntimeConvergenceRecord_WithTensionRefs(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-conv-refs", "convergence with tension refs")
	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:  "wf-conv-refs",
		Kind:        "boundary_mismatch",
		Description: "domain type leak",
		Severity:    "significant",
		Status:      archaeodomain.TensionUnresolved,
	})

	runtime := bindingRuntime(f)
	record, err := runtime.CreateConvergenceRecord(context.Background(), archaeoconvergence.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         "wf-conv-refs",
		Title:              "Resolve domain type leak",
		Question:           "What is the correct boundary?",
		RelevantTensionIDs: []string{tension.ID},
	})
	if err != nil {
		t.Fatalf("CreateConvergenceRecord: %v", err)
	}
	if record == nil {
		t.Fatal("expected non-nil record")
	}

	// Validate via the projection that the tension ref is stored.
	history, err := runtime.ConvergenceHistory(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("ConvergenceHistory: %v", err)
	}
	if history == nil || len(history.History) == 0 {
		t.Fatal("expected at least one record in convergence history")
	}
	found := false
	for _, r := range history.History {
		if r.ID == record.ID {
			found = true
			for _, ref := range r.RelevantTensionIDs {
				if ref == tension.ID {
					return
				}
			}
			t.Fatalf("expected tension ref %q in record, got %+v", tension.ID, r.RelevantTensionIDs)
		}
	}
	if !found {
		t.Fatalf("created record %q not found in convergence history", record.ID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Decision records via Runtime binding
// ─────────────────────────────────────────────────────────────────────────────

// TestRuntimeDecisionRecord_CreateAndResolve verifies the decision record
// lifecycle through the Runtime binding: create (open) → resolve (resolved).
func TestRuntimeDecisionRecord_CreateAndResolve(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-decision-rt", "decision record lifecycle via runtime")

	runtime := bindingRuntime(f)

	record, err := runtime.CreateDecisionRecord(context.Background(), archaeodecisions.CreateInput{
		WorkspaceID: f.Workspace,
		WorkflowID:  "wf-decision-rt",
		Kind:        archaeodomain.DecisionKindConvergence,
		Title:       "Split transport from domain",
		Summary:     "After convergence review, agreed to extract transport package",
	})
	if err != nil {
		t.Fatalf("CreateDecisionRecord: %v", err)
	}
	if record == nil {
		t.Fatal("expected non-nil decision record")
	}
	if record.Status != archaeodomain.DecisionStatusOpen {
		t.Fatalf("expected open status, got %q", record.Status)
	}

	resolved, err := runtime.ResolveDecisionRecord(context.Background(), archaeodecisions.ResolveInput{
		WorkflowID: "wf-decision-rt",
		RecordID:   record.ID,
		Status:     archaeodomain.DecisionStatusResolved,
	})
	if err != nil {
		t.Fatalf("ResolveDecisionRecord: %v", err)
	}
	if resolved == nil || resolved.Status != archaeodomain.DecisionStatusResolved {
		t.Fatalf("expected resolved status, got %+v", resolved)
	}
}

// TestRuntimeDecisionRecord_DecisionTrailProjection confirms that a created
// decision is visible in the DecisionTrail projection immediately after creation.
func TestRuntimeDecisionRecord_DecisionTrailProjection(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-decision-trail", "decision trail projection test")

	runtime := bindingRuntime(f)
	record, err := runtime.CreateDecisionRecord(context.Background(), archaeodecisions.CreateInput{
		WorkspaceID: f.Workspace,
		WorkflowID:  "wf-decision-trail",
		Kind:        archaeodomain.DecisionKindStaleResult,
		Title:       "Stale cache result invalidated",
		Summary:     "Cache was stale; invalidated and re-queried",
	})
	if err != nil {
		t.Fatalf("CreateDecisionRecord: %v", err)
	}

	trail, err := runtime.DecisionTrail(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("DecisionTrail: %v", err)
	}
	if trail == nil {
		t.Fatal("expected non-nil decision trail")
	}

	for _, d := range trail.Records {
		if d.ID == record.ID {
			return
		}
	}
	t.Fatalf("decision %q not found in trail, got %d entries", record.ID, len(trail.Records))
}

// ─────────────────────────────────────────────────────────────────────────────
// Plan versions via Runtime binding
// ─────────────────────────────────────────────────────────────────────────────

// TestRuntimePlanVersions_ListAndCompare verifies that PlanVersions returns all
// drafted versions and ComparePlanVersions produces a non-nil diff between them.
func TestRuntimePlanVersions_ListAndCompare(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-plan-versions", "plan versions list and compare")

	plan := bindingPlan("plan-versions-1", "wf-plan-versions")
	// Seed two versions via the fixture.
	f.SeedActivePlan(plan, draftInput("wf-plan-versions", "rev-1"))

	// Modify the plan and seed a second version.
	plan2 := bindingPlan("plan-versions-1", "wf-plan-versions")
	plan2.Title = "Binding plan v2"
	plan2.Steps["inspect"].Description = "Inspect updated implementation"
	f.SeedActivePlan(plan2, draftInput("wf-plan-versions", "rev-2"))

	runtime := bindingRuntime(f)

	versions, err := runtime.PlanVersions(context.Background(), "wf-plan-versions")
	if err != nil {
		t.Fatalf("PlanVersions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected at least 2 plan versions, got %d", len(versions))
	}

	// Active version must be the latest.
	active, err := runtime.ActivePlanVersion(context.Background(), "wf-plan-versions")
	if err != nil {
		t.Fatalf("ActivePlanVersion: %v", err)
	}
	if active == nil {
		t.Fatal("expected non-nil active plan version")
	}
	if active.Version < 2 {
		t.Fatalf("expected active version ≥2, got %d", active.Version)
	}

	// Compare v1 and v2 — result must be non-nil map.
	diff, err := runtime.ComparePlanVersions(context.Background(), "wf-plan-versions", 1, 2)
	if err != nil {
		t.Fatalf("ComparePlanVersions: %v", err)
	}
	if diff == nil {
		t.Fatal("expected non-nil diff from ComparePlanVersions")
	}
}
