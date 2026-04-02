package euclobindings_test

import (
	"context"
	"slices"
	"testing"
	"time"

	euclobindings "github.com/lexcodex/relurpify/archaeo/bindings/euclo"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type bindingStubVerifier struct {
	failure *frameworkplan.ConvergenceFailure
	err     error
}

func (s bindingStubVerifier) Verify(context.Context, frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	return s.failure, s.err
}

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

func TestScenario_Binding_FullMutationEvaluationWithComplexPolicy(t *testing.T) {
	f := testscenario.New(t)
	f.SeedWorkflow("wf-binding-mutations", "aggregate archaeology mutations through euclo binding")

	plan := bindingPlan("plan-binding-mutations", "wf-binding-mutations")
	active := f.SeedActivePlan(plan, draftInput("wf-binding-mutations", "rev-1"))
	step := active.Plan.Steps["inspect"]

	f.SeedMutation(archaeodomain.MutationEvent{
		WorkflowID:  "wf-binding-mutations",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		StepID:      step.ID,
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "observer",
		SourceRef:   "obs-1",
		Description: "non-blocking observation",
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{step.ID},
		},
	})
	f.SeedMutation(archaeodomain.MutationEvent{
		WorkflowID:  "wf-binding-mutations",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		StepID:      step.ID,
		Category:    archaeodomain.MutationConfidenceChange,
		SourceKind:  "analyzer",
		SourceRef:   "confidence-1",
		Description: "confidence degraded",
		Impact:      archaeodomain.ImpactAdvisory,
		Disposition: archaeodomain.DispositionContinue,
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{step.ID},
		},
	})
	f.SeedMutation(archaeodomain.MutationEvent{
		WorkflowID:  "wf-binding-mutations",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		StepID:      step.ID,
		Category:    archaeodomain.MutationStepInvalidation,
		SourceKind:  "executor",
		SourceRef:   "invalidate-1",
		Description: "step invalidated by new evidence",
		Impact:      archaeodomain.ImpactLocalBlocking,
		Disposition: archaeodomain.DispositionInvalidateStep,
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{step.ID},
		},
	})

	runtime := bindingRuntime(f)
	eval, err := runtime.EvaluateExecutionMutations(context.Background(), "wf-binding-mutations", nil, &active.Plan, step)
	if err != nil {
		t.Fatalf("EvaluateExecutionMutations: %v", err)
	}
	if eval == nil {
		t.Fatal("expected mutation evaluation")
	}
	if len(eval.RelevantMutations) != 3 {
		t.Fatalf("expected 3 relevant mutations, got %d", len(eval.RelevantMutations))
	}
	if eval.HighestImpact != archaeodomain.ImpactLocalBlocking {
		t.Fatalf("expected highest impact %q, got %q", archaeodomain.ImpactLocalBlocking, eval.HighestImpact)
	}
	if eval.Disposition != archaeodomain.DispositionInvalidateStep {
		t.Fatalf("expected invalidate_step disposition, got %q", eval.Disposition)
	}
	if !eval.Blocking {
		t.Fatalf("expected blocking evaluation, got %+v", eval)
	}
}

func TestScenario_Binding_PhasePersistenceRoundTrip(t *testing.T) {
	f := testscenario.New(t)
	f.SeedWorkflow("wf-binding-phases", "persist euclo binding phase transitions end-to-end")
	active := f.SeedActivePlan(bindingPlan("plan-binding-phases", "wf-binding-phases"), draftInput("wf-binding-phases", "rev-1"))

	runtime := bindingRuntime(f)
	task := f.Task("wf-binding-phases", "persist euclo binding phase transitions end-to-end", nil)
	state := f.NewState()
	state.Set("euclo.living_plan", &active.Plan)

	if _, err := runtime.PhaseService().RecordState(context.Background(), task, state, nil, archaeodomain.PhaseArchaeology, "", nil); err != nil {
		t.Fatalf("RecordState archaeology: %v", err)
	}
	if _, err := runtime.PhaseService().RecordState(context.Background(), task, state, nil, archaeodomain.PhasePlanFormation, "", nil); err != nil {
		t.Fatalf("RecordState plan formation: %v", err)
	}

	driver := runtime.PhaseDriver(euclobindings.DriverConfig{})
	driver.EnterExecution(context.Background(), task, state, active.Plan.Steps["inspect"])
	driver.EnterVerification(context.Background(), task, state, active.Plan.Steps["inspect"], nil)
	driver.Complete(context.Background(), task, state, active.Plan.Steps["inspect"], nil)

	testscenario.AssertPhase(t, f, "wf-binding-phases", archaeodomain.PhaseCompleted)

	timeline, err := runtime.WorkflowTimeline(context.Background(), "wf-binding-phases")
	if err != nil {
		t.Fatalf("WorkflowTimeline: %v", err)
	}
	phases := make([]string, 0, len(timeline))
	for _, event := range timeline {
		if event.EventType != archaeoevents.EventWorkflowPhaseTransitioned {
			continue
		}
		phase, _ := event.Metadata["phase"].(string)
		if phase != "" {
			phases = append(phases, phase)
		}
	}
	want := []string{
		string(archaeodomain.PhaseArchaeology),
		string(archaeodomain.PhasePlanFormation),
		string(archaeodomain.PhaseExecution),
		string(archaeodomain.PhaseVerification),
		string(archaeodomain.PhaseCompleted),
	}
	if len(phases) < len(want) || !slices.Equal(phases[len(phases)-len(want):], want) {
		t.Fatalf("expected trailing phase sequence %v, got %v", want, phases)
	}
}

func TestScenario_Binding_ArchaeologyService_ProviderBackedRefresh(t *testing.T) {
	TestRuntimeArchaeologyServiceUsesProviderBackedRequests(t)
}

func TestRuntimeArchaeologyLoop_PlanExecutionFeedsBackIntoArchaeo(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-euclo-loop", "prove archaeology plan execution feedback loop")

	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      "wf-euclo-loop",
		ExplorationID:   "explore-loop",
		SourceRef:       "gap-loop",
		Kind:            "boundary_mismatch",
		Description:     "execution must resolve an archaeology-tracked boundary mismatch",
		Severity:        "significant",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-loop-1",
	})

	plan := bindingPlan("plan-euclo-loop", "wf-euclo-loop")
	plan.StepOrder = []string{"inspect", "implement"}
	plan.Steps["inspect"].AnchorDependencies = []string{"anchor:boundary"}
	plan.Steps["implement"] = &frameworkplan.PlanStep{
		ID:                 "implement",
		Description:        "Apply the verified boundary fix",
		Scope:              []string{"symbol.implement"},
		DependsOn:          []string{"inspect"},
		AnchorDependencies: []string{"anchor:boundary"},
		ConfidenceScore:    0.92,
		Status:             frameworkplan.PlanStepPending,
		CreatedAt:          f.Now(),
		UpdatedAt:          f.Now(),
	}
	plan.ConvergenceTarget = &frameworkplan.ConvergenceTarget{
		TensionIDs: []string{tension.ID},
		PatternIDs: []string{"pattern:boundary-wrapper"},
	}

	runtime := bindingRuntime(f)
	runtime.ConvVerifier = bindingStubVerifier{}

	versioned, err := runtime.PlanService().DraftVersion(context.Background(), plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-euclo-loop",
		DerivedFromExploration: "explore-loop",
		BasedOnRevision:        "rev-loop-1",
		SemanticSnapshotRef:    "snapshot-loop-1",
		TensionRefs:            []string{tension.ID},
		PatternRefs:            []string{"pattern:boundary-wrapper"},
		AnchorRefs:             []string{"anchor:boundary"},
	})
	if err != nil {
		t.Fatalf("DraftVersion: %v", err)
	}

	active, err := runtime.PlanService().ActivateVersion(context.Background(), "wf-euclo-loop", versioned.Version)
	if err != nil {
		t.Fatalf("ActivateVersion: %v", err)
	}
	if active == nil {
		t.Fatal("expected active plan version")
	}
	if active.Status != archaeodomain.LivingPlanVersionActive {
		t.Fatalf("expected active plan status, got %q", active.Status)
	}
	if active.WorkflowID != "wf-euclo-loop" || active.Plan.ID != "plan-euclo-loop" || active.Version != versioned.Version {
		t.Fatalf("unexpected active plan version: %+v", active)
	}
	if active.DerivedFromExploration != "explore-loop" || active.BasedOnRevision != "rev-loop-1" || active.SemanticSnapshotRef != "snapshot-loop-1" {
		t.Fatalf("expected archaeology provenance on active plan, got %+v", active)
	}
	if len(active.TensionRefs) != 1 || active.TensionRefs[0] != tension.ID {
		t.Fatalf("expected tension refs to persist, got %+v", active.TensionRefs)
	}
	if len(active.PatternRefs) != 1 || active.PatternRefs[0] != "pattern:boundary-wrapper" {
		t.Fatalf("expected pattern refs to persist, got %+v", active.PatternRefs)
	}

	handoff, err := runtime.ExecutionHandoffRecorder().Record(context.Background(), f.Task("wf-euclo-loop", "handoff active archaeology plan into execution", map[string]any{
		"based_on_revision": "rev-loop-1",
	}), f.NewState(), &active.Plan, active.Plan.Steps["inspect"])
	if err != nil {
		t.Fatalf("Record handoff: %v", err)
	}
	if handoff == nil {
		t.Fatal("expected execution handoff")
	}
	if handoff.PlanID != active.Plan.ID || handoff.PlanVersion != active.Version || handoff.StepID != "inspect" {
		t.Fatalf("unexpected handoff: %+v", handoff)
	}

	f.SeedMutation(archaeodomain.MutationEvent{
		WorkflowID:  "wf-euclo-loop",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		StepID:      "inspect",
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "verification",
		SourceRef:   "checkpoint-inspect",
		Description: "execution checkpoint observed no blocking divergence",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{"inspect"},
		},
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   f.Now(),
	})

	eval, err := runtime.EvaluateExecutionMutations(context.Background(), "wf-euclo-loop", handoff, &active.Plan, active.Plan.Steps["inspect"])
	if err != nil {
		t.Fatalf("EvaluateExecutionMutations: %v", err)
	}
	if eval == nil {
		t.Fatal("expected mutation evaluation")
	}
	if eval.Blocking {
		t.Fatalf("expected non-blocking execution mutation evaluation, got %+v", eval)
	}
	if eval.Disposition != archaeodomain.DispositionContinue {
		t.Fatalf("expected continue disposition, got %+v", eval)
	}

	updatedTension, err := runtime.UpdateTensionStatus(context.Background(), "wf-euclo-loop", tension.ID, archaeodomain.TensionResolved, nil)
	if err != nil {
		t.Fatalf("UpdateTensionStatus: %v", err)
	}
	if updatedTension == nil || updatedTension.Status != archaeodomain.TensionResolved {
		t.Fatalf("expected resolved tension after execution feedback, got %+v", updatedTension)
	}

	result := &core.Result{Success: true, Data: map[string]any{"completed_steps": []string{"inspect", "implement"}}}
	failure, err := runtime.VerificationService().FinalizeConvergence(context.Background(), &active.Plan, result)
	if err != nil {
		t.Fatalf("FinalizeConvergence: %v", err)
	}
	if failure != nil {
		t.Fatalf("expected verification success, got %+v", failure)
	}
	if active.Plan.ConvergenceTarget == nil || active.Plan.ConvergenceTarget.VerifiedAt == nil {
		t.Fatalf("expected convergence target to be marked verified, got %+v", active.Plan.ConvergenceTarget)
	}

	tensions, err := runtime.TensionsByWorkflow(context.Background(), "wf-euclo-loop")
	if err != nil {
		t.Fatalf("TensionsByWorkflow: %v", err)
	}
	if len(tensions) != 1 || tensions[0].ID != tension.ID || tensions[0].Status != archaeodomain.TensionResolved {
		t.Fatalf("expected resolved tension in runtime view, got %+v", tensions)
	}

	timeline, err := runtime.WorkflowTimeline(context.Background(), "wf-euclo-loop")
	if err != nil {
		t.Fatalf("WorkflowTimeline: %v", err)
	}
	var sawHandoff, sawVerified bool
	for _, entry := range timeline {
		switch entry.EventType {
		case archaeoevents.EventExecutionHandoffRecorded:
			sawHandoff = true
		case archaeoevents.EventConvergenceVerified:
			sawVerified = true
		}
	}
	if !sawHandoff || !sawVerified {
		t.Fatalf("expected handoff and convergence verification events in timeline, got %+v", timeline)
	}

	activeProj, err := runtime.ActivePlanProjection(context.Background(), "wf-euclo-loop")
	if err != nil {
		t.Fatalf("ActivePlanProjection: %v", err)
	}
	if activeProj == nil || activeProj.ActivePlanVersion == nil || activeProj.ActivePlanVersion.Version != active.Version {
		t.Fatalf("expected active plan projection to reflect version %d, got %+v", active.Version, activeProj)
	}
	if activeProj.ConvergenceState == nil || activeProj.ConvergenceState.Status != archaeodomain.ConvergenceStatusVerified {
		t.Fatalf("expected resolved convergence state in active plan projection, got %+v", activeProj)
	}

	summary, err := runtime.TensionSummaryByWorkflow(context.Background(), "wf-euclo-loop")
	if err != nil {
		t.Fatalf("TensionSummaryByWorkflow: %v", err)
	}
	if summary == nil {
		t.Fatal("expected workflow tension summary")
	}
	if summary.Active != 0 || summary.Resolved != 1 {
		t.Fatalf("expected resolved tension summary after feedback, got %+v", summary)
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
			Status:       archaeodomain.ConvergenceResolutionResolved,
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
