package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/execution"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestSwitchAgentDepsParity(t *testing.T) {
	rt := bootRuntimeForSwitchDepsTest(t)

	agentCfg := &execution.Config{Name: "euclo", Model: rt.Config.InferenceModel}
	base := rt.paradigmDeps()
	switched := rt.switchAgentDeps(agentCfg)
	if switched == nil {
		t.Fatal("expected switch deps")
	}
	if switched.Config != agentCfg {
		t.Fatalf("switch config = %p, want %p", switched.Config, agentCfg)
	}
	if switched.Model != base.Model {
		t.Fatalf("model deps changed: got %p want %p", switched.Model, base.Model)
	}
	if switched.Registry != base.Registry {
		t.Fatalf("registry deps changed: got %p want %p", switched.Registry, base.Registry)
	}
	if switched.WorkingMemory != base.WorkingMemory {
		t.Fatalf("working memory deps changed: got %p want %p", switched.WorkingMemory, base.WorkingMemory)
	}
	if switched.IndexManager != base.IndexManager {
		t.Fatalf("index manager deps changed: got %p want %p", switched.IndexManager, base.IndexManager)
	}
	if switched.SearchEngine != base.SearchEngine {
		t.Fatalf("search engine deps changed: got %p want %p", switched.SearchEngine, base.SearchEngine)
	}
	if switched.StreamTrigger != base.StreamTrigger {
		t.Fatalf("stream trigger deps changed: got %p want %p", switched.StreamTrigger, base.StreamTrigger)
	}
	if switched.OutputIngester != base.OutputIngester {
		t.Fatalf("output ingester deps changed: got %p want %p", switched.OutputIngester, base.OutputIngester)
	}
	if switched.IngestOutputs != base.IngestOutputs {
		t.Fatalf("ingest outputs deps changed: got %t want %t", switched.IngestOutputs, base.IngestOutputs)
	}
	if switched.PromptRegistry != base.PromptRegistry {
		t.Fatalf("prompt registry deps changed: got %p want %p", switched.PromptRegistry, base.PromptRegistry)
	}
	if switched.AgentLifecycle != base.AgentLifecycle {
		t.Fatalf("agent lifecycle deps changed: got %p want %p", switched.AgentLifecycle, base.AgentLifecycle)
	}
	if _, err := instantiateAgent(switched); err != nil {
		t.Fatalf("instantiate switched deps: %v", err)
	}
}

func TestSwitchAgentPathBuildsEuclo(t *testing.T) {
	rt := bootRuntimeForSwitchDepsTest(t)
	rt.Config.InferenceModel = rt.Workspace.EffectiveContract.AgentSpec.Model.Name
	if err := rt.applyResolvedAgentState("euclo", rt.Workspace.EffectiveContract, rt.Workspace.CompiledPolicy); err != nil {
		t.Fatalf("apply resolved agent state: %v", err)
	}
	if _, ok := rt.Agent.(*euclo.Agent); !ok {
		t.Fatalf("rt.Agent is %T, want *euclo.Agent", rt.Agent)
	}
}

func bootRuntimeForSwitchDepsTest(t *testing.T) *Runtime {
	t.Helper()

	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agents", "euclo.yaml")
	manifestData, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "userconfig", "config", "testdata", "contracts", "document_current.yaml")))
	if err != nil {
		t.Fatalf("read manifest fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := fs.WriteFileSecure(manifestPath, manifestData); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	cfg := ConfigForWorkspace(Config{AgentName: "euclo"}, workspace)
	cfg.ManifestPath = manifestPath
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
	return rt
}
