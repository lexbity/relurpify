package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	eucloexec "codeburg.org/lexbit/relurpify/named/euclo/execution"
)

type stubArchaeoInspector struct {
	requestHistory *eucloexec.RequestHistoryView
	activePlan     *eucloexec.ActivePlanView
	learningQueue  *eucloexec.LearningQueueView
	tensions       []eucloexec.TensionView
}

func (s *stubArchaeoInspector) RequestHistory(ctx context.Context, workflowID string) (*eucloexec.RequestHistoryView, error) {
	return s.requestHistory, nil
}

func (s *stubArchaeoInspector) ActivePlan(ctx context.Context, workflowID string) (*eucloexec.ActivePlanView, error) {
	return s.activePlan, nil
}

func (s *stubArchaeoInspector) LearningQueue(ctx context.Context, workflowID string) (*eucloexec.LearningQueueView, error) {
	return s.learningQueue, nil
}

func (s *stubArchaeoInspector) TensionsByWorkflow(ctx context.Context, workflowID string) ([]eucloexec.TensionView, error) {
	return s.tensions, nil
}

func stubArchaeoWorkspace(t *testing.T, store memory.WorkflowStateStore, service *countingService, inspector archaeoInspector) *ayenitd.WorkspaceConfig {
	t.Helper()
	origOpenWorkspace := openWorkspaceFn
	origInspectorFn := newArchaeoInspectorFn
	t.Cleanup(func() {
		openWorkspaceFn = origOpenWorkspace
		newArchaeoInspectorFn = origInspectorFn
	})

	var captured ayenitd.WorkspaceConfig
	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		captured = cfg
		sm := ayenitd.NewServiceManager()
		if service != nil {
			sm.Register("archaeo-test", service)
		}
		return &ayenitd.Workspace{
			Environment: ayenitd.WorkspaceEnvironment{
				WorkflowStore: store,
			},
			ServiceManager: sm,
		}, nil
	}
	newArchaeoInspectorFn = func(ws *ayenitd.Workspace) archaeoInspector {
		return inspector
	}
	return &captured
}

func TestArchaeoPlanPrintsActiveStepID(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	service := &countingService{}
	captured := stubArchaeoWorkspace(t, nil, service, &stubArchaeoInspector{
		activePlan: &eucloexec.ActivePlanView{
			WorkflowID:   "wf-1",
			Phase:        "execution",
			ActiveStepID: "step-2",
			ActivePlan: &eucloexec.VersionedPlanView{
				PlanID:  "plan-1",
				Version: 3,
				Status:  "active",
			},
		},
	})

	cmd := newArchaeoPlanCmd()
	if err := cmd.Flags().Set("workflow", "wf-1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	if !captured.SkipASTIndex {
		t.Fatal("expected inspection workspace to skip AST indexing")
	}
	if service.starts != 1 {
		t.Fatalf("expected service manager to start once, got %d", service.starts)
	}
	got := out.String()
	if !strings.Contains(got, "active step: step-2") {
		t.Fatalf("unexpected plan output: %q", got)
	}
	if !strings.Contains(got, "plan: plan-1 v3 (active)") {
		t.Fatalf("unexpected plan output: %q", got)
	}
}

func TestArchaeoPlanNoWorkflowID(t *testing.T) {
	cmd := newArchaeoPlanCmd()
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "--workflow is required") {
		t.Fatalf("expected missing workflow error, got %v", err)
	}
}

func TestArchaeoTensionsPrintsKindSeverity(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	stubArchaeoWorkspace(t, nil, nil, &stubArchaeoInspector{
		tensions: []eucloexec.TensionView{{
			ID:          "tension-1",
			Kind:        "consistency",
			Severity:    "high",
			Status:      "active",
			Description: "stale reference",
		}},
	})

	cmd := newArchaeoTensionsCmd()
	if err := cmd.Flags().Set("workflow", "wf-1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "kind=consistency") || !strings.Contains(got, "severity=high") || !strings.Contains(got, "status=active") {
		t.Fatalf("unexpected tensions output: %q", got)
	}
}

func TestArchaeoTensionsEmptyList(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	stubArchaeoWorkspace(t, nil, nil, &stubArchaeoInspector{})

	cmd := newArchaeoTensionsCmd()
	if err := cmd.Flags().Set("workflow", "wf-1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No tensions found for workflow wf-1.") {
		t.Fatalf("unexpected empty tensions output: %q", out.String())
	}
}

func TestArchaeoHistoryPrintsRequestCounts(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	stubArchaeoWorkspace(t, nil, nil, &stubArchaeoInspector{
		requestHistory: &eucloexec.RequestHistoryView{
			WorkflowID: "wf-1",
			Pending:    2,
			Running:    1,
			Completed:  4,
			Failed:     1,
			Canceled:   0,
			Requests: []eucloexec.RequestRecordView{{
				ID:      "req-1",
				Kind:    "analysis",
				Status:  "running",
				Summary: "resolve stale cache",
			}},
		},
	})

	cmd := newArchaeoHistoryCmd()
	if err := cmd.Flags().Set("workflow", "wf-1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "pending=2 running=1 completed=4 failed=1 canceled=0") {
		t.Fatalf("unexpected history output: %q", got)
	}
	if !strings.Contains(got, "req-1 | kind=analysis | status=running | resolve stale cache") {
		t.Fatalf("unexpected history output: %q", got)
	}
}

func TestArchaeoLearningPrintsPendingQueue(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	stubArchaeoWorkspace(t, nil, nil, &stubArchaeoInspector{
		learningQueue: &eucloexec.LearningQueueView{
			WorkflowID:         "wf-1",
			PendingGuidanceIDs: []string{"guidance-1", "guidance-2"},
			BlockingLearning:   []string{"learn-9"},
			PendingLearning: []eucloexec.LearningInteractionView{{
				ID:       "learn-1",
				Status:   "pending",
				Blocking: true,
				Prompt:   "confirm behavior",
			}},
		},
	})

	cmd := newArchaeoLearningCmd()
	if err := cmd.Flags().Set("workflow", "wf-1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "pending guidance: guidance-1, guidance-2") {
		t.Fatalf("unexpected learning output: %q", got)
	}
	if !strings.Contains(got, "blocking: learn-9") {
		t.Fatalf("unexpected learning output: %q", got)
	}
	if !strings.Contains(got, "learn-1 | status=pending | blocking=true | confirm behavior") {
		t.Fatalf("unexpected learning output: %q", got)
	}
}

func TestArchaeoWorkflowsListsIDs(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-old",
		TaskID:      "task-old",
		Instruction: "older",
		Status:      memory.WorkflowRunStatusCompleted,
		CreatedAt:   now.Add(-2 * time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-new",
		TaskID:      "task-new",
		Instruction: "newer",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}

	service := &countingService{}
	captured := stubArchaeoWorkspace(t, store, service, nil)

	cmd := newArchaeoWorkflowsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !captured.SkipASTIndex {
		t.Fatal("expected inspection workspace to skip AST indexing")
	}
	if service.starts != 1 {
		t.Fatalf("expected service manager to start once, got %d", service.starts)
	}
	if !strings.Contains(got, "wf-new") || !strings.Contains(got, "wf-old") {
		t.Fatalf("unexpected workflows output: %q", got)
	}
	if strings.Index(got, "wf-new") > strings.Index(got, "wf-old") {
		t.Fatalf("expected workflows to be sorted by update time, got %q", got)
	}
}

func TestArchaeoJSONFlag(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	service := &countingService{}
	captured := stubArchaeoWorkspace(t, nil, service, &stubArchaeoInspector{
		activePlan: &eucloexec.ActivePlanView{
			WorkflowID:   "wf-1",
			Phase:        "execution",
			ActiveStepID: "step-2",
			ActivePlan: &eucloexec.VersionedPlanView{
				PlanID:  "plan-1",
				Version: 3,
				Status:  "active",
			},
		},
	})

	cmd := newArchaeoCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--json", "plan", "--workflow", "wf-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !captured.SkipASTIndex {
		t.Fatal("expected inspection workspace to skip AST indexing")
	}
	if service.starts != 1 {
		t.Fatalf("expected service manager to start once, got %d", service.starts)
	}

	var got eucloexec.ActivePlanView
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal json output: %v; output=%q", err, out.String())
	}
	if got.WorkflowID != "wf-1" || got.ActiveStepID != "step-2" || got.ActivePlan == nil || got.ActivePlan.PlanID != "plan-1" {
		t.Fatalf("unexpected json output: %+v", got)
	}
}

func TestArchaeoOpenWorkspaceWithSkipASTIndex(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeAgentManifestFixture(t, ws, "testfu", true)

	origOpenWorkspace := openWorkspaceFn
	t.Cleanup(func() { openWorkspaceFn = origOpenWorkspace })
	captured := ayenitd.WorkspaceConfig{}
	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		captured = cfg
		return &ayenitd.Workspace{ServiceManager: ayenitd.NewServiceManager()}, nil
	}

	if _, err := openWorkspaceForInspection(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	if !captured.SkipASTIndex {
		t.Fatal("expected inspection workspace to skip AST indexing")
	}
}
