package interaction

import "testing"

func TestDefaultBudget(t *testing.T) {
	b := DefaultBudget()
	if b.MaxQuestionsPerPhase != 3 {
		t.Errorf("MaxQuestionsPerPhase: got %d", b.MaxQuestionsPerPhase)
	}
	if b.MaxTransitions != 3 {
		t.Errorf("MaxTransitions: got %d", b.MaxTransitions)
	}
}

func TestBudget_RecordFrame(t *testing.T) {
	b := DefaultBudget()
	b.MaxFramesTotal = 2
	if !b.RecordFrame() {
		t.Error("first frame should be within budget")
	}
	if !b.RecordFrame() {
		t.Error("second frame should be within budget")
	}
	if b.RecordFrame() {
		t.Error("third frame should exceed budget")
	}
	if b.FrameCount() != 3 {
		t.Errorf("frame count: got %d", b.FrameCount())
	}
}

func TestBudget_RecordQuestion(t *testing.T) {
	b := DefaultBudget()
	b.MaxQuestionsPerPhase = 2
	if !b.RecordQuestion("scope") {
		t.Error("first question should be within budget")
	}
	if !b.RecordQuestion("scope") {
		t.Error("second question should be within budget")
	}
	if b.RecordQuestion("scope") {
		t.Error("third question should exceed budget")
	}
	// Different phase should be independent.
	if !b.RecordQuestion("clarify") {
		t.Error("first question in different phase should be within budget")
	}
}

func TestBudget_RecordTransition(t *testing.T) {
	b := DefaultBudget()
	b.MaxTransitions = 1
	if !b.RecordTransition() {
		t.Error("first transition should be within budget")
	}
	if b.RecordTransition() {
		t.Error("second transition should exceed budget")
	}
}

func TestBudget_RecordSkip(t *testing.T) {
	b := DefaultBudget()
	b.MaxPhasesSkippable = 1
	if !b.RecordSkip() {
		t.Error("first skip should be within budget")
	}
	if b.RecordSkip() {
		t.Error("second skip should exceed budget")
	}
}

func TestBudget_ExhaustedReason(t *testing.T) {
	b := DefaultBudget()
	if r := b.ExhaustedReason(); r != "" {
		t.Errorf("fresh budget should not be exhausted: %q", r)
	}

	b.MaxFramesTotal = 1
	b.RecordFrame()
	b.RecordFrame()
	if r := b.ExhaustedReason(); r != "frame_budget_exceeded" {
		t.Errorf("got %q", r)
	}
}

func TestBudget_NilSafe(t *testing.T) {
	var b *InteractionBudget
	if !b.RecordFrame() {
		t.Error("nil budget should always return true")
	}
	if !b.RecordQuestion("x") {
		t.Error("nil budget should always return true")
	}
	if !b.RecordTransition() {
		t.Error("nil budget should always return true")
	}
	if b.FrameCount() != 0 {
		t.Error("nil budget frame count should be 0")
	}
}

func TestNewBudgetFromConfig(t *testing.T) {
	cfg := InteractionConfig{
		Budget: InteractionBudgetConfig{
			MaxQuestions:   5,
			MaxTransitions: 10,
			MaxFrames:      50,
		},
	}
	b := NewBudget(cfg)
	if b.MaxQuestionsPerPhase != 5 {
		t.Errorf("MaxQuestionsPerPhase: got %d", b.MaxQuestionsPerPhase)
	}
	if b.MaxTransitions != 10 {
		t.Errorf("MaxTransitions: got %d", b.MaxTransitions)
	}
	if b.MaxFramesTotal != 50 {
		t.Errorf("MaxFramesTotal: got %d", b.MaxFramesTotal)
	}
}
