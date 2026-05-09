package interaction

import (
	"testing"
	"time"
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
	if got := frame.Payload["default_choice"]; got != "review" {
		t.Fatalf("default choice payload = %#v", got)
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
