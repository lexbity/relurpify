package testsuite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo"
)

func TestLiveWorkspaceHandshakeBuildsWorkspaceEnvironment(t *testing.T) {
	workspace := t.TempDir()
	relurpifyCfg := filepath.Join(workspace, "relurpify_cfg")
	if err := os.MkdirAll(filepath.Join(relurpifyCfg, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(relurpifyCfg, "agents", "euclo.yaml")
	if err := os.WriteFile(manifestPath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: euclo
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ${workspace}/**
        justification: read workspace
  agent:
    implementation: euclo
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relurpifyCfg, "config.yaml"), []byte("provider: ollama\nmodel: qwen2.5-coder:14b\nsandbox_backend: gvisor\nagent: euclo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := agentenv.WorkspaceConfig{
		Workspace:         workspace,
		ManifestPath:      manifestPath,
		ConfigPath:        filepath.Join(relurpifyCfg, "config.yaml"),
		InferenceProvider: "ollama",
		InferenceModel:    "qwen2.5-coder:14b",
		AgentName:         "euclo",
		AgentsDir:         filepath.Join(relurpifyCfg, "agents"),
		SandboxBackend:    "gvisor",
	}
	env, err := agentenv.BuildWorkspaceEnvironment(context.Background(), cfg, euclo.GetRegistrationFuncs())
	if err != nil {
		t.Fatal(err)
	}
	if env == nil {
		t.Fatal("expected workspace environment")
	}
	if env.Registry == nil {
		t.Fatal("expected capability registry")
	}
	if env.PromptRegistry == nil {
		t.Fatal("expected prompt registry")
	}
}
