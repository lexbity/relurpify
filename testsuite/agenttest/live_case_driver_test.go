package agenttest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestLiveCaseDriverUsesExecutionReportArtifact(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema: relurpify/agent/v1
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
      name: qwen2.5-coder:14b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, "agent.yaml")),
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}

	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-1")
	prepared, err := PrepareRun(suite, CaseSpec{Name: "smoke", Prompt: "hello"}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	report := CaseReport{Name: "smoke", Model: "qwen2.5-coder:14b", Success: true, Output: "ok", ArtifactsDir: prepared.Descriptor.ExecutionArtifactsDir}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prepared.Descriptor.ExecutionDir, "report.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	vs, err := NewVerificationSuite(prepared.Descriptor)
	if err != nil {
		t.Fatal(err)
	}
	result, err := vs.Verify(context.Background(), prepared, suite, CaseSpec{Name: "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful verification, got %+v", result)
	}
	if !vs.ArtifactExists(filepath.Join(prepared.Descriptor.ExecutionDir, "report.json")) {
		t.Fatal("expected execution report artifact to exist")
	}
}
