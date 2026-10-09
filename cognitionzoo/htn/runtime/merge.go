package runtime

import (
	"fmt"
	"sort"

	"codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// MergeHTNBranches merges isolated HTN branch results into the parent envelope.
// Only HTN-owned execution metadata and completed-step state are merged.
func MergeHTNBranches(parent *contextdata.Envelope, branches []plan.BranchExecutionResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	envs := make([]*contextdata.Envelope, 0, len(branches))
	units := make([]contextdata.BranchMergeUnit, 0, len(branches))
	completedSet := make(map[string]struct{})
	for _, stepID := range completedStepsFromEnvelope(parent) {
		if stepID != "" {
			completedSet[stepID] = struct{}{}
		}
	}

	for i, branch := range branches {
		if branch.State == nil {
			return fmt.Errorf("htn branch merge requires isolated state")
		}
		envs = append(envs, branch.State)
		units = append(units, contextdata.BranchMergeUnit{
			Index: i,
			ID:    branch.Step.ID,
			Delta: branch.Delta,
			Env:   branch.State,
		})

		for _, stepID := range completedStepsFromEnvelope(branch.State) {
			if stepID != "" {
				completedSet[stepID] = struct{}{}
			}
		}
	}

	if err := contextdata.ValidateBranchMerge(envs); err != nil {
		return err
	}
	if _, err := parent.ApplyBranchMerges(units); err != nil {
		return err
	}
	completed := orderedCompletedSteps(parent, completedSet)
	parent.SetWorkingValueWithClass(legacyPlanCompletedStepsKey, completed, contextdata.MemoryClassTask)
	parent.SetWorkingValueWithClass(contextKeyCompletedSteps, completed, contextdata.MemoryClassTask)
	execution := loadExecutionState(parent)
	execution.CompletedSteps = append([]string(nil), completed...)
	publishExecutionState(parent, execution)
	return nil
}

func orderedCompletedSteps(parent *contextdata.Envelope, completedSet map[string]struct{}) []string {
	if len(completedSet) == 0 {
		return nil
	}
	if state, _, err := LoadStateFromEnvelope(parent); err == nil && state != nil && state.Plan != nil {
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
