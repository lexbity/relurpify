package agenttest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/manifest"
)

func TestRunnerRunSuiteUsesPreparedRunExecutor(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
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
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(manifest.DirName, "agent.manifest.yaml")),
			Recording: RecordingSpec{Mode: "live"},
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
			Cases: []CaseSpec{{Name: "smoke", Prompt: "hello"}},
		},
	}

	originalExecutor := PreparedRunExecutorFn
	t.Cleanup(func() { PreparedRunExecutorFn = originalExecutor })
	var gotDescriptor string
	var gotOutputRoot string
	PreparedRunExecutorFn = func(ctx context.Context, descriptorPath string, outputRoot string, serviceID string, out io.Writer) error {
		gotDescriptor = descriptorPath
		gotOutputRoot = outputRoot
		if out != nil {
			_, _ = out.Write([]byte("prepared execution complete"))
		}
		return nil
	}

	report, err := (&Runner{}).RunSuite(context.Background(), suite, RunOptions{TargetWorkspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if gotDescriptor == "" {
		t.Fatal("expected prepared run executor to be called")
	}
	if gotOutputRoot == "" {
		t.Fatal("expected output root to be passed to prepared run executor")
	}
	if len(report.Cases) != 1 || !report.Cases[0].Success {
		t.Fatalf("expected prepared run case success, got %+v", report.Cases)
	}
	if !strings.Contains(report.Cases[0].Output, "prepared execution complete") {
		t.Fatalf("expected output captured from prepared executor, got %q", report.Cases[0].Output)
	}
}
