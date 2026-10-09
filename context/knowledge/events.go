package knowledge

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// publishTimeout bounds how long Publish waits on a full subscriber buffer
// before dropping the event and accounting it.
const publishTimeout = 10 * time.Millisecond

// EventKind identifies an artifact-knowledge event.
type EventKind string

const (
	EventBootstrapComplete    EventKind = "knowledge.bootstrap_complete"
	EventCodeRevisionChanged  EventKind = "knowledge.code_revision_changed"
	EventChunkStaled          EventKind = "knowledge.chunk_staled"
	EventChunkIngested        EventKind = "knowledge.chunk_ingested"
	EventTombstonePreserved   EventKind = "knowledge.tombstone_preserved"
	EventInvalidationDegraded EventKind = "knowledge.invalidation_degraded"
	EventPatternConfirmed     EventKind = "knowledge.pattern_confirmed"
	EventAnchorConfirmed      EventKind = "knowledge.anchor_confirmed"
	EventIndexEntryProduced   EventKind = "knowledge.index_entry_produced"
	EventUserStatement        EventKind = "knowledge.user_statement"
)

// Event is the in-process event envelope.
type Event struct {
	Kind      EventKind `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload,omitempty"`
}

// BootstrapCompletePayload reports bootstrap indexing completion.
type BootstrapCompletePayload struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	IndexedFiles  int    `json:"indexed_files,omitempty"`
}

// CodeRevisionChangedPayload reports revision drift.
type CodeRevisionChangedPayload struct {
	WorkspaceRoot string   `json:"workspace_root,omitempty"`
	NewRevision   string   `json:"new_revision,omitempty"`
	AffectedPaths []string `json:"affected_paths,omitempty"`
}

// ChunkStaledPayload reports chunks excluded by invalidation or stream-time staleness.
type ChunkStaledPayload struct {
	WorkspaceRoot string   `json:"workspace_root,omitempty"`
	WorkflowID    string   `json:"workflow_id,omitempty"`
	ChunkIDs      []string `json:"chunk_ids,omitempty"`
	AffectedPaths []string `json:"affected_paths,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

// ChunkIngestedPayload reports a newly ingested chunk.
type ChunkIngestedPayload struct {
	WorkspaceRoot  string   `json:"workspace_root,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	WorkflowID     string   `json:"workflow_id,omitempty"`
	NodeID         string   `json:"node_id,omitempty"`
	ChunkID        string   `json:"chunk_id,omitempty"`
	ContentHash    string   `json:"content_hash,omitempty"`
	SourceOrigin   string   `json:"source_origin,omitempty"`
	TokenEstimate  int      `json:"token_estimate,omitempty"`
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

// TombstonePreservedPayload reports that identical content matched a tombstoned
// chunk and was deliberately not resurrected.
type TombstonePreservedPayload struct {
	ChunkID     string `json:"chunk_id,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// InvalidationDegradedPayload reports a knowledge-invalidation loop that has
// failed repeatedly and is now retrying with backoff.
type InvalidationDegradedPayload struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	FailureCount  int    `json:"failure_count"`
	Error         string `json:"error,omitempty"`
}

// EventBus is a lightweight in-process artifact event broker. Publish blocks
// briefly (publishTimeout) on a full subscriber buffer before dropping and
// accounting the event, so a slow subscriber degrades to counted loss rather
// than unbounded producer latency.
type EventBus struct {
	mu             sync.RWMutex
	nextID         int
	subscribers    map[int]chan Event
	bootstrapReady bool
	dropped        atomic.Uint64
	tel            telemetry.Telemetry
}

// SetTelemetry wires the sink that receives knowledge.event_dropped and other
// bus-level health signals. Safe to call before concurrent use.
func (b *EventBus) SetTelemetry(tel telemetry.Telemetry) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.tel = tel
	b.mu.Unlock()
}

// DroppedTotal reports how many published events were dropped because a
// subscriber buffer stayed full through the publish timeout.
func (b *EventBus) DroppedTotal() uint64 {
	if b == nil {
		return 0
	}
	return b.dropped.Load()
}

// Subscribe registers a buffered event stream.
func (b *EventBus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers == nil {
		b.subscribers = make(map[int]chan Event)
	}
	b.nextID++
	id := b.nextID
	ch := make(chan Event, buffer)
	b.subscribers[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if current, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(current)
		}
	}
}

// Publish fans an event out to current subscribers. A subscriber whose buffer
// is full is waited on for publishTimeout before the event is dropped and the
// per-bus drop counter is incremented.
func (b *EventBus) Publish(event Event) {
	if b == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		if b.deliver(ch, event) {
			total := b.dropped.Add(1)
			b.emitDropLocked(event, total)
		}
	}
}

// deliver performs the non-blocking send fast path, then a bounded wait before
// reporting a drop.
func (b *EventBus) deliver(ch chan Event, event Event) (dropped bool) {
	select {
	case ch <- event:
		return false
	default:
	}
	timer := time.NewTimer(publishTimeout)
	defer timer.Stop()
	select {
	case ch <- event:
		return false
	case <-timer.C:
		return true
	}
}

// emitDropLocked surfaces a dropped knowledge event on the telemetry trail.
func (b *EventBus) emitDropLocked(event Event, total uint64) {
	if b.tel == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventEventDropped,
		Message:   "knowledge event dropped",
		Timestamp: event.Timestamp,
		Metadata: map[string]any{
			"kind":          string(event.Kind),
			"dropped_total": total,
		},
	}
	telemetry.StampCorrelation(context.Background(), &ev)
	b.tel.Emit(ev)
}

// EmitBootstrapComplete publishes a workspace bootstrap completion event.
func (b *EventBus) EmitBootstrapComplete(payload BootstrapCompletePayload) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.bootstrapReady = true
	b.mu.Unlock()
	b.Publish(Event{Kind: EventBootstrapComplete, Timestamp: time.Now().UTC(), Payload: payload})
}

// BootstrapReady reports whether bootstrap completion has been observed.
func (b *EventBus) BootstrapReady() bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bootstrapReady
}

// EmitCodeRevisionChanged publishes a revision drift event.
func (b *EventBus) EmitCodeRevisionChanged(payload CodeRevisionChangedPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{Kind: EventCodeRevisionChanged, Timestamp: time.Now().UTC(), Payload: payload})
}

// EmitChunkStaled publishes a chunk staleness event.
func (b *EventBus) EmitChunkStaled(payload ChunkStaledPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{Kind: EventChunkStaled, Timestamp: time.Now().UTC(), Payload: payload})
}

// EmitChunkIngested publishes a chunk ingestion event.
func (b *EventBus) EmitChunkIngested(payload ChunkIngestedPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{Kind: EventChunkIngested, Timestamp: time.Now().UTC(), Payload: payload})
}

// EmitTombstonePreserved publishes a tombstone-preserved event.
func (b *EventBus) EmitTombstonePreserved(payload TombstonePreservedPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{Kind: EventTombstonePreserved, Timestamp: time.Now().UTC(), Payload: payload})
}

// EmitInvalidationDegraded publishes a knowledge-invalidation health event.
func (b *EventBus) EmitInvalidationDegraded(payload InvalidationDegradedPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{Kind: EventInvalidationDegraded, Timestamp: time.Now().UTC(), Payload: payload})
}
