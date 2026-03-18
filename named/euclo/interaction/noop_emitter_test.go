package interaction

import (
	"context"
	"testing"
)

// TestNoopEmitter_AdvancesAllFrameKinds verifies that NoopEmitter can handle
// every frame kind without error, producing a valid auto-response.
func TestNoopEmitter_AdvancesAllFrameKinds(t *testing.T) {
	kinds := []FrameKind{
		FrameProposal, FrameQuestion, FrameCandidates, FrameComparison,
		FrameDraft, FrameResult, FrameStatus, FrameSummary, FrameTransition, FrameHelp,
	}
	for _, kind := range kinds {
		emitter := &NoopEmitter{}
		ctx := context.Background()

		frame := InteractionFrame{
			Kind: kind,
			Actions: []ActionSlot{
				{ID: "default_action", Label: "Default", Default: true},
			},
		}
		if err := emitter.Emit(ctx, frame); err != nil {
			t.Errorf("%s: emit error: %v", kind, err)
			continue
		}
		resp, err := emitter.AwaitResponse(ctx)
		if err != nil {
			t.Errorf("%s: await error: %v", kind, err)
			continue
		}
		if resp.ActionID != "default_action" {
			t.Errorf("%s: got %q, want default_action", kind, resp.ActionID)
		}
	}
}

// TestNoopEmitter_TransitionFrame_RejectsDefault verifies that a transition frame
// with "reject" as default causes NoopEmitter to reject the transition.
func TestNoopEmitter_TransitionFrame_RejectsDefault(t *testing.T) {
	emitter := &NoopEmitter{}
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{
		Kind: FrameTransition,
		Content: TransitionContent{
			FromMode: "code",
			ToMode:   "debug",
			Reason:   "test",
		},
		Actions: []ActionSlot{
			{ID: "accept", Label: "Switch"},
			{ID: "reject", Label: "Stay", Default: true},
		},
	})
	resp, _ := emitter.AwaitResponse(ctx)
	if resp.ActionID != "reject" {
		t.Errorf("got %q, want reject", resp.ActionID)
	}
}

// TestNoopEmitter_PhaseMachineIntegration runs a simple machine with NoopEmitter
// and verifies it completes all phases.
func TestNoopEmitter_PhaseMachineIntegration(t *testing.T) {
	emitter := &NoopEmitter{}
	machine := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "test",
		Emitter: emitter,
		Phases: []PhaseDefinition{
			{
				ID:    "first",
				Label: "First",
				Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					_ = mc.Emitter.Emit(context.Background(), InteractionFrame{
						Kind:  FrameProposal,
						Phase: "first",
						Actions: []ActionSlot{
							{ID: "confirm", Label: "Confirm", Default: true},
						},
					})
					_, _ = mc.Emitter.AwaitResponse(context.Background())
					return PhaseOutcome{Advance: true}, nil
				}),
			},
			{
				ID:    "second",
				Label: "Second",
				Handler: PhaseHandlerFunc(func(_ context.Context, _ PhaseMachineContext) (PhaseOutcome, error) {
					return PhaseOutcome{Advance: true}, nil
				}),
			},
		},
	})

	if err := machine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(emitter.Frames) != 1 {
		t.Errorf("frames: got %d, want 1", len(emitter.Frames))
	}
}
