package blackboard

import (
	"fmt"
	"reflect"

	"github.com/lexcodex/relurpify/framework/core"
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
	Delta      core.DirtyContextDelta
}

// MergeBlackboardBranches merges isolated branch deltas into the parent
// context, rejecting conflicting writes or non-state mutations.
func MergeBlackboardBranches(parent *core.Context, branches []BlackboardBranchResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	merged := core.NewContext()
	type deltaEntry struct {
		source string
		value  any
	}
	changed := make(map[string]deltaEntry)
	for _, branch := range branches {
		if branch.State == nil {
			return fmt.Errorf("blackboard branch merge requires isolated state")
		}
		label := branch.SourceName
		if label == "" {
			label = "unknown"
		}
		if len(branch.Delta.VariableValues) > 0 {
			return fmt.Errorf("blackboard branch merge conflict: source %s changed context variables outside merge policy", label)
		}
		if len(branch.Delta.KnowledgeValues) > 0 {
			return fmt.Errorf("blackboard branch merge conflict: source %s changed context knowledge outside merge policy", label)
		}
		if branch.Delta.HistoryChanged || branch.Delta.CompressedChanged || branch.Delta.LogChanged || branch.Delta.PhaseChanged {
			return fmt.Errorf("blackboard branch merge conflict: source %s changed interaction history outside merge policy", label)
		}
		for key, value := range branch.Delta.StateValues {
			if existing, ok := changed[key]; ok {
				if !reflect.DeepEqual(existing.value, value) {
					return fmt.Errorf("blackboard branch merge conflict on state key %q between sources %s and %s", key, existing.source, label)
				}
				continue
			}
			changed[key] = deltaEntry{source: label, value: value}
			merged.Set(key, value)
		}
	}
	parent.Merge(merged)
	if bb := LoadFromContext(parent, parent.GetString("task.instruction")); bb != nil {
		parent.Set(contextKeyRuntimeActive, bb)
	}
	return nil
}
