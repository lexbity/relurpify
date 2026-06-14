package agenttest

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

const (
	sandboxOK = "sandbox-ok"
)

// requiresRunsc skips the test when the Docker gVisor runtime is not available.
// It also gates behind RELURPIFY_SANDBOX_INTEGRATION so a plain `go test ./...`
// does not contact the Docker daemon (which can trigger desktop privilege
// prompts) just because runsc/docker happen to be installed.
func requiresRunsc(t *testing.T) {
	t.Helper()
	if testing.Short() || os.Getenv("RELURPIFY_SANDBOX_INTEGRATION") == "" {
		t.Skip("set RELURPIFY_SANDBOX_INTEGRATION=1 to run sandbox integration tests (they exec docker/runsc)")
	}
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("skipping: runsc not found on PATH")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping: docker not found on PATH")
	}
	out, err := exec.Command("docker", "info", "--format", "{{.Runtimes}}").CombinedOutput()
	if err != nil {
		t.Skipf("skipping: docker info failed: %v", err)
	}
	if !strings.Contains(string(out), "runsc") {
		t.Skip("skipping: runsc runtime not registered with Docker daemon")
	}
}

func TestNewWorkspaceSandboxRunner_NoBackend(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()

	_, err := NewWorkspaceSandboxRunner(ctx, ws, "bogus")
	if err == nil {
		t.Fatal("expected error for bogus backend")
	}
	if !errors.Is(err, ErrNoSandboxBackend) {
		t.Errorf("error should wrap ErrNoSandboxBackend, got: %v", err)
	}
}

func TestNewWorkspaceSandboxRunner_AvailableBackend(t *testing.T) {
	requiresRunsc(t)

	ctx := context.Background()
	ws := t.TempDir()

	runner, err := NewWorkspaceSandboxRunner(ctx, ws, "")
	if err != nil {
		t.Fatalf("NewWorkspaceSandboxRunner failed: %v", err)
	}
	if runner == nil {
		t.Fatal("runner is nil")
	}

	// Execute a trivial command to prove the sandbox works.
	res, err := runner.Run(ctx, sandbox.CommandRequest{
		Args: []string{"echo", sandboxOK},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != sandboxOK {
		t.Errorf("stdout = %q, want %q", res.Stdout, sandboxOK)
	}
}

func TestRequireSandbox_SkipsOnNoBackend(t *testing.T) {
	// This test verifies the skip behavior by using a bogus backend.
	// RequireSandbox should call t.Skip, which this test captures by
	// checking that the runner is nil after the call (the test function
	// itself should not reach the assertion if skip works).
	ctx := context.Background()
	ws := t.TempDir()

	runner := RequireSandbox(t, ctx, ws, "bogus")
	// If we reach here, RequireSandbox did NOT skip, which means
	// ErrNoSandboxBackend was not detected. This is a test failure,
	// but we use the runner result to detect it.
	if runner != nil {
		t.Error("RequireSandbox with bogus backend should have skipped or failed")
	}
}

func TestNewWorkspaceSandboxRunner_WorkspaceIsolation(t *testing.T) {
	requiresRunsc(t)

	ctx := context.Background()
	wsA := t.TempDir()
	wsB := t.TempDir()

	runnerA, err := NewWorkspaceSandboxRunner(ctx, wsA, "")
	if err != nil {
		t.Fatalf("runner A failed: %v", err)
	}
	runnerB, err := NewWorkspaceSandboxRunner(ctx, wsB, "")
	if err != nil {
		t.Fatalf("runner B failed: %v", err)
	}

	// Write a file in workspace A.
	markerFile := "marker.txt"
	_, err = runnerA.Run(ctx, sandbox.CommandRequest{
		Args: []string{"sh", "-c", "echo 'workspace-a-data' > " + markerFile},
	})
	if err != nil {
		t.Fatalf("write in ws A failed: %v", err)
	}

	// Verify the file exists in workspace A.
	res, err := runnerA.Run(ctx, sandbox.CommandRequest{
		Args: []string{"cat", markerFile},
	})
	if err != nil {
		t.Fatalf("read in ws A failed: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "workspace-a-data" {
		t.Errorf("ws A content = %q, want %q", res.Stdout, "workspace-a-data")
	}

	// Verify workspace B does NOT see the file.
	_, err = runnerB.Run(ctx, sandbox.CommandRequest{
		Args: []string{"cat", markerFile},
	})
	if err == nil {
		t.Error("workspace B should not see file written in workspace A")
	}

	// Also verify on the host that each workspace is a real temp dir.
	if _, err := os.Stat(filepath.Join(wsA, markerFile)); err != nil {
		t.Errorf("marker file missing from host ws A: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsB, markerFile)); err == nil {
		t.Error("marker file unexpectedly present in host ws B")
	}
}
