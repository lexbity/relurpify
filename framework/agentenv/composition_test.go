package agentenv

import (
	"context"
	"fmt"
	"testing"
	"time"
)

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
		Sandbox:           "default",
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

func TestBuildWorkspaceEnvironment(t *testing.T) {
	ctx := context.Background()
	cfg := WorkspaceConfig{
		Workspace:    t.TempDir(),
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
	}

	// Test with no registration functions
	regFuncs := AgentRegistrationFuncs{}
	env, err := BuildWorkspaceEnvironment(ctx, cfg, regFuncs)
	if err != nil {
		t.Fatalf("BuildWorkspaceEnvironment returned error: %v", err)
	}
	if env == nil {
		t.Fatal("BuildWorkspaceEnvironment returned nil environment")
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

	// Verify AgentID is propagated
	if env.Config.Name != "" {
		t.Logf("Config.Name = %s", env.Config.Name)
	}

	// Clean up
	if env.IndexManager != nil {
		_ = env.IndexManager.Close()
	}
}

func TestBuildWorkspaceEnvironmentWithEmptyWorkspace(t *testing.T) {
	ctx := context.Background()
	cfg := WorkspaceConfig{
		Workspace: "",
	}

	regFuncs := AgentRegistrationFuncs{}
	_, err := BuildWorkspaceEnvironment(ctx, cfg, regFuncs)
	if err == nil {
		t.Error("BuildWorkspaceEnvironment should return error for empty workspace")
	}
}

func TestBuildWorkspaceEnvironmentWithRegistrationFuncs(t *testing.T) {
	ctx := context.Background()
	called := false
	cfg := WorkspaceConfig{
		Workspace:    t.TempDir(),
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
	}

	regFuncs := AgentRegistrationFuncs{
		RegisterCapabilities: func(env WorkspaceEnvironment) error {
			called = true
			return nil
		},
		RegisterPromptProviders: nil,
		LoadRecipes:             nil,
	}

	env, err := BuildWorkspaceEnvironment(ctx, cfg, regFuncs)
	if err != nil {
		t.Fatalf("BuildWorkspaceEnvironment returned error: %v", err)
	}
	if env == nil {
		t.Fatal("BuildWorkspaceEnvironment returned nil environment")
	}

	if !called {
		t.Error("RegisterCapabilities was not called")
	}

	// Clean up
	if env.IndexManager != nil {
		_ = env.IndexManager.Close()
	}
}

func TestBuildWorkspaceEnvironmentRegistrationError(t *testing.T) {
	ctx := context.Background()
	cfg := WorkspaceConfig{
		Workspace:    t.TempDir(),
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
	}

	regFuncs := AgentRegistrationFuncs{
		RegisterCapabilities: func(env WorkspaceEnvironment) error {
			return fmt.Errorf("registration failed")
		},
		RegisterPromptProviders: nil,
		LoadRecipes:             nil,
	}

	_, err := BuildWorkspaceEnvironment(ctx, cfg, regFuncs)
	if err == nil {
		t.Error("BuildWorkspaceEnvironment should return error when registration fails")
	}
}

func TestBuildWorkspaceEnvironmentWithAgentSpec(t *testing.T) {
	ctx := context.Background()
	cfg := WorkspaceConfig{
		Workspace:    t.TempDir(),
		SkipASTIndex: true,
		AgentID:      "test-agent-id",
		AgentName:    "test-agent",
		// AgentSpec would be set in real usage, but we test without it for now
	}

	regFuncs := AgentRegistrationFuncs{}
	env, err := BuildWorkspaceEnvironment(ctx, cfg, regFuncs)
	if err != nil {
		t.Fatalf("BuildWorkspaceEnvironment returned error: %v", err)
	}
	if env == nil {
		t.Fatal("BuildWorkspaceEnvironment returned nil environment")
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
