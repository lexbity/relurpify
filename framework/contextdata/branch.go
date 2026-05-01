package contextdata

import (
	"fmt"
	"sort"
	"time"
)

// BranchState captures the execution state for a branch.
// Branches are created during parallel execution paths in the graph.
type BranchState struct {
	// Envelope is the branch's execution context.
	Envelope *Envelope

	// BranchID uniquely identifies this branch.
	BranchID string

	// ParentBranchID identifies the branch this was cloned from, if any.
	ParentBranchID string

	// CreatedAt records when the branch was created.
	CreatedAt time.Time

	// Delta tracks changes made on this branch relative to its parent.
	Delta BranchDelta
}

// BranchDelta tracks mutations made on a branch.
type BranchDelta struct {
	// WorkingMemoryAdded keys added to working memory.
	WorkingMemoryAdded []string

	// WorkingMemoryModified keys modified in working memory.
	WorkingMemoryModified []string

	// WorkingMemoryDeleted keys removed from working memory.
	WorkingMemoryDeleted []string

	// RetrievalPerformed records retrieval operations triggered on this branch.
	RetrievalPerformed []string
}

// CloneEnvelope creates a deep copy of an envelope for branch execution.
// Working memory state and references are copied together.
func CloneEnvelope(env *Envelope, newBranchID string) *Envelope {
	if env == nil {
		return nil
	}

	now := time.Now().UTC()
	workingDataCopy := env.WorkingDataSnapshot()
	refsCopy := env.ReferencesSnapshot()

	return &Envelope{
		TaskID:            env.TaskID,
		SessionID:         env.SessionID,
		NodeID:            env.NodeID,
		References:        refsCopy,
		WorkingData:       workingDataCopy,
		CheckpointRequest: nil, // Branch clones don't inherit checkpoint requests
		AssemblyMetadata:  env.AssemblyMetadataSnapshot(),
		createdAt:         now,
	}
}

// MergeBranchEnvelopes unions multiple branch envelopes into a single envelope.
// References and working memory entries are unioned without duplication.
//
// Merge rules:
//   - Working memory: Union of all keys, last-write-wins on conflict
//   - Streamed context: Deduplicated union of chunk references
//   - Retrieval: Union of all retrieval references
//   - Checkpoints: Union of all checkpoint references
func MergeBranchEnvelopes(taskID, sessionID string, envelopes []*Envelope) (*Envelope, error) {
	if len(envelopes) == 0 {
		return NewEnvelope(taskID, sessionID), nil
	}

	now := time.Now().UTC()
	merged := NewEnvelope(taskID, sessionID)
	merged.AssemblyMetadata.AssembledAt = now

	// Track seen keys for deduplication
	seenWorkingKeys := make(map[string]struct{})
	seenChunkIDs := make(map[ChunkID]struct{})
	seenRetrievalIDs := make(map[string]struct{})
	seenCheckpointIDs := make(map[string]struct{})

	// Collect all working memory keys and their values
	// Use last-write-wins ordering (later envelopes override earlier ones)
	workingMemoryUnion := make(map[string]any)

	for _, env := range envelopes {
		if env == nil {
			continue
		}
		workingData := env.WorkingDataSnapshot()
		refs := env.ReferencesSnapshot()

		// Merge working memory data
		for k, v := range workingData {
			workingMemoryUnion[k] = v
		}

		// Merge streamed context references (deduplicate by chunk ID)
		for _, ref := range refs.StreamedContext {
			if _, seen := seenChunkIDs[ref.ChunkID]; !seen {
				seenChunkIDs[ref.ChunkID] = struct{}{}
				merged.References.StreamedContext = append(
					merged.References.StreamedContext, ref)
			}
		}

		// Merge working memory references
		for _, ref := range refs.WorkingMemory {
			key := ref.TaskID + "/" + ref.Key
			if _, seen := seenWorkingKeys[key]; !seen {
				seenWorkingKeys[key] = struct{}{}
				merged.References.WorkingMemory = append(
					merged.References.WorkingMemory, ref)
			}
		}

		// Merge retrieval references (deduplicate by query ID)
		for _, ref := range refs.Retrieval {
			if _, seen := seenRetrievalIDs[ref.QueryID]; !seen {
				seenRetrievalIDs[ref.QueryID] = struct{}{}
				merged.References.Retrieval = append(
					merged.References.Retrieval, ref)
			}
		}

		// Merge checkpoint references (deduplicate by checkpoint ID)
		for _, ref := range refs.Checkpoints {
			if _, seen := seenCheckpointIDs[ref.CheckpointID]; !seen {
				seenCheckpointIDs[ref.CheckpointID] = struct{}{}
				merged.References.Checkpoints = append(
					merged.References.Checkpoints, ref)
			}
		}
	}

	// Sort streamed context by rank for determinism
	sort.Slice(merged.References.StreamedContext, func(i, j int) bool {
		return merged.References.StreamedContext[i].Rank <
			merged.References.StreamedContext[j].Rank
	})

	// Set the unioned working data
	merged.WorkingData = workingMemoryUnion

	return merged, nil
}

// ComputeBranchDelta calculates the difference between a parent and child envelope.
// This is used to track what changed on a branch.
func ComputeBranchDelta(parent, child *Envelope) BranchDelta {
	if parent == nil || child == nil {
		return BranchDelta{}
	}

	delta := BranchDelta{}
	parentWorkingData := parent.WorkingDataSnapshot()
	childWorkingData := child.WorkingDataSnapshot()
	parentRefs := parent.ReferencesSnapshot()
	childRefs := child.ReferencesSnapshot()

	parentKeys := make(map[string]struct{})
	for k := range parentWorkingData {
		parentKeys[k] = struct{}{}
	}

	childKeys := make(map[string]struct{})
	for k := range childWorkingData {
		childKeys[k] = struct{}{}
		if _, existed := parentKeys[k]; !existed {
			delta.WorkingMemoryAdded = append(delta.WorkingMemoryAdded, k)
		}
		// Note: Modification detection would require value comparison
		// For now, we consider any existing key as potentially modified
		if _, existed := parentKeys[k]; existed {
			// Could add deep equality check here
			delta.WorkingMemoryModified = append(delta.WorkingMemoryModified, k)
		}
	}

	for k := range parentKeys {
		if _, exists := childKeys[k]; !exists {
			delta.WorkingMemoryDeleted = append(delta.WorkingMemoryDeleted, k)
		}
	}

	// Track retrieval operations
	parentRetrievalIDs := make(map[string]struct{})
	for _, ref := range parentRefs.Retrieval {
		parentRetrievalIDs[ref.QueryID] = struct{}{}
	}

	for _, ref := range childRefs.Retrieval {
		if _, existed := parentRetrievalIDs[ref.QueryID]; !existed {
			delta.RetrievalPerformed = append(delta.RetrievalPerformed, ref.QueryID)
		}
	}

	return delta
}

// BranchMergeError is returned when branch merge operations fail.
type BranchMergeError struct {
	Reason  string
	Details string
}

func (e *BranchMergeError) Error() string {
	return fmt.Sprintf("branch merge error: %s (%s)", e.Reason, e.Details)
}

// ValidateBranchMerge checks if branches can be safely merged.
// Returns an error if there are irreconcilable conflicts.
func ValidateBranchMerge(envelopes []*Envelope) error {
	if len(envelopes) < 2 {
		return nil // Nothing to validate
	}

	// Check that all envelopes belong to the same task
	var taskID string
	for i, env := range envelopes {
		if env == nil {
			continue
		}
		if i == 0 {
			taskID = env.TaskID
		} else if env.TaskID != taskID {
			return &BranchMergeError{
				Reason:  "task_mismatch",
				Details: fmt.Sprintf("envelope %d has task %s, expected %s", i, env.TaskID, taskID),
			}
		}
	}

	// Additional validation rules can be added here:
	// - Check for conflicting checkpoint requests
	// - Validate streamed context compatibility
	// - Ensure retrieval references don't have circular dependencies

	return nil
}

// DeduplicateChunkReferences removes duplicate chunk references, keeping
// the one with the best rank (lowest rank number).
func DeduplicateChunkReferences(refs []ChunkReference) []ChunkReference {
	if len(refs) == 0 {
		return nil
	}

	// Group by chunk ID, keep the one with best rank
	bestRefs := make(map[ChunkID]ChunkReference)
	for _, ref := range refs {
		existing, ok := bestRefs[ref.ChunkID]
		if !ok || ref.Rank < existing.Rank {
			bestRefs[ref.ChunkID] = ref
		}
	}

	// Convert back to slice and sort by rank
	result := make([]ChunkReference, 0, len(bestRefs))
	for _, ref := range bestRefs {
		result = append(result, ref)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})

	return result
}
