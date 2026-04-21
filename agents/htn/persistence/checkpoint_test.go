package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

func TestCheckpointPersistenceEncodeAndDecode(t *testing.T) {
	// Create a sample HTN state snapshot
	snapshot := &runtime.HTNState{
		SchemaVersion: runtime.HTNSchemaVersion,
		Task: runtime.TaskState{
			ID:          "task_001",
			Type:        core.TaskType("code"),
			Instruction: "Write a function",
			Metadata: map[string]string{
				"priority": "high",
			},
		},
		Method: runtime.MethodState{
			Name:          "analyze_and_code",
			TaskType:      core.TaskType("code"),
			Priority:      1,
			SubtaskCount:  3,
			OperatorCount: 3,
		},
		Plan: &core.Plan{
			Goal: "Write and test the function",
			Steps: []core.PlanStep{
				{
					ID:          "analyze_and_code.1",
					Description: "Analyze requirements",
					Tool:        "react",
				},
				{
					ID:          "analyze_and_code.2",
					Description: "Write implementation",
					Tool:        "react",
				},
			},
		},
		Execution: runtime.ExecutionState{
			WorkflowID:         "wf_001",
			RunID:              "run_001",
			CompletedSteps:     []string{"analyze_and_code.1"},
			LastCompletedStep:  "analyze_and_code.1",
			PlannedStepCount:   2,
			CompletedStepCount: 1,
			Resumed:            false,
		},
		Metrics: runtime.Metrics{
			PlannedStepCount:   2,
			CompletedStepCount: 1,
		},
		Termination:        "",
		RetrievalApplied:   true,
		ResumeCheckpointID: "",
	}

	cp := &checkpointPersistence{
		workflowID: "wf_001",
		runID:      "run_001",
		taskID:     "task_001",
	}

	// Test encoding
	encoded, err := cp.encodeSnapshot(snapshot)
	if err != nil {
		t.Fatalf("Failed to encode snapshot: %v", err)
	}
	if encoded == "" {
		t.Fatal("Encoded snapshot is empty")
	}

	// Test decoding
	decoded, err := cp.decodeSnapshot(encoded)
	if err != nil {
		t.Fatalf("Failed to decode snapshot: %v", err)
	}

	// Verify decoded snapshot
	if decoded.Task.ID != snapshot.Task.ID {
		t.Errorf("Task ID mismatch: expected %s, got %s", snapshot.Task.ID, decoded.Task.ID)
	}
	if decoded.Method.Name != snapshot.Method.Name {
		t.Errorf("Method name mismatch: expected %s, got %s", snapshot.Method.Name, decoded.Method.Name)
	}
	if len(decoded.Execution.CompletedSteps) != len(snapshot.Execution.CompletedSteps) {
		t.Errorf("Completed steps mismatch: expected %d, got %d", len(snapshot.Execution.CompletedSteps), len(decoded.Execution.CompletedSteps))
	}
	if decoded.Execution.CompletedStepCount != snapshot.Execution.CompletedStepCount {
		t.Errorf("Completed step count mismatch: expected %d, got %d", snapshot.Execution.CompletedStepCount, decoded.Execution.CompletedStepCount)
	}
}

func TestCheckpointRestoreToContext(t *testing.T) {
	snapshot := &runtime.HTNState{
		SchemaVersion: runtime.HTNSchemaVersion,
		Task: runtime.TaskState{
			ID:          "task_001",
			Type:        core.TaskType("code"),
			Instruction: "Implement feature X",
		},
		Method: runtime.MethodState{
			Name:     "code_method",
			TaskType: core.TaskType("code"),
		},
		Plan: &core.Plan{
			Goal: "Feature implementation",
			Steps: []core.PlanStep{
				{ID: "step.1", Description: "Analyze"},
				{ID: "step.2", Description: "Implement"},
			},
		},
		Execution: runtime.ExecutionState{
			CompletedSteps:     []string{"step.1"},
			PlannedStepCount:   2,
			CompletedStepCount: 1,
		},
		Termination: "",
	}

	state := core.NewContext()
	cp := &checkpointPersistence{}

	err := cp.restoreSnapshotToContext(state, snapshot)
	if err != nil {
		t.Fatalf("Failed to restore snapshot: %v", err)
	}

	// Verify state is properly restored
	if raw, ok := state.Get(runtime.ContextKeyTask); !ok {
		t.Fatal("Task state not restored")
	} else {
		var restored runtime.TaskState
		if !runtime.DecodeContextValue(raw, &restored) {
			t.Fatal("Failed to decode restored task state")
		}
		if restored.ID != snapshot.Task.ID {
			t.Errorf("Task ID mismatch: expected %s, got %s", snapshot.Task.ID, restored.ID)
		}
	}

	if raw, ok := state.Get(runtime.ContextKeySelectedMethod); !ok {
		t.Fatal("Method state not restored")
	} else {
		var restored runtime.MethodState
		if !runtime.DecodeContextValue(raw, &restored) {
			t.Fatal("Failed to decode restored method state")
		}
		if restored.Name != snapshot.Method.Name {
			t.Errorf("Method name mismatch: expected %s, got %s", snapshot.Method.Name, restored.Name)
		}
	}

	if raw, ok := state.Get(runtime.ContextKeyPlan); !ok {
		t.Fatal("Plan not restored")
	} else {
		var restored core.Plan
		if !runtime.DecodeContextValue(raw, &restored) {
			t.Fatal("Failed to decode restored plan")
		}
		if len(restored.Steps) != len(snapshot.Plan.Steps) {
			t.Errorf("Plan steps mismatch: expected %d, got %d", len(snapshot.Plan.Steps), len(restored.Steps))
		}
	}

	completed := runtime.CompletedStepsFromContext(state)
	if len(completed) != 1 || completed[0] != "step.1" {
		t.Errorf("Completed steps mismatch: expected [step.1], got %v", completed)
	}
}

func TestCheckpointMetadataGeneration(t *testing.T) {
	snapshot := &runtime.HTNState{
		SchemaVersion: runtime.HTNSchemaVersion,
		Task: runtime.TaskState{
			ID:   "task_001",
			Type: core.TaskType("code"),
		},
		Method: runtime.MethodState{
			Name: "test_method",
		},
		Execution: runtime.ExecutionState{
			CompletedSteps:     []string{"step.1", "step.2"},
			PlannedStepCount:   3,
			CompletedStepCount: 2,
			LastCompletedStep:  "step.2",
		},
		Termination: "completed",
	}

	cp := &checkpointPersistence{}
	metadata := cp.checkpointMetadata(snapshot)

	if metadata["schema_version"] != runtime.HTNSchemaVersion {
		t.Errorf("Schema version mismatch: expected %d, got %v", runtime.HTNSchemaVersion, metadata["schema_version"])
	}
	if metadata["method_name"] != "test_method" {
		t.Errorf("Method name mismatch: expected test_method, got %v", metadata["method_name"])
	}
	if metadata["planned_steps"] != 3 {
		t.Errorf("Planned steps mismatch: expected 3, got %v", metadata["planned_steps"])
	}
	if metadata["completed_steps"] != 2 {
		t.Errorf("Completed steps mismatch: expected 2, got %v", metadata["completed_steps"])
	}
	if metadata["last_completed_step"] != "step.2" {
		t.Errorf("Last completed step mismatch: expected step.2, got %v", metadata["last_completed_step"])
	}
}

func TestSaveCheckpointPublishesArtifactReference(t *testing.T) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf_001",
		TaskID:      "task_001",
		TaskType:    core.TaskType("code"),
		Instruction: "Implement feature X",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run_001",
		WorkflowID: "wf_001",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "htn",
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	state := core.NewContext()
	runtime.PublishTaskState(state, &core.Task{
		ID:          "task_001",
		Type:        core.TaskType("code"),
		Instruction: "Implement feature X",
	})
	state.Set(runtime.ContextKeySelectedMethod, runtime.MethodState{
		Name:     "code_method",
		TaskType: core.TaskType("code"),
	})
	runtime.PublishPlanState(state, &core.Plan{
		Goal: "Feature implementation",
		Steps: []core.PlanStep{
			{ID: "step.1", Description: "Analyze"},
			{ID: "step.2", Description: "Implement"},
		},
	})
	runtime.PublishExecutionState(state, runtime.ExecutionState{
		WorkflowID:         "wf_001",
		RunID:              "run_001",
		CompletedSteps:     []string{"step.1"},
		LastCompletedStep:  "step.1",
		PlannedStepCount:   2,
		CompletedStepCount: 1,
	})

	if err := SaveCheckpoint(context.Background(), state, store, "wf_001", "run_001"); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	rawRef, ok := state.Get(runtime.ContextKeyCheckpointRef)
	if !ok {
		t.Fatal("expected checkpoint ref in state")
	}
	ref, ok := rawRef.(core.ArtifactReference)
	if !ok {
		t.Fatalf("expected core.ArtifactReference, got %T", rawRef)
	}
	if ref.Kind != "htn_checkpoint" {
		t.Fatalf("expected htn_checkpoint ref, got %q", ref.Kind)
	}
	if ref.WorkflowID != "wf_001" || ref.RunID != "run_001" {
		t.Fatalf("unexpected checkpoint ref scope: %#v", ref)
	}
	if state.GetString(runtime.ContextKeyCheckpointSummary) == "" {
		t.Fatal("expected checkpoint summary in state")
	}
}

func TestCheckpointSummarization(t *testing.T) {
	snapshot := &runtime.HTNState{
		SchemaVersion: runtime.HTNSchemaVersion,
		Task: runtime.TaskState{
			ID:   "task_001",
			Type: core.TaskType("code"),
		},
		Method: runtime.MethodState{
			Name: "analyze_method",
		},
		Execution: runtime.ExecutionState{
			PlannedStepCount:   5,
			CompletedStepCount: 3,
		},
		Termination: "in_progress",
	}

	cp := &checkpointPersistence{}
	summary := cp.summarizeCheckpoint(snapshot)

	if summary == "" {
		t.Fatal("Checkpoint summary is empty")
	}

	// Verify summary contains key information
	if !contains(summary, "task_001") {
		t.Error("Summary missing task ID")
	}
	if !contains(summary, "analyze_method") {
		t.Error("Summary missing method name")
	}
	if !contains(summary, "3/5") {
		t.Error("Summary missing step progress")
	}
}

func TestCheckpointIDGeneration(t *testing.T) {
	cp := &checkpointPersistence{}

	id1 := cp.generateCheckpointID()
	time.Sleep(time.Millisecond)
	id2 := cp.generateCheckpointID()

	if id1 == "" || id2 == "" {
		t.Fatal("Generated checkpoint ID is empty")
	}
	if id1 == id2 {
		t.Fatal("Generated checkpoint IDs are not unique")
	}
	if !contains(id1, "htn_checkpoint_") {
		t.Errorf("Invalid checkpoint ID format: %s", id1)
	}
}

func TestHTNStateValidationAfterDecode(t *testing.T) {
	// Create an invalid snapshot (missing schema version)
	snapshot := &runtime.HTNState{
		Task: runtime.TaskState{ID: "task_001"},
	}

	cp := &checkpointPersistence{}
	encoded, err := cp.encodeSnapshot(snapshot)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// Decoding should normalize and validate
	decoded, err := cp.decodeSnapshot(encoded)
	if err != nil {
		t.Fatalf("Failed to decode with validation: %v", err)
	}

	// Verify normalization occurred
	if decoded.SchemaVersion != runtime.HTNSchemaVersion {
		t.Errorf("Schema version not normalized: expected %d, got %d", runtime.HTNSchemaVersion, decoded.SchemaVersion)
	}
}

func TestPersistRecoveryMetadata(t *testing.T) {
	state := core.NewContext()

	diagnosis := "Retry with narrower scope"
	notes := []string{"Note 1", "Note 2"}
	stepID := "step.1"
	err := NewTestError("step failed")

	persistRecoveryMetadata(state, diagnosis, notes, stepID, err)

	// Verify metadata was persisted
	if diag := state.GetString(runtime.ContextKeyLastRecoveryDiag); diag != diagnosis {
		t.Errorf("Recovery diagnosis mismatch: expected %q, got %q", diagnosis, diag)
	}

	if raw, ok := state.Get(runtime.ContextKeyLastRecoveryNotes); ok {
		var restoredNotes []string
		if runtime.DecodeContextValue(raw, &restoredNotes) {
			if len(restoredNotes) != len(notes) {
				t.Errorf("Recovery notes count mismatch: expected %d, got %d", len(notes), len(restoredNotes))
			}
		}
	} else {
		t.Fatal("Recovery notes not persisted")
	}

	if step := state.GetString(runtime.ContextKeyLastFailureStep); step != stepID {
		t.Errorf("Failure step mismatch: expected %s, got %s", stepID, step)
	}
}

func TestPersistDispatchMetadata(t *testing.T) {
	state := core.NewContext()

	dispatcher := "capability"
	target := "agent:react"
	reason := "explicit_target"

	persistDispatchMetadata(state, dispatcher, target, reason)

	// Verify metadata was persisted
	if raw, ok := state.Get(runtime.ContextKeyCheckpoint); ok {
		var metadata map[string]any
		if runtime.DecodeContextValue(raw, &metadata) {
			if metadata["mode"] != dispatcher {
				t.Errorf("Dispatcher mismatch: expected %s, got %v", dispatcher, metadata["mode"])
			}
			if metadata["resolved_target"] != target {
				t.Errorf("Target mismatch: expected %s, got %v", target, metadata["resolved_target"])
			}
			if metadata["reason"] != reason {
				t.Errorf("Reason mismatch: expected %s, got %v", reason, metadata["reason"])
			}
			// Timestamp should be persisted (may be int64 or float64 due to JSON encoding)
			if ts, ok := metadata["timestamp"]; !ok {
				t.Fatal("Timestamp not persisted")
			} else if ts != nil {
				switch v := ts.(type) {
				case int64:
					if v == 0 {
						t.Errorf("Invalid timestamp: %v", ts)
					}
				case float64:
					if v == 0 {
						t.Errorf("Invalid timestamp: %v", ts)
					}
				default:
					t.Errorf("Unexpected timestamp type: %T", ts)
				}
			}
		} else {
			t.Fatal("Failed to decode dispatch metadata")
		}
	} else {
		t.Fatal("Dispatch metadata not persisted")
	}
}

// Helper functions

func contains(str, substr string) bool {
	for i := 0; i < len(str)-len(substr)+1; i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type TestError struct {
	msg string
}

func (e *TestError) Error() string {
	return e.msg
}

func NewTestError(msg string) error {
	return &TestError{msg: msg}
}
