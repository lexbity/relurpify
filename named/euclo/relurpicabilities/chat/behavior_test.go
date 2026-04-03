package chat

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestInspectBehaviorPrefersSemanticReviewOverReflectionFallback(t *testing.T) {
	env := testutil.Env(t)
	state := core.NewContext()
	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "inspect-semantic",
			Instruction: "inspect this API compatibility change",
			Context: map[string]any{
				"workspace": ".",
				"context_file_contents": []map[string]any{{
					"path":    "api.go",
					"content": "package sample\n\nfunc Exported(input string) string { return input }\n",
				}},
			},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-inspect-semantic",
			RunID:                       "run-inspect-semantic",
			PrimaryRelurpicCapabilityID: Inspect,
		},
	}

	result, err := NewInspectBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("inspect behavior returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful inspect result, got %+v", result)
	}

	artifacts := artifactsFromResultData(result)
	if !hasArtifactProducer(artifacts, "euclo:review.semantic", euclotypes.ArtifactKindReviewFindings) {
		t.Fatalf("expected semantic review artifact, got %#v", artifacts)
	}
	if hasArtifactID(artifacts, "chat_inspect_review_reflection_fallback") {
		t.Fatalf("expected reflection fallback to stay unused, got %#v", artifacts)
	}
	if !hasArtifactKind(artifacts, euclotypes.ArtifactKindCompatibilityAssessment) {
		t.Fatalf("expected compatibility assessment artifact, got %#v", artifacts)
	}

	raw, ok := state.Get("euclo.review_findings")
	if !ok || raw == nil {
		t.Fatalf("expected semantic review findings in state")
	}
	reviewPayload, _ := raw.(map[string]any)
	if reviewPayload == nil || reviewPayload["approval_decision"] == nil {
		t.Fatalf("expected semantic review payload with approval decision, got %#v", raw)
	}
}

func TestImplementBehaviorReviewSuggestImplementBlocksAutomaticMutationOnSemanticReview(t *testing.T) {
	env := testutil.Env(t)
	env.Registry = capability.NewRegistry()
	if err := env.Registry.Register(testutil.FileWriteTool{}); err != nil {
		t.Fatalf("register write tool: %v", err)
	}

	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "remove exported API",
		"compatibility_after_surface": map[string]any{
			"functions": []map[string]any{},
			"types":     []map[string]any{},
		},
	})
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:      "intake",
		Kind:    euclotypes.ArtifactKindIntake,
		Summary: "implement if safe and fix findings for this API change",
		Payload: map[string]any{"instruction": "implement if safe and fix findings for this API change"},
	}})

	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "implement-review-blocked",
			Instruction: "implement if safe and fix findings for this API change",
			Context: map[string]any{
				"workspace": ".",
				"context_file_contents": []map[string]any{{
					"path":    "api.go",
					"content": "package sample\n\nfunc Exported(input string) string { return input }\n",
				}},
			},
		},
		State:       state,
		Environment: env,
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID: "review_suggest_implement",
		},
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-implement-review-blocked",
			RunID:                       "run-implement-review-blocked",
			PrimaryRelurpicCapabilityID: Implement,
		},
	}

	result, err := NewImplementBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("implement behavior returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful blocked-automation result, got %+v", result)
	}

	artifacts := artifactsFromResultData(result)
	if !hasArtifactProducer(artifacts, "euclo:review.semantic", euclotypes.ArtifactKindReviewFindings) {
		t.Fatalf("expected semantic review artifact, got %#v", artifacts)
	}
	if hasArtifactID(artifacts, "review_safe_edit") {
		t.Fatalf("did not expect automatic edit artifact on blocked review, got %#v", artifacts)
	}

	raw, ok := state.Get("euclo.review_findings")
	if !ok || raw == nil {
		t.Fatalf("expected review findings in state")
	}
	reviewPayload, _ := raw.(map[string]any)
	approval, _ := reviewPayload["approval_decision"].(map[string]any)
	if approval == nil || approval["status"] != "blocked" {
		t.Fatalf("expected blocked approval decision, got %#v", reviewPayload)
	}

	traceRaw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok || traceRaw == nil {
		t.Fatalf("expected behavior trace in state")
	}
	trace, _ := traceRaw.(execution.Trace)
	if len(trace.RecipeIDs) != 0 {
		t.Fatalf("did not expect default implement recipes to run, got %#v", trace)
	}
	if len(trace.SpecializedCapabilityIDs) == 0 || trace.SpecializedCapabilityIDs[0] != "euclo:review.implement_if_safe" {
		t.Fatalf("expected specialized implement-if-safe trace, got %#v", trace)
	}
}

func TestExecuteSpecializedImplementBehavior_TDDProfileSelectsTDDCapability(t *testing.T) {
	env := testutil.Env(t)
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:      "intake",
		Kind:    euclotypes.ArtifactKindIntake,
		Summary: "write tests first, then implement",
		Payload: map[string]any{"instruction": "write tests first, then implement"},
	}})

	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "implement-tdd-specialized",
			Instruction: "write tests first, then implement Reverse using TDD",
			Context:     map[string]any{"workspace": "."},
		},
		State:       state,
		Environment: env,
		Mode:        euclotypes.ModeResolution{ModeID: "tdd"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID: "test_driven_generation",
		},
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-implement-tdd-specialized",
			RunID:                       "run-implement-tdd-specialized",
			PrimaryRelurpicCapabilityID: Implement,
		},
	}

	result, handled, err := executeSpecializedImplementBehavior(context.Background(), in, nil)
	if !handled {
		t.Fatal("expected TDD profile to be handled by specialized implement behavior")
	}
	if err == nil {
		t.Fatal("expected TDD capability to fail without configured runtime in unit test environment")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed specialized result, got %+v", result)
	}
	traceRaw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok || traceRaw == nil {
		t.Fatal("expected specialized capability trace in state")
	}
	trace, _ := traceRaw.(execution.Trace)
	if len(trace.SpecializedCapabilityIDs) == 0 || trace.SpecializedCapabilityIDs[0] != "euclo:tdd.red_green_refactor" {
		t.Fatalf("expected TDD specialized trace, got %#v", trace)
	}
}

func artifactsFromResultData(result *core.Result) []euclotypes.Artifact {
	if result == nil || result.Data == nil {
		return nil
	}
	raw, ok := result.Data["artifacts"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []euclotypes.Artifact:
		return typed
	case []any:
		out := make([]euclotypes.Artifact, 0, len(typed))
		for _, item := range typed {
			if artifact, ok := item.(euclotypes.Artifact); ok {
				out = append(out, artifact)
			}
		}
		return out
	default:
		return nil
	}
}

func hasArtifactID(artifacts []euclotypes.Artifact, id string) bool {
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return true
		}
	}
	return false
}

func hasArtifactKind(artifacts []euclotypes.Artifact, kind euclotypes.ArtifactKind) bool {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return true
		}
	}
	return false
}

func hasArtifactProducer(artifacts []euclotypes.Artifact, producer string, kind euclotypes.ArtifactKind) bool {
	for _, artifact := range artifacts {
		if artifact.ProducerID == producer && artifact.Kind == kind {
			return true
		}
	}
	return false
}

func TestSupportingRoutinesLocalReviewEmitsReviewArtifact(t *testing.T) {
	routines := NewSupportingRoutines()
	if len(routines) < 1 {
		t.Fatal("expected supporting routines")
	}
	state := core.NewContext()
	in := euclorelurpic.RoutineInput{
		Task: &core.Task{
			Instruction: "inspect security posture of the handler",
			Context: map[string]any{
				"context_file_contents": []map[string]any{{"path": "handler.go", "content": "package api"}},
			},
		},
		State: state,
		Work:  euclorelurpic.WorkContext{PrimaryCapabilityID: Ask},
	}
	artifacts, err := routines[0].Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("local review routine: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != euclotypes.ArtifactKindReviewFindings {
		t.Fatalf("expected review findings artifact, got %#v", artifacts)
	}
	payload, _ := artifacts[0].Payload.(map[string]any)
	if payload == nil || payload["review_source"] != LocalReview {
		t.Fatalf("unexpected payload %#v", artifacts[0].Payload)
	}
}

func TestSupportingRoutinesTargetedVerificationReadsPipelineVerify(t *testing.T) {
	routines := NewSupportingRoutines()
	if len(routines) < 2 {
		t.Fatal("expected targeted verification routine")
	}
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status": "fail",
		"checks": []any{map[string]any{"name": "go_test", "status": "fail"}},
	})
	in := euclorelurpic.RoutineInput{
		Task:  &core.Task{Instruction: "repair verification"},
		State: state,
		Work:  euclorelurpic.WorkContext{PrimaryCapabilityID: Implement},
	}
	artifacts, err := routines[1].Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("targeted verification routine: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != euclotypes.ArtifactKindVerificationSummary {
		t.Fatalf("expected verification summary artifact, got %#v", artifacts)
	}
	payload, _ := artifacts[0].Payload.(map[string]any)
	if payload == nil || payload["overall_status"] != "fail" {
		t.Fatalf("expected failed status in payload, got %#v", payload)
	}
}

func TestAskBehaviorCompletesWithStubModel(t *testing.T) {
	env := testutil.Env(t)
	env.Config.MaxIterations = 2
	state := core.NewContext()
	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "ask-stub",
			Instruction: "why does the handler reject unsigned requests?",
			Context:     map[string]any{"workspace": "."},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-ask-stub",
			RunID:                       "run-ask-stub",
			PrimaryRelurpicCapabilityID: Ask,
		},
	}
	result, err := NewAskBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("ask behavior: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	arts := artifactsFromResultData(result)
	if !hasArtifactID(arts, "chat_ask_answer") {
		t.Fatalf("expected chat_ask_answer artifact, got %#v", arts)
	}
	if _, ok := state.Get("pipeline.analyze"); !ok {
		t.Fatal("expected pipeline.analyze")
	}
}

func TestAskBehaviorOptionsPathAppendsPlanCandidatesWhenInstructionRequestsComparison(t *testing.T) {
	env := testutil.Env(t)
	env.Config.MaxIterations = 2
	state := core.NewContext()
	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "ask-options",
			Instruction: "Compare options for caching strategy and tradeoffs",
			Context:     map[string]any{"workspace": "."},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-ask-options",
			RunID:                       "run-ask-options",
			PrimaryRelurpicCapabilityID: Ask,
		},
	}
	result, err := NewAskBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("ask behavior: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	arts := artifactsFromResultData(result)
	if !hasArtifactID(arts, "chat_ask_plan_candidates") {
		t.Fatalf("expected plan candidates artifact for options-style ask, got %#v", arts)
	}
}
