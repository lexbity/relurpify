package contextdata

import (
	"strings"
	"time"
)

// CheckpointRequest records a node-originated checkpoint request.
type CheckpointRequest struct {
	RequestedBy        string
	Reason             string
	Priority           int
	EvictWorkingMemory bool
	RequestedAt        time.Time
}

// RequestCheckpoint sets a checkpoint request on the envelope.
func (e *Envelope) RequestCheckpoint(reason string, priority int, evictMemory bool) {
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
func (e *Envelope) ClearCheckpointRequest() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.CheckpointRequest = nil
}

// AddCheckpointReference adds a checkpoint reference to the envelope.
func (e *Envelope) AddCheckpointReference(ref CheckpointReference) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.References.Checkpoints = append(e.References.Checkpoints, cloneCheckpointReference(ref))
}

// AssemblyMetadataSnapshot returns a point-in-time copy of the assembly metadata.
func (e *Envelope) AssemblyMetadataSnapshot() AssemblyMeta {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.AssemblyMetadata
}

// SetAssemblyMetadata replaces the assembly metadata.
func (e *Envelope) SetAssemblyMetadata(meta AssemblyMeta) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AssemblyMetadata = meta
}

// Clone returns a deep copy of the envelope.
func (e *Envelope) Clone() *Envelope {
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
		CheckpointRequest: nil,
		AssemblyMetadata:  assemblyMetadata,
		createdAt:         createdAt,
	}
	clone.References = refs
	return clone
}

// HandoffPolicy controls which parts of an envelope survive a filtered handoff.
type HandoffPolicy struct {
	PreserveWorkingMemory    bool
	WorkingKeys              []string
	WorkingPrefixes          []string
	PreserveStreamedContext  bool
	PreserveRetrieval        bool
	PreserveCheckpoints      bool
	PreserveAssemblyMetadata bool
	PreserveNodeID           bool
}

// DefaultHandoffPolicy preserves the data typically needed when handing work
// from one agent boundary to another.
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
// boundary handoff.
func (e *Envelope) HandoffClone() *Envelope {
	return e.Clone()
}

// HandoffSnapshot returns a filtered envelope using the supplied policy.
func (e *Envelope) HandoffSnapshot(policy HandoffPolicy) *Envelope {
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
		snapshot.References.Checkpoints = make([]CheckpointReference, len(refs.Checkpoints))
		for i, ref := range refs.Checkpoints {
			snapshot.References.Checkpoints[i] = cloneCheckpointReference(ref)
		}
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
	if other == nil {
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
