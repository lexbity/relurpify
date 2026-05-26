package agenttest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

func TestVerifyPreparedRunWritesVerificationReport(t *testing.T) {
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
			Manifest:  filepath.ToSlash(filepath.Join(cfgload.DirName, "agent.yaml")),
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
	for _, path := range []string{
		filepath.Join(prepared.Descriptor.SetupLogsDir, "agenttest.log"),
		filepath.Join(prepared.Descriptor.SetupTelemetryDir, "agenttest.jsonl"),
		filepath.Join(prepared.Descriptor.ExecutionLogsDir, "agenttest.log"),
		filepath.Join(prepared.Descriptor.ExecutionTelemetryDir, "agenttest.jsonl"),
		preparedRunReportPath(prepared.Descriptor),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := VerifyPreparedRun(context.Background(), prepared, CaseReport{Success: true, Output: "ok"}, suite, CaseSpec{Name: "smoke"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report == nil || !report.Success {
		t.Fatalf("expected successful verification report, got %+v", report)
	}
	if _, err := os.Stat(preparedRunVerificationPath(prepared.Descriptor)); err != nil {
		t.Fatalf("verification report not written: %v", err)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected artifact checks")
	}
}

func TestEvaluateFileContentExpectations(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "hello.go")
	if err := os.WriteFile(path, []byte("package hello\n\nfunc Hello() string {\n  return \"hello world\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, failures := evaluateFileContentExpectations([]FileContentExpectation{
		{
			Path:     "hello.go",
			Contains: []string{"hello world"},
		},
	}, workspace)
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %v", failures)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected a passing result, got %+v", results)
	}

	results, failures = evaluateFileContentExpectations([]FileContentExpectation{
		{
			Path:     "hello.go",
			Contains: []string{"goodbye"},
		},
	}, workspace)
	if len(failures) == 0 {
		t.Fatal("expected failure for missing content")
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("expected a failing result, got %+v", results)
	}
}
