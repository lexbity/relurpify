package analysis

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
)

// DecompositionStrategy defines how to split a goal into sub-goals.
type DecompositionStrategy string

const (
	DecompositionStrategyPredicates   DecompositionStrategy = "by_predicates" // One sub-goal per predicate
	DecompositionStrategySequential   DecompositionStrategy = "sequential"    // Linear dependency chain
	DecompositionStrategyHierarchical DecompositionStrategy = "hierarchical"  // Parent-child relationships
	DecompositionStrategyDomainBased  DecompositionStrategy = "domain_based"  // Group by semantic domain
)

// SubGoal represents a decomposed goal.
type SubGoal struct {
	ID           string            // Unique identifier
	Description  string            // What to accomplish
	Predicates   []types.Predicate // Target predicates
	Importance   float32           // 0.0-1.0 relative importance
	Dependencies []string          // Sub-goal IDs that must complete first
	DependsOn    []string          // Which parent goals this is part of
	Timestamp    time.Time
}

// GoalDecomposition represents the result of decomposing a goal.
type GoalDecomposition struct {
	OriginalGoal   types.GoalCondition
	SubGoals       []*SubGoal
	Strategy       DecompositionStrategy
	Rationale      string          // Why goal was decomposed this way
	AmbiguityInfo  *AmbiguityScore // Ambiguities that triggered decomposition
	ExecutionOrder []string        // IDs in recommended execution order
	Timestamp      time.Time
}

// GoalDecomposer breaks down goals into sub-goals.
type GoalDecomposer struct {
	strategies    map[DecompositionStrategy]DecompositionFunc
	maxSubGoals   int // Max sub-goals to create (default 8)
	minPredicates int // Min predicates to consider for decomposition
	mu            sync.RWMutex
}

// DecompositionFunc is a function that decomposes a goal.
type DecompositionFunc func(goal types.GoalCondition, ambiguity *AmbiguityScore) []*SubGoal

// NewGoalDecomposer creates a decomposer with default strategies.
func NewGoalDecomposer() *GoalDecomposer {
	gd := &GoalDecomposer{
		strategies:    make(map[DecompositionStrategy]DecompositionFunc),
		maxSubGoals:   8,
		minPredicates: 2,
	}

	// Register default strategies
	gd.RegisterStrategy(DecompositionStrategyPredicates, gd.decomposeByPredicates)
	gd.RegisterStrategy(DecompositionStrategySequential, gd.decomposeSequential)
	gd.RegisterStrategy(DecompositionStrategyHierarchical, gd.decomposeHierarchical)

	return gd
}

// RegisterStrategy registers a custom decomposition strategy.
func (gd *GoalDecomposer) RegisterStrategy(name DecompositionStrategy, fn DecompositionFunc) {
	if gd == nil || fn == nil {
		return
	}

	gd.mu.Lock()
	defer gd.mu.Unlock()

	gd.strategies[name] = fn
}

// SetMaxSubGoals sets the maximum number of sub-goals to create.
func (gd *GoalDecomposer) SetMaxSubGoals(max int) {
	if gd != nil && max > 0 {
		gd.maxSubGoals = max
	}
}

// Decompose breaks a goal into sub-goals using the specified strategy.
func (gd *GoalDecomposer) Decompose(
	goal types.GoalCondition,
	strategy DecompositionStrategy,
	ambiguity *AmbiguityScore,
) *GoalDecomposition {
	if gd == nil {
		return nil
	}

	gd.mu.RLock()
	strategyFn, exists := gd.strategies[strategy]
	gd.mu.RUnlock()

	if !exists {
		// Fall back to predicate decomposition
		strategyFn = gd.decomposeByPredicates
	}

	// Decompose using strategy
	subGoals := strategyFn(goal, ambiguity)
	if len(subGoals) == 0 {
		return nil
	}

	// Cap at maxSubGoals
	if len(subGoals) > gd.maxSubGoals {
		subGoals = subGoals[:gd.maxSubGoals]
	}

	// Compute execution order
	executionOrder := gd.computeExecutionOrder(subGoals)

	decomp := &GoalDecomposition{
		OriginalGoal:   goal,
		SubGoals:       subGoals,
		Strategy:       strategy,
		AmbiguityInfo:  ambiguity,
		ExecutionOrder: executionOrder,
		Timestamp:      time.Now().UTC(),
		Rationale:      gd.generateRationale(strategy, len(subGoals)),
	}

	return decomp
}

// decomposeByPredicates creates one sub-goal per predicate.
func (gd *GoalDecomposer) decomposeByPredicates(goal types.GoalCondition, ambiguity *AmbiguityScore) []*SubGoal {
	if len(goal.Predicates) < gd.minPredicates {
		return nil // Not complex enough
	}

	var subGoals []*SubGoal
	importance := 1.0 / float32(len(goal.Predicates))

	for i, pred := range goal.Predicates {
		subGoal := &SubGoal{
			ID:          fmt.Sprintf("sg_%d_pred_%d", time.Now().UnixNano(), i),
			Description: fmt.Sprintf("Satisfy predicate: %s", pred),
			Predicates:  []types.Predicate{pred},
			Importance:  importance,
			Timestamp:   time.Now().UTC(),
		}
		subGoals = append(subGoals, subGoal)
	}

	return subGoals
}

// decomposeSequential creates a chain of dependent sub-goals.
func (gd *GoalDecomposer) decomposeSequential(goal types.GoalCondition, ambiguity *AmbiguityScore) []*SubGoal {
	if len(goal.Predicates) < gd.minPredicates {
		return nil
	}

	var subGoals []*SubGoal
	var prevID string

	for i, pred := range goal.Predicates {
		subGoalID := fmt.Sprintf("sg_%d_seq_%d", time.Now().UnixNano(), i)
		subGoal := &SubGoal{
			ID:          subGoalID,
			Description: fmt.Sprintf("Step %d: %s", i+1, pred),
			Predicates:  []types.Predicate{pred},
			Importance:  float32(len(goal.Predicates)-i) / float32(len(goal.Predicates)),
			Timestamp:   time.Now().UTC(),
		}

		// Create dependency chain
		if prevID != "" {
			subGoal.Dependencies = []string{prevID}
		}

		subGoals = append(subGoals, subGoal)
		prevID = subGoalID
	}

	return subGoals
}

// decomposeHierarchical creates hierarchical levels of goals.
func (gd *GoalDecomposer) decomposeHierarchical(goal types.GoalCondition, ambiguity *AmbiguityScore) []*SubGoal {
	if len(goal.Predicates) < gd.minPredicates {
		return nil
	}

	var subGoals []*SubGoal

	// Level 1: High-level objectives
	midpoint := (len(goal.Predicates) + 1) / 2
	level1ID := fmt.Sprintf("sg_%d_l1", time.Now().UnixNano())
	level1 := &SubGoal{
		ID:          level1ID,
		Description: fmt.Sprintf("Objective 1: Handle %d predicates", midpoint),
		Predicates:  goal.Predicates[:midpoint],
		Importance:  0.6,
		Timestamp:   time.Now().UTC(),
	}
	subGoals = append(subGoals, level1)

	// Level 1: Second objective
	level2ID := fmt.Sprintf("sg_%d_l1b", time.Now().UnixNano())
	level2 := &SubGoal{
		ID:          level2ID,
		Description: fmt.Sprintf("Objective 2: Handle %d predicates", len(goal.Predicates)-midpoint),
		Predicates:  goal.Predicates[midpoint:],
		Importance:  0.4,
		Timestamp:   time.Now().UTC(),
	}
	subGoals = append(subGoals, level2)

	return subGoals
}

// computeExecutionOrder determines a valid execution order based on dependencies.
func (gd *GoalDecomposer) computeExecutionOrder(subGoals []*SubGoal) []string {
	if len(subGoals) == 0 {
		return nil
	}

	// Build dependency map
	depMap := make(map[string][]string)
	for _, sg := range subGoals {
		depMap[sg.ID] = sg.Dependencies
	}

	// Topological sort
	var order []string
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(id string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		if visiting[id] {
			return // Cycle detected, skip
		}

		visiting[id] = true
		for _, dep := range depMap[id] {
			visit(dep)
		}
		visiting[id] = false
		visited[id] = true
		order = append(order, id)
	}

	for _, sg := range subGoals {
		visit(sg.ID)
	}

	return order
}

// generateRationale creates a human-readable explanation for the decomposition.
func (gd *GoalDecomposer) generateRationale(strategy DecompositionStrategy, numSubGoals int) string {
	switch strategy {
	case DecompositionStrategyPredicates:
		return fmt.Sprintf("Goal decomposed into %d independent sub-goals (one per predicate) for parallel planning", numSubGoals)
	case DecompositionStrategySequential:
		return fmt.Sprintf("Goal decomposed into %d sequential steps with strict ordering", numSubGoals)
	case DecompositionStrategyHierarchical:
		return fmt.Sprintf("Goal decomposed into %d hierarchical objectives", numSubGoals)
	default:
		return fmt.Sprintf("Goal decomposed into %d sub-goals", numSubGoals)
	}
}

// ShouldDecompose determines if a goal should be split into sub-goals.
func (gd *GoalDecomposer) ShouldDecompose(goal types.GoalCondition, ambiguity *AmbiguityScore) bool {
	if gd == nil || len(goal.Predicates) < gd.minPredicates {
		return false
	}

	// Decompose if goal is complex or ambiguous
	if len(goal.Predicates) >= 4 {
		return true
	}

	if ambiguity != nil && ambiguity.OverallScore > 0.5 {
		return true
	}

	return false
}

// ChooseBestStrategy selects the most appropriate decomposition strategy.
func (gd *GoalDecomposer) ChooseBestStrategy(goal types.GoalCondition, ambiguity *AmbiguityScore) DecompositionStrategy {
	if gd == nil {
		return DecompositionStrategyPredicates
	}

	// High ambiguity → hierarchical (handles uncertainty better)
	if ambiguity != nil && ambiguity.OverallScore > 0.7 {
		return DecompositionStrategyHierarchical
	}

	// Many predicates with dependencies → sequential
	if len(goal.Predicates) > 4 {
		return DecompositionStrategySequential
	}

	// Default to predicate-based
	return DecompositionStrategyPredicates
}

// FormatDecompositionReport generates a human-readable decomposition report.
func (gd *GoalDecomposer) FormatDecompositionReport(decomp *GoalDecomposition) string {
	if decomp == nil {
		return "No decomposition available"
	}

	var sb strings.Builder
	sb.WriteString("Goal Decomposition Report\n")
	sb.WriteString("=========================\n\n")
	sb.WriteString(fmt.Sprintf("Original Goal: %s\n", decomp.OriginalGoal.Description))
	sb.WriteString(fmt.Sprintf("Strategy: %s\n", decomp.Strategy))
	sb.WriteString(fmt.Sprintf("Number of Sub-Goals: %d\n", len(decomp.SubGoals)))
	sb.WriteString(fmt.Sprintf("Rationale: %s\n\n", decomp.Rationale))

	sb.WriteString("Sub-Goals (in execution order):\n")
	for i, sgID := range decomp.ExecutionOrder {
		var sg *SubGoal
		for _, s := range decomp.SubGoals {
			if s.ID == sgID {
				sg = s
				break
			}
		}
		if sg == nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, sg.Description))
		sb.WriteString(fmt.Sprintf("     Importance: %.0f%%\n", sg.Importance*100))
		if len(sg.Predicates) > 0 {
			sb.WriteString(fmt.Sprintf("     Predicates: %v\n", sg.Predicates))
		}
		if len(sg.Dependencies) > 0 {
			sb.WriteString(fmt.Sprintf("     Depends on: %v\n", len(sg.Dependencies)))
		}
	}

	return sb.String()
}
