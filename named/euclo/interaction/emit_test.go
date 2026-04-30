package interaction

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type captureTelemetry struct {
	mu     sync.Mutex
	events []core.Event
}

func (c *captureTelemetry) Emit(event core.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *captureTelemetry) snapshot() []core.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]core.Event, len(c.events))
	copy(out, c.events)
	return out
}

func TestEmitFrame_WritesToEnvelope(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	frame := NewOutcomeFeedbackFrame("task-1", "session-1", "done")

	if err := EmitFrame(context.Background(), frame, env, nil); err != nil {
		t.Fatalf("EmitFrame failed: %v", err)
	}

	got, ok := env.GetWorkingValue(frameStorageKey(0))
	if !ok {
		t.Fatal("expected frame in envelope")
	}
	if got != frame {
		t.Fatal("expected stored frame to be same instance")
	}
}

func TestEmitFrame_EmitsTelemetryEvent(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	sink := &captureTelemetry{}
	ctx := core.WithTelemetry(context.Background(), sink)
	frame := NewCandidateSelectionFrame("task-1", "session-1", []string{"a", "b"})

	if err := EmitFrame(ctx, frame, env, nil); err != nil {
		t.Fatalf("EmitFrame failed: %v", err)
	}

	events := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != core.EventType("euclo.interaction.frame.emitted") {
		t.Fatalf("expected emitted event, got %q", events[0].Type)
	}
	if got := events[0].Metadata["frame_id"]; got != frame.ID {
		t.Fatalf("expected frame_id %q, got %v", frame.ID, got)
	}
}

func TestEmitFrame_NilTelemetry_NoOp(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	frame := NewOutcomeFeedbackFrame("task-1", "session-1", "done")

	if err := EmitFrame(context.Background(), frame, env, nil); err != nil {
		t.Fatalf("EmitFrame failed: %v", err)
	}
}

func TestEmitFrame_SequenceIncrement(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	first := NewOutcomeFeedbackFrame("task-1", "session-1", "done")
	second := NewOutcomeFeedbackFrame("task-1", "session-1", "done")

	if err := EmitFrame(context.Background(), first, env, nil); err != nil {
		t.Fatalf("EmitFrame first failed: %v", err)
	}
	if err := EmitFrame(context.Background(), second, env, nil); err != nil {
		t.Fatalf("EmitFrame second failed: %v", err)
	}
	if first.Seq != 0 {
		t.Fatalf("expected first seq 0, got %d", first.Seq)
	}
	if second.Seq != 1 {
		t.Fatalf("expected second seq 1, got %d", second.Seq)
	}
}
