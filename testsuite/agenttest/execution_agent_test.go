package agenttest

import (
	"context"
	"io"
	"testing"
)

func TestExecutorAssemblesAllDepsFields(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.agent == nil {
		t.Fatal("agent is nil after Execute")
	}
}

func TestExecutorCreatesAndInitializesAgent(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.agent == nil {
		t.Fatal("agent is nil — Initialize did not run")
	}
}

func TestExecutorAgentInitializeFailureSurfaces(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("pre-check Execute should succeed: %v", err)
	}

	deps := exec.assembleDeps(desc, exec.telemetry)
	deps.Registry = nil

	err := exec.createAgent(deps)
	if err == nil {
		t.Fatal("expected error for nil registry, got nil")
	}
}

func TestExecutorNilAgentLifecycleDoesNotPanic(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute with nil AgentLifecycle should not panic: %v", err)
	}

	if exec.agent == nil {
		t.Fatal("agent should be non-nil")
	}
}
