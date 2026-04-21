package runtime

import (
	"fmt"
	"reflect"
	"sort"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

// MergeHTNBranches merges isolated HTN branch results into the parent context.
// Only HTN-owned execution metadata and completed-step state are merged.
func MergeHTNBranches(parent *core.Context, branches []graph.BranchExecutionResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	merged := core.NewContext()
	type deltaEntry struct {
		step  string
		value any
	}
	changed := make(map[string]deltaEntry)
	completedSet := make(map[string]struct{})
	for _, stepID := range completedStepsFromContext(parent) {
		if stepID != "" {
			completedSet[stepID] = struct{}{}
		}
	}

	for _, branch := range branches {
		if branch.State == nil {
			return fmt.Errorf("htn branch merge requires isolated state")
		}
		if len(branch.Delta.SideEffects.VariableWrites) > 0 {
			return fmt.Errorf("htn branch merge conflict: step %s changed context variables outside merge policy", branch.Step.ID)
		}
		for key, value := range branch.Delta.SideEffects.KnowledgeWrites {
			if !htnBranchMergeAllowedKnowledgeKey(key) {
				return fmt.Errorf("htn branch merge conflict: step %s changed context knowledge %q outside merge policy", branch.Step.ID, key)
			}
			if existing, ok := changed["knowledge:"+key]; ok {
				if !reflect.DeepEqual(existing.value, value) {
					return fmt.Errorf("htn branch merge conflict on knowledge key %q between steps %s and %s", key, existing.step, branch.Step.ID)
				}
				continue
			}
			changed["knowledge:"+key] = deltaEntry{step: branch.Step.ID, value: value}
			merged.SetKnowledge(key, value)
		}
		if branch.Delta.SideEffects.HistoryChanged || branch.Delta.SideEffects.CompressedChanged || branch.Delta.SideEffects.LogChanged || branch.Delta.SideEffects.PhaseChanged {
			return fmt.Errorf("htn branch merge conflict: step %s changed interaction history outside merge policy", branch.Step.ID)
		}

		for key, value := range branch.Delta.StateWrites {
			switch key {
			case legacyPlanCompletedStepsKey, contextKeyCompletedSteps, contextKeyExecution, contextKeyMetrics, contextKeyState, contextKeyStateError:
				continue
			}
			if !htnBranchMergeAllowedKey(key) {
				return fmt.Errorf("htn branch merge conflict: step %s changed state key %q outside merge policy", branch.Step.ID, key)
			}
			if htnBranchEphemeralKey(key) {
				changed[key] = deltaEntry{step: branch.Step.ID, value: value}
				merged.Set(key, value)
				continue
			}
			if existing, ok := changed[key]; ok {
				if !reflect.DeepEqual(existing.value, value) {
					return fmt.Errorf("htn branch merge conflict on state key %q between steps %s and %s", key, existing.step, branch.Step.ID)
				}
				continue
			}
			changed[key] = deltaEntry{step: branch.Step.ID, value: value}
			merged.Set(key, value)
		}

		for _, stepID := range completedStepsFromContext(branch.State) {
			if stepID != "" {
				completedSet[stepID] = struct{}{}
			}
		}
	}

	parent.Merge(merged)
	completed := orderedCompletedSteps(parent, completedSet)
	parent.Set(legacyPlanCompletedStepsKey, completed)
	parent.Set(contextKeyCompletedSteps, completed)
	execution := loadExecutionState(parent)
	execution.CompletedSteps = append([]string(nil), completed...)
	publishExecutionState(parent, execution)
	return nil
}

func htnBranchMergeAllowedKey(key string) bool {
	switch key {
	case contextKeyLastDispatch,
		contextKeyLastRecoveryNotes,
		contextKeyLastRecoveryDiag,
		contextKeyLastFailureStep,
		contextKeyLastFailureError,
		"htn.current_step",
		"htn.current_step_id",
		"htn.current_operator_executor",
		"htn.current_operator_task_type":
		return true
	default:
		return false
	}
}

func htnBranchEphemeralKey(key string) bool {
	switch key {
	case contextKeyLastDispatch,
		contextKeyLastRecoveryNotes,
		contextKeyLastRecoveryDiag,
		contextKeyLastFailureStep,
		contextKeyLastFailureError,
		"htn.current_step",
		"htn.current_step_id",
		"htn.current_operator_executor",
		"htn.current_operator_task_type":
		return true
	default:
		return false
	}
}

func htnBranchMergeAllowedKnowledgeKey(key string) bool {
	switch key {
	case contextKnowledgeSummary:
		return true
	default:
		return false
	}
}

func orderedCompletedSteps(parent *core.Context, completedSet map[string]struct{}) []string {
	if len(completedSet) == 0 {
		return nil
	}
	if state, _, err := LoadStateFromContext(parent); err == nil && state != nil && state.Plan != nil {
		ordered := make([]string, 0, len(completedSet))
		for _, step := range state.Plan.Steps {
			if _, ok := completedSet[step.ID]; ok {
				ordered = append(ordered, step.ID)
			}
		}
		if len(ordered) > 0 {
			return ordered
		}
	}
	ordered := make([]string, 0, len(completedSet))
	for stepID := range completedSet {
		ordered = append(ordered, stepID)
	}
	sort.Strings(ordered)
	return ordered
}
