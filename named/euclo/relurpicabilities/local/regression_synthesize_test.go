package local

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestRegressionSynthesizeCapability_EligibleForBugfixIntake(t *testing.T) {
	capability := NewRegressionSynthesizeCapability(agentenv.AgentEnvironment{})
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		ID:      "intake",
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "fix the failing auth bug with TDD"},
	}})
	result := capability.Eligible(artifacts, euclotypes.CapabilitySnapshot{HasWriteTools: true})
	if !result.Eligible {
		t.Fatalf("expected regression synthesis to be eligible, got %#v", result)
	}
}

func TestRegressionSynthesizeCapability_ProducesReproductionAndAnalysis(t *testing.T) {
	state := core.NewContext()
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "bugfix",
			Instruction: "fix the nil pointer panic in login flow",
		},
		State: state,
	}

	result := NewRegressionSynthesizeCapability(agentenv.AgentEnvironment{}).Execute(context.Background(), env)
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed status, got %#v", result)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("expected two artifacts, got %#v", result.Artifacts)
	}
	if result.Artifacts[0].Kind != euclotypes.ArtifactKindReproduction {
		t.Fatalf("expected reproduction artifact, got %#v", result.Artifacts[0])
	}
	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %#v", result.Artifacts[0].Payload)
	}
	if !valueTruthy(payload["synthesized"]) {
		t.Fatalf("expected synthesized reproducer payload, got %#v", payload)
	}
	if payload["suggested_test_name"] == "" {
		t.Fatalf("expected suggested test name, got %#v", payload)
	}
}

func TestShouldSynthesizeRegressionForTDD_SkipsWhenConcreteReproductionExists(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.reproduction", map[string]any{
		"reproduced":  true,
		"synthesized": false,
	})
	env := euclotypes.ExecutionEnvelope{
		Task:  &core.Task{Instruction: "fix the failing login regression"},
		State: state,
	}
	if shouldSynthesizeRegressionForTDD(env) {
		t.Fatal("expected synthesis to be skipped when non-synthesized reproduction exists")
	}
}

func TestTDDRedPhaseInstruction_UsesSynthesizedReproducer(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.reproduction", map[string]any{
		"synthesized":         true,
		"symptom":             "login fails after token refresh",
		"expected_failure":    "request returns unauthorized",
		"suggested_test_name": "test_login_refresh_regression",
		"acceptance_criteria": []string{"capture the reported bug symptom"},
	})
	env := euclotypes.ExecutionEnvelope{
		Task:  &core.Task{Instruction: "fix the login refresh bug"},
		State: state,
	}
	instruction := tddRedPhaseInstruction(env)
	if instruction == "" || instruction == taskInstruction(env.Task) {
		t.Fatalf("expected augmented red-phase instruction, got %q", instruction)
	}
	if !containsAll(instruction, []string{"Reproducer target", "test_login_refresh_regression", "unauthorized"}) {
		t.Fatalf("expected reproducer details in instruction, got %q", instruction)
	}
}

func containsAll(text string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}
