package interaction

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestResumeFrame_FindsPendingFrame(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	first := NewOutcomeFeedbackFrame("task-1", "session-1", "done")
	second := NewOutcomeFeedbackFrame("task-1", "session-1", "done")

	if err := EmitFrame(nil, first, env, nil); err != nil {
		t.Fatalf("EmitFrame first failed: %v", err)
	}
	if err := EmitFrame(nil, second, env, nil); err != nil {
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

	env.SetWorkingValue(frameStorageKey(128), frame, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.interaction.frame_seq", 129, contextdata.MemoryClassTask)

	got, ok := ResumeFrame(env)
	if !ok {
		t.Fatal("expected frame to be found")
	}
	if got != frame {
		t.Fatal("expected frame stored under numeric key")
	}
}
