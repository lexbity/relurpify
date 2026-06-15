package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBootNoManifest_EucloAgent verifies the runtime boots successfully when
// the agents/ directory has been removed entirely. The agent spec is sourced
// from the built-in contract, not from a file manifest.
func TestBootNoManifest_EucloAgent(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	if err := os.RemoveAll(filepath.Join(workspace, "relurpify_cfg", "agents")); err != nil {
		t.Fatalf("remove agents dir: %v", err)
	}

	cfg := ConfigForWorkspace(Config{AgentName: AgentLabelEuclo}, workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	t.Cleanup(func() {
		_ = rt.Close(context.Background())
	})

	if _, ok := rt.Agent.(*euclo.Agent); !ok {
		t.Fatalf("rt.Agent is %T, want *euclo.Agent", rt.Agent)
	}
}

// TestBootNoManifest_NoAgentsDir verifies boot when agents/ is absent.
func TestBootNoManifest_NoAgentsDir(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	if err := os.RemoveAll(filepath.Join(workspace, "relurpify_cfg", "agents")); err != nil {
		t.Fatalf("remove agents dir: %v", err)
	}

	cfg := ConfigForWorkspace(Config{}, workspace)
	cfg.AgentName = AgentLabelEuclo
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime with no agents dir: %v", err)
	}
	t.Cleanup(func() {
		_ = rt.Close(context.Background())
	})

	if rt.AgentWorkspace() == nil || rt.AgentWorkspace().AgentSpec == nil {
		t.Fatal("AgentSpec must be populated after fileless boot")
	}
}
