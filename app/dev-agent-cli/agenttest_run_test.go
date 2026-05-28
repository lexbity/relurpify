package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func TestNewAgentTestPreparedRunCmdRegistersDescriptorFlag(t *testing.T) {
	cmd := newAgentTestPreparedRunCmd()
	if cmd == nil {
		t.Fatal("expected command")
	}
	if cmd.Flags().Lookup("descriptor") == nil {
		t.Fatal("expected descriptor flag")
	}
}

func TestPreparedRunDescriptorLoadRejectsMissingPath(t *testing.T) {
	if _, err := loadPreparedRunDescriptor(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected missing descriptor to fail")
	}
}

func TestExecutePreparedRunUsesDescriptorWorkspaceAndBackend(t *testing.T) {
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
  image: ghcr.io/lexcodex/relurpify/runtime:0.4.1
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

	suite := &agenttest.Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   agenttest.SuiteMeta{Name: "euclo.code"},
		Spec: agenttest.SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(cfgload.DirName, "agent.yaml")),
			Models: []agenttest.ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}
	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-1")
	desc, err := agenttest.BuildPreparedRunDescriptor(suite, agenttest.CaseSpec{Name: "smoke", Prompt: "hello"}, suite.Spec.Models[0], agenttest.RunOptions{}, workspace, runRoot, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	desc.BackendBinary = "ollama"
	desc.BackendService = "ollama"
	desc.BackendResetStrategy = "restart"
	descPath := filepath.Join(desc.SetupDir, "prepared_run.json")
	if err := desc.Write(descPath); err != nil {
		t.Fatal(err)
	}

	originalOpen := openPreparedRunWorkspaceFn
	t.Cleanup(func() { openPreparedRunWorkspaceFn = originalOpen })
	openPreparedRunWorkspaceFn = func(ctx context.Context, d *agenttest.PreparedRunDescriptor, outputRoot string, opts preparedRunOverrides) (*agentenv.Workspace, *preparedRunWorkspaceTarget, error) {
		target, err := buildPreparedRunWorkspaceTarget(d, outputRoot, opts)
		if err != nil {
			return nil, nil, err
		}
		ws := &agentenv.Workspace{ServiceManager: agentenv.NewServiceManager()}
		ws.ServiceManager.Register("alpha", fakePreparedRunService{})
		return ws, target, nil
	}
	originalExecute := executePreparedRunAgentTaskFn
	t.Cleanup(func() { executePreparedRunAgentTaskFn = originalExecute })
	executePreparedRunAgentTaskFn = func(ctx context.Context, ws *agentenv.Workspace, desc *agenttest.PreparedRunDescriptor, out io.Writer) (*core.Result, error) {
		if out != nil {
			_, _ = out.Write([]byte("prepared execution complete"))
		}
		return &core.Result{
			Success: true,
			NodeID:  "node-1",
			Data: core.NewToolResultPayload(map[string]any{
				"projection": map[string]any{
					"plan_id":   "plan-1",
					"stable_id": "mutation-1",
					"status":    "updated",
				},
			}),
		}, nil
	}

	var out bytes.Buffer
	if err := executePreparedRunToWriter(context.Background(), descPath, runRoot, preparedRunOverrides{}, "alpha", &out); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	expectedWorkspace := filepath.Join(runRoot, "execution", "workspace")
	if !strings.Contains(got, "workspace: "+expectedWorkspace) {
		t.Fatalf("expected workspace root from run output, got %q", got)
	}
	if !strings.Contains(got, "config: "+filepath.Join(expectedWorkspace, "relurpify_cfg", "config.yaml")) {
		t.Fatalf("expected config path from execution workspace, got %q", got)
	}
	if !strings.Contains(got, "backend: ollama/http://127.0.0.1:11434") {
		t.Fatalf("expected backend details from descriptor, got %q", got)
	}
	setupLogPath := filepath.Join(desc.SetupLogsDir, "agenttest.log")
	setupLogBytes, err := os.ReadFile(setupLogPath)
	if err != nil {
		t.Fatalf("read setup log: %v", err)
	}
	setupLog := string(setupLogBytes)
	if !strings.Contains(setupLog, "starting services") {
		t.Fatalf("expected setup log to include service start message, got %q", setupLog)
	}
	if !strings.Contains(setupLog, "restarting service") {
		t.Fatalf("expected setup log to include service restart message, got %q", setupLog)
	}
	setupTelemetryPath := filepath.Join(desc.SetupTelemetryDir, "agenttest.jsonl")
	setupEvents, err := agenttest.ReadTelemetryJSONL(setupTelemetryPath)
	if err != nil {
		t.Fatalf("read setup telemetry: %v", err)
	}
	if len(setupEvents) == 0 {
		t.Fatal("expected setup telemetry events")
	}
	foundBackendMetadata := false
	for _, ev := range setupEvents {
		if ev.Type != core.EventType("prepared_run.setup_start") {
			continue
		}
		if ev.Metadata["backend_family"] == "ollama" && ev.Metadata["backend_endpoint"] == "http://127.0.0.1:11434" && ev.Metadata["backend_binary"] == "ollama" && ev.Metadata["backend_service"] == "ollama" {
			foundBackendMetadata = true
			break
		}
	}
	if !foundBackendMetadata {
		t.Fatalf("expected backend metadata in setup telemetry, got %+v", setupEvents)
	}
	executionTelemetryPath := filepath.Join(runRoot, "execution", "telemetry", "agenttest.jsonl")
	executionEvents, err := agenttest.ReadTelemetryJSONL(executionTelemetryPath)
	if err != nil {
		t.Fatalf("read execution telemetry: %v", err)
	}
	if len(executionEvents) == 0 {
		t.Fatal("expected execution telemetry events")
	}
	reportPath := filepath.Join(runRoot, "execution", "report.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected report at %s: %v", reportPath, err)
	}
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	projection, ok := report["projection"].(map[string]any)
	if !ok {
		t.Fatalf("expected projection in report, got %#v", report["projection"])
	}
	if projection["plan_id"] != "plan-1" || projection["stable_id"] != "mutation-1" {
		t.Fatalf("unexpected projection summary: %#v", projection)
	}
}

func TestPreparedRunOverridesApplyToDescriptor(t *testing.T) {
	desc := &agenttest.PreparedRunDescriptor{
		BackendProvider: "ollama",
		BackendFamily:   "ollama",
		BackendEndpoint: "http://127.0.0.1:11434",
		BackendBinary:   "ollama",
		BackendService:  "ollama",
	}
	applyPreparedRunOverrides(desc, preparedRunOverrides{
		BackendProvider: "lmstudio",
		BackendFamily:   "lmstudio",
		BackendEndpoint: "http://127.0.0.1:1234",
		BackendBinary:   "lmstudio",
		BackendService:  "lmstudio",
	})
	if desc.BackendProvider != "lmstudio" || desc.BackendFamily != "lmstudio" || desc.BackendEndpoint != "http://127.0.0.1:1234" {
		t.Fatalf("overrides not applied: %+v", desc)
	}
}

type fakePreparedRunService struct{}

func (fakePreparedRunService) Start(context.Context) error { return nil }
func (fakePreparedRunService) Stop() error                 { return nil }
