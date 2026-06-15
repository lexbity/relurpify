package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBootTurnProductPath verifies the real product turn entry through
// SubmitTurn: boot → turn → compile → persist → clean Close.
// This exercises the identical orchestrate graph, capability registry,
// sandbox runner wiring, and BKC compiler that the TUI uses.
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
	defer cancelHITL()

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

	compileCtx, compileCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer compileCancel()
	_, rec, err := rt.Compiler.Compile(compileCtx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:  "test compilation of workspace",
			Limit: 10,
		},
		MaxTokens: 4000,
		Metadata:  map[string]any{"source": "boot_turn_test"},
	})
	if err != nil {
		t.Fatalf("compiler compile: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil CompilationRecord from compiler")
	}
	if rec.RequestID == "" {
		t.Fatal("expected CompilationRecord.RequestID to be set")
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestBootTurnEmptyWorkspace verifies a turn still completes and the BKC
// compiler is wired and functional even with no ingestable files.
func TestBootTurnEmptyWorkspace(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider:  "offline",
		SeedFiles: nil,
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
	defer cancelHITL()

	runner.reset()
	scenario.set("echo")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := rt.SubmitTurn(ctx, "hello", execution.TaskTypeAnalysis, nil, nil)
	if err != nil {
		t.Fatalf("submit turn on empty workspace: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("turn result = %#v", result)
	}

	compileCtx, compileCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer compileCancel()
	_, rec, err := rt.Compiler.Compile(compileCtx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:  "empty",
			Limit: 5,
		},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("compiler compile on empty workspace: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil CompilationRecord from compiler on empty workspace")
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestBootTurnToolCall verifies a tool:file_read scenario produces a tool
// call through the capability registry (sandbox path) and the turn completes.
func TestBootTurnToolCall(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"demo.txt": "demo content\n",
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
	defer cancelHITL()

	runner.reset()
	scenario.set("tool:file_read:demo.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := rt.SubmitTurn(ctx, "read demo.txt", execution.TaskTypeAnalysis, nil, nil)
	if err != nil {
		t.Fatalf("submit turn: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("turn result = %#v", result)
	}

	compileCtx, compileCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer compileCancel()
	_, rec, err := rt.Compiler.Compile(compileCtx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:  fmt.Sprintf("file read of %s", "demo.txt"),
			Limit: 10,
		},
		MaxTokens: 4000,
	})
	if err != nil {
		t.Fatalf("compiler compile: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil CompilationRecord from compiler")
	}
	if rec.RequestID == "" {
		t.Fatal("expected CompilationRecord.RequestID to be set")
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
	return ""
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

// verify compiler types are importable and callable at compile time.
var _ = (&compiler.Compiler{}).Compile

// TestBootTurnRealConfig verifies a turn through the real checked-in
// relurpify_cfg/ tree, ensuring the V1 config format is parseable and
// produces a functional workspace (AC-4/AC-5).
func TestBootTurnRealConfig(t *testing.T) {
	workspace := t.TempDir()
	repoRoot := findRepoRoot()
	if err := copyDir(filepath.Join(repoRoot, "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg")); err != nil {
		t.Fatalf("copy relurpify_cfg: %v", err)
	}

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
		t.Fatalf("boot runtime with real config: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	runner.reset()
	scenario.set("echo")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := rt.SubmitTurn(ctx, "hello", execution.TaskTypeAnalysis, nil, nil)
	if err != nil {
		t.Fatalf("submit turn on real config: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("turn result = %#v", result)
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
