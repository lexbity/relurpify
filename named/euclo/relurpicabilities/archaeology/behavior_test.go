package archaeology

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// mockPlanStore is an in-memory PlanStore used for unit tests.
type mockPlanStore struct {
	plans       map[string]*frameworkplan.LivingPlan
	updatedStep *frameworkplan.PlanStep
}

func newMockPlanStore() *mockPlanStore {
	return &mockPlanStore{plans: map[string]*frameworkplan.LivingPlan{}}
}

func (m *mockPlanStore) SavePlan(_ context.Context, plan *frameworkplan.LivingPlan) error {
	m.plans[plan.ID] = plan
	return nil
}

func (m *mockPlanStore) LoadPlan(_ context.Context, planID string) (*frameworkplan.LivingPlan, error) {
	return m.plans[planID], nil
}

func (m *mockPlanStore) LoadPlanByWorkflow(_ context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	for _, p := range m.plans {
		if p.WorkflowID == workflowID {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockPlanStore) UpdateStep(_ context.Context, _, _ string, step *frameworkplan.PlanStep) error {
	m.updatedStep = step
	return nil
}

func (m *mockPlanStore) InvalidateStep(_ context.Context, _, _ string, _ frameworkplan.InvalidationRule) error {
	return nil
}

func (m *mockPlanStore) DeletePlan(_ context.Context, _ string) error { return nil }

func (m *mockPlanStore) ListPlans(_ context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

type mockArchaeoAccess struct {
	requests  *execution.RequestHistoryView
	active    *execution.ActivePlanView
	learning  *execution.LearningQueueView
	tensions  []execution.TensionView
	summary   *execution.TensionSummaryView
	drafted   *execution.VersionedPlanView
	activated *execution.VersionedPlanView
}

func (m *mockArchaeoAccess) RequestHistory(context.Context, string) (*execution.RequestHistoryView, error) {
	return m.requests, nil
}

func (m *mockArchaeoAccess) ActivePlan(context.Context, string) (*execution.ActivePlanView, error) {
	return m.active, nil
}

func (m *mockArchaeoAccess) LearningQueue(context.Context, string) (*execution.LearningQueueView, error) {
	return m.learning, nil
}

func (m *mockArchaeoAccess) TensionsByWorkflow(context.Context, string) ([]execution.TensionView, error) {
	return append([]execution.TensionView(nil), m.tensions...), nil
}

func (m *mockArchaeoAccess) TensionSummaryByWorkflow(context.Context, string) (*execution.TensionSummaryView, error) {
	return m.summary, nil
}

func (m *mockArchaeoAccess) PlanVersions(context.Context, string) ([]execution.VersionedPlanView, error) {
	return nil, nil
}

func (m *mockArchaeoAccess) ActivePlanVersion(context.Context, string) (*execution.VersionedPlanView, error) {
	return m.activated, nil
}

func (m *mockArchaeoAccess) DraftPlanVersion(_ context.Context, plan *frameworkplan.LivingPlan, input execution.DraftPlanInput) (*execution.VersionedPlanView, error) {
	m.drafted = &execution.VersionedPlanView{
		ID:                     "version-1",
		WorkflowID:             input.WorkflowID,
		PlanID:                 plan.ID,
		Version:                1,
		Status:                 "draft",
		DerivedFromExploration: input.DerivedFromExploration,
		BasedOnRevision:        input.BasedOnRevision,
		SemanticSnapshotRef:    input.SemanticSnapshotRef,
		PatternRefs:            append([]string(nil), input.PatternRefs...),
		TensionRefs:            append([]string(nil), input.TensionRefs...),
		AnchorRefs:             append([]string(nil), input.AnchorRefs...),
		Plan:                   *plan,
	}
	return m.drafted, nil
}

func (m *mockArchaeoAccess) ActivatePlanVersion(_ context.Context, workflowID string, version int) (*execution.VersionedPlanView, error) {
	if m.drafted == nil {
		return nil, nil
	}
	record := *m.drafted
	record.WorkflowID = workflowID
	record.Version = version
	record.Status = "active"
	m.activated = &record
	return m.activated, nil
}

func TestEnrichArchaeoExecutionInputUsesServiceBundle(t *testing.T) {
	mock := &mockArchaeoAccess{
		requests: &execution.RequestHistoryView{
			WorkflowID: "wf-1",
			Requests: []execution.RequestRecordView{
				{ID: "req-1"},
			},
		},
		active: &execution.ActivePlanView{
			WorkflowID: "wf-1",
			ActivePlan: &execution.VersionedPlanView{
				PlanID:                 "plan-1",
				Version:                3,
				PatternRefs:            []string{"pattern:a"},
				AnchorRefs:             []string{"anchor:a"},
				TensionRefs:            []string{"tension:from-plan"},
				BasedOnRevision:        "rev-1",
				SemanticSnapshotRef:    "snapshot-1",
				DerivedFromExploration: "explore-1",
			},
		},
		learning: &execution.LearningQueueView{
			WorkflowID: "wf-1",
			PendingLearning: []execution.LearningInteractionView{
				{ID: "learn-1"},
			},
		},
		tensions: []execution.TensionView{
			{ID: "tension-1", PatternIDs: []string{"pattern:b"}, AnchorRefs: []string{"anchor:b"}},
		},
		summary: &execution.TensionSummaryView{WorkflowID: "wf-1", Total: 1, Active: 1},
	}
	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{Archaeo: mock},
		Work: eucloruntime.UnitOfWork{
			WorkflowID: "wf-1",
		},
	}

	enriched := enrichArchaeoExecutionInput(context.Background(), in)
	if len(enriched.patternRefs) != 2 {
		t.Fatalf("expected enriched pattern refs, got %#v", enriched.patternRefs)
	}
	if len(enriched.tensionIDs) != 2 {
		t.Fatalf("expected tension refs from plan+tensions, got %#v", enriched.tensionIDs)
	}
	if len(enriched.learningRefs) != 1 || enriched.learningRefs[0] != "learn-1" {
		t.Fatalf("expected learning refs, got %#v", enriched.learningRefs)
	}
	if enriched.semanticSnapshotRef != "snapshot-1" {
		t.Fatalf("expected semantic snapshot ref to propagate, got %q", enriched.semanticSnapshotRef)
	}
}

// TestLivingPlanStepFromStateFound verifies that a step stored in state is
// returned correctly by reference when queried by ID.
func TestLivingPlanStepFromStateFound(t *testing.T) {
	state := core.NewContext()
	plan := &frameworkplan.LivingPlan{
		ID: "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "first step", ConfidenceScore: 0.8},
		},
	}
	state.Set("euclo.living_plan", plan)

	step := livingPlanStepFromState(state, "step-1")
	if step == nil {
		t.Fatal("expected step to be found, got nil")
	}
	if step.ID != "step-1" {
		t.Fatalf("expected step-1, got %q", step.ID)
	}
	if step.ConfidenceScore != 0.8 {
		t.Fatalf("expected confidence 0.8, got %f", step.ConfidenceScore)
	}
}

// TestLivingPlanStepFromStateNotFound verifies that a missing step ID returns nil.
func TestLivingPlanStepFromStateNotFound(t *testing.T) {
	state := core.NewContext()
	plan := &frameworkplan.LivingPlan{
		ID:    "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{},
	}
	state.Set("euclo.living_plan", plan)

	step := livingPlanStepFromState(state, "nonexistent-step")
	if step != nil {
		t.Fatalf("expected nil for missing step, got %+v", step)
	}
}

// TestLivingPlanStepFromStateNilState verifies that a nil state returns nil.
func TestLivingPlanStepFromStateNilState(t *testing.T) {
	step := livingPlanStepFromState(nil, "step-1")
	if step != nil {
		t.Fatalf("expected nil for nil state, got %+v", step)
	}
}

// TestBlockingLearningIDsFromRoutineArtifactsNone verifies that an empty
// artifact slice returns nil.
func TestBlockingLearningIDsFromRoutineArtifactsNone(t *testing.T) {
	ids := blockingLearningIDsFromRoutineArtifacts(nil)
	if ids != nil {
		t.Fatalf("expected nil, got %v", ids)
	}
}

// TestBlockingLearningIDsFromRoutineArtifactsNoBlocking verifies that a
// convergence-guard artifact with an empty blocking list returns nil.
func TestBlockingLearningIDsFromRoutineArtifactsNoBlocking(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{
			ProducerID: ConvergenceGuard,
			Payload: map[string]any{
				"learning_queue": map[string]any{
					"blocking": []string{},
					"count":    0,
				},
			},
		},
	}
	ids := blockingLearningIDsFromRoutineArtifacts(artifacts)
	if ids != nil {
		t.Fatalf("expected nil for empty blocking list, got %v", ids)
	}
}

// TestBlockingLearningIDsFromRoutineArtifactsWithBlocking verifies that
// blocking IDs are extracted from the convergence-guard artifact payload.
func TestBlockingLearningIDsFromRoutineArtifactsWithBlocking(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{
			ProducerID: "some-other-routine",
			Payload: map[string]any{
				"learning_queue": map[string]any{
					"blocking": []string{"learn-A"},
				},
			},
		},
		{
			ProducerID: ConvergenceGuard,
			Payload: map[string]any{
				"learning_queue": map[string]any{
					"blocking": []string{"learn-1", "learn-2"},
					"count":    2,
				},
			},
		},
	}
	ids := blockingLearningIDsFromRoutineArtifacts(artifacts)
	if len(ids) != 2 {
		t.Fatalf("expected 2 blocking IDs, got %v", ids)
	}
	if ids[0] != "learn-1" || ids[1] != "learn-2" {
		t.Fatalf("unexpected blocking IDs: %v", ids)
	}
}

// TestRecordStepAttemptCompleted verifies that a completed attempt is appended
// to the living plan step's history and its status advances to completed.
func TestRecordStepAttemptCompleted(t *testing.T) {
	store := newMockPlanStore()
	state := core.NewContext()
	plan := &frameworkplan.LivingPlan{
		ID: "plan-rc",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:              "step-1",
				ConfidenceScore: 0.9,
				Status:          frameworkplan.PlanStepPending,
			},
		},
	}
	state.Set("euclo.living_plan", plan)

	in := execution.ExecuteInput{
		State: state,
		Work: eucloruntime.UnitOfWork{
			WorkflowID: "wf-rc",
			PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
				PlanID: "plan-rc",
			},
		},
		ServiceBundle: execution.ServiceBundle{PlanStore: store},
	}

	recordStepAttempt(context.Background(), in, "step-1", "completed", "", "git:abc1234")

	if store.updatedStep == nil {
		t.Fatal("expected PlanStore.UpdateStep to be called")
	}
	if store.updatedStep.Status != frameworkplan.PlanStepCompleted {
		t.Fatalf("expected status completed, got %q", store.updatedStep.Status)
	}
	if len(store.updatedStep.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(store.updatedStep.History))
	}
	attempt := store.updatedStep.History[0]
	if attempt.Outcome != "completed" {
		t.Fatalf("expected outcome completed, got %q", attempt.Outcome)
	}
	if attempt.GitCheckpoint != "git:abc1234" {
		t.Fatalf("expected git checkpoint, got %q", attempt.GitCheckpoint)
	}
}

// TestRecordStepAttemptFailed verifies that a failed attempt records the
// failure reason and sets step status to failed.
func TestRecordStepAttemptFailed(t *testing.T) {
	store := newMockPlanStore()
	state := core.NewContext()
	plan := &frameworkplan.LivingPlan{
		ID: "plan-rf",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-2": {ID: "step-2", Status: frameworkplan.PlanStepPending},
		},
	}
	state.Set("euclo.living_plan", plan)

	in := execution.ExecuteInput{
		State: state,
		Work: eucloruntime.UnitOfWork{
			PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{PlanID: "plan-rf"},
		},
		ServiceBundle: execution.ServiceBundle{PlanStore: store},
	}

	recordStepAttempt(context.Background(), in, "step-2", "failed", "recipe returned nil", "")

	if store.updatedStep == nil {
		t.Fatal("expected PlanStore.UpdateStep to be called")
	}
	if store.updatedStep.Status != frameworkplan.PlanStepFailed {
		t.Fatalf("expected status failed, got %q", store.updatedStep.Status)
	}
	attempt := store.updatedStep.History[0]
	if attempt.FailureReason != "recipe returned nil" {
		t.Fatalf("expected failure reason, got %q", attempt.FailureReason)
	}
}

// TestRecordStepAttemptNoPlanStore verifies that recordStepAttempt is a no-op
// when no PlanStore is configured.
func TestRecordStepAttemptNoPlanStore(t *testing.T) {
	in := execution.ExecuteInput{
		Work: eucloruntime.UnitOfWork{
			PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{PlanID: "plan-x"},
		},
	}
	// Must not panic.
	recordStepAttempt(context.Background(), in, "step-x", "completed", "", "")
}

// TestSubmitPlanReviewGuidanceNoOpenQuestions verifies that submitPlanReviewGuidance
// is a no-op when the review result has no open_questions.
func TestSubmitPlanReviewGuidanceNoOpenQuestions(t *testing.T) {
	broker := guidance.NewGuidanceBroker(time.Second)
	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{GuidanceBroker: broker},
	}
	reviewResult := &core.Result{
		Success: true,
		Data:    map[string]any{"summary": "looks good"},
	}
	// Must not panic and must not emit a guidance request.
	submitPlanReviewGuidance(in, "plan-1", reviewResult)

	pending := broker.PendingRequests()
	if len(pending) != 0 {
		t.Fatalf("expected no pending guidance requests, got %d", len(pending))
	}
}

// TestSubmitPlanReviewGuidanceWithOpenQuestions verifies that a GuidanceRequest
// is submitted when open_questions are present in the review result.
func TestSubmitPlanReviewGuidanceWithOpenQuestions(t *testing.T) {
	broker := guidance.NewGuidanceBroker(time.Second)
	in := execution.ExecuteInput{
		Work:          eucloruntime.UnitOfWork{WorkflowID: "wf-sq"},
		ServiceBundle: execution.ServiceBundle{GuidanceBroker: broker},
	}
	reviewResult := &core.Result{
		Success: true,
		Data: map[string]any{
			"summary":        "review found issues",
			"open_questions": []string{"Is auth scope sufficient?", "What rollback plan?"},
		},
	}

	submitPlanReviewGuidance(in, "plan-sq", reviewResult)

	pending := broker.PendingRequests()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending guidance request, got %d", len(pending))
	}
	if pending[0].Kind != guidance.GuidanceAmbiguity {
		t.Fatalf("expected GuidanceAmbiguity, got %q", pending[0].Kind)
	}
}

func TestPersistCompiledPlanPersistsActivatedVersion(t *testing.T) {
	mock := &mockArchaeoAccess{}
	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{Archaeo: mock},
		Work: eucloruntime.UnitOfWork{
			WorkflowID: "wf-2",
			SemanticInputs: eucloruntime.SemanticInputBundle{
				ExplorationID: "explore-2",
				PatternRefs:   []string{"pattern:a"},
				TensionRefs:   []string{"tension:a"},
			},
		},
	}
	payload := map[string]any{
		"title":   "Compile plan",
		"summary": "compile plan summary",
		"steps": []map[string]any{
			{"id": "step-1", "description": "first step", "scope": []string{"symbol:a"}},
			{"id": "step-2", "description": "second step", "depends_on": []string{"step-1"}},
		},
	}

	versioned, err := persistCompiledPlan(context.Background(), in, payload, enrichedArchaeoInput{
		basedOnRevision:     "rev-2",
		semanticSnapshotRef: "snapshot-2",
		anchorRefs:          []string{"anchor:a"},
	})
	if err != nil {
		t.Fatalf("persistCompiledPlan returned error: %v", err)
	}
	if versioned == nil || versioned.Status != "active" {
		t.Fatalf("expected active versioned plan, got %#v", versioned)
	}
	if versioned.Plan.WorkflowID != "wf-2" {
		t.Fatalf("expected persisted workflow id, got %q", versioned.Plan.WorkflowID)
	}
	if len(versioned.Plan.StepOrder) != 2 {
		t.Fatalf("expected two plan steps, got %#v", versioned.Plan.StepOrder)
	}
}
