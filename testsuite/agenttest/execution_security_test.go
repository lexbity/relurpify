package agenttest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

type fakeRunner struct{}

func (f fakeRunner) Run(_ context.Context, req sandbox.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{}, nil
}

func validDescriptorWithWorkspace(t *testing.T, workspace string) *PreparedRunDescriptor {
	t.Helper()
	return &PreparedRunDescriptor{
		RunID:                "r1",
		SuitePath:            "/suite.yaml",
		SuiteName:            "suite",
		CaseName:             "case",
		AgentName:            "agent",
		Instruction:          "do it",
		WorkspaceRoot:        workspace,
		RunRoot:              filepath.Join(workspace, "run"),
		DerivedWorkspaceRoot: workspace,
		ConfigPath:           filepath.Join(workspace, "config.yaml"),
		AgentsDir:            filepath.Join(workspace, "agents"),
		LogsDir:              filepath.Join(workspace, "logs"),
		TelemetryDir:         filepath.Join(workspace, "telemetry"),
		SetupDir:             filepath.Join(workspace, "setup"),
		ExecutionDir:         filepath.Join(workspace, "exec"),
		BackendSelection:     PreparedRunSelectionSingle,
		BackendProvider:      "ollama",
		BackendFamily:        "ollama",
		BackendEndpoint:      "http://127.0.0.1:11434",
		MaxIterations:        8,
		MaxRetries:           0,
	}
}

func TestExecutorBuildsSecurityRuntime(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	err := exec.Execute(context.Background(), desc, io.Discard)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.security == nil {
		t.Fatal("security runtime is nil after Execute")
	}
	if exec.security.Runner == nil {
		t.Fatal("security.Runner is nil")
	}
	if exec.security.CommandPolicy == nil {
		t.Fatal("security.CommandPolicy is nil")
	}
}

func TestExecutorSecurityDefaultDenyWhenNoPermissionManager(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	err := exec.security.CommandPolicy.AllowCommand(context.Background(), sandbox.CommandRequest{
		Args: []string{"echo", "blocked"},
	})
	if err == nil {
		t.Fatal("expected default-deny policy error, got nil")
	}
}

func TestExecutorBuildsCapabilityRuntime(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.capability == nil {
		t.Fatal("capability runtime is nil after Execute")
	}
	if exec.capability.Registry == nil {
		t.Fatal("capability.Registry is nil")
	}
	if exec.capability.IndexManager == nil {
		t.Fatal("capability.IndexManager is nil")
	}
	if exec.capability.SearchEngine == nil {
		t.Fatal("capability.SearchEngine is nil")
	}
	if exec.capability.IndexManager.GraphDB == nil {
		t.Fatal("capability.IndexManager.GraphDB is nil")
	}
}

func TestExecutorCleanupClosesIndexManager(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	exec.cleanup()

	if exec.capability != nil && exec.capability.IndexManager != nil {
		err := exec.capability.IndexManager.Close(context.Background())
		_ = err // second close is expected to be safe (idempotent)
	}
}

func TestExecutorCleanupRunsOnBuildFailure(t *testing.T) {
	dir := t.TempDir()
	wsPath := filepath.Join(dir, "workspace_file")
	if err := os.WriteFile(wsPath, []byte("not a directory"), fs.PublicFileMode); err != nil {
		t.Fatal(err)
	}

	desc := validDescriptorWithWorkspace(t, wsPath)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	err := exec.Execute(context.Background(), desc, io.Discard)
	if err == nil {
		t.Fatal("expected error from capability build with file-as-workspace")
	}

	if !strings.Contains(err.Error(), "capability") {
		t.Fatalf("error does not mention capability: %v", err)
	}

	if exec.security != nil && exec.security.Runner != nil {
		_ = exec.security.Runner
	}
}

func TestExecutorFailsOnInvalidWorkspace(t *testing.T) {
	ws := "/nonexistent_path_for_test"
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	err := exec.Execute(context.Background(), desc, io.Discard)
	if err == nil {
		t.Fatal("expected error for invalid workspace")
	}
}
