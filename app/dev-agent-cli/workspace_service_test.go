package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/core"
)

type countingService struct {
	starts int
	stops  int
}

func (s *countingService) Start(ctx context.Context) error {
	s.starts++
	return nil
}

func (s *countingService) Stop() error {
	s.stops++
	return nil
}

func stubWorkspaceSession(t *testing.T, serviceIDs ...string) (*countingService, *countingService) {
	t.Helper()
	first := &countingService{}
	second := &countingService{}
	origOpenWorkspace := openWorkspaceFn
	t.Cleanup(func() { openWorkspaceFn = origOpenWorkspace })
	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		sm := ayenitd.NewServiceManager()
		for i, id := range serviceIDs {
			switch i {
			case 0:
				sm.Register(id, first)
			case 1:
				sm.Register(id, second)
			default:
				sm.Register(id, &countingService{})
			}
		}
		return &ayenitd.Workspace{
			Environment: ayenitd.WorkspaceEnvironment{
				Config: &core.Config{Name: cfg.AgentName, Model: cfg.InferenceModel, InferenceEndpoint: cfg.InferenceEndpoint},
			},
			ServiceManager: sm,
		}, nil
	}
	return first, second
}

func TestWorkspaceProbeFailsRequiredCheck(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	origProbe := probeWorkspaceFn
	t.Cleanup(func() { probeWorkspaceFn = origProbe })
	var captured ayenitd.WorkspaceConfig
	probeWorkspaceFn = func(cfg ayenitd.WorkspaceConfig) []ayenitd.ProbeResult {
		captured = cfg
		return []ayenitd.ProbeResult{
			{Name: "workspace_directory", Required: true, OK: false, Message: "missing"},
			{Name: "disk_space", Required: false, OK: true, Message: "ok"},
		}
	}
	cmd := newWorkspaceProbeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected required probe failure")
	}
	if !captured.SkipASTIndex {
		t.Fatal("expected probe config to skip AST indexing")
	}
	if !strings.Contains(out.String(), "workspace_directory") {
		t.Fatalf("unexpected probe output: %q", out.String())
	}
}

func TestWorkspaceStatusPrintsResolvedSummary(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	cmd := newWorkspaceStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "workspace:") {
		t.Fatalf("unexpected status output: %q", out.String())
	}
	if !strings.Contains(out.String(), "manifest:") {
		t.Fatalf("status output missing manifest path: %q", out.String())
	}
}

func TestWorkspaceInitCreatesConfig(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)

	cmd := newWorkspaceInitCmd()
	if err := cmd.Flags().Set("agent", "testfu"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("model", "test-model"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ws, "relurpify_cfg", "relurpify.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "default_model:") {
		t.Fatalf("init config missing default_model: %s", string(data))
	}
	if !strings.Contains(string(data), "agent: testfu") {
		t.Fatalf("init config missing agent hint: %s", string(data))
	}
	if !strings.Contains(out.String(), "Created workspace config") {
		t.Fatalf("unexpected init output: %q", out.String())
	}
}

func TestServiceListPrintsIDs(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	stubWorkspaceSession(t, "scheduler", "bkc.workspace_bootstrap")

	cmd := newServiceListCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "scheduler") || !strings.Contains(out.String(), "bkc.workspace_bootstrap") {
		t.Fatalf("unexpected service list output: %q", out.String())
	}
}

func TestServiceRestartCallsServiceLifecycle(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	first, _ := stubWorkspaceSession(t, "scheduler")

	cmd := newServiceRestartCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{"scheduler"}); err != nil {
		t.Fatal(err)
	}
	if first.starts < 2 {
		t.Fatalf("expected service to be started twice, got %d", first.starts)
	}
	if first.stops < 1 {
		t.Fatalf("expected service to be stopped once, got %d", first.stops)
	}
	if !strings.Contains(out.String(), "Restarted service scheduler") {
		t.Fatalf("unexpected restart output: %q", out.String())
	}
}

func TestServiceRestartUnknownID(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)
	stubWorkspaceSession(t, "scheduler")

	cmd := newServiceRestartCmd()
	if err := cmd.RunE(cmd, []string{"missing"}); err == nil {
		t.Fatal("expected missing service error")
	}
}
