package euclotui

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

func TestTUIFrameEmitter_Emit(t *testing.T) {
	emitter := NewTUIFrameEmitter(nil) // nil program — no tea.Send
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "code",
		Phase: "intent",
		Content: interaction.ProposalContent{
			Interpretation: "test",
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}

	if err := emitter.Emit(context.Background(), frame); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if len(emitter.Frames()) != 1 {
		t.Errorf("frames: got %d, want 1", len(emitter.Frames()))
	}
}

func TestTUIFrameEmitter_AwaitResponse(t *testing.T) {
	emitter := NewTUIFrameEmitter(nil)

	// Send a response from another goroutine.
	go func() {
		emitter.Resolve(interaction.UserResponse{ActionID: "confirm"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resp, err := emitter.AwaitResponse(ctx)
	if err != nil {
		t.Fatalf("AwaitResponse: %v", err)
	}
	if resp.ActionID != "confirm" {
		t.Errorf("action: got %q, want confirm", resp.ActionID)
	}
}

func TestTUIFrameEmitter_AwaitResponseCanceled(t *testing.T) {
	emitter := NewTUIFrameEmitter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancel

	_, err := emitter.AwaitResponse(ctx)
	if err == nil {
		t.Error("expected error on canceled context")
	}
}
