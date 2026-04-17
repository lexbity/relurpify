package archaeology

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestScenario_ExploreBehavior_OfflineScenarioModel(t *testing.T) {
	TestExploreBehaviorExecutesOfflineWithScenarioStubModel(t)
}

func TestScenario_CompilePlanBehavior_OfflineScenarioModel(t *testing.T) {
	TestCompilePlanBehaviorExecutesOfflineWithScenarioStubModel(t)
}

func TestScenario_ImplementPlanBehavior_BlockingLearningGate(t *testing.T) {
	TestImplementPlanBehaviorBlocksOnLearningGateOffline(t)
}

func TestScenario_ExploreBehavior_PersistsPatternsAndQueuesLearning(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-scenario-patterns", "persist exploration patterns through fixture stores")
	broker := archaeolearning.NewBroker(time.Minute)

	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{
			PatternStore:   f.PatternStore,
			LearningBroker: broker,
		},
		Work: eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-scenario-patterns",
			PrimaryRelurpicCapabilityID: Explore},
		},
	}
	artifacts := []euclotypes.Artifact{{
		ID:   "scenario-explore-artifact",
		Kind: euclotypes.ArtifactKindExplore,
		Payload: map[string]any{
			"patterns": []any{
				map[string]any{
					"name":      "Boundary Wrapper",
					"summary":   "The service layer wraps cross-boundary coordination.",
					"files":     []any{"service.go", "runtime.go"},
					"relevance": 0.91,
					"kind":      "boundary",
				},
			},
		},
	}}

	persistExplorationPatterns(context.Background(), in, artifacts)

	records, err := f.PatternStore.ListByStatus(context.Background(), patterns.PatternStatusProposed, "wf-scenario-patterns")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one proposed pattern, got %d", len(records))
	}
	if records[0].Title != "Boundary Wrapper" {
		t.Fatalf("unexpected persisted pattern title: %q", records[0].Title)
	}
	if len(records[0].Instances) != 2 {
		t.Fatalf("expected two pattern instances, got %#v", records[0].Instances)
	}
	pending := broker.PendingInteractions()
	if len(pending) != 1 {
		t.Fatalf("expected one pending learning interaction, got %d", len(pending))
	}
	if pending[0].SubjectID != records[0].ID {
		t.Fatalf("expected learning interaction subject %q, got %q", records[0].ID, pending[0].SubjectID)
	}
}

func TestScenario_CompilePlanReview_PersistsCommentAndQueuesGuidance(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-scenario-review", "persist compile-plan review side effects")
	broker := guidance.NewGuidanceBroker(time.Minute)

	in := execution.ExecuteInput{
		ServiceBundle: execution.ServiceBundle{
			CommentStore:   f.CommentStore,
			GuidanceBroker: broker,
		},
		Work: eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-scenario-review",
			PrimaryRelurpicCapabilityID: CompilePlan},
		},
	}
	reviewResult := &core.Result{
		Success: true,
		Data: map[string]any{
			"summary":        "compile review surfaced follow-up questions",
			"open_questions": []string{"Should step 2 be split?", "What is the rollback boundary?"},
		},
	}

	persistPlanReviewComment(context.Background(), in, "plan-scenario-review", reviewResult)

	comments, err := f.CommentStore.ListForPattern(context.Background(), "plan-scenario-review")
	if err != nil {
		t.Fatalf("ListForPattern: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected one persisted plan review comment, got %d", len(comments))
	}
	if comments[0].IntentType != patterns.CommentOpenQuestion {
		t.Fatalf("expected open-question intent, got %q", comments[0].IntentType)
	}

	submitPlanReviewGuidance(in, "plan-scenario-review", reviewResult)
	pending := broker.PendingRequests()
	if len(pending) != 1 {
		t.Fatalf("expected one pending guidance request, got %d", len(pending))
	}
	if pending[0].Context["plan_id"] != "plan-scenario-review" {
		t.Fatalf("expected plan id in guidance context, got %#v", pending[0].Context)
	}
}

type scenarioGitTool struct{}

type fixtureArchaeoAccess struct {
	fixture *testscenario.Fixture
}

func (a fixtureArchaeoAccess) RequestHistory(context.Context, string) (*execution.RequestHistoryView, error) {
	return nil, nil
}

func (a fixtureArchaeoAccess) ActivePlan(context.Context, string) (*execution.ActivePlanView, error) {
	return nil, nil
}

func (a fixtureArchaeoAccess) LearningQueue(context.Context, string) (*execution.LearningQueueView, error) {
	return nil, nil
}

func (a fixtureArchaeoAccess) TensionsByWorkflow(context.Context, string) ([]execution.TensionView, error) {
	return nil, nil
}

func (a fixtureArchaeoAccess) TensionSummaryByWorkflow(context.Context, string) (*execution.TensionSummaryView, error) {
	return nil, nil
}

func (a fixtureArchaeoAccess) PlanVersions(ctx context.Context, workflowID string) ([]execution.VersionedPlanView, error) {
	if a.fixture == nil {
		return nil, nil
	}
	versions, err := a.fixture.PlansService().ListVersions(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]execution.VersionedPlanView, 0, len(versions))
	for _, version := range versions {
		out = append(out, adaptScenarioVersionedPlan(version))
	}
	return out, nil
}

func (a fixtureArchaeoAccess) ActivePlanVersion(ctx context.Context, workflowID string) (*execution.VersionedPlanView, error) {
	if a.fixture == nil {
		return nil, nil
	}
	active, err := a.fixture.PlansService().LoadActiveVersion(ctx, workflowID)
	if err != nil || active == nil {
		return nil, err
	}
	view := adaptScenarioVersionedPlan(*active)
	return &view, nil
}

func (a fixtureArchaeoAccess) DraftPlanVersion(context.Context, *frameworkplan.LivingPlan, execution.DraftPlanInput) (*execution.VersionedPlanView, error) {
	return nil, nil
}

func (a fixtureArchaeoAccess) ActivatePlanVersion(context.Context, string, int) (*execution.VersionedPlanView, error) {
	return nil, nil
}

func adaptScenarioVersionedPlan(record archaeodomain.VersionedLivingPlan) execution.VersionedPlanView {
	plan := record.Plan
	if len(plan.StepOrder) == 0 && len(plan.Steps) > 0 {
		plan.StepOrder = make([]string, 0, len(plan.Steps))
		for stepID := range plan.Steps {
			plan.StepOrder = append(plan.StepOrder, stepID)
		}
		sort.Strings(plan.StepOrder)
	}
	return execution.VersionedPlanView{
		ID:                     record.ID,
		WorkflowID:             record.WorkflowID,
		PlanID:                 plan.ID,
		Version:                record.Version,
		Status:                 string(record.Status),
		DerivedFromExploration: record.DerivedFromExploration,
		BasedOnRevision:        record.BasedOnRevision,
		SemanticSnapshotRef:    record.SemanticSnapshotRef,
		PatternRefs:            append([]string(nil), record.PatternRefs...),
		AnchorRefs:             append([]string(nil), record.AnchorRefs...),
		TensionRefs:            append([]string(nil), record.TensionRefs...),
		Plan:                   plan,
	}
}

func (scenarioGitTool) Name() string { return "cli_git" }
func (scenarioGitTool) Description() string {
	return "executes git commands for archaeology scenario tests"
}
func (scenarioGitTool) Category() string { return "scm" }
func (scenarioGitTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "args", Type: "array", Required: true},
		{Name: "working_directory", Type: "string", Required: true},
	}
}
func (scenarioGitTool) Execute(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	rawArgs, _ := args["args"].([]string)
	if len(rawArgs) == 0 {
		if list, ok := args["args"].([]any); ok {
			rawArgs = make([]string, 0, len(list))
			for _, item := range list {
				rawArgs = append(rawArgs, fmt.Sprint(item))
			}
		}
	}
	workingDir := fmt.Sprint(args["working_directory"])
	cmd := exec.CommandContext(ctx, "git", rawArgs...)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &core.ToolResult{
			Success: false,
			Error:   err.Error(),
			Data:    map[string]interface{}{"stdout": string(output), "stderr": string(output)},
		}, nil
	}
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"stdout": string(output)}}, nil
}
func (scenarioGitTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (scenarioGitTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (scenarioGitTool) Tags() []string                                  { return []string{core.TagExecute, core.TagDestructive} }

func seedImplementPlanGraph(t *testing.T, f *testscenario.Fixture, root string, affected ...string) {
	t.Helper()
	if err := f.Graph.UpsertNode(graphdb.NodeRecord{ID: root, Kind: graphdb.NodeKind("symbol"), SourceID: f.Workspace}); err != nil {
		t.Fatalf("upsert root graph node: %v", err)
	}
	for _, nodeID := range affected {
		if err := f.Graph.UpsertNode(graphdb.NodeRecord{ID: nodeID, Kind: graphdb.NodeKind("symbol"), SourceID: f.Workspace}); err != nil {
			t.Fatalf("upsert affected graph node: %v", err)
		}
		if err := f.Graph.Link(root, nodeID, graphdb.EdgeKind("depends_on"), "", 1, nil); err != nil {
			t.Fatalf("link graph edge: %v", err)
		}
	}
}

func scenarioPlanPayload(plan *frameworkplan.LivingPlan) map[string]any {
	steps := make([]map[string]any, 0, len(plan.StepOrder))
	for _, stepID := range plan.StepOrder {
		step := plan.Steps[stepID]
		if step == nil {
			continue
		}
		steps = append(steps, map[string]any{
			"id":               step.ID,
			"description":      step.Description,
			"scope":            append([]string(nil), step.Scope...),
			"anchor_deps":      append([]string(nil), step.AnchorDependencies...),
			"confidence_score": step.ConfidenceScore,
			"depends_on":       append([]string(nil), step.DependsOn...),
			"status":           step.Status,
		})
	}
	return map[string]any{
		"plan_id":      plan.ID,
		"plan_version": plan.Version,
		"title":        plan.Title,
		"workflow_id":  plan.WorkflowID,
		"steps":        steps,
		"summary":      plan.Title,
	}
}

func scenarioCheckpointRefs(payload map[string]any) []string {
	switch typed := payload["checkpoint_refs"].(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := fmt.Sprint(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func TestScenario_ImplementPlan_PersistsStepHistoryBlastRadiusAndCheckpoint(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-scenario-implement", "execute a compiled plan through real fixture stores")
	seedImplementPlanGraph(t, f, "symbol:service", "symbol:repo", "symbol:api")

	plan := &frameworkplan.LivingPlan{
		ID:         "plan-scenario-implement",
		WorkflowID: "wf-scenario-implement",
		Title:      "Implement scenario plan",
		Version:    1,
		StepOrder:  []string{"step-1"},
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:              "step-1",
				Description:     "Implement the service boundary change",
				Scope:           []string{"symbol:service"},
				ConfidenceScore: 0.92,
				Status:          frameworkplan.PlanStepPending,
				CreatedAt:       f.Now(),
				UpdatedAt:       f.Now(),
			},
		},
		CreatedAt: f.Now(),
		UpdatedAt: f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-scenario-implement",
		BasedOnRevision: "rev-1",
	})
	loadedPlan, err := f.PlanStore.LoadPlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("load seeded plan: %v", err)
	}

	env, model := testutil.EnvWithScenarioModel(t,
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"implement step completed"}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"checkpoint review completed"}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Respond JSON"},
			Response:       &core.LLMResponse{Text: `{"issues":[],"approve":true}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"gap detection clean"}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Respond JSON"},
			Response:       &core.LLMResponse{Text: `{"issues":[],"approve":true,"gap_status":"clean"}`},
		},
	)
	if err := env.Registry.Register(scenarioGitTool{}); err != nil {
		t.Fatalf("register scenario git tool: %v", err)
	}

	state := f.NewState()
	state.Set("euclo.active_plan_version", execution.VersionedPlanView{
		WorkflowID:             active.WorkflowID,
		PlanID:                 active.Plan.ID,
		Version:                active.Version,
		Status:                 string(active.Status),
		DerivedFromExploration: active.DerivedFromExploration,
		BasedOnRevision:        active.BasedOnRevision,
		SemanticSnapshotRef:    active.SemanticSnapshotRef,
		PatternRefs:            append([]string(nil), active.PatternRefs...),
		AnchorRefs:             append([]string(nil), active.AnchorRefs...),
		TensionRefs:            append([]string(nil), active.TensionRefs...),
		Plan:                   active.Plan,
	})
	state.Set("euclo.living_plan", loadedPlan)
	state.Set("pipeline.plan", scenarioPlanPayload(loadedPlan))

	in := execution.ExecuteInput{
		Task:        f.Task("wf-scenario-implement", "Execute the compiled plan end to end", nil),
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-scenario-implement",
			PrimaryRelurpicCapabilityID: ImplementPlan,
			PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
				WorkflowID:   "wf-scenario-implement",
				PlanID:       plan.ID,
				PlanVersion:  active.Version,
				IsPlanBacked: true,
				StepIDs:      []string{"step-1"},
			},
			SemanticInputs: eucloruntime.SemanticInputBundle{
				WorkflowID:  "wf-scenario-implement",
				PatternRefs: []string{"pattern:service"},
				TensionRefs: []string{"tension:boundary"},
			}},
		},
		ServiceBundle: execution.ServiceBundle{
			PlanStore: f.PlanStore,
			GraphDB:   f.Graph,
		},
	}

	result, err := NewImplementPlanBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("implement-plan behavior returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful implement-plan result, got %+v", result)
	}

	rawBlast, ok := state.Get("euclo.current_step_blast_radius")
	if !ok || rawBlast == nil {
		t.Fatalf("expected current step blast radius in state")
	}
	blast, ok := rawBlast.(map[string]any)
	if !ok {
		t.Fatalf("expected blast radius payload map, got %#v", rawBlast)
	}
	if got, _ := blast["affected_count"].(int); got != 2 {
		t.Fatalf("expected affected_count 2, got %#v", blast)
	}

	loaded, err := f.PlanStore.LoadPlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("load updated plan: %v", err)
	}
	step := loaded.Steps["step-1"]
	if step == nil {
		t.Fatalf("expected persisted step")
	}
	if step.Status != frameworkplan.PlanStepCompleted {
		t.Fatalf("expected completed step status, got %q", step.Status)
	}
	if len(step.History) != 1 {
		t.Fatalf("expected one step history entry, got %d", len(step.History))
	}

	rawVerify, ok := state.Get("pipeline.verify")
	if !ok || rawVerify == nil {
		t.Fatalf("expected pipeline.verify in state")
	}
	verifyPayload, ok := rawVerify.(map[string]any)
	if !ok {
		t.Fatalf("expected verification payload map, got %#v", rawVerify)
	}
	refs := scenarioCheckpointRefs(verifyPayload)
	if len(refs) == 0 || refs[0] != "checkpoint_step-1" {
		t.Fatalf("expected logical checkpoint ref for completed step, got %#v", refs)
	}

	model.AssertExhausted(t)
}

func TestScenario_ImplementPlan_FailedStepCreatesDeferredIssueAndPersistsFailedAttempt(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-scenario-gap", "surface failed-step deferral through real fixture stores")
	seedImplementPlanGraph(t, f, "symbol:boundary", "symbol:caller")

	plan := &frameworkplan.LivingPlan{
		ID:         "plan-scenario-gap",
		WorkflowID: "wf-scenario-gap",
		Title:      "Gap detection plan",
		Version:    1,
		StepOrder:  []string{"step-gap"},
		Steps: map[string]*frameworkplan.PlanStep{
			"step-gap": {
				ID:                 "step-gap",
				Description:        "Update the boundary contract",
				Scope:              []string{"symbol:boundary"},
				AnchorDependencies: []string{"anchor:boundary"},
				ConfidenceScore:    0.9,
				Status:             frameworkplan.PlanStepPending,
				CreatedAt:          f.Now(),
				UpdatedAt:          f.Now(),
			},
		},
		CreatedAt: f.Now(),
		UpdatedAt: f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-scenario-gap",
		BasedOnRevision: "rev-1",
	})
	loadedPlan, err := f.PlanStore.LoadPlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("load seeded gap plan: %v", err)
	}

	env, model := testutil.EnvWithScenarioModel(t,
		testutil.ScenarioModelTurn{
			Err: fmt.Errorf("implement step exploded"),
		},
	)
	if err := env.Registry.Register(scenarioGitTool{}); err != nil {
		t.Fatalf("register scenario git tool: %v", err)
	}

	state := f.NewState()
	state.Set("euclo.active_plan_version", execution.VersionedPlanView{
		WorkflowID:             active.WorkflowID,
		PlanID:                 active.Plan.ID,
		Version:                active.Version,
		Status:                 string(active.Status),
		DerivedFromExploration: active.DerivedFromExploration,
		BasedOnRevision:        active.BasedOnRevision,
		SemanticSnapshotRef:    active.SemanticSnapshotRef,
		PatternRefs:            append([]string(nil), active.PatternRefs...),
		AnchorRefs:             append([]string(nil), active.AnchorRefs...),
		TensionRefs:            append([]string(nil), active.TensionRefs...),
		Plan:                   active.Plan,
	})
	state.Set("euclo.living_plan", loadedPlan)
	state.Set("pipeline.plan", scenarioPlanPayload(loadedPlan))

	in := execution.ExecuteInput{
		Task:        f.Task("wf-scenario-gap", "Execute the plan and surface gap detection", nil),
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-scenario-gap",
			PrimaryRelurpicCapabilityID: ImplementPlan,
			PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
				WorkflowID:   "wf-scenario-gap",
				PlanID:       plan.ID,
				PlanVersion:  active.Version,
				IsPlanBacked: true,
				StepIDs:      []string{"step-gap"},
			},
			SemanticInputs: eucloruntime.SemanticInputBundle{
				WorkflowID:  "wf-scenario-gap",
				PatternRefs: []string{"pattern:boundary"},
				TensionRefs: []string{"tension:drift"},
			}},
		},
		ServiceBundle: execution.ServiceBundle{
			PlanStore: f.PlanStore,
			GraphDB:   f.Graph,
		},
	}

	result, err := NewImplementPlanBehavior().Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected implement-plan failure when the step execution errors")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed implement-plan result, got %+v", result)
	}

	rawIssues, ok := state.Get("euclo.deferred_execution_issues")
	if !ok || rawIssues == nil {
		t.Fatalf("expected deferred execution issue in state")
	}
	issues, ok := rawIssues.([]eucloruntime.DeferredExecutionIssue)
	if !ok || len(issues) == 0 {
		t.Fatalf("expected deferred execution issues slice, got %#v", rawIssues)
	}
	if issues[0].StepID != "step-gap" {
		t.Fatalf("expected deferred issue to bind to step-gap, got %#v", issues[0])
	}
	if issues[0].Kind != eucloruntime.DeferredIssueNonfatalFailure {
		t.Fatalf("expected nonfatal failure deferred issue, got %#v", issues[0])
	}

	loaded, err := f.PlanStore.LoadPlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("load updated plan: %v", err)
	}
	step := loaded.Steps["step-gap"]
	if step == nil {
		t.Fatalf("expected persisted gap step")
	}
	if step.Status != frameworkplan.PlanStepFailed {
		t.Fatalf("expected failed step status, got %q", step.Status)
	}
	if len(step.History) != 1 {
		t.Fatalf("expected one failed step history entry, got %#v", step.History)
	}
	if step.History[0].FailureReason == "" {
		t.Fatalf("expected failure reason in step history, got %#v", step.History[0])
	}

	model.AssertExhausted(t)
}

func TestScenario_LoadBoundPlan_UsesArchaeoServiceActiveVersion(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-scenario-service-backed", "load active plan through the archaeology service bundle")

	plan := &frameworkplan.LivingPlan{
		ID:         "plan-scenario-service-backed",
		WorkflowID: "wf-scenario-service-backed",
		Title:      "Service-backed plan",
		Version:    1,
		StepOrder:  []string{"step-service"},
		Steps: map[string]*frameworkplan.PlanStep{
			"step-service": {
				ID:              "step-service",
				Description:     "Execute the active service-backed plan step",
				Scope:           []string{"symbol:loader"},
				ConfidenceScore: 0.88,
				Status:          frameworkplan.PlanStepPending,
				CreatedAt:       f.Now(),
				UpdatedAt:       f.Now(),
			},
		},
		CreatedAt: f.Now(),
		UpdatedAt: f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-scenario-service-backed",
		BasedOnRevision: "rev-1",
	})
	in := execution.ExecuteInput{
		Task:  f.Task("wf-scenario-service-backed", "Load the active service-backed plan", nil),
		State: f.NewState(),
		Work: eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-scenario-service-backed",
			PrimaryRelurpicCapabilityID: ImplementPlan,
			PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
				WorkflowID:   "wf-scenario-service-backed",
				PlanID:       plan.ID,
				PlanVersion:  active.Version,
				IsPlanBacked: true,
				StepIDs:      []string{"step-service"},
			}},
		},
		ServiceBundle: execution.ServiceBundle{
			Archaeo: fixtureArchaeoAccess{fixture: f},
		},
	}

	result, err := in.ServiceBundle.Archaeo.ActivePlanVersion(context.Background(), "wf-scenario-service-backed")
	if err != nil {
		t.Fatalf("ActivePlanVersion: %v", err)
	}
	if result == nil {
		t.Fatalf("expected active plan version from archaeology service")
	}
	if result.PlanID != plan.ID || result.Version != active.Version {
		t.Fatalf("unexpected active plan version view: %#v", result)
	}
	loaded, err := loadBoundPlan(context.Background(), in)
	if err != nil {
		t.Fatalf("loadBoundPlan: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loadBoundPlan to return the active plan view")
	}
	if loaded.PlanID != plan.ID || loaded.Version != active.Version {
		t.Fatalf("unexpected loadBoundPlan view: %#v", loaded)
	}
	if len(loaded.Plan.StepOrder) != 1 || loaded.Plan.StepOrder[0] != "step-service" {
		t.Fatalf("expected step order from service-backed plan, got %#v", loaded.Plan.StepOrder)
	}
}
