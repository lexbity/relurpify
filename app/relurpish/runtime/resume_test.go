package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// fakeLifecycleRepo records lifecycle writes; unlisted operations are not
// exercised by the runtime turn path.
type fakeLifecycleRepo struct {
	agentlifecycle.Repository

	mu        sync.Mutex
	workflows map[string]*agentlifecycle.WorkflowRecord
	runs      map[string]*agentlifecycle.WorkflowRunRecord
	statuses  []string
}

func newFakeLifecycleRepo() *fakeLifecycleRepo {
	return &fakeLifecycleRepo{
		workflows: map[string]*agentlifecycle.WorkflowRecord{},
		runs:      map[string]*agentlifecycle.WorkflowRunRecord{},
	}
}

func (f *fakeLifecycleRepo) CreateWorkflow(_ context.Context, workflow agentlifecycle.WorkflowRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	record := workflow
	f.workflows[workflow.WorkflowID] = &record
	return nil
}

func (f *fakeLifecycleRepo) GetWorkflow(_ context.Context, workflowID string) (*agentlifecycle.WorkflowRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if record, ok := f.workflows[workflowID]; ok {
		return record, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeLifecycleRepo) CreateRun(_ context.Context, run agentlifecycle.WorkflowRunRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	record := run
	f.runs[run.RunID] = &record
	return nil
}

func (f *fakeLifecycleRepo) UpdateRunStatus(_ context.Context, runID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, status)
	if record, ok := f.runs[runID]; ok {
		record.Status = status
		now := time.Now()
		record.CompletedAt = &now
	}
	return nil
}

func (f *fakeLifecycleRepo) seedWorkflow(workflowID, instruction string) {
	_ = f.CreateWorkflow(context.Background(), agentlifecycle.WorkflowRecord{
		WorkflowID: workflowID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Metadata:   map[string]any{"instruction": instruction},
	})
}

func TestResumeExecutesARealTurnFromTheRecordedTask(t *testing.T) {
	repo := newFakeLifecycleRepo()
	repo.seedWorkflow("wf-1", "fix the failing test in pkg/x")
	executor := &recordingExecutor{}
	rt := &Runtime{Config: Config{Workspace: "/workspace"}, AgentLifecycle: repo}
	rt.setAgent(executor)

	outcome, err := rt.ResumeSession(context.Background(), "wf-1", "")
	if err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if executor.execCount != 1 {
		t.Fatalf("executed turns = %d, want 1", executor.execCount)
	}
	if outcome.TaskID == "" {
		t.Fatal("outcome must carry the executed task ID")
	}
	if outcome.WorkflowID != "wf-1" {
		t.Fatalf("outcome workflow = %q, want wf-1", outcome.WorkflowID)
	}
	if !strings.Contains(executor.lastTask.Instruction, "fix the failing test in pkg/x") {
		t.Fatalf("re-grounding instruction = %q", executor.lastTask.Instruction)
	}
	if got := rt.ActiveWorkflowID(); got != "" {
		t.Fatalf("active workflow ID after resume = %q, want empty", got)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	completed := 0
	for _, run := range repo.runs {
		if run.Status == "completed" {
			completed++
		}
	}
	if completed != 1 {
		t.Fatalf("completed run records = %d, want 1", completed)
	}
}

func TestResumeFollowUpOverridesRegrounding(t *testing.T) {
	repo := newFakeLifecycleRepo()
	repo.seedWorkflow("wf-2", "original task")
	executor := &recordingExecutor{}
	rt := &Runtime{Config: Config{Workspace: "/workspace"}, AgentLifecycle: repo}
	rt.setAgent(executor)

	if _, err := rt.ResumeSession(context.Background(), "wf-2", "now also update the docs"); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if executor.lastTask.Instruction != "now also update the docs" {
		t.Fatalf("instruction = %q, want the caller's follow-up", executor.lastTask.Instruction)
	}
}

func TestResumeRefusedWhileRunActive(t *testing.T) {
	executor := &recordingExecutor{}
	rt := &Runtime{Config: Config{Workspace: "/workspace"}, AgentLifecycle: newFakeLifecycleRepo()}
	rt.setAgent(executor)
	rt.setActiveWorkflowID("wf-active")

	_, err := rt.ResumeSession(context.Background(), "wf-1", "")
	if !errors.Is(err, ErrResumeBusy) {
		t.Fatalf("err = %v, want ErrResumeBusy", err)
	}
}

func TestResumeTranscriptOnlyPathRequiresFollowUp(t *testing.T) {
	executor := &recordingExecutor{}
	rt := &Runtime{Config: Config{Workspace: "/workspace"}, AgentLifecycle: newFakeLifecycleRepo()}
	rt.setAgent(executor)

	if _, err := rt.ResumeSession(context.Background(), "", ""); err == nil || !strings.Contains(err.Error(), "follow-up") {
		t.Fatalf("err = %v, want follow-up requirement", err)
	}
	outcome, err := rt.ResumeSession(context.Background(), "", "ship it")
	if err != nil {
		t.Fatalf("ResumeSession with follow-up: %v", err)
	}
	if executor.execCount != 1 || outcome.TaskID == "" {
		t.Fatalf("executed=%d taskID=%q, want a real executed turn", executor.execCount, outcome.TaskID)
	}
}

func TestResumeUnknownWorkflowFailsLoudly(t *testing.T) {
	executor := &recordingExecutor{}
	rt := &Runtime{Config: Config{Workspace: "/workspace"}, AgentLifecycle: newFakeLifecycleRepo()}
	rt.setAgent(executor)

	if _, err := rt.ResumeSession(context.Background(), "wf-missing", ""); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want workflow-not-found", err)
	}
	if executor.execCount != 0 {
		t.Fatalf("executed turns = %d, want 0", executor.execCount)
	}
}

func TestRunTaskCreatesResumableWorkflowRecord(t *testing.T) {
	repo := newFakeLifecycleRepo()
	executor := &recordingExecutor{}
	rt := &Runtime{Config: Config{Workspace: "/workspace"}, AgentLifecycle: repo}
	rt.setAgent(executor)

	result, err := rt.RunTask(context.Background(), &execution.Task{ID: "task-9", Instruction: "implement the feature", Type: string(execution.TaskTypeExecute)})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if got := rt.ActiveWorkflowID(); got != "" {
		t.Fatalf("active workflow ID after run = %q, want empty", got)
	}
	stamped, _ := result.Metadata["workflow_id"].(string)
	if stamped == "" {
		t.Fatal("result metadata must carry the workflow ID for the autosave")
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	workflow, ok := repo.workflows[stamped]
	if !ok {
		t.Fatalf("workflow %q not persisted", stamped)
	}
	if workflow.Metadata["instruction"] != "implement the feature" {
		t.Fatalf("workflow instruction = %v", workflow.Metadata["instruction"])
	}
	for _, run := range repo.runs {
		if run.Status != "completed" {
			t.Fatalf("run status = %q, want completed", run.Status)
		}
	}
	if len(repo.statuses) == 0 || repo.statuses[len(repo.statuses)-1] != "completed" {
		t.Fatalf("run status transitions = %v", repo.statuses)
	}
}

func TestActiveWorkflowIDClearedOnlyForItsOwnRun(t *testing.T) {
	rt := &Runtime{}
	rt.setActiveWorkflowID("wf-a")
	rt.setActiveWorkflowID("wf-b")
	rt.clearActiveWorkflowID("wf-a")
	if got := rt.ActiveWorkflowID(); got != "wf-b" {
		t.Fatalf("active workflow ID = %q, want wf-b (a stale defer must not clear a newer run)", got)
	}
	rt.clearActiveWorkflowID("wf-b")
	if got := rt.ActiveWorkflowID(); got != "" {
		t.Fatalf("active workflow ID = %q, want empty", got)
	}
}
