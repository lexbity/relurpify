package blackboard

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

type ExecutionMode string

const (
	ExecutionModeSingleFireSerial ExecutionMode = "single_fire_serial"
)

type BranchMergePolicy string

const (
	BranchMergePolicyRejectConflicts BranchMergePolicy = "reject_conflicts"
)

// BlackboardBranchResult captures one isolated branch context for future
// parallel KS execution. The current runtime remains single-fire serial, but
// these semantics define how concurrent KS branches would be merged safely.
type BlackboardBranchResult struct {
	SourceName string
	State      *contextdata.Envelope
}

// MergeBlackboardBranches merges isolated branch states into the parent
// context using envelope-native merge semantics. Conflicting writes are rejected.
func MergeBlackboardBranches(parent *contextdata.Envelope, branches []BlackboardBranchResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	// Track keys written by each branch to detect conflicts
	writtenKeys := make(map[string]string) // key -> branch name
	for _, branch := range branches {
		if branch.State == nil {
			return fmt.Errorf("blackboard branch merge requires isolated state")
		}
		label := branch.SourceName
		if label == "" {
			label = "unknown"
		}
		// Merge working data from branch into parent
		for key, value := range branch.State.WorkingData {
			if existingBranch, exists := writtenKeys[key]; exists {
				return fmt.Errorf("blackboard branch merge conflict: key %q written by both %q and %q", key, existingBranch, label)
			}
			parent.WorkingData[key] = value
			writtenKeys[key] = label
		}
	}
	if bb := LoadFromContext(parent, envelopeGetString(parent, "task.instruction")); bb != nil {
		envelopeSet(parent, contextKeyRuntimeActive, bb)
	}
	return nil
}
