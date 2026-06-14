package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/execution"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBootTurnProductPath verifies the real product turn entry through
// SubmitTurn: boot → turn → clean Close. This exercises the identical
// orchestrate graph, capability registry, and sandbox runner wiring that
// the TUI uses.
func TestBootTurnProductPath(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"notes.txt": "initial workspace notes\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	runner := &recordingRunner{}
	scenario := &scenarioState{}
	offline := &offlineScenarioModel{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.ModelFactoryWrapper = func(base model.ModelFactory) model.ModelFactory {
		return func(tel model.Telemetry, debug bool) model.LanguageModel {
			offline.inner = base(tel, debug)
			offline.scenario = func() string { return scenario.get() }
			return offline
		}
	}
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer func() {
		cancelHITL()
	}()

	notesPath := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(notesPath, []byte("updated content\n"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	runGit(t, workspace, "add", "notes.txt")
	runGit(t, workspace, "commit", "-m", "test update")

	runner.reset()
	scenario.set("echo")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := rt.SubmitTurn(ctx, "review", execution.TaskTypeAnalysis, map[string]any{
		"source": "slice4",
	}, nil)
	if err != nil {
		t.Fatalf("submit turn: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("turn result = %#v", result)
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
