package euclo

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type stubProvider struct{}

func (stubProvider) Descriptor() core.ProviderDescriptor                    { return core.ProviderDescriptor{ID: "stub"} }
func (stubProvider) Initialize(context.Context, core.ProviderRuntime) error { return nil }
func (stubProvider) RegisterCapabilities(context.Context, core.CapabilityRegistrar) error {
	return nil
}
func (stubProvider) ListSessions(context.Context) ([]core.ProviderSession, error) { return nil, nil }
func (stubProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{}, nil
}
func (stubProvider) Close(context.Context) error { return nil }

func TestHelperExtractorsAndAggregators(t *testing.T) {
	if got := stringValue("x"); got != "x" {
		t.Fatalf("unexpected stringValue: %q", got)
	}
	if got := stringValue(123); got != "" {
		t.Fatalf("expected non-string to collapse to empty string, got %q", got)
	}
	if got := mapValue(map[string]any{"a": 1}); got == nil || got["a"] != 1 {
		t.Fatalf("unexpected mapValue result: %#v", got)
	}
	if got := mapValue("nope"); got != nil {
		t.Fatalf("expected non-map to return nil, got %#v", got)
	}

	comment := commentInputValue(map[string]any{
		"intent_type":  " review ",
		"author_kind":  " agent ",
		"body":         " hello ",
		"trust_class":  " builtin ",
		"corpus_scope": " workspace ",
	})
	if comment == nil || comment.Body != "hello" || comment.AuthorKind != "agent" {
		t.Fatalf("unexpected comment input: %#v", comment)
	}

	task := &core.Task{Context: map[string]any{"workspace": " workspace-id ", "workflow_id": " wf-1 "}}
	if got := workspaceIDFromTask(task, core.NewContext()); got != "workspace-id" {
		t.Fatalf("unexpected workspace id: %q", got)
	}
	if got := workflowIDFromTaskState(task, core.NewContext()); got != "wf-1" {
		t.Fatalf("unexpected workflow id: %q", got)
	}

	if payload, ok := learningResolutionPayload(nil, nil); ok || payload != nil {
		t.Fatalf("expected empty learning resolution payload, got %#v %v", payload, ok)
	}

	state := core.NewContext()
	state.Set("euclo.deferred_issue_ids", []any{" one ", "two", "one", nil})
	issues := deferredIssueIDsFromState(state, []string{"base", "two"})
	if len(issues) != 3 || issues[0] != "base" || issues[1] != "two" || issues[2] != "one" {
		t.Fatalf("unexpected deferred issues: %#v", issues)
	}

	agent := &Agent{
		RuntimeProviders: []core.Provider{stubProvider{}},
	}
	state.Set("euclo.runtime_providers", []core.Provider{stubProvider{}})
	providers := agent.runtimeProviders(state)
	if len(providers) != 2 {
		t.Fatalf("expected runtime providers to merge, got %#v", providers)
	}

	state.Set("euclo.artifacts", []euclotypes.Artifact{{Kind: euclotypes.ArtifactKindIntake}, {Kind: euclotypes.ArtifactKindFinalReport}})
	if kinds := collectArtifactKindsFromState(state); len(kinds) != 2 || kinds[0] != string(euclotypes.ArtifactKindIntake) || kinds[1] != string(euclotypes.ArtifactKindFinalReport) {
		t.Fatalf("unexpected artifact kinds: %#v", kinds)
	}

	result := &core.Result{}
	state.Set("euclo.execution_status", "executing")
	state.Set("euclo.assurance_class", "verified")
	state.Set("pipeline.final_output", map[string]any{"result_class": "blocked", "assurance_class": "ignored"})
	agent.applyRuntimeResultMetadata(result, state)
	if result.Metadata["execution_status"] != "executing" {
		t.Fatalf("expected execution status metadata, got %#v", result.Metadata)
	}
	if result.Metadata["result_class"] != "blocked" {
		t.Fatalf("expected result class to be inferred, got %#v", result.Metadata)
	}
	if result.Metadata["assurance_class"] != "verified" {
		t.Fatalf("expected assurance class to preserve explicit state value, got %#v", result.Metadata)
	}

	seedPersistedInteractionState(&core.Task{Context: map[string]any{"euclo.interaction_state": "seeded"}}, state)
	if got := state.GetString("euclo.interaction_state"); got != "seeded" {
		t.Fatalf("expected persisted interaction state to seed, got %q", got)
	}

	agent.Config = &core.Config{Telemetry: nil}
	if agent.ConfigTelemetry() != nil {
		t.Fatal("expected nil telemetry")
	}
}

func TestBlockingWorkAndShortPaths(t *testing.T) {
	if taskHasExplicitWorkflow(nil) {
		t.Fatal("expected nil task to have no explicit workflow")
	}
	if taskHasExplicitWorkflow(&core.Task{}) {
		t.Fatal("expected missing workflow to be false")
	}
	if !taskHasExplicitWorkflow(&core.Task{Context: map[string]any{"workflow_id": " wf-1 "}}) {
		t.Fatal("expected explicit workflow to be detected")
	}

	if !bundleHasBlockingWork(&core.Task{}, &executionReadBundle{learningQueue: &archaeoprojections.LearningQueueProjection{PendingGuidanceIDs: []string{"g"}}}) {
		t.Fatal("expected pending guidance to block work")
	}
	if !bundleHasBlockingWork(&core.Task{}, &executionReadBundle{learningQueue: &archaeoprojections.LearningQueueProjection{BlockingLearning: []string{"b"}}}) {
		t.Fatal("expected blocking learning to block work")
	}
	if !bundleHasBlockingWork(&core.Task{Context: map[string]any{"current_step_id": "s"}}, &executionReadBundle{activePlan: &archaeoprojections.ActivePlanProjection{ActivePlanVersion: &archaeodomain.VersionedLivingPlan{Plan: frameworkplan.LivingPlan{StepOrder: []string{"s"}, Steps: map[string]*frameworkplan.PlanStep{"s": {ID: "s", Status: frameworkplan.PlanStepInProgress}}}}}}) {
		t.Fatal("expected active step to block work")
	}
	if bundleHasBlockingWork(nil, nil) {
		t.Fatal("expected nil bundle to be non-blocking")
	}

	agent := &Agent{}
	if prep := agent.prepareSummaryFastPathExecution(context.Background(), &core.Task{}, core.NewContext(), executionPreparation{}); prep.summaryFastPath || prep.skipReason != "" {
		t.Fatalf("expected no summary fast path when no bundle exists, got %#v", prep)
	}
}
