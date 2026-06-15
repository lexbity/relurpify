package runtime

import (
	"context"
	"path/filepath"
	"testing"

	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestBootNoManifest_EucloAgent(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

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
