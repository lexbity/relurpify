package contextdata

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Envelope is the execution context passed to graph nodes.
// It carries references to streamed context, working memory, and retrieval state
// without duplicating the underlying data.
//
// Streamed context is read-only to graph nodes. Working memory is mutable.
// Retrieval results are stored as references. Checkpoints may be requested.
type Envelope struct {
	mu sync.RWMutex

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
	if e == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
		e.WorkingData = make(map[string]any)
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
	if e == nil {
		return nil, false
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.WorkingData == nil {
		return nil, false
	}
	val, ok := e.WorkingData[key]
	return val, ok
}

// DeleteWorkingValue removes a value from working memory.
func (e *Envelope) DeleteWorkingValue(key string) {
	if e == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
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

// ClearWorkingData removes all working memory entries for this envelope's task.
// This is called by StateModeFresh paradigms at Execute entry.
func (e *Envelope) ClearWorkingData() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.WorkingData == nil {
		return
	}
	// Clear all keys for this task
	keysToDelete := make([]string, 0)
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID {
			keysToDelete = append(keysToDelete, ref.Key)
		}
	}
	for _, key := range keysToDelete {
		delete(e.WorkingData, key)
	}
	// Remove references
	newRefs := make([]WorkingMemoryReference, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID != e.TaskID {
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
	e.mu.Lock()
	defer e.mu.Unlock()
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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.CheckpointRequest = nil
}

// AddRetrievalReference adds a retrieval result reference to the envelope.
// Called after graph nodes trigger retrieval operations.
func (e *Envelope) AddRetrievalReference(ref RetrievalReference) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.References.Retrieval = append(e.References.Retrieval, ref)
}

// AddStreamedContextReference adds a streamed context chunk reference.
// This is primarily called by the compiler during context assembly.
func (e *Envelope) AddStreamedContextReference(ref ChunkReference) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.References.StreamedContext = append(e.References.StreamedContext, ref)
}

// StreamedChunkIDs returns the IDs of all chunks in the streamed context.
// This is read-only data assembled by the compiler.
func (e *Envelope) StreamedChunkIDs() []ChunkID {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

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
	e.mu.RLock()
	defer e.mu.RUnlock()

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
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.WorkingData) == 0 && e.References.IsEmpty()
}

// WorkingDataSnapshot returns a point-in-time copy of working memory data.
func (e *Envelope) WorkingDataSnapshot() map[string]any {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.WorkingData == nil {
		return nil
	}
	out := make(map[string]any, len(e.WorkingData))
	for k, v := range e.WorkingData {
		out[k] = v
	}
	return out
}

// ReferencesSnapshot returns a point-in-time copy of the reference bundle.
func (e *Envelope) ReferencesSnapshot() ReferenceBundle {
	if e == nil {
		return ReferenceBundle{}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.References.Clone()
}

// AssemblyMetadataSnapshot returns a point-in-time copy of the assembly metadata.
func (e *Envelope) AssemblyMetadataSnapshot() AssemblyMeta {
	if e == nil {
		return AssemblyMeta{}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.AssemblyMetadata
}

// SetAssemblyMetadata replaces the assembly metadata.
func (e *Envelope) SetAssemblyMetadata(meta AssemblyMeta) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AssemblyMetadata = meta
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
	return e.WorkingDataSnapshot()
}

// Clone returns a deep copy of the envelope.
func (e *Envelope) Clone() *Envelope {
	if e == nil {
		return nil
	}
	workingData := e.WorkingDataSnapshot()
	refs := e.ReferencesSnapshot()
	e.mu.RLock()
	assemblyMetadata := e.AssemblyMetadata
	createdAt := e.createdAt
	e.mu.RUnlock()
	clone := &Envelope{
		TaskID:            e.TaskID,
		SessionID:         e.SessionID,
		NodeID:            e.NodeID,
		WorkingData:       workingData,
		CheckpointRequest: nil, // Don't clone checkpoint requests
		AssemblyMetadata:  assemblyMetadata,
		createdAt:         createdAt,
	}
	// Clone references (shallow copy is sufficient for references)
	clone.References = refs
	return clone
}

// HandoffPolicy controls which parts of an envelope survive a filtered handoff.
type HandoffPolicy struct {
	// PreserveWorkingMemory keeps selected working-memory keys.
	PreserveWorkingMemory bool

	// WorkingKeys preserves exact working-memory keys when present.
	WorkingKeys []string

	// WorkingPrefixes preserves keys with any of these prefixes.
	WorkingPrefixes []string

	// PreserveStreamedContext retains streamed-context references.
	PreserveStreamedContext bool

	// PreserveRetrieval retains retrieval references.
	PreserveRetrieval bool

	// PreserveCheckpoints retains checkpoint references.
	PreserveCheckpoints bool

	// PreserveAssemblyMetadata copies assembly metadata.
	PreserveAssemblyMetadata bool

	// PreserveNodeID copies the current node identifier.
	PreserveNodeID bool
}

// DefaultHandoffPolicy preserves the data typically needed when handing work
// from one agent boundary to another. It intentionally keeps references and
// selected working memory, but it does not invent any new payload shape.
func DefaultHandoffPolicy() HandoffPolicy {
	return HandoffPolicy{
		PreserveWorkingMemory:    true,
		PreserveStreamedContext:  true,
		PreserveRetrieval:        true,
		PreserveCheckpoints:      true,
		PreserveAssemblyMetadata: true,
		PreserveNodeID:           true,
	}
}

// HandoffClone returns a cloned envelope suitable for the default agent
// boundary handoff. This is the common-case transfer mechanism.
func (e *Envelope) HandoffClone() *Envelope {
	return e.Clone()
}

// HandoffSnapshot returns a filtered envelope using the supplied policy.
// It keeps the task/session boundary intact while dropping unlisted state.
func (e *Envelope) HandoffSnapshot(policy HandoffPolicy) *Envelope {
	if e == nil {
		return nil
	}
	workingData := e.WorkingDataSnapshot()
	refs := e.ReferencesSnapshot()
	snapshot := &Envelope{
		TaskID:      e.TaskID,
		SessionID:   e.SessionID,
		WorkingData: make(map[string]any),
		References:  ReferenceBundle{},
		createdAt:   e.createdAt,
	}
	if policy.PreserveNodeID {
		snapshot.NodeID = e.NodeID
	}
	if policy.PreserveAssemblyMetadata {
		e.mu.RLock()
		snapshot.AssemblyMetadata = e.AssemblyMetadata
		e.mu.RUnlock()
	}
	if policy.PreserveWorkingMemory {
		snapshot.WorkingData = cloneWorkingDataForHandoff(workingData, policy)
		snapshot.References.WorkingMemory = cloneWorkingMemoryRefsForHandoff(refs.WorkingMemory, e.TaskID, policy)
	}
	if policy.PreserveStreamedContext {
		snapshot.References.StreamedContext = append([]ChunkReference(nil), refs.StreamedContext...)
	}
	if policy.PreserveRetrieval {
		snapshot.References.Retrieval = append([]RetrievalReference(nil), refs.Retrieval...)
	}
	if policy.PreserveCheckpoints {
		snapshot.References.Checkpoints = append([]CheckpointReference(nil), refs.Checkpoints...)
	}
	return snapshot
}

func cloneWorkingDataForHandoff(workingData map[string]any, policy HandoffPolicy) map[string]any {
	if len(workingData) == 0 {
		return map[string]any{}
	}
	keys := make(map[string]struct{}, len(policy.WorkingKeys))
	for _, key := range policy.WorkingKeys {
		key = strings.TrimSpace(key)
		if key != "" {
			keys[key] = struct{}{}
		}
	}
	prefixes := make([]string, 0, len(policy.WorkingPrefixes))
	for _, prefix := range policy.WorkingPrefixes {
		if prefix = strings.TrimSpace(prefix); prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	out := make(map[string]any)
	for key, value := range workingData {
		if len(keys) > 0 {
			if _, ok := keys[key]; !ok {
				if !hasWorkingPrefix(key, prefixes) {
					continue
				}
			}
		} else if len(prefixes) > 0 && !hasWorkingPrefix(key, prefixes) {
			continue
		}
		out[key] = value
	}
	return out
}

func cloneWorkingMemoryRefsForHandoff(refs []WorkingMemoryReference, taskID string, policy HandoffPolicy) []WorkingMemoryReference {
	if len(refs) == 0 {
		return nil
	}
	keys := make(map[string]struct{}, len(policy.WorkingKeys))
	for _, key := range policy.WorkingKeys {
		key = strings.TrimSpace(key)
		if key != "" {
			keys[key] = struct{}{}
		}
	}
	prefixes := make([]string, 0, len(policy.WorkingPrefixes))
	for _, prefix := range policy.WorkingPrefixes {
		if prefix = strings.TrimSpace(prefix); prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	out := make([]WorkingMemoryReference, 0, len(refs))
	for _, ref := range refs {
		if ref.TaskID != taskID {
			continue
		}
		if len(keys) > 0 {
			if _, ok := keys[ref.Key]; !ok && !hasWorkingPrefix(ref.Key, prefixes) {
				continue
			}
		} else if len(prefixes) > 0 && !hasWorkingPrefix(ref.Key, prefixes) {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func hasWorkingPrefix(key string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// Merge merges working data from another envelope into this one.
// Source envelope data takes precedence on conflicts.
func (e *Envelope) Merge(other *Envelope) {
	if e == nil || other == nil {
		return
	}
	otherWorkingData := other.WorkingDataSnapshot()
	otherRefs := other.ReferencesSnapshot()
	if len(otherWorkingData) == 0 && len(otherRefs.WorkingMemory) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
		e.WorkingData = make(map[string]any)
	}
	for k, v := range otherWorkingData {
		e.WorkingData[k] = v
	}
	// Merge working memory references
	for _, ref := range otherRefs.WorkingMemory {
		found := false
		for i, existingRef := range e.References.WorkingMemory {
			if existingRef.TaskID == ref.TaskID && existingRef.Key == ref.Key {
				e.References.WorkingMemory[i] = ref
				found = true
				break
			}
		}
		if !found {
			e.References.WorkingMemory = append(e.References.WorkingMemory, ref)
		}
	}
}

// SetHandleScoped stores a value with a scope identifier.
func (e *Envelope) SetHandleScoped(key string, value any, scope string) {
	if e == nil {
		return
	}
	scopedKey := fmt.Sprintf("%s:%s", scope, key)
	e.SetWorkingValue(scopedKey, value, MemoryClassTask)
}

// GetHandle retrieves a scoped value.
func (e *Envelope) GetHandle(key string) (any, bool) {
	if e == nil {
		return nil, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.WorkingData == nil {
		return nil, false
	}
	// Try exact match first
	if val, ok := e.WorkingData[key]; ok {
		return val, ok
	}
	// Try scoped keys (find the most recent scope)
	for i := len(e.References.WorkingMemory) - 1; i >= 0; i-- {
		ref := e.References.WorkingMemory[i]
		if ref.Key == key && ref.TaskID == e.TaskID {
			if val, ok := e.WorkingData[key]; ok {
				return val, ok
			}
		}
	}
	return nil, false
}

// SetExecutionPhase sets the current execution phase.
func (e *Envelope) SetExecutionPhase(phase string) {
	if e == nil {
		return
	}
	e.SetWorkingValue("_execution_phase", phase, MemoryClassTask)
}

// GetExecutionPhase returns the current execution phase.
func (e *Envelope) GetExecutionPhase() string {
	if e == nil {
		return ""
	}
	val, _ := e.GetWorkingValue("_execution_phase")
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// AddInteraction adds an interaction record to the envelope.
func (e *Envelope) AddInteraction(interaction map[string]any) {
	if e == nil {
		return
	}
	key := "_interactions"
	var interactions []map[string]any
	if val, ok := e.GetWorkingValue(key); ok {
		if arr, ok := val.([]map[string]any); ok {
			interactions = arr
		}
	}
	interactions = append(interactions, interaction)
	e.SetWorkingValue(key, interactions, MemoryClassTask)
}

// GetInteractions returns all interactions recorded in the envelope.
func (e *Envelope) GetInteractions() []map[string]any {
	if e == nil {
		return nil
	}
	val, _ := e.GetWorkingValue("_interactions")
	if arr, ok := val.([]map[string]any); ok {
		return arr
	}
	return nil
}

// StringSliceFromContext extracts a string slice from working memory.
func (e *Envelope) StringSliceFromContext(key string) []string {
	if e == nil {
		return nil
	}
	val, _ := e.GetWorkingValue(key)
	if arr, ok := val.([]string); ok {
		return arr
	}
	if arr, ok := val.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// String returns a summary of the envelope for logging.
func (e *Envelope) String() string {
	if e == nil {
		return "<nil envelope>"
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return fmt.Sprintf("Envelope{TaskID:%s NodeID:%s Working:%d Streamed:%d Retrieval:%d}",
		e.TaskID, e.NodeID, len(e.WorkingData),
		len(e.References.StreamedContext), len(e.References.Retrieval))
}
