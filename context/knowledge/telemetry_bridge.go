package knowledge

import (
	"context"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// EventBusTelemetryBridge subscribes to a knowledge EventBus and re-emits the
// chunk lifecycle signals — committed, staled, invalidated — as framework
// telemetry events (FR-14). This is the missing bridge that connects the
// in-process artifact broker to the durable diagnostic trail.
//
// The bridge never blocks the bus: EventBus.Publish is non-blocking, and the
// bridge's forwarding goroutine consumes the buffered subscription as fast as
// it can. Close unsubscribes and terminates the goroutine.
type EventBusTelemetryBridge struct {
	sink   telemetry.Telemetry
	cancel func()
	done   chan struct{}
}

// NewEventBusTelemetryBridge wires the bridge onto the bus. A nil bus or sink
// yields a no-op bridge that never starts a goroutine.
func NewEventBusTelemetryBridge(bus *EventBus, sink telemetry.Telemetry) *EventBusTelemetryBridge {
	bridge := &EventBusTelemetryBridge{
		sink: sink,
		done: make(chan struct{}),
	}
	if bus == nil || sink == nil {
		return bridge
	}
	ch, unsub := bus.Subscribe(64)
	bridge.cancel = unsub
	go bridge.run(ch)
	return bridge
}

// run forwards bus events to the telemetry sink until the subscription closes.
func (b *EventBusTelemetryBridge) run(ch <-chan Event) {
	defer close(b.done)
	for event := range ch {
		b.forward(event)
	}
}

// forward maps one bus event to its telemetry spelling.
func (b *EventBusTelemetryBridge) forward(event Event) {
	if b == nil || b.sink == nil {
		return
	}
	switch event.Kind {
	case EventChunkIngested:
		payload, ok := event.Payload.(ChunkIngestedPayload)
		if !ok {
			return
		}
		b.emit(event.Timestamp, telemetry.EventChunkCommitted, "chunk committed", map[string]any{
			"chunk_id":         payload.ChunkID,
			"content_hash":     payload.ContentHash,
			"source_origin":    payload.SourceOrigin,
			"token_estimate":   payload.TokenEstimate,
			"session_id":       payload.SessionID,
			"workflow_id":      payload.WorkflowID,
			"node_id":          payload.NodeID,
			"source_chunk_ids": append([]string(nil), payload.SourceChunkIDs...),
		})
	case EventChunkStaled:
		payload, ok := event.Payload.(ChunkStaledPayload)
		if !ok {
			return
		}
		b.emit(event.Timestamp, telemetry.EventChunkStaled, "chunk staled", map[string]any{
			"chunk_ids":      payload.ChunkIDs,
			"affected_paths": payload.AffectedPaths,
			"reason":         payload.Reason,
			"workspace_root": payload.WorkspaceRoot,
			"workflow_id":    payload.WorkflowID,
		})
	case EventTombstonePreserved:
		payload, ok := event.Payload.(TombstonePreservedPayload)
		if !ok {
			return
		}
		b.emit(event.Timestamp, telemetry.EventTombstonePreserved, "tombstone preserved", map[string]any{
			"chunk_id":     payload.ChunkID,
			"content_hash": payload.ContentHash,
			"kind":         payload.Kind,
		})
	case EventCodeRevisionChanged:
		payload, ok := event.Payload.(CodeRevisionChangedPayload)
		if !ok {
			return
		}
		b.emit(event.Timestamp, telemetry.EventChunkInvalidated, "chunk invalidated", map[string]any{
			"new_revision":   payload.NewRevision,
			"affected_paths": payload.AffectedPaths,
			"workspace_root": payload.WorkspaceRoot,
		})
	case EventBootstrapComplete:
		payload, ok := event.Payload.(BootstrapCompletePayload)
		if !ok {
			return
		}
		b.emit(event.Timestamp, telemetry.EventBootstrapComplete, "knowledge bootstrap complete", map[string]any{
			"workspace_root": payload.WorkspaceRoot,
			"indexed_files":  payload.IndexedFiles,
		})
	}
}

func (b *EventBusTelemetryBridge) emit(timestamp time.Time, eventType telemetry.EventType, message string, metadata map[string]any) {
	ev := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: timestamp,
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(context.Background(), &ev)
	b.sink.Emit(ev)
}

// Close unsubscribes from the bus and waits for the forwarding goroutine to
// terminate. Safe to call multiple times.
func (b *EventBusTelemetryBridge) Close() {
	if b == nil {
		return
	}
	if b.cancel != nil {
		cancel := b.cancel
		b.cancel = nil
		cancel()
		<-b.done
	}
}
