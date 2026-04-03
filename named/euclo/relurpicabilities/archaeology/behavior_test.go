package archaeology

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
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

func TestPersistExplorationPatternsSavesRecordsAndQueuesLearning(t *testing.T) {
	db, err := patterns.OpenSQLite(filepath.Join(t.TempDir(), "patterns.db"))
	if err != nil {
		t.Fatalf("open pattern db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	patternStore, err := patterns.NewSQLitePatternStore(db)
	if err != nil {
		t.Fatalf("new pattern store: %v", err)
	}
	broker := archaeolearning.NewBroker(time.Minute)
	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{
			PatternStore:   patternStore,
			LearningBroker: broker,
		},
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-patterns",
			PrimaryRelurpicCapabilityID: Explore,
		},
	}
	artifacts := []euclotypes.Artifact{{
		ID:   "explore-artifact",
		Kind: euclotypes.ArtifactKindExplore,
		Payload: map[string]any{
			"patterns": []any{
				map[string]any{
					"name":      "Service Layer",
					"summary":   "Services coordinate storage and providers.",
					"relevance": 0.82,
					"files":     []any{"service.go", "runtime.go"},
					"kind":      "architectural",
				},
			},
		},
	}}

	persistExplorationPatterns(context.Background(), in, artifacts)

	records, err := patternStore.ListByStatus(context.Background(), patterns.PatternStatusProposed, "wf-patterns")
	if err != nil {
		t.Fatalf("list proposed patterns: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one proposed pattern, got %d", len(records))
	}
	if records[0].Title != "Service Layer" {
		t.Fatalf("expected persisted title from pattern name, got %q", records[0].Title)
	}
	if len(records[0].Instances) != 2 {
		t.Fatalf("expected file instances to persist, got %#v", records[0].Instances)
	}
	pending := broker.PendingInteractions()
	if len(pending) != 1 || pending[0].SubjectID != records[0].ID {
		t.Fatalf("expected learning interaction for persisted pattern, got %#v", pending)
	}
}

func TestCompilePlanReviewPersistsCommentAndGuidance(t *testing.T) {
	db, err := patterns.OpenSQLite(filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatalf("open comment db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	commentStore, err := patterns.NewSQLiteCommentStore(db)
	if err != nil {
		t.Fatalf("new comment store: %v", err)
	}
	guidanceBroker := guidance.NewGuidanceBroker(time.Minute)
	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{
			CommentStore:   commentStore,
			GuidanceBroker: guidanceBroker,
		},
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-review",
			PrimaryRelurpicCapabilityID: CompilePlan,
		},
	}
	reviewResult := &core.Result{
		Success: true,
		Data: map[string]any{
			"open_questions": []any{"Should the migration be staged?"},
		},
	}
	reviewResult.Data["summary"] = "compile review surfaced one open question"

	persistPlanReviewComment(context.Background(), in, "plan-review-1", reviewResult)
	comments, err := commentStore.ListForPattern(context.Background(), "plan-review-1")
	if err != nil {
		t.Fatalf("list plan comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected one plan review comment, got %d", len(comments))
	}

	submitPlanReviewGuidance(in, "plan-review-1", reviewResult)
	pending := guidanceBroker.PendingRequests()
	if len(pending) != 1 {
		t.Fatalf("expected one pending guidance request, got %d", len(pending))
	}
	if pending[0].Kind != guidance.GuidanceAmbiguity {
		t.Fatalf("expected ambiguity guidance, got %q", pending[0].Kind)
	}
}

func TestCompilePlanBehaviorExecutesOfflineWithScenarioStubModel(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Reconcile surfaced patterns"},
			Response:       &core.LLMResponse{Text: `{"goal":"reconcile","steps":[],"dependencies":{},"files":[]}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Shape a full executable implementation plan"},
			Response:       &core.LLMResponse{Text: `{"goal":"shape","steps":[{"id":"step-1","description":"Implement the compiled direction","tool":"file_read","params":{"path":"README.md"},"expected":"implementation plan drafted","verification":"review the draft","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"review delegate finished"}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Respond JSON"},
			Response:       &core.LLMResponse{Text: `{"issues":[],"approve":true}`},
		},
	)

	env := testutil.Env(t)
	env.Model = model
	env.Config.Model = "scenario-stub"
	env.Config.MaxIterations = 1

	mock := &mockArchaeoAccess{}
	state := core.NewContext()
	state.Set("pipeline.plan", map[string]any{
		"title":   "Compiled archaeology plan",
		"summary": "compiled executable plan",
		"steps": []map[string]any{
			{"id": "step-1", "description": "Implement the compiled direction", "scope": []string{"pkg/service.go"}},
		},
	})

	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "task-phase4-compile",
			Instruction: "Compile an archaeology-grounded implementation plan",
			Context: map[string]any{
				"workspace": ".",
			},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-phase4-compile",
			PrimaryRelurpicCapabilityID: CompilePlan,
			SemanticInputs: eucloruntime.SemanticInputBundle{
				ExplorationID: "explore-1",
				PatternRefs:   []string{"pattern:a"},
				TensionRefs:   []string{"tension:a"},
			},
		},
		ServiceBundle: execution.ServiceBundle{Archaeo: mock},
	}

	result, err := NewCompilePlanBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("compile-plan behavior returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful compile-plan result, got %+v", result)
	}
	if mock.activated == nil || mock.activated.Status != "active" {
		t.Fatalf("expected activated plan version, got %#v", mock.activated)
	}
	raw, ok := state.Get("euclo.active_plan_version")
	if !ok || raw == nil {
		t.Fatalf("expected active plan version in state")
	}
	payload := planArtifactFromState(state)
	if !compiledPlanReady(payload) {
		t.Fatalf("expected compiled plan payload in state, got %#v", payload)
	}
	if got := strings.TrimSpace(state.GetString("euclo.active_exploration_id")); got != "explore-1" {
		t.Fatalf("expected active exploration id to be seeded, got %q", got)
	}
	if got := strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id")); got == "" {
		t.Fatal("expected active exploration snapshot id to be seeded")
	}
	model.AssertExhausted(t)
}

func TestExploreBehaviorExecutesOfflineWithScenarioStubModel(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:patterns","value":[{"name":"Adapter","summary":"wraps external behavior","files":["pkg/service.go"],"relevance":0.8}]}]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:prospectives","value":[{"title":"Split transport boundary","summary":"separate transport and domain coordination","tradeoffs":["more files"],"confidence":0.7}]}]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:coherence_assessment","value":{"status":"coherent","notes":["patterns align"],"risks":["migration cost"]}}]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:convergence_assessment","value":{"status":"ready","recommended_direction":"split transport boundary","open_questions":["How to migrate callers?"]}}]}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Shape the exploration findings into candidate engineering directions"},
			Response:       &core.LLMResponse{Text: `{"goal":"explore","steps":[{"id":"step-1","description":"Investigate migration seams","tool":"file_read","params":{"path":"pkg/service.go"},"expected":"candidate direction identified","verification":"review the candidate","files":["pkg/service.go"]}],"dependencies":{},"files":["pkg/service.go"]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"review delegate finished"}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Respond JSON"},
			Response:       &core.LLMResponse{Text: `{"issues":[],"approve":true}`},
		},
	)

	env := testutil.Env(t)
	env.Model = model
	env.Config.Model = "scenario-stub"
	env.Config.MaxIterations = 1

	mock := &mockArchaeoAccess{
		requests: &execution.RequestHistoryView{
			WorkflowID: "wf-phase4-explore",
			Requests:   []execution.RequestRecordView{{ID: "req-1"}},
		},
		active: &execution.ActivePlanView{
			WorkflowID: "wf-phase4-explore",
			ActivePlan: &execution.VersionedPlanView{
				PlanID:                 "plan-1",
				Version:                2,
				PatternRefs:            []string{"pattern:a"},
				TensionRefs:            []string{"tension:a"},
				BasedOnRevision:        "rev-1",
				SemanticSnapshotRef:    "snapshot-1",
				DerivedFromExploration: "explore-1",
			},
		},
		learning: &execution.LearningQueueView{
			WorkflowID:      "wf-phase4-explore",
			PendingLearning: []execution.LearningInteractionView{{ID: "learn-1"}},
		},
		tensions: []execution.TensionView{
			{ID: "tension-1", PatternIDs: []string{"pattern:b"}, Description: "boundary mismatch", Status: "unresolved"},
		},
		summary: &execution.TensionSummaryView{WorkflowID: "wf-phase4-explore", Total: 1, Active: 1, Unresolved: 1},
	}

	state := core.NewContext()
	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "task-phase4-explore",
			Instruction: "Explore archaeology-grounded candidate directions",
			Context: map[string]any{
				"workspace":    ".",
				"workflow_id":  "wf-phase4-explore",
				"corpus_scope": "workspace",
			},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                      "wf-phase4-explore",
			PrimaryRelurpicCapabilityID:     Explore,
			SupportingRelurpicCapabilityIDs: []string{PatternSurface, ProspectiveAssess, ConvergenceGuard, CoherenceAssess},
			SemanticInputs: eucloruntime.SemanticInputBundle{
				WorkflowID:            "wf-phase4-explore",
				ExplorationID:         "explore-1",
				PatternRefs:           []string{"pattern:a"},
				TensionRefs:           []string{"tension:a"},
				RequestProvenanceRefs: []string{"req-1"},
			},
		},
		ServiceBundle: execution.ServiceBundle{Archaeo: mock},
	}

	result, err := NewExploreBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("explore behavior returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful explore result, got %+v", result)
	}
	if _, ok := state.Get("pipeline.explore"); !ok {
		t.Fatalf("expected pipeline.explore in state")
	}
	if _, ok := state.Get("euclo.plan_candidates"); !ok {
		t.Fatalf("expected euclo.plan_candidates in state")
	}
	if got := strings.TrimSpace(state.GetString("euclo.active_exploration_id")); got != "explore-1" {
		t.Fatalf("expected active exploration id to be seeded, got %q", got)
	}
	if got := strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id")); got == "" {
		t.Fatal("expected active exploration snapshot id to be seeded")
	}
	artifacts := euclotypes.ArtifactStateFromContext(state).All()
	if len(artifacts) == 0 {
		t.Fatalf("expected produced artifacts in state")
	}
	model.AssertExhausted(t)
}

func TestImplementPlanBehaviorBlocksOnLearningGateOffline(t *testing.T) {
	env := testutil.Env(t)
	state := core.NewContext()
	state.Set("pipeline.plan", map[string]any{
		"title":   "Compiled archaeology plan",
		"summary": "compiled executable plan",
		"steps": []map[string]any{
			{"id": "step-1", "description": "Implement the compiled direction", "scope": []string{"pkg/service.go"}},
		},
	})

	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "task-phase4-implement",
			Instruction: "Implement an archaeology-grounded plan",
			Context: map[string]any{
				"workspace":    ".",
				"workflow_id":  "wf-phase4-implement",
				"corpus_scope": "workspace",
			},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                      "wf-phase4-implement",
			PrimaryRelurpicCapabilityID:     ImplementPlan,
			SupportingRelurpicCapabilityIDs: []string{ConvergenceGuard},
			SemanticInputs: eucloruntime.SemanticInputBundle{
				WorkflowID:    "wf-phase4-implement",
				ExplorationID: "explore-1",
				PatternRefs:   []string{"pattern:a"},
				TensionRefs:   []string{"tension:a"},
			},
		},
		RunSupportingRoutine: func(ctx context.Context, routineID string, task *core.Task, state *core.Context, work eucloruntime.UnitOfWork, env agentenv.AgentEnvironment, bundle execution.ServiceBundle) ([]euclotypes.Artifact, error) {
			if routineID != ConvergenceGuard {
				return nil, nil
			}
			return []euclotypes.Artifact{{
				ID:         "archaeology_convergence_guard",
				Kind:       euclotypes.ArtifactKindPlanCandidates,
				Summary:    "blocking learning present",
				ProducerID: ConvergenceGuard,
				Status:     "produced",
				Payload: map[string]any{
					"learning_queue": map[string]any{
						"blocking": []string{"learn-1", "learn-2"},
						"count":    2,
					},
				},
			}}, nil
		},
	}

	result, err := NewImplementPlanBehavior().Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected blocking learning gate error")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed implement-plan result, got %+v", result)
	}
	raw, ok := state.Get("euclo.deferred_execution_issues")
	if !ok || raw == nil {
		t.Fatalf("expected deferred execution issues in state")
	}
	issues, ok := raw.([]eucloruntime.DeferredExecutionIssue)
	if !ok || len(issues) != 1 {
		t.Fatalf("expected one deferred execution issue, got %#v", raw)
	}
	if issues[0].RecommendedReentry != "archaeology" {
		t.Fatalf("expected archaeology reentry, got %q", issues[0].RecommendedReentry)
	}
	if got := strings.TrimSpace(state.GetString("euclo.active_exploration_id")); got != "explore-1" {
		t.Fatalf("expected active exploration id to be seeded, got %q", got)
	}
	if got := strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id")); got == "" {
		t.Fatal("expected active exploration snapshot id to be seeded")
	}
}

func TestEnrichedArchaeoInputPayloadCapturesTensionsAndSummary(t *testing.T) {
	e := enrichedArchaeoInput{
		patternRefs: []string{"p-a", "p-b"},
		tensionIDs:  []string{"t-1", "t-2"},
		tensions: []execution.TensionView{
			{ID: "t-1", Description: "API drift", PatternIDs: []string{"p-a"}, AnchorRefs: []string{"a1"}},
			{ID: "t-2", Description: "Conflicting module boundaries", PatternIDs: []string{"p-b"}},
		},
		tensionSummary: &execution.TensionSummaryView{WorkflowID: "wf-x", Total: 2, Active: 2, Unresolved: 1},
		explorationID:  "ex-1",
	}
	payload := e.payload()
	if payload["tension_summary"] == nil {
		t.Fatal("expected tension_summary in payload")
	}
	tensions, ok := payload["tensions"].([]execution.TensionView)
	if !ok || len(tensions) != 2 {
		t.Fatalf("expected tension views, got %#v", payload["tensions"])
	}
	summary := e.summary()
	if !strings.Contains(summary, "tension") || !strings.Contains(summary, "pattern") {
		t.Fatalf("expected summary to reference patterns and tensions, got %q", summary)
	}
}

func TestPersistCompiledPlanMergesConflictingTensionRefsUniquely(t *testing.T) {
	mock := &mockArchaeoAccess{}
	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{Archaeo: mock},
		Work: eucloruntime.UnitOfWork{
			WorkflowID: "wf-merge",
			SemanticInputs: eucloruntime.SemanticInputBundle{
				TensionRefs: []string{"t-user", "t-shared"},
			},
		},
	}
	payload := map[string]any{
		"title": "Plan", "summary": "s",
		"steps": []map[string]any{{"id": "step-1", "description": "do work", "scope": []string{"pkg/x"}}},
	}
	enr := enrichedArchaeoInput{
		tensionIDs: []string{"t-shared", "t-archaeo-only"},
	}
	versioned, err := persistCompiledPlan(context.Background(), in, payload, enr)
	if err != nil {
		t.Fatalf("persistCompiledPlan: %v", err)
	}
	if versioned == nil || mock.drafted == nil {
		t.Fatalf("expected drafted plan, got versioned=%v drafted=%v", versioned, mock.drafted)
	}
	refs := mock.drafted.TensionRefs
	if len(refs) != 3 {
		t.Fatalf("expected 3 unique tension refs merged from work+enriched, got %#v", refs)
	}
}

func TestScopeExpansionRoutineProducesContextExpansionArtifact(t *testing.T) {
	r := scopeExpandRoutine{}
	artifacts, err := r.Execute(context.Background(), euclorelurpic.RoutineInput{
		Task:  &core.Task{Context: map[string]any{"workflow_id": "wf-scope"}},
		State: core.NewContext(),
		Work: euclorelurpic.WorkContext{
			PatternRefs: []string{"pattern:auth", "pattern:data"},
		},
	})
	if err != nil {
		t.Fatalf("scope expansion routine: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != euclotypes.ArtifactKindContextExpansion {
		t.Fatalf("unexpected artifacts: %#v", artifacts)
	}
	payload, _ := artifacts[0].Payload.(map[string]any)
	if payload == nil || payload["operation"] != ScopeExpansionAssess {
		t.Fatalf("unexpected payload %#v", artifacts[0].Payload)
	}
}
