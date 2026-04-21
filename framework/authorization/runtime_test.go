package authorization

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

func TestSelectSandboxRuntimeDefaultsToGVisor(t *testing.T) {
	rt, err := selectSandboxRuntime(RuntimeConfig{}, &manifest.AgentManifest{
		Spec: manifest.ManifestSpec{
			Image: "ghcr.io/lexcodex/relurpify/runtime:latest",
		},
	})
	if err != nil {
		t.Fatalf("selectSandboxRuntime: %v", err)
	}
	if got := rt.Name(); got != "gvisor" {
		t.Fatalf("runtime.Name() = %q, want gvisor", got)
	}
}

func TestSelectSandboxRuntimeSelectsDocker(t *testing.T) {
	dir := t.TempDir()
	rt, err := selectSandboxRuntime(RuntimeConfig{
		Backend: "docker",
		BaseFS:  dir,
		Image:   "ghcr.io/lexcodex/relurpify/runtime:latest",
		Sandbox: sandbox.SandboxConfig{ContainerRuntime: "docker"},
	}, &manifest.AgentManifest{
		Spec: manifest.ManifestSpec{
			Image: "ghcr.io/lexcodex/relurpify/runtime:latest",
		},
	})
	if err != nil {
		t.Fatalf("selectSandboxRuntime: %v", err)
	}
	if got := rt.Name(); got != "docker" {
		t.Fatalf("runtime.Name() = %q, want docker", got)
	}
}

func TestRegisterAgentFailsWhenSelectedBackendUnavailable(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agent.manifest.yaml")
	manifestData := []byte(`apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: test-agent
  version: 1.0.0
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - path: /tmp
        action: fs:read
  agent:
    implementation: react
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`)
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := RegisterAgent(context.Background(), RuntimeConfig{
		ManifestPath: manifestPath,
		Backend:      "docker",
		BaseFS:       dir,
		Sandbox:      sandbox.SandboxConfig{ContainerRuntime: filepath.Join(dir, "missing-docker")},
	})
	if err == nil {
		t.Fatal("expected RegisterAgent to fail when docker is unavailable")
	}
}
