package local

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestFailedVerificationRepairCapability_EligibleWithWriteAndExecuteTools(t *testing.T) {
	capability := NewFailedVerificationRepairCapability(agentenv.AgentEnvironment{})
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "intake", Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "fix failing verification"}},
		{ID: "verify", Kind: euclotypes.ArtifactKindVerification, Payload: map[string]any{"status": "fail"}},
	})
	result := capability.Eligible(artifacts, euclotypes.CapabilitySnapshot{HasWriteTools: true, HasExecuteTools: true})
	if !result.Eligible {
		t.Fatalf("expected capability to be eligible, got %#v", result)
	}
}

func TestFailedVerificationRepairCapability_FailsWithoutConfiguredRuntime(t *testing.T) {
	state := core.NewContext()
	mergeStateArtifactsToContext(state, []euclotypes.Artifact{
		{ID: "intake", Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "fix failing verification"}},
		{ID: "verify", Kind: euclotypes.ArtifactKindVerification, Payload: map[string]any{
			"status":  "fail",
			"summary": "go test failed",
			"checks":  []map[string]any{{"name": "go_test", "status": "fail", "files_under_check": []string{"foo.go"}}},
		}},
	})
	env := euclotypes.ExecutionEnvelope{
		Task:  &core.Task{ID: "repair-task", Instruction: "fix failing verification", Context: map[string]any{"workspace": "."}},
		State: state,
		RunID: "run-repair",
	}

	result := (&failedVerificationRepairCapability{}).Execute(context.Background(), env)
	if result.Status != euclotypes.ExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.FailureInfo == nil || result.FailureInfo.Code != "failed_verification_repair_runtime_unavailable" {
		t.Fatalf("expected runtime prerequisite failure, got %#v", result.FailureInfo)
	}
}

func TestVerificationPayloadFailedHelpers(t *testing.T) {
	payload := map[string]any{
		"status": "fail",
		"checks": []map[string]any{
			{"name": "go_test_pkg", "status": "fail", "files_under_check": []string{"pkg/foo.go"}},
			{"name": "go_test_other", "status": "pass"},
		},
	}
	if !VerificationPayloadFailed(payload) {
		t.Fatal("expected failing payload to be detected")
	}
	files := verificationFilesUnderCheck(payload)
	if len(files) != 1 || files[0] != "pkg/foo.go" {
		t.Fatalf("expected files under check, got %#v", files)
	}
	names := verificationFailingCheckNames(payload)
	if len(names) != 1 || names[0] != "go_test_pkg" {
		t.Fatalf("expected failing check names, got %#v", names)
	}
}
