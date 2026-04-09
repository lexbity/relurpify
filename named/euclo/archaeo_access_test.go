package euclo

import (
	"context"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
)

func TestAgentArchaeoAccessNilAgentReturnsNilViews(t *testing.T) {
	access := agentArchaeoAccess{}
	ctx := context.Background()

	if view, err := access.RequestHistory(ctx, "wf-1"); view != nil || err != nil {
		t.Fatalf("expected nil request history, got %#v %v", view, err)
	}
	if view, err := access.ActivePlan(ctx, "wf-1"); view != nil || err != nil {
		t.Fatalf("expected nil active plan, got %#v %v", view, err)
	}
	if view, err := access.LearningQueue(ctx, "wf-1"); view != nil || err != nil {
		t.Fatalf("expected nil learning queue, got %#v %v", view, err)
	}
	if views, err := access.TensionsByWorkflow(ctx, "wf-1"); views != nil || err != nil {
		t.Fatalf("expected nil tensions, got %#v %v", views, err)
	}
	if view, err := access.TensionSummaryByWorkflow(ctx, "wf-1"); view != nil || err != nil {
		t.Fatalf("expected nil tension summary, got %#v %v", view, err)
	}
	if views, err := access.PlanVersions(ctx, "wf-1"); views != nil || err != nil {
		t.Fatalf("expected nil plan versions, got %#v %v", views, err)
	}
	if view, err := access.ActivePlanVersion(ctx, "wf-1"); view != nil || err != nil {
		t.Fatalf("expected nil active plan version, got %#v %v", view, err)
	}
	if view, err := access.DraftPlanVersion(ctx, &frameworkplan.LivingPlan{}, eucloexec.DraftPlanInput{}); view != nil || err != nil {
		t.Fatalf("expected nil draft version, got %#v %v", view, err)
	}
	if view, err := access.ActivatePlanVersion(ctx, "wf-1", 1); view != nil || err != nil {
		t.Fatalf("expected nil activated version, got %#v %v", view, err)
	}
}

func TestArchaeoAccessConversionHelpers(t *testing.T) {
	history := &archaeoprojections.RequestHistoryProjection{
		WorkflowID: "wf-1",
		Pending:    1,
		Running:    2,
		Completed:  3,
		Failed:     4,
		Canceled:   5,
		Requests: []archaeodomain.RequestRecord{
			{
				ID:          "req-1",
				Kind:        archaeodomain.RequestKind("analysis"),
				Status:      archaeodomain.RequestStatus("running"),
				Title:       "  Title  ",
				Description: "fallback",
				SubjectRefs: []string{"a", "b"},
				RequestedAt: testTime(1),
				UpdatedAt:   testTime(2),
			},
		},
	}
	historyView := requestHistoryView(history)
	if historyView.WorkflowID != "wf-1" || len(historyView.Requests) != 1 {
		t.Fatalf("unexpected request history view: %#v", historyView)
	}
	if got := historyView.Requests[0].Scope; got != "a,b" {
		t.Fatalf("unexpected request scope: %q", got)
	}
	if got := historyView.Requests[0].Summary; got != "Title" {
		t.Fatalf("unexpected request summary: %q", got)
	}

	planVersion := archaeodomain.VersionedLivingPlan{
		ID:                     "ver-1",
		WorkflowID:             "wf-1",
		Plan:                   frameworkplan.LivingPlan{ID: "plan-1", StepOrder: []string{"step-1"}},
		Version:                3,
		Status:                 archaeodomain.LivingPlanVersionActive,
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "snap-1",
		PatternRefs:            []string{"p1"},
		AnchorRefs:             []string{"a1"},
		TensionRefs:            []string{"t1"},
	}
	planView := versionedPlanView(planVersion)
	if planView.PlanID != "plan-1" || planView.Version != 3 {
		t.Fatalf("unexpected versioned plan view: %#v", planView)
	}

	activePlan := &archaeoprojections.ActivePlanProjection{
		WorkflowID: "wf-1",
		PhaseState: &archaeodomain.WorkflowPhaseState{CurrentPhase: archaeodomain.PhaseExecution},
		ActivePlanVersion: &archaeodomain.VersionedLivingPlan{
			Plan: frameworkplan.LivingPlan{
				StepOrder: []string{"step-1", "step-2"},
				Steps: map[string]*frameworkplan.PlanStep{
					"step-1": {ID: "step-1", Status: frameworkplan.PlanStepCompleted},
					"step-2": {ID: "step-2", Status: frameworkplan.PlanStepInProgress},
				},
			},
		},
	}
	activeView := activePlanView(activePlan)
	if activeView.Phase != string(archaeodomain.PhaseExecution) || activeView.ActiveStepID != "step-2" {
		t.Fatalf("unexpected active plan view: %#v", activeView)
	}

	queue := &archaeoprojections.LearningQueueProjection{
		WorkflowID:         "wf-1",
		PendingGuidanceIDs: []string{"g-1"},
		BlockingLearning:   []string{"b-1"},
		PendingLearning: []archaeolearning.Interaction{
			{
				ID:          "learn-1",
				Status:      archaeolearning.StatusPending,
				Title:       "  Learn title  ",
				Description: "fallback description",
				SubjectID:   "sub-1",
				Evidence:    []archaeolearning.EvidenceRef{{RefID: "ref-1"}, {RefID: " ref-2 "}},
			},
		},
	}
	queueView := learningQueueView(queue)
	if queueView.WorkflowID != "wf-1" || len(queueView.PendingLearning) != 1 {
		t.Fatalf("unexpected learning queue view: %#v", queueView)
	}
	if got := queueView.PendingLearning[0].Prompt; got != "Learn title" {
		t.Fatalf("unexpected learning prompt: %q", got)
	}
	if got := queueView.PendingLearning[0].Evidence; len(got) != 2 || got[1] != "ref-2" {
		t.Fatalf("unexpected learning evidence: %#v", got)
	}

	tensions := tensionViews([]archaeodomain.Tension{{
		ID:                 "t-1",
		Kind:               "gap",
		Description:        "description",
		Severity:           "high",
		Status:             archaeodomain.TensionResolved,
		PatternIDs:         []string{"p-1"},
		AnchorRefs:         []string{"a-1"},
		SymbolScope:        []string{"s-1"},
		RelatedPlanStepIDs: []string{"step-1"},
		BasedOnRevision:    "rev-1",
	}})
	if len(tensions) != 1 || tensions[0].Status != string(archaeodomain.TensionResolved) {
		t.Fatalf("unexpected tension view: %#v", tensions)
	}

	summaryView := tensionSummaryView(&archaeodomain.TensionSummary{
		WorkflowID: "wf-1",
		Total:      4,
		Active:     2,
		Accepted:   1,
		Resolved:   1,
		Unresolved: 0,
	})
	if summaryView.Total != 4 || summaryView.Accepted != 1 {
		t.Fatalf("unexpected tension summary view: %#v", summaryView)
	}

	versions := versionedPlanViews([]archaeodomain.VersionedLivingPlan{{ID: "ver-2", Plan: frameworkplan.LivingPlan{ID: "plan-2"}}})
	if len(versions) != 1 || versions[0].ID != "ver-2" {
		t.Fatalf("unexpected versioned plan slice: %#v", versions)
	}

	if got := firstNonEmptyStringValue(" ", " second "); got != "second" {
		t.Fatalf("unexpected first non-empty value: %q", got)
	}
}

func testTime(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}
