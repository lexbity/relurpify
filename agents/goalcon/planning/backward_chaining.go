package planning

import (
	"codeburg.org/lexbit/relurpify/agents/goalcon/audit"
	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// PlanningResult captures the solver output.
type PlanningResult struct {
	Plan        *core.Plan
	Depth       int
	Unsatisfied []types.Predicate
}

// Solver performs deterministic backward chaining.
type Solver struct {
	Operators       *types.OperatorRegistry
	MaxDepth        int
	Recorder        *audit.MetricsRecorder // Optional metrics recorder for quality-based operator ranking
	FailedOperators map[string]int         // Phase 6: Track failed operators and their failure counts
}

// Solve resolves the goal against the world state.
func (s *Solver) Solve(goal types.GoalCondition, ws *types.WorldState) PlanningResult {
	if ws == nil {
		ws = types.NewWorldState()
	}
	maxDepth := s.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 10
	}
	visited := make(map[types.Predicate]bool)
	ordered := make([]*types.Operator, 0)
	unsatSet := make(map[types.Predicate]bool)
	maxSeenDepth := 0

	var resolve func(pred types.Predicate, depth int) bool
	resolve = func(pred types.Predicate, depth int) bool {
		if ws.IsSatisfied(pred) {
			return true
		}
		if visited[pred] {
			unsatSet[pred] = true
			return false
		}
		if depth >= maxDepth {
			unsatSet[pred] = true
			return false
		}
		if depth > maxSeenDepth {
			maxSeenDepth = depth
		}
		visited[pred] = true
		defer delete(visited, pred)

		candidates := s.Operators.OperatorsSatisfying(pred)
		if len(candidates) == 0 {
			unsatSet[pred] = true
			return false
		}

		// Phase 6: Skip failed operators during re-planning
		candidates = s.filterFailedOperators(candidates)
		if len(candidates) == 0 {
			unsatSet[pred] = true
			return false
		}

		// Sort candidates by estimated quality if metrics available
		if s.Recorder != nil {
			candidates = s.sortCandidatesByQuality(candidates)
		}

		for _, op := range candidates {
			allSatisfied := true
			for _, pre := range op.Preconditions {
				if !resolve(pre, depth+1) {
					allSatisfied = false
					break
				}
			}
			if !allSatisfied {
				continue
			}
			if !containsOperator(ordered, op) {
				ordered = append(ordered, op)
			}
			for _, effect := range op.Effects {
				ws.Satisfy(effect)
			}
			return true
		}
		unsatSet[pred] = true
		return false
	}

	for _, pred := range goal.Predicates {
		resolve(pred, 0)
	}

	unsatisfied := make([]types.Predicate, 0, len(unsatSet))
	for pred := range unsatSet {
		if !ws.IsSatisfied(pred) {
			unsatisfied = append(unsatisfied, pred)
		}
	}

	return PlanningResult{
		Plan:        types.BuildPlan(goal.Description, ordered),
		Depth:       maxSeenDepth,
		Unsatisfied: unsatisfied,
	}
}

func containsOperator(ops []*types.Operator, candidate *types.Operator) bool {
	for _, op := range ops {
		if op == candidate {
			return true
		}
	}
	return false
}

// filterFailedOperators removes operators that failed in previous planning attempts (Phase 6).
func (s *Solver) filterFailedOperators(candidates []*types.Operator) []*types.Operator {
	if s == nil || len(s.FailedOperators) == 0 {
		return candidates
	}

	result := make([]*types.Operator, 0, len(candidates))
	for _, op := range candidates {
		// Skip operators that failed in previous attempt
		if _, isFailed := s.FailedOperators[op.Name]; !isFailed {
			result = append(result, op)
		}
	}

	return result
}

// RecordOperatorFailure marks an operator as failed in this planning attempt.
func (s *Solver) RecordOperatorFailure(opName string) {
	if s == nil {
		return
	}

	if s.FailedOperators == nil {
		s.FailedOperators = make(map[string]int)
	}

	s.FailedOperators[opName]++
}

// sortCandidatesByQuality sorts operators by their estimated quality score (highest first).
func (s *Solver) sortCandidatesByQuality(candidates []*types.Operator) []*types.Operator {
	if s == nil || s.Recorder == nil || len(candidates) <= 1 {
		return candidates
	}

	// Use bubble sort for simplicity (typically small candidate lists)
	result := make([]*types.Operator, len(candidates))
	copy(result, candidates)

	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			scoreI := s.Recorder.EstimateOperatorQuality(result[i].Name)
			scoreJ := s.Recorder.EstimateOperatorQuality(result[j].Name)
			if scoreJ > scoreI {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}
