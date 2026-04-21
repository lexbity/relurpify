package interaction_test

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// ---------------------------------------------------------------------------
// TestFrameEmitter
// ---------------------------------------------------------------------------

func TestTestFrameEmitter_EmitAndFrames(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	f := interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"}
	if err := e.Emit(context.Background(), f); err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if e.FrameCount() != 1 {
		t.Fatalf("expected 1 frame, got %d", e.FrameCount())
	}
	frames := e.Frames()
	if len(frames) != 1 || frames[0].Phase != "p1" {
		t.Fatalf("unexpected frames: %v", frames)
	}
}

func TestTestFrameEmitter_ScriptedResponse(t *testing.T) {
	e := interaction.NewTestFrameEmitter(
		interaction.ScriptedResponse{Phase: "p1", ActionID: "confirm"},
	)
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "chat",
		Phase: "p1",
	})
	resp, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("AwaitResponse error: %v", err)
	}
	if resp.ActionID != "confirm" {
		t.Fatalf("expected scripted 'confirm', got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_ScriptedResponseWithKindMatch(t *testing.T) {
	e := interaction.NewTestFrameEmitter(
		interaction.ScriptedResponse{Kind: "proposal", ActionID: "reject"},
	)
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Phase: "p1",
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "reject" {
		t.Fatalf("expected 'reject' from kind-matched script, got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_ScriptPhaseMismatchFallsThrough(t *testing.T) {
	// Script matches phase "p2" but frame is "p1" → falls through to default
	e := interaction.NewTestFrameEmitter(
		interaction.ScriptedResponse{Phase: "p2", ActionID: "reject"},
	)
	e.DefaultActionID = "confirm"
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Phase: "p1",
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Default: true},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "confirm" {
		t.Fatalf("expected fallthrough to DefaultActionID, got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_AutoSkip(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	e.AutoSkip = true
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameQuestion,
		Phase: "p1",
		Actions: []interaction.ActionSlot{
			{ID: "skip", Label: "Skip"},
			{ID: "confirm", Label: "Confirm"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "skip" {
		t.Fatalf("expected 'skip' from AutoSkip, got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_DefaultActionID(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	e.DefaultActionID = "proceed"
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameResult,
		Phase: "p1",
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "proceed" {
		t.Fatalf("expected 'proceed' from DefaultActionID, got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_FramesDefaultAction(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Phase: "p1",
		Actions: []interaction.ActionSlot{
			{ID: "go", Default: true},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "go" {
		t.Fatalf("expected frame default action 'go', got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_FallsBackToFirstAction(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Phase: "p1",
		Actions: []interaction.ActionSlot{
			{ID: "first"},
			{ID: "second"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "first" {
		t.Fatalf("expected first action fallback, got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_EmptyFramesReturnsEmpty(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	resp, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ActionID != "" {
		t.Fatalf("expected empty response, got %q", resp.ActionID)
	}
}

func TestTestFrameEmitter_FramesOfKind(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameResult})
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})

	proposals := e.FramesOfKind(interaction.FrameProposal)
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposal frames, got %d", len(proposals))
	}
	results := e.FramesOfKind(interaction.FrameResult)
	if len(results) != 1 {
		t.Fatalf("expected 1 result frame, got %d", len(results))
	}
}

func TestTestFrameEmitter_FramesByPhase(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal, Phase: "p1"})
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameResult, Phase: "p2"})
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameStatus, Phase: "p1"})

	p1 := e.FramesByPhase("p1")
	if len(p1) != 2 {
		t.Fatalf("expected 2 frames for p1, got %d", len(p1))
	}
}

func TestTestFrameEmitter_Reset(t *testing.T) {
	e := interaction.NewTestFrameEmitter(
		interaction.ScriptedResponse{ActionID: "confirm"},
	)
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	e.Reset()
	if e.FrameCount() != 0 {
		t.Fatalf("expected 0 frames after reset, got %d", e.FrameCount())
	}
}

func TestTestFrameEmitter_AssertFrameCount(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	if err := e.AssertFrameCount(1); err != nil {
		t.Fatalf("AssertFrameCount(1) error: %v", err)
	}
	if err := e.AssertFrameCount(2); err == nil {
		t.Fatal("expected error for wrong count")
	}
}

func TestTestFrameEmitter_AssertHasFrameKind(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	if err := e.AssertHasFrameKind(interaction.FrameProposal); err != nil {
		t.Fatalf("AssertHasFrameKind error: %v", err)
	}
	if err := e.AssertHasFrameKind(interaction.FrameResult); err == nil {
		t.Fatal("expected error for missing kind")
	}
}

func TestTestFrameEmitter_AssertNoFrameKind(t *testing.T) {
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	if err := e.AssertNoFrameKind(interaction.FrameResult); err != nil {
		t.Fatalf("AssertNoFrameKind error: %v", err)
	}
	if err := e.AssertNoFrameKind(interaction.FrameProposal); err == nil {
		t.Fatal("expected error when kind is present")
	}
}

func TestTestFrameEmitter_FrameCallback(t *testing.T) {
	called := 0
	e := interaction.NewTestFrameEmitter()
	e.FrameCallback = func(_ interaction.InteractionFrame) { called++ }
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameResult})
	if called != 2 {
		t.Fatalf("expected callback called twice, got %d", called)
	}
}

func TestTestFrameEmitter_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := interaction.NewTestFrameEmitter()
	_ = e.Emit(ctx, interaction.InteractionFrame{Kind: interaction.FrameProposal})
	_, err := e.AwaitResponse(ctx)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// InteractionTelemetry
// ---------------------------------------------------------------------------

func TestInteractionTelemetry_NilInnerNoPanic(t *testing.T) {
	tel := interaction.NewInteractionTelemetry(nil)
	tel.EmitFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	tel.EmitResponse(interaction.UserResponse{ActionID: "confirm"}, "p1", "chat", time.Millisecond)
	tel.EmitPhaseSkip("p2", "chat", "auto")
	tel.EmitTransition("chat", "debug", "trigger", true)
	tel.EmitBudgetLimit("frame_exceeded", "p1", "chat")
}

func TestInteractionTelemetry_NilReceiverNoPanic(t *testing.T) {
	var tel *interaction.InteractionTelemetry
	tel.EmitFrame(interaction.InteractionFrame{})
	tel.EmitResponse(interaction.UserResponse{}, "", "", 0)
	tel.EmitPhaseSkip("", "", "")
	tel.EmitTransition("", "", "", false)
	tel.EmitBudgetLimit("", "", "")
}

func TestInteractionTelemetry_EmitsWithRealInner(t *testing.T) {
	stub := &stubTelemetry{}
	tel := interaction.NewInteractionTelemetry(stub)
	tel.EmitFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	if stub.count != 1 {
		t.Fatalf("expected 1 telemetry event, got %d", stub.count)
	}
	tel.EmitResponse(interaction.UserResponse{ActionID: "confirm"}, "p1", "chat", 50*time.Millisecond)
	tel.EmitPhaseSkip("p2", "chat", "condition")
	tel.EmitTransition("chat", "debug", "user_request", false)
	tel.EmitBudgetLimit("transition_exceeded", "p1", "chat")
	if stub.count != 5 {
		t.Fatalf("expected 5 telemetry events, got %d", stub.count)
	}
}

// ---------------------------------------------------------------------------
// TelemetryEmitter
// ---------------------------------------------------------------------------

func TestTelemetryEmitter_DelegatesToInner(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	stub := &stubTelemetry{}
	tel := interaction.NewInteractionTelemetry(stub)
	e := interaction.NewTelemetryEmitter(noop, tel)

	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "chat",
		Phase: "p1",
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Default: true},
		},
	})
	if stub.count != 1 {
		t.Fatalf("expected 1 telemetry event from Emit, got %d", stub.count)
	}
	resp, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("AwaitResponse error: %v", err)
	}
	if resp.ActionID != "confirm" {
		t.Fatalf("expected confirm, got %q", resp.ActionID)
	}
	if stub.count != 2 {
		t.Fatalf("expected 2 telemetry events total, got %d", stub.count)
	}
}

func TestTelemetryEmitter_NilTelemetryNoPanic(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	e := interaction.NewTelemetryEmitter(noop, nil)
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Actions: []interaction.ActionSlot{{ID: "continue", Default: true}},
	})
	_, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Stub helpers
// ---------------------------------------------------------------------------

type stubTelemetry struct {
	count  int
	events []core.Event
}

func (s *stubTelemetry) Emit(e core.Event) {
	s.count++
	s.events = append(s.events, e)
}
