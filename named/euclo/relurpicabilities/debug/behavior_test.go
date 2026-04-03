package debug

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestInvestigateBehaviorSynthesizesReproducerBeforePatchWhenMissing(t *testing.T) {
	env := testutil.Env(t)
	state := core.NewContext()
	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "debug-reproducer",
			Instruction: "fix the failing regression in the request parser",
			Context: map[string]any{
				"workspace": ".",
			},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-debug-reproducer",
			RunID:                       "run-debug-reproducer",
			PrimaryRelurpicCapabilityID: Investigate,
		},
	}

	result, err := NewInvestigateBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("debug investigate returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful debug investigate result, got %+v", result)
	}

	artifacts := debugArtifactsFromResult(result)
	reproduction := findArtifactByKind(artifacts, euclotypes.ArtifactKindReproduction)
	if reproduction == nil {
		t.Fatalf("expected synthesized reproduction artifact, got %#v", artifacts)
	}
	payload, _ := reproduction.Payload.(map[string]any)
	if payload == nil || payload["synthesized"] != true {
		t.Fatalf("expected synthesized reproduction payload, got %#v", reproduction.Payload)
	}
	if reproduction.ProducerID != "euclo:test.regression_synthesize" {
		t.Fatalf("expected reproducer from regression synthesis, got %#v", reproduction)
	}

	raw, ok := state.Get("euclo.reproduction")
	if !ok || raw == nil {
		t.Fatalf("expected reproduction state to be populated")
	}
	statePayload, _ := raw.(map[string]any)
	if statePayload == nil || statePayload["synthesized"] != true {
		t.Fatalf("expected synthesized reproduction in state, got %#v", raw)
	}
}

func debugArtifactsFromResult(result *core.Result) []euclotypes.Artifact {
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

func findArtifactByKind(artifacts []euclotypes.Artifact, kind euclotypes.ArtifactKind) *euclotypes.Artifact {
	for i := range artifacts {
		if artifacts[i].Kind == kind {
			return &artifacts[i]
		}
	}
	return nil
}

func TestInvestigationSummaryStageContract(t *testing.T) {
	stage := &investigationSummaryStage{task: &core.Task{Instruction: "debug flaky test"}}
	if stage.Name() != "debug_investigation_summary" {
		t.Fatalf("name: %s", stage.Name())
	}
	contract := stage.Contract()
	if contract.Metadata.OutputKey != "euclo.debug_investigation_summary" || contract.Metadata.InputKey != "pipeline.analyze" {
		t.Fatalf("unexpected contract metadata: %#v", contract.Metadata)
	}
}

func TestRepairReadinessStageContract(t *testing.T) {
	stage := &repairReadinessStage{task: &core.Task{Instruction: "debug flaky test"}}
	if stage.Name() != "debug_repair_readiness" {
		t.Fatalf("name: %s", stage.Name())
	}
	contract := stage.Contract()
	if contract.Metadata.OutputKey != "euclo.debug_repair_readiness" || contract.Metadata.InputKey != "euclo.debug_investigation_summary" {
		t.Fatalf("unexpected contract metadata: %#v", contract.Metadata)
	}
}

func TestInvestigateBehaviorSkipsRegressionSynthesisWhenReproductionConcrete(t *testing.T) {
	env := testutil.Env(t)
	state := core.NewContext()
	state.Set("euclo.reproduction", map[string]any{"method": "go test", "concrete": true})
	in := execution.ExecuteInput{
		Task: &core.Task{
			ID:          "debug-no-synth",
			Instruction: "fix the failing regression in the request parser",
			Context:     map[string]any{"workspace": "."},
		},
		State:       state,
		Environment: env,
		Work: eucloruntime.UnitOfWork{
			WorkflowID:                  "wf-debug-no-synth",
			RunID:                       "run-debug-no-synth",
			PrimaryRelurpicCapabilityID: Investigate,
		},
	}

	result, err := NewInvestigateBehavior().Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("debug investigate returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	artifacts := debugArtifactsFromResult(result)
	for _, a := range artifacts {
		if a.ProducerID == "euclo:test.regression_synthesize" {
			t.Fatalf("did not expect regression synthesis artifact when reproduction is concrete, got %#v", a)
		}
	}
}
