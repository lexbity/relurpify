package agentenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// openEnvForTest calls OpenWorkspace with ScopeEmbeddedAgent and extracts the
// environment. It replaces the deleted broad context builder for tests
// that exercise the embedded-agent bootstrap path.
func openEnvForTest(ctx context.Context, cfg WorkspaceConfig, securityBundle *cfgsecurity.Bundle, regFuncs AgentRegistrationFuncs) (*AgentContext, error) {
	cfg.SecurityBundle = securityBundle
	cfg.Scope = ScopeEmbeddedAgent
	ws, err := OpenWorkspace(ctx, cfg, regFuncs)
	if err != nil {
		return nil, err
	}
	return &ws.Environment, nil
}

// fakeRunner implements sandbox.CommandRunner for tests.
type fakeRunner struct{}

func (f *fakeRunner) Run(_ context.Context, _ sandbox.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{}, nil
}

var _ sandbox.CommandRunner = (*fakeRunner)(nil)

func TestWorkspaceConfig(t *testing.T) {
	cfg := WorkspaceConfig{
		Workspace:         "/test/workspace",
		ManifestPath:      "/test/manifest.yaml",
		ConfigPath:        "/test/config.yaml",
		InferenceProvider: "test-provider",
		InferenceEndpoint: "http://localhost:8080",
		InferenceModel:    "test-model",
		AgentName:         "test-agent",
		AgentsDir:         "/test/agents",
		SandboxBackend:    "docker",
		AuditLimit:        100,
		HITLTimeout:       5 * time.Minute,
		LogPath:           "/test/log.txt",
		TelemetryPath:     "/test/telemetry.db",
		SkipASTIndex:      false,
		MaxIterations:     10,
		DebugLLM:          true,
		DebugAgent:        false,
		AgentID:           "test-agent-id",
	}

	if cfg.Workspace != "/test/workspace" {
		t.Errorf("Workspace = %s, want /test/workspace", cfg.Workspace)
	}
	if cfg.ManifestPath != "/test/manifest.yaml" {
		t.Errorf("ManifestPath = %s, want /test/manifest.yaml", cfg.ManifestPath)
	}
	if cfg.InferenceProvider != "test-provider" {
		t.Errorf("InferenceProvider = %s, want test-provider", cfg.InferenceProvider)
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", cfg.MaxIterations)
	}
	if cfg.AgentID != "test-agent-id" {
		t.Errorf("AgentID = %s, want test-agent-id", cfg.AgentID)
	}
}

func TestOpenEnvForTestBasic(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}
	cfg := WorkspaceConfig{
		Workspace:    workspace,
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "test-model",
			},
		},
	}

	// Test with no registration functions
	regFuncs := AgentRegistrationFuncs{}
	env, err := openEnvForTest(ctx, cfg, securityBundle, regFuncs)
	if err != nil {
		t.Fatalf("openEnvForTest returned error: %v", err)
	}
	if env == nil {
		t.Fatal("openEnvForTest returned nil environment")
	}

	// Verify basic fields are populated
	if env.Config == nil {
		t.Error("Config should not be nil")
	}
	if env.Registry == nil {
		t.Error("Registry should not be nil")
	}
	if env.IndexManager == nil {
		t.Error("IndexManager should not be nil")
	}
	if env.SearchEngine == nil {
		t.Error("SearchEngine should not be nil")
	}
	if env.WorkingMemory == nil {
		t.Error("WorkingMemory should not be nil")
	}
	if env.PromptRegistry == nil {
		t.Error("PromptRegistry should not be nil")
	}
	if env.FileScope == nil {
		t.Error("FileScope should not be nil")
	} else {
		wantCfg := filepath.ToSlash(filepath.Join(workspace, "relurpify_cfg"))
		wantGit := filepath.ToSlash(filepath.Join(workspace, ".git"))
		if !containsString(env.FileScope.ProtectedPaths, wantCfg) {
			t.Errorf("FileScope protected paths missing %s", wantCfg)
		}
		if !containsString(env.FileScope.ProtectedPaths, wantGit) {
			t.Errorf("FileScope protected paths missing %s", wantGit)
		}
	}

	// Verify AgentID is propagated
	if env.Config.Name != "" {
		t.Logf("Config.Name = %s", env.Config.Name)
	}

	// Clean up
	if env.IndexManager != nil {
		_ = env.IndexManager.Close()
	}
}

func TestOpenEnvForTestRunnerPopulated(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}
	cfg := WorkspaceConfig{
		Workspace:    workspace,
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "test-model",
			},
		},
	}

	regFuncs := AgentRegistrationFuncs{}
	env, err := openEnvForTest(ctx, cfg, securityBundle, regFuncs)
	if err != nil {
		t.Fatalf("openEnvForTest returned error: %v", err)
	}
	if env == nil {
		t.Fatal("openEnvForTest returned nil environment")
	}

	// The CommandRunner should be populated via buildCommandRunner (the fake in TestMain)
	// wrapped in an AuthorizedRunner (Phase 2).
	if env.CommandRunner == nil {
		t.Error("CommandRunner should not be nil")
	}
	if _, ok := env.CommandRunner.(*sandbox.AuthorizedRunner); !ok {
		t.Errorf("expected *sandbox.AuthorizedRunner, got %T", env.CommandRunner)
	}

	if env.IndexManager != nil {
		_ = env.IndexManager.Close()
	}
}

func TestOpenEnvForTestEmbeddedScope(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}
	cfg := WorkspaceConfig{
		Workspace:    workspace,
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "test-model",
			},
		},
	}

	env, err := openEnvForTest(ctx, cfg, securityBundle, AgentRegistrationFuncs{})
	if err != nil {
		t.Fatalf("openEnvForTest should succeed, got: %v", err)
	}
	if env == nil {
		t.Fatal("openEnvForTest returned nil environment")
	}

	// The environment should have a CommandRunner (AuthorizedRunner from the
	// shared security foundation via OpenWorkspace with ScopeEmbeddedAgent).
	if env.CommandRunner == nil {
		t.Error("CommandRunner should not be nil")
	}
	if _, ok := env.CommandRunner.(*sandbox.AuthorizedRunner); !ok {
		t.Errorf("expected *sandbox.AuthorizedRunner, got %T", env.CommandRunner)
	}
}

func TestOpenEnvForTestEmptyWorkspace(t *testing.T) {
	ctx := context.Background()
	cfg := WorkspaceConfig{
		Workspace: "",
	}
	regFuncs := AgentRegistrationFuncs{}
	_, err := openEnvForTest(ctx, cfg, nil, regFuncs)
	if err == nil {
		t.Error("openEnvForTest should return error for empty workspace")
	}
}

func TestOpenEnvForTestWithRegistrationFuncs(t *testing.T) {
	ctx := context.Background()
	called := false
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}
	cfg := WorkspaceConfig{
		Workspace:    workspace,
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "test-model",
			},
		},
	}

	regFuncs := AgentRegistrationFuncs{
		RegisterCapabilities: func(env AgentContext) error {
			called = true
			return nil
		},
		RegisterPromptProviders: nil,
		LoadThoughtRecipes:      nil,
	}

	env, err := openEnvForTest(ctx, cfg, securityBundle, regFuncs)
	if err != nil {
		t.Fatalf("openEnvForTest returned error: %v", err)
	}
	if env == nil {
		t.Fatal("openEnvForTest returned nil environment")
	}

	if !called {
		t.Error("RegisterCapabilities was not called")
	}

	// Clean up
	if env.IndexManager != nil {
		_ = env.IndexManager.Close()
	}
}

func TestOpenEnvForTestRegistrationError(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}
	cfg := WorkspaceConfig{
		Workspace:    workspace,
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "test-model",
			},
		},
	}

	regFuncs := AgentRegistrationFuncs{
		RegisterCapabilities: func(env AgentContext) error {
			return fmt.Errorf("registration failed")
		},
		RegisterPromptProviders: nil,
		LoadThoughtRecipes:      nil,
	}

	_, err = openEnvForTest(ctx, cfg, securityBundle, regFuncs)
	if err == nil {
		t.Error("openEnvForTest should return error when registration fails")
	}
}

func TestOpenEnvForTestWithAgentSpec(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}
	cfg := WorkspaceConfig{
		Workspace:    workspace,
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "test-model",
			},
		},
	}

	regFuncs := AgentRegistrationFuncs{}
	env, err := openEnvForTest(ctx, cfg, securityBundle, regFuncs)
	if err != nil {
		t.Fatalf("openEnvForTest returned error: %v", err)
	}
	if env == nil {
		t.Fatal("openEnvForTest returned nil environment")
	}

	// Verify AgentID is in config
	if env.Config.Name != "test-agent" {
		t.Errorf("Config.Name = %s, want test-agent", env.Config.Name)
	}

	// Clean up
	if env.IndexManager != nil {
		_ = env.IndexManager.Close()
	}
}

func TestOpenEnvForTestRequiresSecurityPolicies(t *testing.T) {
	ctx := context.Background()
	cfg := WorkspaceConfig{
		Workspace:    t.TempDir(),
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
	}

	_, err := openEnvForTest(ctx, cfg, nil, AgentRegistrationFuncs{})
	if err == nil {
		t.Fatal("expected missing security policies to fail")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeSecurityPolicyFixtures(t *testing.T, workspace string) {
	t.Helper()
	securityDir := filepath.Join(workspace, "relurpify_cfg", "security")
	if err := os.MkdirAll(securityDir, 0o755); err != nil {
		t.Fatalf("mkdir security dir: %v", err)
	}
	mustWrite := func(name, body string) {
		path := filepath.Join(securityDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("sandbox.policy.yaml", `schema: relurpify/policy/sandbox/v1
read_only_root: false
protected_paths:
  - relurpify_cfg/workspace.yaml
  - relurpify_cfg/security
  - relurpify_cfg/model/profiles
  - relurpify_cfg/agents
  - relurpify_cfg/tools
no_new_privileges: true
allowed_env_keys: []
denied_env_keys: []
network_rules: []
`)
	mustWrite("shell.policy.yaml", `schema: relurpify/policy/shell/v1
rules:
  - id: deny-git-reset-hard
    pattern: '(^|\s)git\s+reset\s+--hard(\s|$)'
    reason: "Destructive git reset is blocked"
    action: block
`)
	mustWrite("localtool.policy.yaml", `schema: relurpify/policy/localtool/v1
tools:
  cli_git:
    execute: ask
`)
	mustWrite("workspaceingestion.policy.yaml", `schema: relurpify/policy/ingestion/v1
rules:
  - id: allow-workspace-ingestion
    name: "Workspace ingestion"
    priority: 100
    enabled: true
    effect:
      action: allow
      reason: "Allow workspace ingestion for configured sources"
`)

	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools", "shell", "fileops")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "cli_git.tool.yaml"), []byte(`schema: relurpify/tool/v1
name: cli_git
family: fileops
intent: [inspect, repository]
description: Runs git with the provided arguments.
execution:
  backend: subprocess
  command:
    base: ["git"]
  sandbox:
    allowed_root: ${workspace}
    timeout_seconds: 30
  allow_stdin: true
  supports_workdir: true
capability:
  trust_class: builtin_trusted
  risk_class: [execute]
  effect_class: [filesystem_read]
`), 0o644); err != nil {
		t.Fatalf("write cli_git manifest: %v", err)
	}
}
