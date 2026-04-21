package htn

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestHTNHelperCheckpointAndResultFormatting(t *testing.T) {
	compacted := compactHTNCheckpointMap(map[string]any{
		"checkpoint_id":   "cp-1",
		"stage_name":      "dispatch",
		"stage_index":     2,
		"workflow_id":     "wf-1",
		"run_id":          "run-1",
		"schema_version":  1,
		"completed_steps": []any{"a", "b"},
		"snapshot":        map[string]any{"present": true},
	})
	if got := compacted["completed_steps"]; got != 2 {
		t.Fatalf("unexpected completed_steps count: %#v", got)
	}
	if got := compacted["has_snapshot"]; got != true {
		t.Fatalf("unexpected has_snapshot flag: %#v", got)
	}

	task := &core.Task{
		ID: "task-1",
		Context: map[string]any{
			"current_step": core.PlanStep{ID: "step-1", Description: "  do work  "},
		},
	}
	if stepID, title := htnStepMetadata(task); stepID != "step-1" || title != "do work" {
		t.Fatalf("unexpected step metadata: %q %q", stepID, title)
	}
	task.Context["current_step"] = &core.PlanStep{ID: "step-2", Description: "  more work  "}
	if stepID, title := htnStepMetadata(task); stepID != "step-2" || title != "more work" {
		t.Fatalf("unexpected pointer step metadata: %q %q", stepID, title)
	}
	if stepID, title := htnStepMetadata(nil); stepID != "" || title != "" {
		t.Fatalf("expected empty metadata for nil task, got %q %q", stepID, title)
	}

	if got := htnResultSummary(nil, nil); got != "step completed" {
		t.Fatalf("unexpected nil result summary: %q", got)
	}
	if got := htnResultSummary(&core.Result{Success: false, Data: map[string]any{"text": "  done  "}}, nil); got != "done" {
		t.Fatalf("unexpected text result summary: %q", got)
	}
	if got := htnResultSummary(&core.Result{Success: false, Data: map[string]any{}}, nil); got != "step completed" {
		t.Fatalf("unexpected empty result summary: %q", got)
	}
	if got := htnResultSummary(nil, errors.New("boom")); got != "boom" {
		t.Fatalf("unexpected error summary: %q", got)
	}

	if got := resultData(nil); got != nil {
		t.Fatalf("expected nil result data for nil result, got %#v", got)
	}
	if got := resultData(&core.Result{Success: true}); got != nil {
		t.Fatalf("expected nil result data for empty result, got %#v", got)
	}
	if got := resultData(&core.Result{Data: map[string]any{"answer": 42}}); got == nil {
		t.Fatal("expected result data to be returned")
	}

	if got := resultErrorText(&core.Result{Success: true}); got != "" {
		t.Fatalf("unexpected success error text: %q", got)
	}
	if got := resultErrorText(&core.Result{Success: false}); got != "step failed" {
		t.Fatalf("unexpected failure error text: %q", got)
	}
}

func TestRecordingPrimitiveAgentBranchExecutor(t *testing.T) {
	var agent recordingPrimitiveAgent
	branch, err := agent.BranchExecutor()
	if err != nil {
		t.Fatalf("unexpected branch error: %v", err)
	}
	if branch == nil {
		t.Fatal("expected branch executor")
	}
}
