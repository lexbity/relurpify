package planning_test

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon/planning"
)

func TestSolver_EmptyWorldState(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	registry.Register(planning.Operator{
		Name:    "A",
		Effects: []planning.Predicate{"goal"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		nil,
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.Steps) != 1 {
		t.Fatalf("len(result.Plan.Steps) = %d, want 1", len(result.Plan.Steps))
	}
}

func TestSolver_MaxDepthZeroDefaultsTen(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	registry.Register(planning.Operator{
		Name:    "A",
		Effects: []planning.Predicate{"mid"},
	})
	registry.Register(planning.Operator{
		Name:          "B",
		Preconditions: []planning.Predicate{"mid"},
		Effects:       []planning.Predicate{"goal"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 0}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.Steps) != 2 {
		t.Fatalf("len(result.Plan.Steps) = %d, want 2", len(result.Plan.Steps))
	}
}

func TestSolver_FailedOperatorsTracking(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	registry.Register(planning.Operator{
		Name:    "A",
		Effects: []planning.Predicate{"goal"},
	})

	result := (&planning.Solver{
		Operators:       registry,
		MaxDepth:        5,
		FailedOperators: map[string]int{"A": 3},
	}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		&planning.WorldState{},
	)
	if len(result.Unsatisfied) == 0 {
		t.Fatal("expected unsatisfied predicates when only candidate is marked failed")
	}
}

func TestPlanningResult_NilPlan(t *testing.T) {
	registry := &planning.OperatorRegistry{}

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"impossible"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan to be initialized")
	}
	if len(result.Unsatisfied) == 0 {
		t.Fatal("expected unsatisfied predicates")
	}
}
