package intake

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// === Phase 3 Unit Tests (exact spec requirements) ===

func TestNormalizeNilTask(t *testing.T) {
	result, err := NormalizeTaskEnvelope(nil, nil)
	if err != nil {
		t.Errorf("Expected no error for nil task, got %v", err)
	}
	if result != nil {
		t.Error("Expected nil result for nil task")
	}
}

func TestNormalizeBasicInstruction(t *testing.T) {
	task := &core.Task{
		ID:          "task-1",
		Instruction: "Please implement user authentication",
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if envelope.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", envelope.TaskID, "task-1")
	}
	if envelope.TaskType != "analysis" {
		t.Errorf("TaskType = %q, want %q (default)", envelope.TaskType, "analysis")
	}
	if envelope.Instruction != "Please implement user authentication" {
		t.Errorf("Instruction = %q, want %q", envelope.Instruction, "Please implement user authentication")
	}
}

func TestNormalizeFamilyHintFromContext(t *testing.T) {
	task := &core.Task{
		ID:          "task-2",
		Instruction: "Review the code",
		Context: map[string]any{
			"euclo.family": "review",
		},
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if envelope.FamilyHint != "review" {
		t.Errorf("FamilyHint = %q, want %q", envelope.FamilyHint, "review")
	}
}

func TestNormalizeUserFilesFromContext(t *testing.T) {
	task := &core.Task{
		ID:          "task-3",
		Instruction: "Fix the bugs",
		Context: map[string]any{
			"euclo.user_files": []string{"src/main.go", "src/utils.go"},
		},
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(envelope.UserFiles) != 2 {
		t.Errorf("UserFiles length = %d, want 2", len(envelope.UserFiles))
	}
	if envelope.UserFiles[0] != "src/main.go" {
		t.Errorf("UserFiles[0] = %q, want %q", envelope.UserFiles[0], "src/main.go")
	}
}

func TestNormalizeSessionPins(t *testing.T) {
	task := &core.Task{
		ID:          "task-4",
		Instruction: "Update the tests",
		Context: map[string]any{
			"euclo.session_pins": []string{"important_file.go", "key_config.yaml"},
		},
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(envelope.SessionPins) != 2 {
		t.Errorf("SessionPins length = %d, want 2", len(envelope.SessionPins))
	}
	if envelope.SessionPins[0] != "important_file.go" {
		t.Errorf("SessionPins[0] = %q, want %q", envelope.SessionPins[0], "important_file.go")
	}
}

func TestNormalizeEditPermittedFromRegistry(t *testing.T) {
	task := &core.Task{
		ID:          "task-5",
		Instruction: "Refactor the code",
	}

	// hasWriteTools = true
	envelope, err := NormalizeTaskEnvelopeWithRegistry(task, nil, true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !envelope.EditPermitted {
		t.Error("EditPermitted should be true when hasWriteTools is true")
	}

	// hasWriteTools = false
	envelope, err = NormalizeTaskEnvelopeWithRegistry(task, nil, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if envelope.EditPermitted {
		t.Error("EditPermitted should be false when hasWriteTools is false")
	}
}

func TestNormalizeResumedFamilyFromEnvelope(t *testing.T) {
	task := &core.Task{
		ID:          "task-6",
		Instruction: "Continue debugging",
	}

	resume := &ResumeState{
		Family: "debug",
	}

	envelope, err := NormalizeTaskEnvelope(task, resume)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if envelope.ResumedFamily != "debug" {
		t.Errorf("ResumedFamily = %q, want %q", envelope.ResumedFamily, "debug")
	}
}

func TestNormalizeCapabilitySequenceFromEnvelope(t *testing.T) {
	task := &core.Task{
		ID:          "task-7",
		Instruction: "Continue the workflow",
	}

	resume := &ResumeState{
		CapabilitySequence: []string{"analyze", "implement", "test"},
	}

	envelope, err := NormalizeTaskEnvelope(task, resume)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(envelope.CapabilitySequence) != 3 {
		t.Errorf("CapabilitySequence length = %d, want 3", len(envelope.CapabilitySequence))
	}
	if envelope.CapabilitySequence[0] != "analyze" {
		t.Errorf("CapabilitySequence[0] = %q, want %q", envelope.CapabilitySequence[0], "analyze")
	}
}

func TestNormalizeExplicitVerification(t *testing.T) {
	// Boolean verification
	task := &core.Task{
		ID:          "task-8",
		Instruction: "Verify the fix",
		Context: map[string]any{
			"verification": true,
		},
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !envelope.ExplicitVerification {
		t.Error("ExplicitVerification should be true")
	}

	// String verification
	task.Context = map[string]any{
		"verification": "true",
	}
	envelope, err = NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !envelope.ExplicitVerification {
		t.Error("ExplicitVerification should be true for string 'true'")
	}
}

func TestNormalizeWhitespaceTrimming(t *testing.T) {
	task := &core.Task{
		ID:          "task-9",
		Instruction: "   Please    implement   user   authentication   ",
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have normalized whitespace
	if envelope.Instruction != "Please implement user authentication" {
		t.Errorf("Whitespace not normalized: got %q", envelope.Instruction)
	}
}

func TestNormalizeNegativeConstraintSeeds(t *testing.T) {
	task := &core.Task{
		ID:          "task-10",
		Instruction: "Fix this but don't change the API and don't break existing tests",
	}

	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(envelope.NegativeConstraintSeeds) == 0 {
		t.Fatal("Expected at least one negative constraint seed")
	}

	// Check that we found "don't change the API"
	found := false
	for _, seed := range envelope.NegativeConstraintSeeds {
		if seed == "don't change the API" || seed == "don't break existing tests" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find 'don't change the API' or 'don't break existing tests' in seeds: %v", envelope.NegativeConstraintSeeds)
	}
}
