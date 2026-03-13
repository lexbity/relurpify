package goalcon

import "github.com/lexcodex/relurpify/framework/core"

// PlanningResult captures the solver output.
type PlanningResult struct {
	Plan        *core.Plan
	Depth       int
	Unsatisfied []Predicate
}

// Solver performs deterministic backward chaining.
type Solver struct {
	Operators *OperatorRegistry
	MaxDepth  int
}

// Solve resolves the goal against the world state.
func (s *Solver) Solve(goal GoalCondition, ws *WorldState) PlanningResult {
	if ws == nil {
		ws = NewWorldState()
	}
	maxDepth := s.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 10
	}
	visited := make(map[Predicate]bool)
	ordered := make([]*Operator, 0)
	unsatSet := make(map[Predicate]bool)
	maxSeenDepth := 0

	var resolve func(pred Predicate, depth int) bool
	resolve = func(pred Predicate, depth int) bool {
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

	unsatisfied := make([]Predicate, 0, len(unsatSet))
	for pred := range unsatSet {
		if !ws.IsSatisfied(pred) {
			unsatisfied = append(unsatisfied, pred)
		}
	}

	return PlanningResult{
		Plan:        buildPlan(goal.Description, ordered),
		Depth:       maxSeenDepth,
		Unsatisfied: unsatisfied,
	}
}

func containsOperator(ops []*Operator, candidate *Operator) bool {
	for _, op := range ops {
		if op == candidate {
			return true
		}
	}
	return false
}
