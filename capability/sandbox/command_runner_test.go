package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// requiresRunsc skips the test when the Docker gVisor runtime is not available.
// These tests launch real `docker run --runtime runsc` containers, so they are
// gated behind the sandbox-integration opt-in to keep `go test ./...` from
// contacting the Docker daemon (and triggering desktop privilege prompts).
func requiresRunsc(t *testing.T) {
	t.Helper()
	requireSandboxIntegration(t)
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("skipping: runsc not found on PATH")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping: docker not found on PATH")
	}
	// Verify the Docker daemon actually has the runsc runtime configured.
	// The SandboxCommandRunner uses `docker run --runtime runsc`, which
	// fails if gVisor isn't registered with the Docker daemon even when
	// the runsc binary exists.
	out, err := exec.Command("docker", "info", "--format", "{{.Runtimes}}").CombinedOutput()
	if err != nil {
		t.Skipf("skipping: docker info failed: %v", err)
	}
	if !strings.Contains(string(out), "runsc") {
		t.Skip("skipping: runsc runtime not registered with Docker daemon")
	}
}

func TestSandboxCommandRunner_ProcessGroupCleanupAfterTimeout(t *testing.T) {
	requiresRunsc(t)

	rt := NewSandboxRuntime(SandboxConfig{})
	ctx := context.Background()
	if err := rt.Verify(ctx); err != nil {
		t.Fatalf("sandbox verify failed: %v", err)
	}

	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{
		Workspace: t.TempDir(),
	}, rt)
	if err != nil {
		t.Fatalf("NewSandboxCommandRunner failed: %v", err)
	}

	res, err := runner.Run(
		ctx,
		CommandRequest{
			Args:    []string{"sh", "-c", "sleep 60 & SPID=$!; sleep 60 & echo $SPID; wait"},
			Timeout: 100 * time.Millisecond,
		},
	)
	t.Logf("run returned: err=%v", err)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	_ = res
}

func TestSandboxCommandRunner_ProcessGroupCleanupDoesNotPanicOnAlreadyExited(t *testing.T) {
	requiresRunsc(t)

	rt := NewSandboxRuntime(SandboxConfig{})
	ctx := context.Background()
	if err := rt.Verify(ctx); err != nil {
		t.Fatalf("sandbox verify failed: %v", err)
	}

	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{
		Workspace: t.TempDir(),
	}, rt)
	if err != nil {
		t.Fatalf("NewSandboxCommandRunner failed: %v", err)
	}

	res, err := runner.Run(
		ctx,
		CommandRequest{
			Args:    []string{"echo", "hello"},
			Timeout: 5 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.Stdout != "hello\n" && res.Stdout != "hello" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}
