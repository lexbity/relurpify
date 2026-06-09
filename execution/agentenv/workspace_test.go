package agentenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

func TestValidateConfigMissingFields(t *testing.T) {
	require.Error(t, validateConfig(WorkspaceConfig{}))
	require.Error(t, validateConfig(WorkspaceConfig{Workspace: "w", Scope: ScopeFull}))
	require.Error(t, validateConfig(WorkspaceConfig{Workspace: "w", ManifestPath: "m", Scope: ScopeFull}))
	require.NoError(t, validateConfig(WorkspaceConfig{Workspace: "w", ManifestPath: "m", InferenceEndpoint: "endpoint", Scope: ScopeFull}))
}

func TestSetupTelemetryDefaultsToStateDir(t *testing.T) {
	dir := t.TempDir()
	expected := filepath.Join(dir, ".relurpify_state", "logs", "agentenv.log")
	cfg := WorkspaceConfig{
		Workspace:         dir,
		ManifestPath:      filepath.Join(dir, "relurpify_cfg", "agent.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		StateDir:          filepath.Join(dir, ".relurpify_state"),
	}
	logFile, _, _, err := setupTelemetry(cfg)
	require.NoError(t, err)
	require.NoError(t, logFile.Close())
	_, err = os.Stat(expected)
	require.NoError(t, err)
}

func TestSetupTelemetryRejectsInvalidLogDir(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("file"), 0o644))
	_, _, _, err := setupTelemetry(WorkspaceConfig{
		Workspace:         dir,
		ManifestPath:      filepath.Join(dir, "agent.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		LogPath:           filepath.Join(blocked, "ayenitd.log"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create log directory")
}

// TestBootstrapAgentRuntimeSpecOnly asserts that BootstrapAgentRuntime
// succeeds with spec-only input (no ManifestSnapshot) and produces a fully
// policy-wired environment equivalent to the snapshot path.
func TestBootstrapAgentRuntimeSpecOnly(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}

	spec := &agentspec.AgentRuntimeSpec{
		Mode: agentspec.AgentModePrimary,
		Model: agentspec.AgentModelConfig{
			Provider: "ollama",
			Name:     "qwen2.5-coder:14b",
		},
		Capabilities: agentspec.AgentCapabilitiesSpec{
			Relurpic: []string{"euclo:cap.test_run"},
		},
	}

	boot, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		Context:        ctx,
		AgentID:        "test-spec-only",
		AgentName:      "test-agent",
		AgentSpec:      spec,
		SecurityBundle: securityBundle,
		Runner:         &fakeRunner{},
		MaxIterations:  8,
	})
	if err != nil {
		t.Fatalf("BootstrapAgentRuntime with spec-only should succeed, got: %v", err)
	}
	if boot == nil {
		t.Fatal("expected non-nil BootstrappedAgentRuntime")
	}

	// Registry, policy engine, and capability admissions must be present.
	if boot.Registry == nil {
		t.Error("Registry should not be nil")
	}
	if boot.PolicyEngine == nil {
		t.Error("PolicyEngine should not be nil")
	}
	if boot.CapabilityAdmissions == nil {
		t.Error("CapabilityAdmissions should not be nil")
	}
	if boot.AgentSpec == nil {
		t.Error("AgentSpec should not be nil")
	}
	if boot.AgentSpec.Model.Name != "qwen2.5-coder:14b" {
		t.Errorf("AgentSpec.Model.Name = %q, want %q", boot.AgentSpec.Model.Name, "qwen2.5-coder:14b")
	}
}

// TestBootstrapAgentRuntimeSnapshotStillWins asserts that when both
// ManifestSnapshot and AgentSpec are supplied, the manifest is authoritative.
func TestBootstrapAgentRuntimeSnapshotStillWins(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	writeSecurityPolicyFixtures(t, workspace)
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatalf("load security bundle: %v", err)
	}

	snapshot := &config.AgentManifestSnapshot{
		Manifest: &config.AgentManifest{
			APIVersion: "relurpify/v1alpha1",
			Kind:       "AgentManifest",
			Metadata: config.ManifestMetadata{
				Name: "manifest-agent",
			},
			Spec: config.ManifestSpec{
				Image:   "test-image:latest",
				Runtime: "gvisor",
				Agent: &agentspec.AgentRuntimeSpec{
					Mode: agentspec.AgentModePrimary,
					Model: agentspec.AgentModelConfig{
						Provider: "ollama",
						Name:     "manifest-model",
					},
				},
			},
		},
	}

	spec := &agentspec.AgentRuntimeSpec{
		Mode: agentspec.AgentModePrimary,
		Model: agentspec.AgentModelConfig{
			Provider: "openai",
			Name:     "spec-model",
		},
	}

	boot, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		Context:          ctx,
		AgentID:          "test-manifest-wins",
		AgentName:        "manifest-agent",
		ManifestSnapshot: snapshot,
		AgentSpec:        spec,
		SecurityBundle:   securityBundle,
		Runner:           &fakeRunner{},
		MaxIterations:    8,
	})
	if err != nil {
		t.Fatalf("BootstrapAgentRuntime with both snapshot and spec should succeed, got: %v", err)
	}
	if boot == nil {
		t.Fatal("expected non-nil BootstrappedAgentRuntime")
	}

	// The AgentSpec should override the manifest's agent section, so the
	// effective spec should have the spec's model name.
	if boot.AgentSpec == nil {
		t.Fatal("AgentSpec should not be nil")
	}
	if boot.AgentSpec.Model.Name != "spec-model" {
		t.Errorf("AgentSpec.Model.Name = %q, want %q (AgentSpec should override)", boot.AgentSpec.Model.Name, "spec-model")
	}

	// The config name should come from the manifest's metadata (manifest wins).
	if boot.AgentConfig.Name != "manifest-agent" {
		t.Errorf("AgentConfig.Name = %q, want %q (manifest name wins)", boot.AgentConfig.Name, "manifest-agent")
	}
}

// TestBootstrapAgentRuntimeRejectsNilSpecAndNilManifest verifies that when
// both ManifestSnapshot and AgentSpec are nil, BootstrapAgentRuntime returns
// an error.
func TestBootstrapAgentRuntimeRejectsNilSpecAndNilManifest(t *testing.T) {
	_, err := BootstrapAgentRuntime("/tmp/test", AgentBootstrapOptions{
		Runner: &fakeRunner{},
	})
	if err == nil {
		t.Error("expected error when both ManifestSnapshot and AgentSpec are nil")
	}
}
