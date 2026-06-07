package interaction

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

func TestResumeFrame_FindsPendingFrame(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	first := NewOutcomeFeedbackFrame("task-1", "session-1", "done")
	second := NewOutcomeFeedbackFrame("task-1", "session-1", "done")

	if err := EmitFrame(context.TODO(), first, env, nil); err != nil {
		t.Fatalf("EmitFrame first failed: %v", err)
	}
	if err := EmitFrame(context.TODO(), second, env, nil); err != nil {
		t.Fatalf("EmitFrame second failed: %v", err)
	}

	now := time.Now().UTC()
	first.RespondedAt = &now

	got, ok := ResumeFrame(env)
	if !ok {
		t.Fatal("expected pending frame")
	}
	if got != second {
		t.Fatal("expected most recent pending frame")
	}
}

func TestResumeFrame_NonePresent(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	if got, ok := ResumeFrame(env); ok || got != nil {
		t.Fatalf("expected no frame, got=%v ok=%v", got, ok)
	}
}

func TestResumeFrame_FrameKeyFormat(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	frame := NewOutcomeFeedbackFrame("task-1", "session-1", "done")
	frame.Seq = 128

	contextdata.SetTyped(env, frameStorageKey(128), frame)
	contextdata.SetTyped(env, "euclo.interaction.frame_seq", 129)

	got, ok := ResumeFrame(env)
	if !ok {
		t.Fatal("expected frame to be found")
	}
	if got != frame {
		t.Fatal("expected frame stored under numeric key")
	}
}

func TestResumeClarificationFrame(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	frame := NewClarificationFrame("task-1", "session-1", "Question?", []string{"yes", "no"}, nil)
	if err := EmitFrame(context.TODO(), frame, env, nil); err != nil {
		t.Fatalf("EmitFrame failed: %v", err)
	}
	got, ok := ResumeClarificationFrame(env)
	if !ok {
		t.Fatal("expected clarification frame to resume")
	}
	if got != frame {
		t.Fatal("expected clarification frame instance")
	}
}
