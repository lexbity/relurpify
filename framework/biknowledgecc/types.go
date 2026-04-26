package biknowledgecc

import (
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

type ChunkID = knowledge.ChunkID
type EdgeID = knowledge.EdgeID
type FreshnessState = knowledge.FreshnessState
type CompilerPath = knowledge.CompilerPath
type ViewKind = knowledge.ViewKind
type EdgeKind = knowledge.EdgeKind
type ProvenanceSource = knowledge.ProvenanceSource
type ChunkProvenance = knowledge.ChunkProvenance
type ChunkBody = knowledge.ChunkBody
type ChunkView = knowledge.ChunkView
type KnowledgeChunk = knowledge.KnowledgeChunk
type ChunkEdge = knowledge.ChunkEdge
type ChunkStore = knowledge.ChunkStore

const (
	FreshnessValid      = knowledge.FreshnessValid
	FreshnessStale      = knowledge.FreshnessStale
	FreshnessInvalid    = knowledge.FreshnessInvalid
	FreshnessUnverified = knowledge.FreshnessUnverified

	CompilerDeterministic = knowledge.CompilerDeterministic
	CompilerLLMAssisted   = knowledge.CompilerLLMAssisted
	CompilerUserDirect    = knowledge.CompilerUserDirect

	ViewKindPattern    = knowledge.ViewKindPattern
	ViewKindDecision   = knowledge.ViewKindDecision
	ViewKindConstraint = knowledge.ViewKindConstraint
	ViewKindPlanStep   = knowledge.ViewKindPlanStep
	ViewKindAnchor     = knowledge.ViewKindAnchor
	ViewKindTension    = knowledge.ViewKindTension
	ViewKindIntent     = knowledge.ViewKindIntent

	EdgeKindGrounds            = knowledge.EdgeKindGrounds
	EdgeKindContradicts        = knowledge.EdgeKindContradicts
	EdgeKindRefines            = knowledge.EdgeKindRefines
	EdgeKindGeneralizes        = knowledge.EdgeKindGeneralizes
	EdgeKindExemplifies        = knowledge.EdgeKindExemplifies
	EdgeKindDerivesFrom        = knowledge.EdgeKindDerivesFrom
	EdgeKindComposedOf         = knowledge.EdgeKindComposedOf
	EdgeKindSupersedes         = knowledge.EdgeKindSupersedes
	EdgeKindRequiresContext    = knowledge.EdgeKindRequiresContext
	EdgeKindAmplifies          = knowledge.EdgeKindAmplifies
	EdgeKindInvalidates        = knowledge.EdgeKindInvalidates
	EdgeKindDependsOnCodeState = knowledge.EdgeKindDependsOnCodeState
	EdgeKindConfirmed          = knowledge.EdgeKindConfirmed
	EdgeKindRejected           = knowledge.EdgeKindRejected
	EdgeKindRefinedBy          = knowledge.EdgeKindRefinedBy
	EdgeKindDeferred           = knowledge.EdgeKindDeferred
)

// EventKind identifies a live compiler event.
type EventKind string

const (
	EventBootstrapComplete   EventKind = "biknowledgecc.bootstrap_complete"
	EventCodeRevisionChanged EventKind = "biknowledgecc.code_revision_changed"
	EventChunkStaled         EventKind = "biknowledgecc.chunk_staled"
	EventPatternConfirmed    EventKind = "biknowledgecc.pattern_confirmed"
	EventAnchorConfirmed     EventKind = "biknowledgecc.anchor_confirmed"
	EventIndexEntryProduced  EventKind = "biknowledgecc.index_entry_produced"
	EventUserStatement       EventKind = "biknowledgecc.user_statement"
)

// Event is the live compiler bus envelope.
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

// CodeRevisionChangedPayload reports git revision drift.
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

// EventBus is a lightweight in-process compiler event broker.
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
	b.Publish(Event{
		Kind:      EventBootstrapComplete,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
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

// EmitCodeRevisionChanged publishes a git revision change event.
func (b *EventBus) EmitCodeRevisionChanged(payload CodeRevisionChangedPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{
		Kind:      EventCodeRevisionChanged,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}

// EmitChunkStaled publishes a chunk staleness event.
func (b *EventBus) EmitChunkStaled(payload ChunkStaledPayload) {
	if b == nil {
		return
	}
	b.Publish(Event{
		Kind:      EventChunkStaled,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	})
}

// Module lets archaeology-specific behavior attach to the compiler core.
type Module interface {
	Attach(*Compiler)
}
