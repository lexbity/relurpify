package contextdata

import (
	"context"
	"fmt"
	"time"
)

// Envelope is the execution context passed to graph nodes.
// It carries references to streamed context, working memory, and retrieval state
// without duplicating the underlying data.
//
// Streamed context is read-only to graph nodes. Working memory is mutable.
// Retrieval results are stored as references. Checkpoints may be requested.
type Envelope struct {
	// TaskID identifies the execution scope for this envelope.
	TaskID string

	// SessionID identifies the session this envelope belongs to.
	SessionID string

	// NodeID identifies the current node executing with this envelope.
	NodeID string

	// References holds all tier references in this envelope.
	References ReferenceBundle

	// WorkingData holds the actual working memory values (mutable tier only).
	// Streamed context and retrieval results are accessed via references.
	WorkingData map[string]any

	// CheckpointRequest is set when a node requests a checkpoint.
	// The compiler owns the actual checkpoint materialization.
	CheckpointRequest *CheckpointRequest

	// AssemblyMetadata tracks compiler assembly state.
	AssemblyMetadata AssemblyMeta

	// createdAt tracks when this envelope was created.
	createdAt time.Time
}

// AssemblyMeta tracks compiler-specific metadata for envelope assembly.
type AssemblyMeta struct {
	// CompilationID identifies the compilation that assembled this envelope.
	CompilationID string

	// EventLogSeq is the event log sequence number at assembly time.
	EventLogSeq uint64

	// BudgetTokens is the token budget used for assembly.
	BudgetTokens int

	// ShortfallTokens is any budget shortfall encountered.
	ShortfallTokens int

	// AssembledAt records when the envelope was assembled.
	AssembledAt time.Time
}

// CheckpointRequest records a node-originated checkpoint request.
// The compiler materializes checkpoints; nodes only request them.
type CheckpointRequest struct {
	// RequestedBy is the node ID that requested the checkpoint.
	RequestedBy string

	// Reason describes why the checkpoint was requested.
	Reason string

	// Priority indicates checkpoint urgency (higher = more urgent).
	Priority int

	// EvictWorkingMemory indicates whether to evict working memory after checkpoint.
	EvictWorkingMemory bool

	// RequestedAt records when the request was made.
	RequestedAt time.Time
}

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

// SetWorkingValue stores a value in working memory.
// This is the primary write path for graph nodes.
func (e *Envelope) SetWorkingValue(key string, value any, class MemoryClass) {
	if e == nil || e.WorkingData == nil {
		return
	}

	now := time.Now().UTC()
	e.WorkingData[key] = value

	// Update or create the reference
	found := false
	for i, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID && ref.Key == key {
			e.References.WorkingMemory[i].UpdatedAt = now
			e.References.WorkingMemory[i].Class = class
			// ValueHash could be computed here for change detection
			found = true
			break
		}
	}

	if !found {
		e.References.WorkingMemory = append(e.References.WorkingMemory, WorkingMemoryReference{
			TaskID:    e.TaskID,
			Key:       key,
			Class:     class,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
}

// GetWorkingValue retrieves a value from working memory.
func (e *Envelope) GetWorkingValue(key string) (any, bool) {
	if e == nil || e.WorkingData == nil {
		return nil, false
	}
	val, ok := e.WorkingData[key]
	return val, ok
}

// DeleteWorkingValue removes a value from working memory.
func (e *Envelope) DeleteWorkingValue(key string) {
	if e == nil || e.WorkingData == nil {
		return
	}
	delete(e.WorkingData, key)

	// Remove the reference as well
	newRefs := make([]WorkingMemoryReference, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if !(ref.TaskID == e.TaskID && ref.Key == key) {
			newRefs = append(newRefs, ref)
		}
	}
	e.References.WorkingMemory = newRefs
}

// RequestCheckpoint sets a checkpoint request on the envelope.
// The compiler will materialize the checkpoint when processing this envelope.
func (e *Envelope) RequestCheckpoint(reason string, priority int, evictMemory bool) {
	if e == nil {
		return
	}
	e.CheckpointRequest = &CheckpointRequest{
		RequestedBy:        e.NodeID,
		Reason:             reason,
		Priority:           priority,
		EvictWorkingMemory: evictMemory,
		RequestedAt:        time.Now().UTC(),
	}
}

// ClearCheckpointRequest removes any pending checkpoint request.
// Called by the compiler after materializing the checkpoint.
func (e *Envelope) ClearCheckpointRequest() {
	if e == nil {
		return
	}
	e.CheckpointRequest = nil
}

// AddRetrievalReference adds a retrieval result reference to the envelope.
// Called after graph nodes trigger retrieval operations.
func (e *Envelope) AddRetrievalReference(ref RetrievalReference) {
	if e == nil {
		return
	}
	e.References.Retrieval = append(e.References.Retrieval, ref)
}

// AddStreamedContextReference adds a streamed context chunk reference.
// This is primarily called by the compiler during context assembly.
func (e *Envelope) AddStreamedContextReference(ref ChunkReference) {
	if e == nil {
		return
	}
	e.References.StreamedContext = append(e.References.StreamedContext, ref)
}

// StreamedChunkIDs returns the IDs of all chunks in the streamed context.
// This is read-only data assembled by the compiler.
func (e *Envelope) StreamedChunkIDs() []ChunkID {
	if e == nil {
		return nil
	}
	ids := make([]ChunkID, len(e.References.StreamedContext))
	for i, ref := range e.References.StreamedContext {
		ids[i] = ref.ChunkID
	}
	return ids
}

// WorkingMemoryKeys returns all keys in the working memory for this envelope's task.
func (e *Envelope) WorkingMemoryKeys() []string {
	if e == nil {
		return nil
	}
	keys := make([]string, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID {
			keys = append(keys, ref.Key)
		}
	}
	return keys
}

// IsEmpty returns true if the envelope has no working data and no references.
func (e *Envelope) IsEmpty() bool {
	if e == nil {
		return true
	}
	return len(e.WorkingData) == 0 && e.References.IsEmpty()
}

// ContextKey is the key type for envelope storage in context.Context.
type contextKey struct{}

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
// Use only in contexts where the envelope must exist.
func MustEnvelopeFrom(ctx context.Context) *Envelope {
	env, ok := EnvelopeFrom(ctx)
	if !ok {
		panic("contextdata: envelope not found in context")
	}
	return env
}

// Snapshot returns a point-in-time copy of working memory data.
func (e *Envelope) Snapshot() map[string]any {
	if e == nil || e.WorkingData == nil {
		return nil
	}
	out := make(map[string]any, len(e.WorkingData))
	for k, v := range e.WorkingData {
		out[k] = v
	}
	return out
}

// String returns a summary of the envelope for logging.
func (e *Envelope) String() string {
	if e == nil {
		return "<nil envelope>"
	}
	return fmt.Sprintf("Envelope{TaskID:%s NodeID:%s Working:%d Streamed:%d Retrieval:%d}",
		e.TaskID, e.NodeID, len(e.WorkingData),
		len(e.References.StreamedContext), len(e.References.Retrieval))
}
