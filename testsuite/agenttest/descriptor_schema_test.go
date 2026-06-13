package agenttest

import (
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	ollama = "ollama"
	run1   = "run-1"
)

func TestPreparedRunDescriptorValidateRequiresCoreFields(t *testing.T) {
	var desc PreparedRunDescriptor
	if err := desc.Validate(); err == nil {
		t.Fatal("expected zero descriptor to fail validation")
	}
}

func TestPreparedRunDescriptorNormalizesPaths(t *testing.T) {
	desc := &PreparedRunDescriptor{
		RunID:                " run-1 ",
		SuitePath:            "./suite.yaml",
		SuiteName:            "suite",
		CaseName:             "case",
		AgentName:            "euclo",
		Instruction:          "prompt",
		WorkspaceRoot:        ".",
		RunRoot:              "./relurpify_cfg/test_run/run-1",
		DerivedWorkspaceRoot: "./derived",
		ManifestPath:         "./relurpify_cfg/agent.yaml",
		ConfigPath:           "./relurpify_cfg/config.yaml",
		AgentsDir:            "./relurpify_cfg/agents",
		LogsDir:              "./relurpify_cfg/test_run/run-1/execution/logs",
		TelemetryDir:         "./relurpify_cfg/test_run/run-1/execution/telemetry",
		SetupDir:             "./relurpify_cfg/test_run/run-1/setup",
		ExecutionDir:         "./relurpify_cfg/test_run/run-1/execution",
		BackendSelection:     PreparedRunSelectionSingle,
		BackendProvider:      ollama,
		BackendFamily:        ollama,
		BackendEndpoint:      "http://127.0.0.1:11434",
	}
	if err := desc.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if !filepath.IsAbs(desc.WorkspaceRoot) {
		t.Fatalf("WorkspaceRoot not normalized to absolute path: %q", desc.WorkspaceRoot)
	}
	if !filepath.IsAbs(desc.ManifestPath) {
		t.Fatalf("ManifestPath not normalized to absolute path: %q", desc.ManifestPath)
	}
	if desc.RunID != run1 {
		t.Fatalf("RunID = %q, want run-1", desc.RunID)
	}
}

func TestPreparedRunDescriptorMatrixSelection(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
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
      name: model-a
`)); err != nil {
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, "agent.yaml")),
			Models: []ModelSpec{
				{Name: "model-a", Provider: ollama, Endpoint: "http://127.0.0.1:11434"},
				{Name: "model-b", Provider: "lmstudio", Endpoint: "http://127.0.0.1:1234"},
			},
		},
	}
	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", run1)
	desc, err := BuildPreparedRunDescriptor(suite, CaseSpec{Name: "case", Prompt: "prompt"}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, run1)
	if err != nil {
		t.Fatalf("BuildPreparedRunDescriptor: %v", err)
	}
	if desc.BackendSelection != PreparedRunSelectionMatrix {
		t.Fatalf("BackendSelection = %q, want matrix", desc.BackendSelection)
	}
	if len(desc.BackendMatrix) < 2 {
		t.Fatalf("expected backend matrix entries, got %d", len(desc.BackendMatrix))
	}
}
