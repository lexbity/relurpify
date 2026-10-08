package knowledge

import (
	"sync"
	"testing"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// bridgeRecordingSink captures telemetry events for assertions.
type bridgeRecordingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *bridgeRecordingSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *bridgeRecordingSink) Events() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telemetry.Event(nil), s.events...)
}

func (s *bridgeRecordingSink) waitFor(t *testing.T, eventType telemetry.EventType) telemetry.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range s.Events() {
			if ev.Type == eventType {
				return ev
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("event %s never observed; types=%v", eventType, sinkEventTypes(s))
	return telemetry.Event{}
}

func sinkEventTypes(sink *bridgeRecordingSink) []telemetry.EventType {
	var out []telemetry.EventType
	for _, ev := range sink.Events() {
		out = append(out, ev.Type)
	}
	return out
}

func TestEventBusTelemetryBridgeEmitsChunkCommitted(t *testing.T) {
	bus := &EventBus{}
	sink := &bridgeRecordingSink{}
	bridge := NewEventBusTelemetryBridge(bus, sink)
	defer bridge.Close()

	bus.EmitChunkIngested(ChunkIngestedPayload{
		ChunkID:       "chunk-42",
		ContentHash:   "hash-42",
		SourceOrigin:  string(SourceOriginTool),
		TokenEstimate: 120,
		SessionID:     "session-1",
		WorkflowID:    "workflow-1",
		NodeID:        "node-1",
	})

	ev := sink.waitFor(t, telemetry.EventChunkCommitted)
	if ev.Metadata["chunk_id"] != "chunk-42" {
		t.Fatalf("unexpected chunk_id in %+v", ev.Metadata)
	}
}

func TestEventBusTelemetryBridgeEmitsChunkStaled(t *testing.T) {
	bus := &EventBus{}
	sink := &bridgeRecordingSink{}
	bridge := NewEventBusTelemetryBridge(bus, sink)
	defer bridge.Close()

	bus.EmitChunkStaled(ChunkStaledPayload{
		ChunkIDs:      []string{"chunk-7"},
		Reason:        "code_revision_changed",
		AffectedPaths: []string{"src/main.go"},
	})

	ev := sink.waitFor(t, telemetry.EventChunkStaled)
	if ev.Metadata["reason"] != "code_revision_changed" {
		t.Fatalf("unexpected reason in %+v", ev.Metadata)
	}
}

func TestEventBusTelemetryBridgeEmitsChunkInvalidated(t *testing.T) {
	bus := &EventBus{}
	sink := &bridgeRecordingSink{}
	bridge := NewEventBusTelemetryBridge(bus, sink)
	defer bridge.Close()

	bus.EmitCodeRevisionChanged(CodeRevisionChangedPayload{
		NewRevision:   "rev-200",
		AffectedPaths: []string{"src/lib.go"},
	})

	ev := sink.waitFor(t, telemetry.EventChunkInvalidated)
	if ev.Metadata["new_revision"] != "rev-200" {
		t.Fatalf("unexpected revision in %+v", ev.Metadata)
	}
}

func TestEventBusTelemetryBridgeNilSinkIsNoop(t *testing.T) {
	bus := &EventBus{}
	bridge := NewEventBusTelemetryBridge(bus, nil)
	// A nil sink must not start a goroutine; Close must be safe and immediate.
	done := make(chan struct{})
	go func() {
		bridge.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close on a nil-sink bridge blocked")
	}
}

func TestEventBusTelemetryBridgeCloseIsIdempotent(t *testing.T) {
	bus := &EventBus{}
	sink := &bridgeRecordingSink{}
	bridge := NewEventBusTelemetryBridge(bus, sink)
	bridge.Close()
	bridge.Close() // second close must not panic or block
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-bridge.done:
			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Fatal("bridge goroutine did not terminate after Close")
}
