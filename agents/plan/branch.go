package plan

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// BranchExecutorProvider allows plan execution to allocate an isolated runtime
// executor per branch before any parallel step execution is attempted.
type BranchExecutorProvider interface {
	BranchExecutor() (WorkflowExecutor, error)
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
	// Collect branch envelopes for merge.
	branchEnvelopes := make([]*contextdata.Envelope, 0, len(branches))
	for _, branch := range branches {
		if branch.State != nil {
			branchEnvelopes = append(branchEnvelopes, branch.State)
		}
	}
	if len(branchEnvelopes) == 0 {
		return nil
	}
	// Validate before merge.
	if err := contextdata.ValidateBranchMerge(branchEnvelopes); err != nil {
		return err
	}
	// Merge envelopes.
	merged, err := contextdata.MergeBranchEnvelopes(parent.TaskID, parent.SessionID, branchEnvelopes)
	if err != nil {
		return err
	}
	// Update parent with merged state.
	parent.WorkingData = merged.WorkingData
	parent.References = merged.References
	return nil
}
