//go:build live
// +build live

package agenttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestBuildVerificationContractUsesPreparedRunDescriptor(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, relurpify_cfg, agent_yaml)
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
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
      name: gemma4:12b
`), fs.PublicFileMode); err != nil { // public: test manifest
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: euclo_code},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, agent_yaml)),
			Models: []ModelSpec{{
				Name:     qwen2_5_coder_14b,
				Provider: ollama,
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}

	runRoot := filepath.Join(workspace, relurpify_cfg, "test_run", run1)
	prepared, err := PrepareRun(suite, CaseSpec{Name: smoke, Prompt: hello}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, run1)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := BuildVerificationContract(prepared.Descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if contract.ExecutionReportPath != filepath.Join(prepared.Descriptor.ExecutionDir, "report.json") {
		t.Fatalf("unexpected execution report path: %+v", contract)
	}
}

func TestLoadCaseReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := CaseReport{Name: smoke, Model: qwen2_5_coder_14b, Success: true}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, fs.PublicFileMode); err != nil { // public: test report
		t.Fatal(err)
	}
	loaded, err := LoadCaseReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || !loaded.Success || loaded.Name != smoke {
		t.Fatalf("unexpected report: %+v", loaded)
	}
}
