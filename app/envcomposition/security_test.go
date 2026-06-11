package envcomposition

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type fakeRunner struct{}

func (f fakeRunner) Run(context.Context, sandbox.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{}, nil
}

func TestValidateSecurityRuntimeInputRejectsBackendManifestMismatch(t *testing.T) {
	err := ValidateSecurityRuntimeInput(SecurityRuntimeInput{
		SandboxBackend: "docker",
		ManifestSpec:   &config.ManifestSpec{Runtime: "gvisor"},
	})
	if err == nil {
		t.Fatal("expected backend mismatch error")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("expected incompatibility error, got %v", err)
	}
}

func TestBuildSecurityRuntimeWithExistingRunnerDefaultDeny(t *testing.T) {
	runtime, err := BuildSecurityRuntime(context.Background(), SecurityRuntimeInput{
		ExistingRunner: fakeRunner{},
	})
	if err != nil {
		t.Fatalf("build security runtime: %v", err)
	}
	if runtime.Runner == nil {
		t.Fatal("expected authorized runner")
	}
	if runtime.CommandPolicy == nil {
		t.Fatal("expected command policy")
	}
	err = runtime.CommandPolicy.AllowCommand(context.Background(), sandbox.CommandRequest{
		Args: []string{"echo", "blocked"},
	})
	if err == nil {
		t.Fatal("expected default-deny policy error")
	}
}
