package interaction

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoopEmitter_RecordsFrames(t *testing.T) {
	e := &NoopEmitter{}
	ctx := context.Background()

	frame := InteractionFrame{
		Kind:  FrameProposal,
		Mode:  "code",
		Phase: "understand",
		Content: ProposalContent{
			Interpretation: "test",
		},
		Actions: []ActionSlot{
			{ID: "confirm", Label: "Confirm", Kind: ActionConfirm, Default: true},
		},
	}

	if err := e.Emit(ctx, frame); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if len(e.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(e.Frames))
	}
	if e.Frames[0].Kind != FrameProposal {
		t.Errorf("kind: got %q, want %q", e.Frames[0].Kind, FrameProposal)
	}
}

func TestNoopEmitter_AwaitResponse_Default(t *testing.T) {
	e := &NoopEmitter{}
	ctx := context.Background()

	frame := InteractionFrame{
		Kind: FrameQuestion,
		Actions: []ActionSlot{
			{ID: "opt1", Label: "Option 1", Kind: ActionSelect},
			{ID: "opt2", Label: "Option 2", Kind: ActionSelect, Default: true},
			{ID: "opt3", Label: "Option 3", Kind: ActionSelect},
		},
	}
	_ = e.Emit(ctx, frame)

	resp, err := e.AwaitResponse(ctx)
	if err != nil {
		t.Fatalf("AwaitResponse: %v", err)
	}
	if resp.ActionID != "opt2" {
		t.Errorf("action_id: got %q, want %q", resp.ActionID, "opt2")
	}
}

func TestNoopEmitter_AwaitResponse_NoDefault(t *testing.T) {
	e := &NoopEmitter{}
	ctx := context.Background()

	frame := InteractionFrame{
		Kind: FrameQuestion,
		Actions: []ActionSlot{
			{ID: "first", Label: "First", Kind: ActionSelect},
			{ID: "second", Label: "Second", Kind: ActionSelect},
		},
	}
	_ = e.Emit(ctx, frame)

	resp, err := e.AwaitResponse(ctx)
	if err != nil {
		t.Fatalf("AwaitResponse: %v", err)
	}
	if resp.ActionID != "first" {
		t.Errorf("action_id: got %q, want %q (should pick first when no default)", resp.ActionID, "first")
	}
}

func TestNoopEmitter_AwaitResponse_NoActions(t *testing.T) {
	e := &NoopEmitter{}
	ctx := context.Background()

	frame := InteractionFrame{
		Kind:    FrameStatus,
		Actions: nil,
	}
	_ = e.Emit(ctx, frame)

	resp, err := e.AwaitResponse(ctx)
	if err != nil {
		t.Fatalf("AwaitResponse: %v", err)
	}
	if resp.ActionID != "" {
		t.Errorf("action_id: got %q, want empty", resp.ActionID)
	}
}

func TestNoopEmitter_AwaitResponse_NoFrames(t *testing.T) {
	e := &NoopEmitter{}
	resp, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("AwaitResponse: %v", err)
	}
	if resp.ActionID != "" {
		t.Errorf("action_id: got %q, want empty", resp.ActionID)
	}
}

func TestNoopEmitter_AwaitResponse_RespectsContextCancellation(t *testing.T) {
	e := &NoopEmitter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.AwaitResponse(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestNoopEmitter_Reset(t *testing.T) {
	e := &NoopEmitter{}
	ctx := context.Background()

	_ = e.Emit(ctx, InteractionFrame{Kind: FrameStatus})
	_ = e.Emit(ctx, InteractionFrame{Kind: FrameResult})
	if len(e.Frames) != 2 {
		t.Fatalf("before reset: got %d frames, want 2", len(e.Frames))
	}
	e.Reset()
	if len(e.Frames) != 0 {
		t.Errorf("after reset: got %d frames, want 0", len(e.Frames))
	}
}

func TestNoopEmitter_MultipleFrames(t *testing.T) {
	e := &NoopEmitter{}
	ctx := context.Background()

	frames := []InteractionFrame{
		{Kind: FrameProposal, Phase: "scope", Actions: []ActionSlot{{ID: "confirm", Default: true}}},
		{Kind: FrameQuestion, Phase: "clarify", Actions: []ActionSlot{{ID: "answer", Default: true}}},
		{Kind: FrameCandidates, Phase: "generate", Actions: []ActionSlot{{ID: "select"}}},
	}

	for _, f := range frames {
		if err := e.Emit(ctx, f); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if len(e.Frames) != 3 {
		t.Fatalf("frames: got %d, want 3", len(e.Frames))
	}

	// AwaitResponse uses last emitted frame
	resp, err := e.AwaitResponse(ctx)
	if err != nil {
		t.Fatalf("AwaitResponse: %v", err)
	}
	// Last frame has no default, so picks first action
	if resp.ActionID != "select" {
		t.Errorf("action_id: got %q, want %q", resp.ActionID, "select")
	}
}

func TestNoopEmitter_ImplementsFrameEmitter(t *testing.T) {
	var _ FrameEmitter = (*NoopEmitter)(nil)
}

func TestUserResponse_Fields(t *testing.T) {
	resp := UserResponse{
		ActionID:   "toggle_fix",
		Text:       "fix the auth issue first",
		Selections: []string{"finding-1", "finding-3"},
	}
	if resp.ActionID != "toggle_fix" {
		t.Errorf("action_id: got %q", resp.ActionID)
	}
	if resp.Text != "fix the auth issue first" {
		t.Errorf("text: got %q", resp.Text)
	}
	if len(resp.Selections) != 2 {
		t.Errorf("selections: got %d, want 2", len(resp.Selections))
	}
}

func TestFrameConstruction_FullProposal(t *testing.T) {
	frame := InteractionFrame{
		Kind:  FrameProposal,
		Mode:  "code",
		Phase: "understand",
		Content: ProposalContent{
			Interpretation: "Add retry logic to HTTP client",
			Scope:          []string{"pkg/http/client.go"},
			Approach:       "edit_verify_repair",
			Constraints:    []string{"keep backward compatibility"},
		},
		Actions: []ActionSlot{
			{ID: "confirm", Label: "Looks good", Shortcut: "y", Kind: ActionConfirm, Default: true},
			{ID: "correct", Label: "Let me clarify", Kind: ActionFreetext},
			{ID: "plan", Label: "Plan first", Kind: ActionTransition, TargetPhase: "planning"},
		},
		Continuable: true,
		Metadata: FrameMetadata{
			Timestamp:    time.Now(),
			PhaseIndex:   0,
			PhaseCount:   5,
			ArtifactRefs: []string{"classify-001"},
		},
	}

	if frame.DefaultAction().ID != "confirm" {
		t.Error("default action should be confirm")
	}
	if frame.ActionByID("plan").TargetPhase != "planning" {
		t.Error("plan action should target planning phase")
	}
	if frame.ActionByID("correct").Kind != ActionFreetext {
		t.Error("correct action should be freetext")
	}
}
