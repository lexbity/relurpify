package blackboard

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
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
	State      *core.Context
	Delta      core.BranchContextDelta
}

// MergeBlackboardBranches merges isolated branch deltas into the parent
// context, rejecting conflicting writes or non-state mutations.
func MergeBlackboardBranches(parent *core.Context, branches []BlackboardBranchResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	set := core.NewBranchDeltaSet(len(branches))
	for _, branch := range branches {
		if branch.State == nil {
			return fmt.Errorf("blackboard branch merge requires isolated state")
		}
		label := branch.SourceName
		if label == "" {
			label = "unknown"
		}
		set.Add(label, branch.Delta)
	}
	if err := set.ApplyTo(parent); err != nil {
		return fmt.Errorf("blackboard branch merge conflict: %w", err)
	}
	if bb := LoadFromContext(parent, parent.GetString("task.instruction")); bb != nil {
		parent.Set(contextKeyRuntimeActive, bb)
	}
	return nil
}
