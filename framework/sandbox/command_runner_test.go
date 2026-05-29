package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// requiresRunsc skips the test when the Docker gVisor runtime is not available.
func requiresRunsc(t *testing.T) {
	t.Helper()
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

	runner, err := NewSandboxCommandRunner(&contracts.CommandRunnerConfig{
		Workspace: t.TempDir(),
	}, rt)
	if err != nil {
		t.Fatalf("NewSandboxCommandRunner failed: %v", err)
	}

	_, stderr, err := runner.Run(
		ctx,
		CommandRequest{
			Args:    []string{"sh", "-c", "sleep 60 & SPID=$!; sleep 60 & echo $SPID; wait"},
			Timeout: 100 * time.Millisecond,
		},
	)
	t.Logf("run returned: err=%v stderr=%q", err, stderr)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestSandboxCommandRunner_ProcessGroupCleanupDoesNotPanicOnAlreadyExited(t *testing.T) {
	requiresRunsc(t)

	rt := NewSandboxRuntime(SandboxConfig{})
	ctx := context.Background()
	if err := rt.Verify(ctx); err != nil {
		t.Fatalf("sandbox verify failed: %v", err)
	}

	runner, err := NewSandboxCommandRunner(&contracts.CommandRunnerConfig{
		Workspace: t.TempDir(),
	}, rt)
	if err != nil {
		t.Fatalf("NewSandboxCommandRunner failed: %v", err)
	}

	stdout, stderr, err := runner.Run(
		ctx,
		CommandRequest{
			Args:    []string{"echo", "hello"},
			Timeout: 5 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v (stderr: %s)", err, stderr)
	}
	if stdout != "hello\n" && stdout != "hello" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}
