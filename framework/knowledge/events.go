package knowledge

import (
	"sync"
	"time"
)

// EventKind identifies an artifact-knowledge event.
type EventKind string

const (
	EventBootstrapComplete   EventKind = "knowledge.bootstrap_complete"
	EventCodeRevisionChanged EventKind = "knowledge.code_revision_changed"
	EventChunkStaled         EventKind = "knowledge.chunk_staled"
	EventPatternConfirmed    EventKind = "knowledge.pattern_confirmed"
	EventAnchorConfirmed     EventKind = "knowledge.anchor_confirmed"
	EventIndexEntryProduced  EventKind = "knowledge.index_entry_produced"
	EventUserStatement       EventKind = "knowledge.user_statement"
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

// EventBus is a lightweight in-process artifact event broker.
type EventBus struct {
	mu             sync.RWMutex
	nextID         int
	subscribers    map[int]chan Event
	bootstrapReady bool
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

// Publish fans an event out to current subscribers without blocking.
func (b *EventBus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
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
