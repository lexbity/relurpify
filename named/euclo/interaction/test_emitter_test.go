package interaction

import (
	"context"
	"errors"
	"testing"
)

func TestTestFrameEmitter_RecordsFrames(t *testing.T) {
	emitter := NewTestFrameEmitter()
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{Kind: FrameProposal, Phase: "scope"})
	_ = emitter.Emit(ctx, InteractionFrame{Kind: FrameQuestion, Phase: "clarify"})

	if emitter.FrameCount() != 2 {
		t.Errorf("frame count: got %d, want 2", emitter.FrameCount())
	}
	if err := emitter.AssertHasFrameKind(FrameProposal); err != nil {
		t.Error(err)
	}
	if err := emitter.AssertHasFrameKind(FrameQuestion); err != nil {
		t.Error(err)
	}
	if err := emitter.AssertNoFrameKind(FrameTransition); err != nil {
		t.Error(err)
	}
}

func TestTestFrameEmitter_ScriptedResponses(t *testing.T) {
	emitter := NewTestFrameEmitter(
		ScriptedResponse{Phase: "scope", ActionID: "confirm"},
		ScriptedResponse{Phase: "clarify", ActionID: "skip"},
	)
	ctx := context.Background()

	// First frame → scope.
	_ = emitter.Emit(ctx, InteractionFrame{Kind: FrameProposal, Phase: "scope"})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "confirm" {
		t.Errorf("scope: got %q, want confirm", resp.ActionID)
	}

	// Second frame → clarify.
	_ = emitter.Emit(ctx, InteractionFrame{Kind: FrameQuestion, Phase: "clarify"})
	resp, _ = emitter.AwaitResponse(ctx)
	if resp.ActionID != "skip" {
		t.Errorf("clarify: got %q, want skip", resp.ActionID)
	}
}

func TestTestFrameEmitter_DefaultAction(t *testing.T) {
	emitter := NewTestFrameEmitter()
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{
		Kind:  FrameProposal,
		Phase: "scope",
		Actions: []ActionSlot{
			{ID: "confirm", Label: "Confirm", Default: true},
			{ID: "edit", Label: "Edit"},
		},
	})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "confirm" {
		t.Errorf("got %q, want confirm (default)", resp.ActionID)
	}
}

func TestTestFrameEmitter_ExplicitDefaultActionID(t *testing.T) {
	emitter := NewTestFrameEmitter()
	emitter.DefaultActionID = "always_this"
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{Kind: FrameProposal, Phase: "scope"})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "always_this" {
		t.Errorf("got %q, want always_this", resp.ActionID)
	}
}

func TestTestFrameEmitter_AutoSkip(t *testing.T) {
	emitter := NewTestFrameEmitter()
	emitter.AutoSkip = true
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{
		Kind:  FrameQuestion,
		Phase: "clarify",
		Actions: []ActionSlot{
			{ID: "answer", Label: "Answer"},
			{ID: "skip", Label: "Skip"},
		},
	})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "skip" {
		t.Errorf("got %q, want skip", resp.ActionID)
	}
}

func TestTestFrameEmitter_FramesByPhase(t *testing.T) {
	emitter := NewTestFrameEmitter()
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{Phase: "scope"})
	_ = emitter.Emit(ctx, InteractionFrame{Phase: "generate"})
	_ = emitter.Emit(ctx, InteractionFrame{Phase: "scope"})

	if len(emitter.FramesByPhase("scope")) != 2 {
		t.Error("expected 2 scope frames")
	}
	if len(emitter.FramesByPhase("generate")) != 1 {
		t.Error("expected 1 generate frame")
	}
}

func TestTestFrameEmitter_Reset(t *testing.T) {
	emitter := NewTestFrameEmitter(
		ScriptedResponse{ActionID: "first"},
	)
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{})
	emitter.Reset()

	if emitter.FrameCount() != 0 {
		t.Error("expected 0 frames after reset")
	}

	// Script should also reset.
	_ = emitter.Emit(ctx, InteractionFrame{})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "first" {
		t.Errorf("got %q, want first (script reset)", resp.ActionID)
	}
}

func TestTestFrameEmitter_ScriptKindMatch(t *testing.T) {
	emitter := NewTestFrameEmitter(
		ScriptedResponse{Kind: "question", ActionID: "picked"},
	)
	ctx := context.Background()

	// Emit a proposal first — script shouldn't match.
	_ = emitter.Emit(ctx, InteractionFrame{
		Kind:    FrameProposal,
		Actions: []ActionSlot{{ID: "default", Default: true}},
	})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "default" {
		t.Errorf("proposal: got %q, want default (script mismatch)", resp.ActionID)
	}
}

func TestTestFrameEmitter_FrameCallback(t *testing.T) {
	var called int
	emitter := NewTestFrameEmitter()
	emitter.FrameCallback = func(_ InteractionFrame) { called++ }

	_ = emitter.Emit(context.Background(), InteractionFrame{})
	_ = emitter.Emit(context.Background(), InteractionFrame{})

	if called != 2 {
		t.Errorf("callback: got %d, want 2", called)
	}
}

func TestTestFrameEmitter_AwaitResponse_RespectsContextCancellation(t *testing.T) {
	emitter := NewTestFrameEmitter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := emitter.AwaitResponse(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
