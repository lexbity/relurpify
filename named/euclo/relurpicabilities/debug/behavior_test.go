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
