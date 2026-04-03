package local

import (
	"context"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestTDDRedGreenRefactorCapability_EligibleWithWriteAndExecuteTools(t *testing.T) {
	capability := NewTDDRedGreenRefactorCapability(agentenv.AgentEnvironment{})
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		ID:      "intake",
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "add feature with TDD"},
	}})
	result := capability.Eligible(artifacts, euclotypes.CapabilitySnapshot{
		HasWriteTools:   true,
		HasExecuteTools: true,
	})
	if !result.Eligible {
		t.Fatalf("expected capability to be eligible, got %#v", result)
	}
}

func TestTDDRedGreenRefactorCapability_FailsWithoutExecuteTools(t *testing.T) {
	capability := NewTDDRedGreenRefactorCapability(agentenv.AgentEnvironment{})
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		ID:      "intake",
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "add feature with TDD"},
	}})
	result := capability.Eligible(artifacts, euclotypes.CapabilitySnapshot{
		HasWriteTools: true,
	})
	if result.Eligible {
		t.Fatal("expected capability to be ineligible without execute tools")
	}
}

func TestTDDRedGreenRefactorCapability_FailsWithoutConfiguredRuntime(t *testing.T) {
	state := core.NewContext()
	mergeStateArtifactsToContext(state, []euclotypes.Artifact{{
		ID:      "intake",
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "add feature with TDD"},
	}})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "tdd-task",
			Instruction: "add feature with TDD",
			Context: map[string]any{
				"workspace":             ".",
				"verification_commands": []string{"sh -c true"},
			},
		},
		State: state,
		RunID: "run-tdd",
	}

	result := (&tddRedGreenRefactorCapability{}).Execute(context.Background(), env)
	if result.Status != euclotypes.ExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.FailureInfo == nil || result.FailureInfo.Code != "tdd_runtime_unavailable" {
		t.Fatalf("expected runtime prerequisite failure, got %#v", result.FailureInfo)
	}
}

func TestShouldRunTDDRefactor_DetectsInstructionAndContext(t *testing.T) {
	if !shouldRunTDDRefactor(&core.Task{Instruction: "implement and refactor with TDD"}) {
		t.Fatal("expected refactor request from instruction")
	}
	if !shouldRunTDDRefactor(&core.Task{Instruction: "implement with TDD", Context: map[string]any{"tdd_refactor_requested": true}}) {
		t.Fatal("expected refactor request from task context")
	}
	if shouldRunTDDRefactor(&core.Task{Instruction: "implement with TDD"}) {
		t.Fatal("did not expect refactor request without signal")
	}
}

func TestBuildTDDLifecycleArtifact_TracksPhaseHistory(t *testing.T) {
	state := core.NewContext()
	initializeTDDLifecycle(state, true)
	updateTDDLifecycle(state, "red", "completed", map[string]any{"status": "fail"})
	updateTDDLifecycle(state, "green", "completed", map[string]any{"status": "pass"})
	updateTDDLifecycle(state, "refactor", "completed", map[string]any{"status": "pass"})
	updateTDDLifecycle(state, "complete", "completed", map[string]any{"summary": "done"})

	artifact := buildTDDLifecycleArtifact(tddLifecycleFromState(state))
	if artifact.Kind != euclotypes.ArtifactKindTDDLifecycle {
		t.Fatalf("expected TDD lifecycle artifact, got %q", artifact.Kind)
	}
	payload, ok := artifact.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %#v", artifact.Payload)
	}
	if payload["current_phase"] != "complete" {
		t.Fatalf("expected complete phase, got %#v", payload["current_phase"])
	}
	history, ok := payload["phase_history"].([]map[string]any)
	if !ok || len(history) < 4 {
		t.Fatalf("expected phase history to be preserved, got %#v", payload["phase_history"])
	}
}

func TestApplyRecipeCodePayloadToState_PrefersRecipeStatePipelineCode(t *testing.T) {
	target := core.NewContext()
	recipeState := core.NewContext()
	recipeState.Set("pipeline.code", map[string]any{
		"summary": "recipe pipeline code",
		"final_output": map[string]any{
			"result": map[string]any{
				"file_write": map[string]any{
					"success": true,
					"data": map[string]any{"path": "testsuite/fixtures/strings_test.go"},
				},
			},
		},
	})

	applyRecipeCodePayloadToState(target, recipeState, map[string]any{"summary": "fallback payload"})

	raw, ok := target.Get("pipeline.code")
	if !ok || raw == nil {
		t.Fatal("expected pipeline.code to be set on target state")
	}
	payload, _ := raw.(map[string]any)
	if payload["summary"] != "recipe pipeline code" {
		t.Fatalf("expected recipe state pipeline.code to win, got %#v", payload)
	}
}

func TestTDDPackageGuidance_PreservesGoPackageDeclarations(t *testing.T) {
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Context: map[string]any{
				"context_file_contents": []map[string]any{{
					"path":    "testsuite/fixtures/strings.go",
					"content": "package main\n\nfunc Existing() {}\n",
				}},
			},
		},
	}

	guidance := tddPackageGuidance(env)
	if guidance == "" {
		t.Fatal("expected package guidance to be generated")
	}
	if !strings.Contains(guidance, "testsuite/fixtures/strings.go declares package main") {
		t.Fatalf("expected concrete package guidance, got %q", guidance)
	}
}
