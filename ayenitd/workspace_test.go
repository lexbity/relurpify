//go:build integration

package ayenitd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/ayenitd"
)

// testManifestYAML is a minimal agent manifest sufficient for Open().
const testManifestYAML = `apiVersion: relurpify/v1alpha1
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
  security:
    no_new_privileges: true
  agent:
    implementation: react
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`

// TestOpenWorkspace verifies that ayenitd.Open() successfully initializes
// a workspace session end-to-end: stores, registration, capability bundle,
// scheduler, and environment assembly.
//
// Requirements: Ollama running at localhost:11434 with qwen2.5-coder:14b loaded.
func TestOpenWorkspace(t *testing.T) {
	workspace := t.TempDir()

	// Write a few Go source files so the AST indexer has something to scan.
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte(`package main
func main() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the manifest.
	manifestPath := filepath.Join(workspace, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(testManifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      workspace,
		ManifestPath:   manifestPath,
		InferenceEndpoint: "http://localhost:11434",
		InferenceModel:    "qwen2.5-coder:14b",
		MemoryPath:     filepath.Join(workspace, "memory"),
		SkipASTIndex:   true, // don't block on full indexing in tests
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ws, err := ayenitd.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ws.Close()

	// Assert all required fields are populated.
	if ws.Environment.Registry == nil {
		t.Error("Environment.Registry is nil")
	}
	if ws.Environment.WorkflowStore == nil {
		t.Error("Environment.WorkflowStore is nil")
	}
	if ws.Environment.IndexManager == nil {
		t.Error("Environment.IndexManager is nil")
	}
	if ws.Environment.Memory == nil {
		t.Error("Environment.Memory is nil")
	}
	if ws.Environment.Config == nil {
		t.Error("Environment.Config is nil")
	}
	if ws.Environment.Model == nil {
		t.Error("Environment.Model is nil")
	}
	if ws.Environment.Scheduler == nil {
		t.Error("Environment.Scheduler is nil")
	}
	if ws.Registration == nil {
		t.Error("Registration is nil")
	}
	if ws.AgentSpec == nil {
		t.Error("AgentSpec is nil")
	}
	if ws.Logger == nil {
		t.Error("Logger is nil")
	}
	if ws.Telemetry == nil {
		t.Error("Telemetry is nil")
	}
}

// TestOpenWorkspace_ClosesCleanly verifies that Close() releases all resources
// without error.
func TestOpenWorkspace_ClosesCleanly(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(testManifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      workspace,
		ManifestPath:   manifestPath,
		InferenceEndpoint: "http://localhost:11434",
		InferenceModel:    "qwen2.5-coder:14b",
		MemoryPath:     filepath.Join(workspace, "memory"),
		SkipASTIndex:   true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ws, err := ayenitd.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := ws.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestOpenWorkspace_ProbeBlocksOnBadEndpoint verifies that Open() fails fast
// when Ollama is unreachable rather than blocking indefinitely.
func TestOpenWorkspace_ProbeBlocksOnBadEndpoint(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(testManifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      workspace,
		ManifestPath:   manifestPath,
		InferenceEndpoint: "http://127.0.0.1:19999", // deliberately wrong port
		InferenceModel:    "qwen2.5-coder:14b",
		MemoryPath:     filepath.Join(workspace, "memory"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := ayenitd.Open(ctx, cfg)
	if err == nil {
		t.Fatal("expected Open to fail for unreachable Ollama, got nil")
	}
}
