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
		return eligible[i].Priority() > eligible[j].Priority()
	})
	return eligible
}
