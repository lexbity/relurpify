package runtime

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestBuildAmbiguityFrame(t *testing.T) {
	scored := ScoredClassification{
		Candidates: []ModeCandidate{
			{Mode: "debug", Score: 0.6},
			{Mode: "code", Score: 0.55},
		},
		Ambiguous: true,
	}

	frame := BuildAmbiguityFrame(scored)
	if frame.Kind != interaction.FrameQuestion {
		t.Errorf("kind: got %q, want question", frame.Kind)
	}
	if frame.Mode != "classification" {
		t.Errorf("mode: got %q", frame.Mode)
	}

	content, ok := frame.Content.(interaction.QuestionContent)
	if !ok {
		t.Fatal("expected QuestionContent")
	}
	// Should have debug, code, and planning options.
	if len(content.Options) < 3 {
		t.Errorf("options: got %d, want >= 3", len(content.Options))
	}

	// Verify planning is always present.
	hasPlan := false
	for _, o := range content.Options {
		if o.ID == "planning" {
			hasPlan = true
		}
	}
	if !hasPlan {
		t.Error("expected planning option")
	}

	// First action should be default.
	if len(frame.Actions) == 0 || !frame.Actions[0].Default {
		t.Error("expected first action to be default")
	}
}

func TestBuildAmbiguityFrame_PlanningAlreadyPresent(t *testing.T) {
	scored := ScoredClassification{
		Candidates: []ModeCandidate{
			{Mode: "planning", Score: 0.6},
			{Mode: "code", Score: 0.55},
		},
	}

	frame := BuildAmbiguityFrame(scored)
	content := frame.Content.(interaction.QuestionContent)

	// Count planning occurrences — should be exactly 1.
	planCount := 0
	for _, o := range content.Options {
		if o.ID == "planning" {
			planCount++
		}
	}
	if planCount != 1 {
		t.Errorf("planning count: got %d, want 1", planCount)
	}
}

func TestResolveAmbiguity(t *testing.T) {
	scored := ScoredClassification{
		Candidates: []ModeCandidate{
			{Mode: "debug", Score: 0.6},
			{Mode: "code", Score: 0.55},
		},
	}

	// User selects code.
	mode := ResolveAmbiguity(scored, interaction.UserResponse{ActionID: "code"})
	if mode != "code" {
		t.Errorf("got %q, want code", mode)
	}

	// User selects debug.
	mode = ResolveAmbiguity(scored, interaction.UserResponse{ActionID: "debug"})
	if mode != "debug" {
		t.Errorf("got %q, want debug", mode)
	}

	// User selects planning (extra option).
	mode = ResolveAmbiguity(scored, interaction.UserResponse{ActionID: "planning"})
	if mode != "planning" {
		t.Errorf("got %q, want planning", mode)
	}

	// Unknown selection → fallback to top.
	mode = ResolveAmbiguity(scored, interaction.UserResponse{ActionID: "unknown"})
	if mode != "debug" {
		t.Errorf("got %q, want debug (fallback)", mode)
	}

	// Empty candidates → default code.
	mode = ResolveAmbiguity(ScoredClassification{}, interaction.UserResponse{})
	if mode != "code" {
		t.Errorf("got %q, want code (empty)", mode)
	}
}
