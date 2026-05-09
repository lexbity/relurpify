package orchestrate

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

func TestClarificationFrameToInteractionFrame(t *testing.T) {
	resume := &interaction.ClarificationResumeMetadata{
		ActiveThoughtRecipeID: clarificationThoughtRecipeID,
		ResumeNodeID:          "resume-node-1",
		RouteKind:             RouteKindIntent,
		RouteID:               clarificationThoughtRecipeID,
		StateVersion:          4,
		Unresolved:            true,
		MissingFields:         []string{"target"},
	}
	frame := NewClarificationFrame("task-1", "session-1", clarificationThoughtRecipeID, "What is the target?", []string{"review", "implementation"}, []string{"target"}, resume)
	if frame == nil {
		t.Fatal("expected clarification frame")
	}
	if !frame.Pending() {
		t.Fatal("expected clarification frame to be pending")
	}

	interactionFrame := frame.ToInteractionFrame()
	if interactionFrame == nil {
		t.Fatal("expected interaction frame")
	}
	if interactionFrame.Type != interaction.FrameIntentClarification {
		t.Fatalf("frame type = %q, want %q", interactionFrame.Type, interaction.FrameIntentClarification)
	}
	if interactionFrame.Question != "What is the target?" {
		t.Fatalf("question = %q", interactionFrame.Question)
	}
	if len(interactionFrame.Choices) != 2 || interactionFrame.Choices[0] != "review" {
		t.Fatalf("choices = %#v", interactionFrame.Choices)
	}
	if interactionFrame.DefaultChoice != "review" || interactionFrame.DefaultSlot != "review" {
		t.Fatalf("default choice/slot = %q/%q", interactionFrame.DefaultChoice, interactionFrame.DefaultSlot)
	}
	if interactionFrame.Resume == nil || !interactionFrame.Resume.Unresolved {
		t.Fatalf("expected resume metadata, got %#v", interactionFrame.Resume)
	}
	if got := interactionFrame.Payload["thoughtrecipe_id"]; got != clarificationThoughtRecipeID {
		t.Fatalf("thoughtrecipe id payload = %#v", got)
	}
}

func TestClarificationFrameMarkSkipped(t *testing.T) {
	frame := NewClarificationFrame("task-1", "session-1", clarificationThoughtRecipeID, "Question?", nil, nil, nil)
	frame.MarkSkipped("resolved immediately")
	if frame.Pending() {
		t.Fatal("expected skipped clarification frame to not be pending")
	}
	if !frame.Skipped {
		t.Fatal("expected skipped clarification frame")
	}
	if frame.SkippedReason != "resolved immediately" {
		t.Fatalf("skipped reason = %q", frame.SkippedReason)
	}
	if frame.RespondedAt == nil {
		t.Fatal("expected responded at")
	}
}
