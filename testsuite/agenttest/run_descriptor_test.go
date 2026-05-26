package agenttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

func TestPreparedRunDescriptorRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "suite.yaml")
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
		SourcePath: suitePath,
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(cfgload.DirName, "agent.yaml")),
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
			Execution: SuiteExecutionSpec{Profile: "live", Strict: true},
		},
	}
	caseSpec := CaseSpec{
		Name:   "smoke",
		Prompt: "summarize the repo",
		Setup: SetupSpec{
			Files:     []SetupFileSpec{{Path: "notes.txt", Content: "hello"}},
			StateKeys: map[string]any{"ticket": "REL-1"},
		},
		Expect: ExpectSpec{
			Outcome: &OutcomeSpec{
				FilesChanged: []string{"notes.txt"},
				FilesContain: []FileContentExpectation{{Path: "notes.txt", Contains: []string{"hello"}}},
				Verify: &VerifySpec{
					Steps:  []VerifyStepSpec{{Tool: "go_test", Args: map[string]any{"package": "./..."}}},
					Script: "testsuite/agenttest_fixtures/gosuite/verify.sh",
				},
			},
		},
	}
	opts := RunOptions{
		SkipASTIndex:   true,
		MaxIterations:  7,
		MaxRetries:     2,
		BackendReset:   "server",
		BackendBinary:  "ollama",
		BackendService: "ollama",
	}

	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-123")
	desc, err := BuildPreparedRunDescriptor(suite, caseSpec, suite.Spec.Models[0], opts, workspace, runRoot, "run-123")
	if err != nil {
		t.Fatalf("BuildPreparedRunDescriptor: %v", err)
	}

	if desc.BackendSelection != PreparedRunSelectionSingle {
		t.Fatalf("unexpected backend selection: %q", desc.BackendSelection)
	}
	if desc.SandboxBackend != "gvisor" {
		t.Fatalf("unexpected sandbox backend: %q", desc.SandboxBackend)
	}
	if desc.SetupDir != filepath.Join(runRoot, "setup") {
		t.Fatalf("unexpected setup dir: %q", desc.SetupDir)
	}
	if desc.ExecutionDir != filepath.Join(runRoot, "execution") {
		t.Fatalf("unexpected execution dir: %q", desc.ExecutionDir)
	}
	if desc.ConfigPath != filepath.Join(desc.DerivedWorkspaceRoot, "relurpify_cfg", "config.yaml") {
		t.Fatalf("unexpected config path: %q", desc.ConfigPath)
	}
	if desc.Verification.Script == "" || len(desc.Verification.Steps) != 1 {
		t.Fatalf("unexpected verification contract: %+v", desc.Verification)
	}
	if len(desc.ExpectedArtifacts) == 0 {
		t.Fatal("expected expected_artifacts to be populated")
	}

	outPath := filepath.Join(t.TempDir(), "descriptor.json")
	if err := desc.Write(outPath); err != nil {
		t.Fatalf("Write: %v", err)
	}

	loaded, err := LoadPreparedRunDescriptor(outPath)
	if err != nil {
		t.Fatalf("LoadPreparedRunDescriptor: %v", err)
	}
	if loaded.RunID != desc.RunID || loaded.CaseName != desc.CaseName || loaded.AgentName != desc.AgentName {
		t.Fatalf("round-trip mismatch: got %+v want %+v", loaded, desc)
	}
	if loaded.BackendSelection != desc.BackendSelection {
		t.Fatalf("backend selection changed on round-trip: %q -> %q", desc.BackendSelection, loaded.BackendSelection)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if decoded["run_id"] != desc.RunID {
		t.Fatalf("unexpected run_id in json: %#v", decoded["run_id"])
	}
}
