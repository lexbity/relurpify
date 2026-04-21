package local

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestExecutionProfileSelectEligibleAndExecute(t *testing.T) {
	env := testutil.Env(t)
	cap := NewExecutionProfileSelectCapability(env)
	boring := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "fix the nil pointer"},
	}})
	if cap.Eligible(boring, euclotypes.CapabilitySnapshot{}).Eligible {
		t.Fatal("expected ineligible without profile-selection phrasing")
	}
	good := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "what profile should I use for this change?"},
	}})
	if !cap.Eligible(good, euclotypes.CapabilitySnapshot{}).Eligible {
		t.Fatal("expected eligible")
	}

	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "which execution profile fits a risky refactor?"},
	}})
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "prof-1",
			Instruction: "which execution profile fits a risky refactor?",
			Context:     map[string]any{"workspace": "."},
		},
		State:       state,
		Environment: env,
		Registry:    env.Registry,
	})
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed profile selection, got %+v", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != euclotypes.ArtifactKindProfileSelection {
		t.Fatalf("unexpected artifacts: %#v", result.Artifacts)
	}
	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	if !ok || stringValue(payload["selected_profile"]) == "" {
		t.Fatalf("expected selection payload with selected_profile, got %#v", result.Artifacts[0].Payload)
	}
}
