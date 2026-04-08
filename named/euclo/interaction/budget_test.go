package interaction

import (
	"testing"
)

func TestDefaultBudget(t *testing.T) {
	budget := DefaultBudget()
	if budget == nil {
		t.Fatal("DefaultBudget returned nil")
	}

	if budget.MaxQuestionsPerPhase != 3 {
		t.Errorf("Expected MaxQuestionsPerPhase 3, got %d", budget.MaxQuestionsPerPhase)
	}
	if budget.MaxTransitions != 3 {
		t.Errorf("Expected MaxTransitions 3, got %d", budget.MaxTransitions)
	}
	if budget.MaxFramesTotal != 0 {
		t.Errorf("Expected MaxFramesTotal 0 (unlimited), got %d", budget.MaxFramesTotal)
	}
	if budget.questionsInPhase == nil {
		t.Error("questionsInPhase map should be initialized")
	}
}

func TestNewBudget(t *testing.T) {
	cfg := InteractionConfig{
		Budget: InteractionBudgetConfig{
			MaxQuestions:   5,
			MaxTransitions: 2,
			MaxFrames:      100,
		},
	}

	budget := NewBudget(cfg)
	if budget == nil {
		t.Fatal("NewBudget returned nil")
	}

	if budget.MaxQuestionsPerPhase != 5 {
		t.Errorf("Expected MaxQuestionsPerPhase 5, got %d", budget.MaxQuestionsPerPhase)
	}
	if budget.MaxTransitions != 2 {
		t.Errorf("Expected MaxTransitions 2, got %d", budget.MaxTransitions)
	}
	if budget.MaxFramesTotal != 100 {
		t.Errorf("Expected MaxFramesTotal 100, got %d", budget.MaxFramesTotal)
	}
}

func TestBudgetRecordFrame(t *testing.T) {
	budget := DefaultBudget()

	// Record frames up to limit (unlimited by default)
	for i := 0; i < 10; i++ {
		if !budget.RecordFrame() {
			t.Errorf("RecordFrame should succeed for frame %d", i+1)
		}
	}

	// Test with frame limit
	budget.MaxFramesTotal = 3
	budget.frameCount = 0 // Reset

	for i := 0; i < 3; i++ {
		if !budget.RecordFrame() {
			t.Errorf("RecordFrame should succeed for frame %d within limit", i+1)
		}
	}

	// Fourth frame should exceed limit
	if budget.RecordFrame() {
		t.Error("RecordFrame should fail when exceeding MaxFramesTotal")
	}
}

func TestRecordQuestion(t *testing.T) {
	budget := DefaultBudget()

	// Record questions within limit
	for i := 0; i < 3; i++ {
		if !budget.RecordQuestion("phase1") {
			t.Errorf("RecordQuestion should succeed for question %d", i+1)
		}
	}

	// Fourth question should exceed limit
	if budget.RecordQuestion("phase1") {
		t.Error("RecordQuestion should fail when exceeding MaxQuestionsPerPhase")
	}

	// Different phase should have separate count
	if !budget.RecordQuestion("phase2") {
		t.Error("RecordQuestion should succeed for different phase")
	}
}

func TestRecordSkip(t *testing.T) {
	budget := DefaultBudget()
	budget.MaxPhasesSkippable = 2

	// Record skips within limit
	if !budget.RecordSkip() {
		t.Error("RecordSkip should succeed for first skip")
	}
	if !budget.RecordSkip() {
		t.Error("RecordSkip should succeed for second skip")
	}

	// Third skip should exceed limit
	if budget.RecordSkip() {
		t.Error("RecordSkip should fail when exceeding MaxPhasesSkippable")
	}

	// Test unlimited skips
	budget.MaxPhasesSkippable = 0
	budget.skippedCount = 0

	for i := 0; i < 10; i++ {
		if !budget.RecordSkip() {
			t.Errorf("RecordSkip should succeed for skip %d with unlimited", i+1)
		}
	}
}

func TestBudgetRecordTransition(t *testing.T) {
	budget := DefaultBudget()

	// Record transitions within limit
	for i := 0; i < 3; i++ {
		if !budget.RecordTransition() {
			t.Errorf("RecordTransition should succeed for transition %d", i+1)
		}
	}

	// Fourth transition should exceed limit
	if budget.RecordTransition() {
		t.Error("RecordTransition should fail when exceeding MaxTransitions")
	}
}

func TestExhaustedReason(t *testing.T) {
	budget := DefaultBudget()

	// Initially not exhausted
	if reason := budget.ExhaustedReason(); reason != "" {
		t.Errorf("Expected empty exhausted reason, got %s", reason)
	}

	// Exceed frame budget
	budget.MaxFramesTotal = 5
	budget.frameCount = 6
	if reason := budget.ExhaustedReason(); reason != "frame_budget_exceeded" {
		t.Errorf("Expected 'frame_budget_exceeded', got %s", reason)
	}

	// Exceed transition budget
	budget.MaxFramesTotal = 0
	budget.frameCount = 0
	budget.transitionCount = 4
	budget.MaxTransitions = 3
	if reason := budget.ExhaustedReason(); reason != "transition_budget_exceeded" {
		t.Errorf("Expected 'transition_budget_exceeded', got %s", reason)
	}
}

func TestCountGetters(t *testing.T) {
	budget := DefaultBudget()

	// Record some activity
	budget.RecordFrame()
	budget.RecordFrame()
	budget.RecordTransition()

	if budget.FrameCount() != 2 {
		t.Errorf("FrameCount should return 2, got %d", budget.FrameCount())
	}
	if budget.TransitionCount() != 1 {
		t.Errorf("TransitionCount should return 1, got %d", budget.TransitionCount())
	}
}
