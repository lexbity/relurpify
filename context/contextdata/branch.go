package contextdata

import (
	"fmt"
	"reflect"
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
//
// It is a nil-safe delegate to Envelope.Clone. Callers that need to record
// which branch the clone belongs to carry that identifier alongside the clone
// (for example BranchState.BranchID); it is not part of the envelope.
func CloneEnvelope(env *Envelope) *Envelope {
	if env == nil {
		return nil
	}
	return env.Clone()
}

// ComputeBranchDelta calculates the difference between a parent and child envelope.
// This is used to track what changed on a branch.
//
// Modification detection compares values: a key the branch left byte-identical
// is not a modification. This precision is required by ApplyBranchMerges —
// with an over-approximating "any pre-existing key is modified" rule, an
// untouched branch would re-write a key another branch deleted and resurrect
// it (the Q2 deletion-resurrection defect).
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
	for k, childValue := range childWorkingData {
		childKeys[k] = struct{}{}
		parentValue, existed := parentWorkingData[k]
		if !existed {
			delta.WorkingMemoryAdded = append(delta.WorkingMemoryAdded, k)
			continue
		}
		if !valuesEqual(parentValue, childValue) {
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

// valuesEqual reports whether two working-memory values are deeply equal. It
// backs precise modification detection in ComputeBranchDelta. A value mutated
// in place through a shared pointer compares equal and is therefore not
// re-written, which is correct: the parent observes the same object already.
func valuesEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

// BranchMergeError is returned when branch merge operations fail.
type BranchMergeError struct {
	Reason  string
	Details string
}

func (e *BranchMergeError) Error() string {
	return fmt.Sprintf("branch merge error: %s (%s)", e.Reason, e.Details)
}

// ValidateBranchMerge checks the structural preconditions for merging branch
// envelopes: at least one non-nil envelope and a shared TaskID. It does not
// reject working-memory key conflicts; those are resolved deterministically by
// ApplyBranchMerges in favor of the branch with the higher declaration index
// (the caller records them in MergeStats.Conflicts for observability).
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
