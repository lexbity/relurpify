package projections_test

import (
	"context"
	"testing"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/requests"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestScenarioCoherenceProjectionFull(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-coherence", "build a full coherence projection")

	session, snapshot := f.SeedExploration("wf-coherence", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-coherence",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "baseline exploration snapshot",
	})
	plan := projectionPlan("plan-coherence", "wf-coherence")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-coherence",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})
	draft := f.SeedPlanVersion(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-coherence",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-2",
		SemanticSnapshotRef:    snapshot.ID,
	})
	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      "wf-coherence",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		SourceRef:       "gap:coherence",
		Kind:            "semantic_gap",
		Description:     "execution assumptions drifted",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-2",
	})
	learning := f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:      "wf-coherence",
		ExplorationID:   session.ID,
		Kind:            archaeolearning.InteractionIntentRefinement,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       session.ID,
		Title:           "Confirm intended repair direction",
		Blocking:        true,
		BasedOnRevision: "rev-2",
	})
	convergence := f.SeedConvergenceRecord(archaeoconvergence.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         "wf-coherence",
		ExplorationID:      session.ID,
		PlanID:             active.Plan.ID,
		PlanVersion:        &active.Version,
		Question:           "Can this ambiguity be accepted?",
		RelevantTensionIDs: []string{tension.ID},
		PendingLearningIDs: []string{learning.ID},
	})
	if err := archaeoevents.AppendWorkflowEvent(context.Background(), f.WorkflowStore, "wf-coherence", archaeoevents.EventConvergenceFailed, "coherence remains unresolved", map[string]any{
		"plan_id":                active.Plan.ID,
		"plan_version":           active.Version,
		"based_on_revision":      "rev-2",
		"description":            "unresolved blocking ambiguity remains",
		"unresolved_tension_ids": []string{tension.ID},
	}, f.Now()); err != nil {
		t.Fatalf("append convergence state event: %v", err)
	}
	f.SeedMutation(archaeodomain.MutationEvent{
		ID:              "mutation-confidence",
		WorkflowID:      "wf-coherence",
		PlanID:          active.Plan.ID,
		PlanVersion:     intPtr(active.Version),
		Category:        archaeodomain.MutationConfidenceChange,
		SourceKind:      "convergence",
		SourceRef:       convergence.ID,
		Description:     "confidence dropped after convergence review",
		Impact:          archaeodomain.ImpactPlanRecomputeRequired,
		Disposition:     archaeodomain.DispositionRequireReplan,
		BasedOnRevision: "rev-2",
	})
	f.SeedMutation(archaeodomain.MutationEvent{
		ID:              "mutation-observation",
		WorkflowID:      "wf-coherence",
		PlanID:          active.Plan.ID,
		PlanVersion:     intPtr(active.Version),
		Category:        archaeodomain.MutationObservation,
		SourceKind:      "comment",
		SourceRef:       "comment-1",
		Description:     "extra note for operators",
		Impact:          archaeodomain.ImpactInformational,
		Disposition:     archaeodomain.DispositionContinue,
		BasedOnRevision: "rev-2",
	})

	proj, err := f.ProjectionService().Coherence(context.Background(), "wf-coherence")
	if err != nil {
		t.Fatalf("build coherence projection: %v", err)
	}
	if proj == nil {
		t.Fatalf("expected coherence projection")
	}
	if proj.WorkflowID != "wf-coherence" {
		t.Fatalf("workflow id mismatch: %s", proj.WorkflowID)
	}
	if proj.ActivePlanVersion == nil || proj.ActivePlanVersion.Version != 1 {
		t.Fatalf("expected active plan version 1, got %+v", proj.ActivePlanVersion)
	}
	if len(proj.DraftPlanVersions) != 1 || proj.DraftPlanVersions[0].Version != draft.Version {
		t.Fatalf("expected draft version %d, got %+v", draft.Version, proj.DraftPlanVersions)
	}
	if len(proj.ActiveTensions) != 1 || proj.ActiveTensions[0].ID != tension.ID {
		t.Fatalf("expected active tension %s, got %+v", tension.ID, proj.ActiveTensions)
	}
	if len(proj.PendingLearning) != 1 || proj.PendingLearning[0].ID != learning.ID {
		t.Fatalf("expected pending learning %s, got %+v", learning.ID, proj.PendingLearning)
	}
	if proj.BlockingLearningCount != 1 {
		t.Fatalf("expected 1 blocking learning item, got %d", proj.BlockingLearningCount)
	}
	if proj.BlockingMutationCount != 2 {
		t.Fatalf("expected 2 blocking mutations, got %d", proj.BlockingMutationCount)
	}
	foundConfidenceMutation := false
	for _, mutation := range proj.ConfidenceAffectingMutations {
		if mutation.ID == "mutation-confidence" {
			foundConfidenceMutation = true
			break
		}
	}
	if !foundConfidenceMutation {
		t.Fatalf("expected mutation-confidence in confidence-affecting mutations, got %+v", proj.ConfidenceAffectingMutations)
	}
	if proj.ConvergenceState == nil || proj.ConvergenceState.Status != archaeodomain.ConvergenceStatusFailed {
		t.Fatalf("expected failed convergence state, got %+v", proj.ConvergenceState)
	}
	if len(proj.ConvergenceState.UnresolvedTensionIDs) != 1 || proj.ConvergenceState.UnresolvedTensionIDs[0] != tension.ID {
		t.Fatalf("expected unresolved tension %s, got %+v", tension.ID, proj.ConvergenceState.UnresolvedTensionIDs)
	}
}

func TestScenarioDecisionTrailWorkspace(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-decision", "project workspace decision trail")

	session, _ := f.SeedExploration("wf-decision", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-decision",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "decision workspace seed",
	})
	plan := projectionPlan("plan-decision", "wf-decision")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-decision",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
	})
	openDeferred := f.SeedDeferredDraft(archaeodeferred.CreateInput{
		WorkspaceID:   f.Workspace,
		WorkflowID:    "wf-decision",
		ExplorationID: session.ID,
		PlanID:        active.Plan.ID,
		PlanVersion:   &active.Version,
		AmbiguityKey:  "legacy-open",
		Title:         "Open legacy ambiguity",
	})
	formedDeferred := f.SeedDeferredDraft(archaeodeferred.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         "wf-decision",
		ExplorationID:      session.ID,
		PlanID:             active.Plan.ID,
		PlanVersion:        &active.Version,
		AmbiguityKey:       "legacy-formed",
		Title:              "Formed deferred draft",
		LinkedDraftVersion: intPtr(2),
		LinkedDraftPlanID:  active.Plan.ID,
	})
	convergence, err := f.ConvergenceService().Create(context.Background(), archaeoconvergence.CreateInput{
		WorkspaceID:      f.Workspace,
		WorkflowID:       "wf-decision",
		ExplorationID:    session.ID,
		PlanID:           active.Plan.ID,
		PlanVersion:      &active.Version,
		Question:         "Which workaround should ship?",
		DeferredDraftIDs: []string{openDeferred.ID, formedDeferred.ID},
	})
	if err != nil {
		t.Fatalf("create convergence record: %v", err)
	}
	if _, err := f.ConvergenceService().Resolve(context.Background(), archaeoconvergence.ResolveInput{
		WorkflowID: "wf-decision",
		RecordID:   convergence.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:       archaeodomain.ConvergenceResolutionResolved,
			ChosenOption: "defer legacy path",
			Summary:      "defer the legacy behavior while keeping a tracked draft",
		},
	}); err != nil {
		t.Fatalf("resolve convergence record: %v", err)
	}
	openDecision, err := f.DecisionService().Create(context.Background(), decisionInputForConvergence(f.Workspace, "wf-decision", convergence.ID, active.Plan.ID, active.Version))
	if err != nil {
		t.Fatalf("create open decision: %v", err)
	}
	resolvedDecision, err := f.DecisionService().Create(context.Background(), decisionInputForDeferred(f.Workspace, "wf-decision", formedDeferred.ID, active.Plan.ID, active.Version))
	if err != nil {
		t.Fatalf("create resolved decision: %v", err)
	}
	if _, err := f.DecisionService().Resolve(context.Background(), archaeodecisions.ResolveInput{
		WorkflowID: "wf-decision",
		RecordID:   resolvedDecision.ID,
		Status:     archaeodomain.DecisionStatusResolved,
	}); err != nil {
		t.Fatalf("resolve decision: %v", err)
	}

	trail, err := f.ProjectionService().DecisionTrail(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("build decision trail projection: %v", err)
	}
	if trail == nil {
		t.Fatalf("expected decision trail projection")
	}
	if trail.OpenCount != 1 || trail.Resolved != 1 {
		t.Fatalf("unexpected decision counts: %+v", trail)
	}
	if len(trail.Records) != 2 {
		t.Fatalf("expected 2 decision records, got %d", len(trail.Records))
	}
	if trail.Records[0].ID != openDecision.ID && trail.Records[1].ID != openDecision.ID {
		t.Fatalf("expected open decision %s in trail: %+v", openDecision.ID, trail.Records)
	}

	deferredProj, err := f.ProjectionService().DeferredDrafts(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("build deferred draft projection: %v", err)
	}
	if deferredProj == nil {
		t.Fatalf("expected deferred draft projection")
	}
	if deferredProj.OpenCount != 1 || deferredProj.FormedCount != 1 {
		t.Fatalf("unexpected deferred counts: %+v", deferredProj)
	}

	convergenceProj, err := f.ProjectionService().ConvergenceHistory(context.Background(), f.Workspace)
	if err != nil {
		t.Fatalf("build convergence history projection: %v", err)
	}
	if convergenceProj == nil || convergenceProj.Current == nil {
		t.Fatalf("expected convergence history projection")
	}
	if convergenceProj.ResolvedCount != 1 || convergenceProj.Current.ID != convergence.ID {
		t.Fatalf("unexpected convergence projection: %+v", convergenceProj)
	}
}

func TestScenarioPlanLineageCompare(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-lineage-proj", "compare plan lineage versions")

	session, snapshot := f.SeedExploration("wf-lineage-proj", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-lineage-proj",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "lineage baseline",
	})
	plan := projectionPlan("plan-lineage-proj", "wf-lineage-proj")
	version1 := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-lineage-proj",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})
	version2 := f.SeedPlanVersion(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-lineage-proj",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-2",
		SemanticSnapshotRef:    snapshot.ID,
	})
	if _, err := f.PlansService().ActivateVersion(context.Background(), "wf-lineage-proj", version2.Version); err != nil {
		t.Fatalf("activate version 2: %v", err)
	}
	version3 := f.SeedPlanVersion(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-lineage-proj",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-3",
		SemanticSnapshotRef:    snapshot.ID,
	})

	lineage, err := f.ProjectionService().PlanLineage(context.Background(), "wf-lineage-proj")
	if err != nil {
		t.Fatalf("build lineage projection: %v", err)
	}
	if lineage == nil {
		t.Fatalf("expected lineage projection")
	}
	if lineage.ActiveVersion == nil || lineage.ActiveVersion.Version != version2.Version {
		t.Fatalf("expected active version %d, got %+v", version2.Version, lineage.ActiveVersion)
	}
	if lineage.LatestDraft == nil || lineage.LatestDraft.Version != version3.Version {
		t.Fatalf("expected latest draft version %d, got %+v", version3.Version, lineage.LatestDraft)
	}
	if len(lineage.Versions) != 3 {
		t.Fatalf("expected 3 lineage versions, got %d", len(lineage.Versions))
	}
	if len(lineage.DraftVersions) != 1 || lineage.DraftVersions[0].Version != version3.Version {
		t.Fatalf("expected draft version %d, got %+v", version3.Version, lineage.DraftVersions)
	}
	if version2.ParentVersion == nil || *version2.ParentVersion != version1.Version {
		t.Fatalf("expected version 2 parent to be %d, got %+v", version1.Version, version2.ParentVersion)
	}
	if version3.ParentVersion == nil || *version3.ParentVersion != version2.Version {
		t.Fatalf("expected version 3 parent to be %d, got %+v", version2.Version, version3.ParentVersion)
	}
}

func TestScenarioRequestLifecycleProjection(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-requests", "project request lifecycle")

	session, snapshot := f.SeedExploration("wf-requests", f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      "wf-requests",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "request projection baseline",
	})
	plan := projectionPlan("plan-requests", "wf-requests")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-requests",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})
	requestsSvc := f.RequestsService()

	completed := mustCreateRequest(t, requestsSvc, requests.CreateInput{
		WorkflowID:      "wf-requests",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		PlanID:          active.Plan.ID,
		PlanVersion:     intPtr(active.Version),
		Kind:            archaeodomain.RequestExplorationRefresh,
		Title:           "Refresh exploration",
		RequestedBy:     "scenario",
		SubjectRefs:     []string{"symbol.alpha"},
		Input:           map[string]any{"workspace_id": f.Workspace},
		BasedOnRevision: "rev-1",
	})
	if _, err := requestsSvc.Dispatch(context.Background(), "wf-requests", completed.ID, nil); err != nil {
		t.Fatalf("dispatch completed request: %v", err)
	}
	if _, err := requestsSvc.Claim(context.Background(), requests.ClaimInput{
		WorkflowID: "wf-requests",
		RequestID:  completed.ID,
		ClaimedBy:  "worker-1",
		LeaseTTL:   time.Minute,
	}); err != nil {
		t.Fatalf("claim completed request: %v", err)
	}
	if _, validity, err := requestsSvc.ApplyFulfillment(context.Background(), requests.ApplyFulfillmentInput{
		WorkflowID:        "wf-requests",
		RequestID:         completed.ID,
		CurrentRevision:   "rev-1",
		CurrentSnapshotID: snapshot.ID,
		Fulfillment: archaeodomain.RequestFulfillment{
			Kind:        "semantic_refresh",
			RefID:       "artifact-refresh-1",
			Summary:     "fresh workspace scan",
			ExecutorRef: "worker-1",
		},
	}); err != nil {
		t.Fatalf("apply valid fulfillment: %v", err)
	} else if validity != archaeodomain.RequestValidityValid {
		t.Fatalf("expected valid fulfillment, got %s", validity)
	}

	failed := mustCreateRequest(t, requestsSvc, requests.CreateInput{
		WorkflowID:      "wf-requests",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		PlanID:          active.Plan.ID,
		PlanVersion:     intPtr(active.Version),
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "Surface patterns",
		RequestedBy:     "scenario",
		Input:           map[string]any{"workspace_id": f.Workspace},
		BasedOnRevision: "rev-1",
	})
	if _, err := requestsSvc.Dispatch(context.Background(), "wf-requests", failed.ID, nil); err != nil {
		t.Fatalf("dispatch failed request: %v", err)
	}
	if _, err := requestsSvc.Fail(context.Background(), "wf-requests", failed.ID, "model timeout", false); err != nil {
		t.Fatalf("fail request: %v", err)
	}

	canceled := mustCreateRequest(t, requestsSvc, requests.CreateInput{
		WorkflowID:      "wf-requests",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		PlanID:          active.Plan.ID,
		PlanVersion:     intPtr(active.Version),
		Kind:            archaeodomain.RequestTensionAnalysis,
		Title:           "Analyze tensions",
		RequestedBy:     "scenario",
		Input:           map[string]any{"workspace_id": f.Workspace},
		BasedOnRevision: "rev-1",
	})
	if _, err := requestsSvc.Cancel(context.Background(), "wf-requests", canceled.ID, "operator canceled"); err != nil {
		t.Fatalf("cancel request: %v", err)
	}

	invalidated := mustCreateRequest(t, requestsSvc, requests.CreateInput{
		WorkflowID:      "wf-requests",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		PlanID:          active.Plan.ID,
		PlanVersion:     intPtr(active.Version),
		Kind:            archaeodomain.RequestProspectiveAnalysis,
		Title:           "Prospective analysis",
		RequestedBy:     "scenario",
		SubjectRefs:     []string{"symbol.alpha"},
		Input:           map[string]any{"workspace_id": f.Workspace},
		BasedOnRevision: "rev-1",
	})
	if _, validity, err := requestsSvc.ApplyFulfillment(context.Background(), requests.ApplyFulfillmentInput{
		WorkflowID:        "wf-requests",
		RequestID:         invalidated.ID,
		CurrentRevision:   "rev-2",
		CurrentSnapshotID: snapshot.ID,
		Fulfillment: archaeodomain.RequestFulfillment{
			Kind:     "prospective_analysis",
			RefID:    "artifact-stale-1",
			Summary:  "stale prospective result",
			Metadata: map[string]any{"workspace_id": f.Workspace},
		},
	}); err != nil {
		t.Fatalf("apply invalidated fulfillment: %v", err)
	} else if validity != archaeodomain.RequestValidityInvalidated {
		t.Fatalf("expected invalidated fulfillment, got %s", validity)
	}

	if err := archaeoevents.AppendMutationEvent(context.Background(), f.WorkflowStore, archaeodomain.MutationEvent{
		ID:          "mutation-request-lifecycle",
		WorkflowID:  "wf-requests",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "request",
		SourceRef:   completed.ID,
		Description: "request result updated workspace knowledge",
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   f.Now(),
	}); err != nil {
		t.Fatalf("append mutation event: %v", err)
	}

	history, err := f.ProjectionService().RequestHistory(context.Background(), "wf-requests")
	if err != nil {
		t.Fatalf("build request history projection: %v", err)
	}
	if history == nil {
		t.Fatalf("expected request history projection")
	}
	if history.Completed != 1 || history.Failed != 1 || history.Canceled != 2 {
		t.Fatalf("unexpected request history counts: %+v", history)
	}
	if len(history.Requests) != 4 {
		t.Fatalf("expected 4 request records, got %d", len(history.Requests))
	}

	provenance, err := f.ProjectionService().Provenance(context.Background(), "wf-requests")
	if err != nil {
		t.Fatalf("build provenance projection: %v", err)
	}
	if provenance == nil {
		t.Fatalf("expected provenance projection")
	}
	if len(provenance.Requests) != 4 {
		t.Fatalf("expected 4 provenance requests, got %d", len(provenance.Requests))
	}
	if len(provenance.Mutations) != 1 || provenance.Mutations[0].MutationID != "mutation-request-lifecycle" {
		t.Fatalf("unexpected provenance mutations: %+v", provenance.Mutations)
	}
	if len(provenance.DecisionRefs) != 1 {
		t.Fatalf("expected 1 decision ref from stale fulfillment, got %+v", provenance.DecisionRefs)
	}

	timeline, err := f.ProjectionService().TimelineProjection(context.Background(), "wf-requests")
	if err != nil {
		t.Fatalf("build timeline projection: %v", err)
	}
	if timeline == nil || len(timeline.Timeline) == 0 {
		t.Fatalf("expected timeline events for request lifecycle")
	}
}

func projectionPlan(planID, workflowID string) *frameworkplan.LivingPlan {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return &frameworkplan.LivingPlan{
		ID:         planID,
		WorkflowID: workflowID,
		Title:      "Projection Scenario Plan",
		Version:    1,
		Steps: map[string]*frameworkplan.PlanStep{
			"inspect": {
				ID:              "inspect",
				Description:     "Inspect the target workspace",
				ConfidenceScore: 0.85,
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

func decisionInputForConvergence(workspaceID, workflowID, convergenceID, planID string, version int) archaeodecisions.CreateInput {
	return archaeodecisions.CreateInput{
		WorkspaceID:          workspaceID,
		WorkflowID:           workflowID,
		Kind:                 archaeodomain.DecisionKindConvergence,
		RelatedConvergenceID: convergenceID,
		RelatedPlanID:        planID,
		RelatedPlanVersion:   intPtr(version),
		Title:                "Choose a convergence outcome",
		Summary:              "manual convergence review required",
	}
}

func decisionInputForDeferred(workspaceID, workflowID, deferredID, planID string, version int) archaeodecisions.CreateInput {
	return archaeodecisions.CreateInput{
		WorkspaceID:            workspaceID,
		WorkflowID:             workflowID,
		Kind:                   archaeodomain.DecisionKindDeferredDraft,
		RelatedDeferredDraftID: deferredID,
		RelatedPlanID:          planID,
		RelatedPlanVersion:     intPtr(version),
		Title:                  "Resolve deferred draft",
		Summary:                "deferred draft accepted for later work",
	}
}

func mustCreateRequest(t *testing.T, svc requests.Service, input requests.CreateInput) *archaeodomain.RequestRecord {
	t.Helper()
	record, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("create request %q: %v", input.Title, err)
	}
	if record == nil {
		t.Fatalf("request %q unexpectedly nil", input.Title)
	}
	return record
}

func intPtr(value int) *int {
	return &value
}
