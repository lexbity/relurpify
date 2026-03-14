package blackboard

import (
	"context"
	"fmt"
	"sort"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// Controller drives the blackboard control loop. Each cycle it evaluates all
// registered KS activation conditions, selects the highest-priority eligible
// KS, and invokes it. The loop terminates when the goal is satisfied, no KS
// can activate, or MaxCycles is reached.
type Controller struct {
	Sources   []KnowledgeSource
	MaxCycles int // 0 means use defaultMaxCycles
}

const defaultMaxCycles = 20

// Run executes the blackboard loop until a terminal condition is met.
// It returns the final blackboard state and an error if the loop gets stuck.
func (c *Controller) Run(ctx context.Context, bb *Blackboard, tools *capability.Registry, model core.LanguageModel) error {
	maxCycles := c.MaxCycles
	if maxCycles <= 0 {
		maxCycles = defaultMaxCycles
	}

	for cycle := 0; cycle < maxCycles; cycle++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check terminal condition.
		if bb.IsGoalSatisfied() {
			return nil
		}

		// Evaluate all KS activation conditions.
		eligible := c.eligibleSources(bb)
		if len(eligible) == 0 {
			if bb.IsGoalSatisfied() {
				return nil
			}
			return fmt.Errorf("blackboard: no knowledge source can activate (cycle %d) — goal not satisfied", cycle)
		}

		// Select and execute the highest-priority KS.
		selected := eligible[0]
		if err := selected.Execute(ctx, bb, tools, model); err != nil {
			return fmt.Errorf("blackboard: KS %q failed: %w", selected.Name(), err)
		}
	}

	if bb.IsGoalSatisfied() {
		return nil
	}
	return fmt.Errorf("blackboard: reached cycle limit (%d) without satisfying goal", maxCycles)
}

// Snapshot returns controller metadata derived from the current blackboard
// state. The current prototype controller does not persist cycle counters, so
// callers must provide the last cycle they want exposed.
func (c *Controller) Snapshot(bb *Blackboard, cycle int, termination, lastSource string) ControllerState {
	maxCycles := c.MaxCycles
	if maxCycles <= 0 {
		maxCycles = defaultMaxCycles
	}
	return ControllerState{
		Cycle:         cycle,
		MaxCycles:     maxCycles,
		Termination:   termination,
		LastSource:    lastSource,
		GoalSatisfied: bb != nil && bb.IsGoalSatisfied(),
	}
}

// ExecutionMode describes how the controller schedules eligible knowledge
// sources. Phase 11 keeps the runtime explicitly single-fire serial.
func (c *Controller) ExecutionMode() ExecutionMode {
	return ExecutionModeSingleFireSerial
}

// MergePolicy describes how future isolated KS branch results must be merged.
func (c *Controller) MergePolicy() BranchMergePolicy {
	return BranchMergePolicyRejectConflicts
}

// SelectionPolicy documents the current deterministic source selection rule.
func (c *Controller) SelectionPolicy() string {
	return "highest_priority_then_name"
}

// eligibleSources returns all KS whose CanActivate returns true, sorted
// descending by priority.
func (c *Controller) eligibleSources(bb *Blackboard) []KnowledgeSource {
	var eligible []KnowledgeSource
	for _, ks := range c.Sources {
		if ks.CanActivate(bb) {
			eligible = append(eligible, ks)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		left := ResolveKnowledgeSource(eligible[i])
		right := ResolveKnowledgeSource(eligible[j])
		if left.Spec.Priority == right.Spec.Priority {
			return left.Spec.Name < right.Spec.Name
		}
		return left.Spec.Priority > right.Spec.Priority
	})
	return eligible
}
