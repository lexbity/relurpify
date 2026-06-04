package interaction

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestNewClarificationFrameCarriesStructuredState(t *testing.T) {
	resume := &ClarificationResumeMetadata{
		ActiveThoughtRecipeID: "euclo.thoughtrecipe.intent.clarify",
		ResumeNodeID:          "node-1",
		RouteKind:             "intent",
		RouteID:               "euclo.thoughtrecipe.intent.clarify",
		StateVersion:          7,
		Unresolved:            true,
		MissingFields:         []string{"target"},
	}
	frame := NewClarificationFrame("task-1", "session-1", "What should I clarify?", []string{"review", "implementation"}, resume)
	if frame == nil {
		t.Fatal("expected clarification frame")
	}
	if frame.Type != FrameIntentClarification {
		t.Fatalf("frame type = %q", frame.Type)
	}
	if frame.Question != "What should I clarify?" {
		t.Fatalf("question = %q", frame.Question)
	}
	if len(frame.Choices) != 2 || frame.Choices[1] != "implementation" {
		t.Fatalf("choices = %#v", frame.Choices)
	}
	if frame.DefaultChoice != "review" || frame.DefaultSlot != "review" {
		t.Fatalf("default choice/slot = %q/%q", frame.DefaultChoice, frame.DefaultSlot)
	}
	if frame.Resume == nil || !frame.Resume.Unresolved {
		t.Fatalf("resume metadata = %#v", frame.Resume)
	}
	if frame.Selection == nil {
		t.Fatal("selection payload was nil")
	}
	if frame.Selection.Default != "review" || len(frame.Selection.Options) != 2 {
		t.Fatalf("selection payload = %#v", frame.Selection)
	}
}

func TestNewAskUserFrameFallsBackToAnswerSlot(t *testing.T) {
	frame := NewAskUserFrame("task-1", "session-1", "Question?", nil)
	if frame == nil {
		t.Fatal("expected clarification frame")
	}
	if frame.DefaultSlot != "answer" {
		t.Fatalf("default slot = %q", frame.DefaultSlot)
	}
	if frame.Question != "Question?" {
		t.Fatalf("question = %q", frame.Question)
	}
	if len(frame.Slots) != 1 || frame.Slots[0].ID != "answer" {
		t.Fatalf("slots = %#v", frame.Slots)
	}
}

func TestNewThoughtRecipeSelectionFrameCarriesProjectionSummaries(t *testing.T) {
	recipes := []surface.RecipeProjection{
		{
			RecipeID:  "recipe.review",
			Name:      "Code Review",
			RouteKind: "capability",
			Steps: []surface.ProjectedStep{
				{StepID: "scan", Paradigm: "goalcon", Goal: "Scan code"},
				{StepID: "review", Paradigm: "react", Goal: "Review findings"},
			},
		},
		{
			RecipeID:  "recipe.refactor",
			Name:      "Refactor",
			RouteKind: "capability",
			Steps: []surface.ProjectedStep{
				{StepID: "analyze", Paradigm: "planner", Goal: "Analyze structure"},
			},
			HITLGates: []string{"approve"},
		},
	}

	frame := NewThoughtRecipeSelectionFrame("task-1", "session-1", recipes)
	if frame == nil {
		t.Fatal("expected frame")
	}
	if frame.Type != FrameThoughtRecipeSelection {
		t.Fatalf("frame type = %q, want %q", frame.Type, FrameThoughtRecipeSelection)
	}
	if len(frame.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(frame.Slots))
	}
	if frame.Slots[0].ID != "recipe.review" || frame.Slots[0].Label != "Code Review" {
		t.Fatalf("slot[0] = %+v", frame.Slots[0])
	}
	if frame.Slots[1].ID != "recipe.refactor" || frame.Slots[1].Label != "Refactor" {
		t.Fatalf("slot[1] = %+v", frame.Slots[1])
	}

	// Verify recipe projections are retrievable from payload.
	projs := frame.RecipeProjections()
	if len(projs) != 2 {
		t.Fatalf("expected 2 recipe projections, got %d", len(projs))
	}
	if projs[0].RecipeID != "recipe.review" || len(projs[0].Steps) != 2 {
		t.Fatalf("projection[0] = %+v", projs[0])
	}
	if projs[1].RecipeID != "recipe.refactor" || len(projs[1].HITLGates) != 1 {
		t.Fatalf("projection[1] = %+v", projs[1])
	}
	if projs[1].HITLGates[0] != "approve" {
		t.Fatalf("expected HITL gate 'approve', got %q", projs[1].HITLGates[0])
	}
}

func TestNewThoughtRecipeSelectionFrameEmpty(t *testing.T) {
	frame := NewThoughtRecipeSelectionFrame("task-1", "session-1", nil)
	if frame == nil {
		t.Fatal("expected frame even with empty recipes")
	}
	if frame.Type != FrameThoughtRecipeSelection {
		t.Fatalf("frame type = %q", frame.Type)
	}
	if frame.RecipeProjections() != nil {
		t.Fatal("expected nil projections for empty input")
	}
}

func TestClarificationFrameResponseRoundTrip(t *testing.T) {
	frame := NewClarificationFrame("task-1", "session-1", "What should I clarify?", []string{"review", "implementation"}, nil)
	frame.SetResponse("review", map[string]any{"answer": "review"}, "tester", time.Unix(123, 0).UTC())

	if got, ok := ClarificationResponseValue(frame); !ok || got != "review" {
		t.Fatalf("response value = %q, ok=%v", got, ok)
	}
	if frame.Response == nil || frame.Response.RespondedBy != "tester" {
		t.Fatalf("response = %#v", frame.Response)
	}

	turn := ClarificationTurnFromFrame(frame, 9)
	if turn == nil {
		t.Fatal("expected clarification turn")
	}
	if turn.Question != "What should I clarify?" {
		t.Fatalf("turn question = %q", turn.Question)
	}
	if turn.Answer != "review" {
		t.Fatalf("turn answer = %q", turn.Answer)
	}
	if turn.StateVersion != 9 {
		t.Fatalf("turn state version = %d", turn.StateVersion)
	}
}
