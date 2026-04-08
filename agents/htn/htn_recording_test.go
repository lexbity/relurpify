package htn

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// --- stubs ------------------------------------------------------------------

// stubRuntimeStore implements memory.RuntimeMemoryStore for testing.
type stubRuntimeStore struct {
	declarativeRecords []memory.DeclarativeMemoryRecord
}

func (s *stubRuntimeStore) PutDeclarative(_ context.Context, record memory.DeclarativeMemoryRecord) error {
	s.declarativeRecords = append(s.declarativeRecords, record)
	return nil
}
func (s *stubRuntimeStore) GetDeclarative(_ context.Context, _ string) (*memory.DeclarativeMemoryRecord, bool, error) {
	return nil, false, nil
}
func (s *stubRuntimeStore) SearchDeclarative(_ context.Context, _ memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	return nil, nil
}
func (s *stubRuntimeStore) PutProcedural(_ context.Context, _ memory.ProceduralMemoryRecord) error {
	return nil
}
func (s *stubRuntimeStore) GetProcedural(_ context.Context, _ string) (*memory.ProceduralMemoryRecord, bool, error) {
	return nil, false, nil
}
func (s *stubRuntimeStore) SearchProcedural(_ context.Context, _ memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	return nil, nil
}

// stubWorkflowSurface implements the narrowed workflow interface used by recordingPrimitiveAgent.
type stubWorkflowSurface struct {
	knowledge []memory.KnowledgeRecord
	events    []memory.WorkflowEventRecord
}

func (s *stubWorkflowSurface) PutKnowledge(_ context.Context, record memory.KnowledgeRecord) error {
	s.knowledge = append(s.knowledge, record)
	return nil
}
func (s *stubWorkflowSurface) AppendEvent(_ context.Context, record memory.WorkflowEventRecord) error {
	s.events = append(s.events, record)
	return nil
}

func makeStepTask(stepID, description string) *core.Task {
	return &core.Task{
		ID: "task-" + stepID,
		Context: map[string]any{
			"current_step": core.PlanStep{ID: stepID, Description: description},
		},
	}
}

// --- persistStep tests ------------------------------------------------------

func TestRecordingPrimitiveAgent_SkipsWhenNoCurrentStep(t *testing.T) {
	rt := &stubRuntimeStore{}
	wf := &stubWorkflowSurface{}
	a := &recordingPrimitiveAgent{runtime: rt, workflow: wf, workflowID: "wf-1", runID: "run-1"}
	task := &core.Task{ID: "no-step", Context: map[string]any{}}
	a.persistStep(context.Background(), task, &core.Result{Success: true}, nil)
	if len(rt.declarativeRecords) != 0 {
		t.Fatalf("expected no runtime records, got %d", len(rt.declarativeRecords))
	}
	if len(wf.knowledge) != 0 || len(wf.events) != 0 {
		t.Fatalf("expected no workflow records, got %d knowledge %d events", len(wf.knowledge), len(wf.events))
	}
}

func TestRecordingPrimitiveAgent_SkipsWhenNilTask(t *testing.T) {
	rt := &stubRuntimeStore{}
	a := &recordingPrimitiveAgent{runtime: rt, workflowID: "wf-nil"}
	a.persistStep(context.Background(), nil, nil, nil)
	if len(rt.declarativeRecords) != 0 {
		t.Fatalf("expected no records for nil task, got %d", len(rt.declarativeRecords))
	}
}

func TestRecordingPrimitiveAgent_PersistsSuccessToRuntimeStore(t *testing.T) {
	rt := &stubRuntimeStore{}
	a := &recordingPrimitiveAgent{runtime: rt, workflowID: "wf-2", runID: "run-2"}
	task := makeStepTask("step-1", "do the work")
	task.ID = "task-parent"
	a.persistStep(context.Background(), task, &core.Result{Success: true, Data: map[string]any{"text": "all done"}}, nil)

	if len(rt.declarativeRecords) != 1 {
		t.Fatalf("expected 1 declarative record, got %d", len(rt.declarativeRecords))
	}
	rec := rt.declarativeRecords[0]
	if rec.WorkflowID != "wf-2" {
		t.Errorf("unexpected workflow_id: %q", rec.WorkflowID)
	}
	if rec.TaskID != "task-parent" {
		t.Errorf("unexpected task_id: %q", rec.TaskID)
	}
	if rec.Title != "do the work" {
		t.Errorf("unexpected title: %q", rec.Title)
	}
	if rec.Content != "all done" {
		t.Errorf("unexpected content: %q", rec.Content)
	}
	if !rec.Verified {
		t.Error("expected verified=true for successful step")
	}
	if rec.Kind != memory.DeclarativeMemoryKindFact {
		t.Errorf("unexpected kind: %q", rec.Kind)
	}
	if rec.Metadata["status"] != "completed" {
		t.Errorf("unexpected status metadata: %v", rec.Metadata["status"])
	}
	if rec.Metadata["step_id"] != "step-1" {
		t.Errorf("unexpected step_id metadata: %v", rec.Metadata["step_id"])
	}
}

func TestRecordingPrimitiveAgent_MarksFailureUnverified(t *testing.T) {
	rt := &stubRuntimeStore{}
	a := &recordingPrimitiveAgent{runtime: rt, workflowID: "wf-3", runID: "run-3"}
	task := makeStepTask("step-fail", "risky step")
	a.persistStep(context.Background(), task, nil, errors.New("network timeout"))

	if len(rt.declarativeRecords) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rt.declarativeRecords))
	}
	rec := rt.declarativeRecords[0]
	if rec.Verified {
		t.Error("expected verified=false for failed step")
	}
	if rec.Content != "network timeout" {
		t.Errorf("unexpected content: %q", rec.Content)
	}
	if rec.Metadata["status"] != "failed" {
		t.Errorf("unexpected status: %v", rec.Metadata["status"])
	}
}

func TestRecordingPrimitiveAgent_PersistsSuccessToWorkflow(t *testing.T) {
	wf := &stubWorkflowSurface{}
	a := &recordingPrimitiveAgent{workflow: wf, workflowID: "wf-4", runID: "run-4"}
	task := makeStepTask("step-ok", "normal step")
	a.persistStep(context.Background(), task, &core.Result{Success: true}, nil)

	if len(wf.knowledge) != 1 {
		t.Fatalf("expected 1 knowledge record, got %d", len(wf.knowledge))
	}
	if len(wf.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(wf.events))
	}
	k := wf.knowledge[0]
	if k.Kind != memory.KnowledgeKindFact {
		t.Errorf("unexpected kind: %q", k.Kind)
	}
	if k.Status != "accepted" {
		t.Errorf("unexpected status: %q", k.Status)
	}
	if k.WorkflowID != "wf-4" {
		t.Errorf("unexpected workflow_id: %q", k.WorkflowID)
	}
	if k.StepID != "step-ok" {
		t.Errorf("unexpected step_id: %q", k.StepID)
	}
	ev := wf.events[0]
	if ev.EventType != "step_completed" {
		t.Errorf("unexpected event type: %q", ev.EventType)
	}
	if ev.RunID != "run-4" {
		t.Errorf("unexpected run_id: %q", ev.RunID)
	}
}

func TestRecordingPrimitiveAgent_PersistsFailureToWorkflow(t *testing.T) {
	wf := &stubWorkflowSurface{}
	a := &recordingPrimitiveAgent{workflow: wf, workflowID: "wf-5", runID: "run-5"}
	task := makeStepTask("step-err", "fragile step")
	a.persistStep(context.Background(), task, nil, errors.New("boom"))

	if len(wf.knowledge) != 1 {
		t.Fatalf("expected 1 knowledge record, got %d", len(wf.knowledge))
	}
	k := wf.knowledge[0]
	if k.Kind != memory.KnowledgeKindIssue {
		t.Errorf("expected issue kind, got %q", k.Kind)
	}
	if k.Status != "open" {
		t.Errorf("expected open status, got %q", k.Status)
	}
	ev := wf.events[0]
	if ev.EventType != "step_failed" {
		t.Errorf("expected step_failed event, got %q", ev.EventType)
	}
	if ev.Message != "boom" {
		t.Errorf("unexpected event message: %q", ev.Message)
	}
}

func TestRecordingPrimitiveAgent_SkipsWorkflowWhenIDEmpty(t *testing.T) {
	wf := &stubWorkflowSurface{}
	a := &recordingPrimitiveAgent{workflow: wf, workflowID: "", runID: "run-6"}
	task := makeStepTask("step-noid", "no workflow id")
	a.persistStep(context.Background(), task, &core.Result{Success: true}, nil)
	if len(wf.knowledge) != 0 || len(wf.events) != 0 {
		t.Fatalf("expected no workflow records when workflowID empty, got %d/%d", len(wf.knowledge), len(wf.events))
	}
}

func TestRecordingPrimitiveAgent_NilDelegateExecuteReturnsSuccess(t *testing.T) {
	a := &recordingPrimitiveAgent{}
	result, err := a.Execute(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatal("expected success from nil delegate")
	}
}

func TestRecordingPrimitiveAgent_PointerStepInContext(t *testing.T) {
	rt := &stubRuntimeStore{}
	a := &recordingPrimitiveAgent{runtime: rt, workflowID: "wf-ptr"}
	task := &core.Task{
		ID: "task-ptr",
		Context: map[string]any{
			"current_step": &core.PlanStep{ID: "ptr-step", Description: "  pointer step  "},
		},
	}
	a.persistStep(context.Background(), task, &core.Result{Success: true}, nil)
	if len(rt.declarativeRecords) != 1 {
		t.Fatalf("expected 1 record for pointer step, got %d", len(rt.declarativeRecords))
	}
	if rt.declarativeRecords[0].Title != "pointer step" {
		t.Errorf("unexpected title: %q", rt.declarativeRecords[0].Title)
	}
}

func TestRecordingPrimitiveAgent_NilPtrStepInContextSkips(t *testing.T) {
	rt := &stubRuntimeStore{}
	a := &recordingPrimitiveAgent{runtime: rt, workflowID: "wf-nilptr"}
	var nilStep *core.PlanStep
	task := &core.Task{
		ID:      "task-nilptr",
		Context: map[string]any{"current_step": nilStep},
	}
	a.persistStep(context.Background(), task, &core.Result{Success: true}, nil)
	if len(rt.declarativeRecords) != 0 {
		t.Fatalf("expected no records for nil ptr step, got %d", len(rt.declarativeRecords))
	}
}
