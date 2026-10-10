package plan

import (
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// BranchExecutorProvider allows plan execution to allocate an isolated runtime
// executor per branch before any parallel step execution is attempted.
type BranchExecutorProvider interface {
	BranchExecutor() (StepExecutor, error)
}

// BranchExecutionResult captures the isolated context and step metadata for one
// completed parallel branch.
type BranchExecutionResult struct {
	Step  PlanStep
	State *contextdata.Envelope
	Delta contextdata.BranchDelta
}

func mergePlanBranchEnvelopes(parent *contextdata.Envelope, branches []BranchExecutionResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	// branch.State is populated in ready-step declaration order, so the slice
	// index is the authoritative merge order. Deltas were computed against the
	// quiesced parent before this call.
	envs := make([]*contextdata.Envelope, 0, len(branches))
	units := make([]contextdata.BranchMergeUnit, 0, len(branches))
	for i, branch := range branches {
		if branch.State == nil {
			continue
		}
		envs = append(envs, branch.State)
		units = append(units, contextdata.BranchMergeUnit{
			Index: i,
			ID:    branch.Step.ID,
			Delta: branch.Delta,
			Env:   branch.State,
		})
	}
	if len(units) == 0 {
		return nil
	}
	if err := contextdata.ValidateBranchMerge(envs); err != nil {
		return err
	}
	if _, err := parent.ApplyBranchMerges(units); err != nil {
		return err
	}
	return nil
}
