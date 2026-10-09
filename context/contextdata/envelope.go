package contextdata

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Envelope is the execution context passed to graph nodes.
//
// Locking invariant: all Envelope state is guarded by mu. Callers outside the
// package read through the snapshot accessors and mutate through the setter
// methods; the exported fields are an implementation detail and must not be
// accessed directly. Reads: TaskIDSnapshot, SessionIDSnapshot, NodeIDSnapshot,
// WorkingDataSnapshot, ReferencesSnapshot, AssemblyMetadataSnapshot,
// CheckpointRequestSnapshot, Clone, HandoffSnapshot. Writes: SetTaskID,
// SetSessionID, SetNodeID, SetAssemblyMetadata, SetWorkingValue[WithClass],
// DeleteWorkingValue, AddStreamedContextReference, AddRetrievalReference,
// AddCheckpointReference, RequestCheckpoint, ClearCheckpointRequest.
type Envelope struct {
	mu                sync.RWMutex
	TaskID            string
	SessionID         string
	NodeID            string
	References        ReferenceBundle
	WorkingData       map[string]any
	Origins           map[string]OriginClass
	CheckpointRequest *CheckpointRequest
	AssemblyMetadata  AssemblyMeta
	createdAt         time.Time
}

// AssemblyMeta tracks compiler-specific metadata for envelope assembly.
type AssemblyMeta struct {
	CompilationID   string
	EventLogSeq     uint64
	BudgetTokens    int
	ShortfallTokens int
	AssembledAt     time.Time
	EpochID         uint64
}

type contextKey struct{}

// NewEnvelope creates a new envelope for the given task and session.
func NewEnvelope(taskID, sessionID string) *Envelope {
	return &Envelope{
		TaskID:      taskID,
		SessionID:   sessionID,
		WorkingData: make(map[string]any),
		References:  ReferenceBundle{},
		createdAt:   time.Now().UTC(),
	}
}

// NodeIDSnapshot returns the envelope's current node identifier, or "" when it
// is unset. NodeID is reassignable, so callers outside the package read it
// through this accessor rather than the field.
func (e *Envelope) NodeIDSnapshot() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.NodeID
}

// SetNodeID replaces the envelope's node identifier under the lock.
func (e *Envelope) SetNodeID(nodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.NodeID = nodeID
}

// TaskIDSnapshot returns the envelope's task identifier under the read lock.
func (e *Envelope) TaskIDSnapshot() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.TaskID
}

// SetTaskID replaces the envelope's task identifier under the lock.
func (e *Envelope) SetTaskID(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.TaskID = taskID
}

// SessionIDSnapshot returns the envelope's session identifier under the read lock.
func (e *Envelope) SessionIDSnapshot() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.SessionID
}

// SetSessionID replaces the envelope's session identifier under the lock.
func (e *Envelope) SetSessionID(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.SessionID = sessionID
}

// IsEmpty returns true if the envelope has no working data and no references.
func (e *Envelope) IsEmpty() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.WorkingData) == 0 && e.References.IsEmpty()
}

// String returns a summary of the envelope for logging.
func (e *Envelope) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return fmt.Sprintf("Envelope{TaskID:%s NodeID:%s Working:%d Streamed:%d Retrieval:%d}",
		e.TaskID, e.NodeID, len(e.WorkingData),
		len(e.References.StreamedContext), len(e.References.Retrieval))
}

// WithEnvelope attaches an envelope to a context.
func WithEnvelope(ctx context.Context, env *Envelope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, env)
}

// EnvelopeFrom extracts the envelope from a context.
func EnvelopeFrom(ctx context.Context) (*Envelope, bool) {
	if ctx == nil {
		return nil, false
	}
	val := ctx.Value(contextKey{})
	if val == nil {
		return nil, false
	}
	env, ok := val.(*Envelope)
	return env, ok
}

// MustEnvelopeFrom extracts the envelope from a context, panicking if not present.
func MustEnvelopeFrom(ctx context.Context) *Envelope {
	env, ok := EnvelopeFrom(ctx)
	if !ok {
		panic("contextdata: envelope not found in context")
	}
	return env
}
