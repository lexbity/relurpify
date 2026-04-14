package planning_test

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon/audit"
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

func TestSolver_NilRegistry(t *testing.T) {
	result := (&planning.Solver{Operators: nil, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan to be initialized")
	}
	if len(result.Unsatisfied) == 0 {
		t.Fatal("expected unsatisfied predicates when registry is nil")
	}
}

func TestSolver_EmptyGoal(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	registry.Register(planning.Operator{
		Name:    "A",
		Effects: []planning.Predicate{"goal"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	// Empty goal should result in empty plan
	if len(result.Plan.Steps) != 0 {
		t.Fatalf("expected 0 steps for empty goal, got %d", len(result.Plan.Steps))
	}
}

func TestSolver_MultiplePredicates(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	registry.Register(planning.Operator{
		Name:    "OpA",
		Effects: []planning.Predicate{"predA"},
	})
	registry.Register(planning.Operator{
		Name:    "OpB",
		Effects: []planning.Predicate{"predB"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"predA", "predB"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Plan.Steps))
	}
}

func TestSolver_DuplicateOperatorsNotAdded(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	// Operator A produces "mid" and "goal"
	registry.Register(planning.Operator{
		Name:    "A",
		Effects: []planning.Predicate{"mid", "goal"},
	})
	// Operator B produces "goal" and requires "mid"
	registry.Register(planning.Operator{
		Name:          "B",
		Preconditions: []planning.Predicate{"mid"},
		Effects:       []planning.Predicate{"goal"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	// Should only need operator A once even though both could produce "goal"
	if len(result.Plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d: %+v", len(result.Plan.Steps), result.Plan.Steps)
	}
}

func TestSolver_RecordOperatorFailure_NilSolver(t *testing.T) {
	var solver *planning.Solver
	// Should not panic
	solver.RecordOperatorFailure("op1")
}

func TestSolver_RecordOperatorFailure_NilMap(t *testing.T) {
	solver := &planning.Solver{
		Operators:       &planning.OperatorRegistry{},
		MaxDepth:        5,
		FailedOperators: nil,
	}

	solver.RecordOperatorFailure("op1")
	solver.RecordOperatorFailure("op1")
	solver.RecordOperatorFailure("op2")

	if solver.FailedOperators["op1"] != 2 {
		t.Errorf("expected op1 to have 2 failures, got %d", solver.FailedOperators["op1"])
	}
	if solver.FailedOperators["op2"] != 1 {
		t.Errorf("expected op2 to have 1 failure, got %d", solver.FailedOperators["op2"])
	}
}

func TestSolver_WithRecorderQualitySorting(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	// Both operators can produce "goal"
	registry.Register(planning.Operator{
		Name:    "FastOp",
		Effects: []planning.Predicate{"goal"},
	})
	registry.Register(planning.Operator{
		Name:    "SlowOp",
		Effects: []planning.Predicate{"goal"},
	})

	// Create a metrics recorder with quality data
	recorder := audit.NewMetricsRecorder(nil)
	// FastOp has high success rate
	_ = recorder.RecordOperatorExecution("FastOp", true, 50)
	_ = recorder.RecordOperatorExecution("FastOp", true, 50)
	// SlowOp has lower success rate
	_ = recorder.RecordOperatorExecution("SlowOp", false, 200)
	_ = recorder.RecordOperatorExecution("SlowOp", true, 200)

	solver := &planning.Solver{
		Operators:       registry,
		MaxDepth:        5,
		Recorder:        recorder,
		FailedOperators: map[string]int{},
	}

	result := solver.Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Plan.Steps))
	}
}

func TestSolver_NoOperatorsForPredicate(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	// Register operator that produces something else
	registry.Register(planning.Operator{
		Name:    "OtherOp",
		Effects: []planning.Predicate{"other"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"unachievable"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan to be initialized")
	}
	if len(result.Unsatisfied) == 0 {
		t.Fatal("expected unsatisfied predicates when no operators available")
	}
}

func TestSolver_ChainedPreconditions(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	// Chain: C requires B, B requires A, A requires nothing
	registry.Register(planning.Operator{
		Name:          "OpC",
		Preconditions: []planning.Predicate{"stateB"},
		Effects:       []planning.Predicate{"stateC"},
	})
	registry.Register(planning.Operator{
		Name:          "OpB",
		Preconditions: []planning.Predicate{"stateA"},
		Effects:       []planning.Predicate{"stateB"},
	})
	registry.Register(planning.Operator{
		Name:    "OpA",
		Effects: []planning.Predicate{"stateA"},
	})

	result := (&planning.Solver{Operators: registry, MaxDepth: 10}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"stateC"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.Steps) != 3 {
		t.Fatalf("expected 3 steps in chain, got %d", len(result.Plan.Steps))
	}
	if result.Depth != 2 { // 0-indexed: A=0, B=1, C=2
		t.Errorf("expected max depth 2, got %d", result.Depth)
	}
}

func TestSolver_AlreadySatisfied(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	registry.Register(planning.Operator{
		Name:    "OpGoal",
		Effects: []planning.Predicate{"goal"},
	})

	ws := &planning.WorldState{}
	ws.Satisfy("goal")

	result := (&planning.Solver{Operators: registry, MaxDepth: 5}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"goal"}},
		ws,
	)
	if result.Plan == nil {
		t.Fatal("expected plan")
	}
	if len(result.Plan.Steps) != 0 {
		t.Fatalf("expected 0 steps when goal already satisfied, got %d", len(result.Plan.Steps))
	}
	if len(result.Unsatisfied) != 0 {
		t.Errorf("expected 0 unsatisfied, got %d", len(result.Unsatisfied))
	}
}

func TestSolver_MaxDepthReached(t *testing.T) {
	registry := &planning.OperatorRegistry{}
	// Deep chain that exceeds max depth
	registry.Register(planning.Operator{
		Name:          "OpD",
		Preconditions: []planning.Predicate{"stateC"},
		Effects:       []planning.Predicate{"stateD"},
	})
	registry.Register(planning.Operator{
		Name:          "OpC",
		Preconditions: []planning.Predicate{"stateB"},
		Effects:       []planning.Predicate{"stateC"},
	})
	registry.Register(planning.Operator{
		Name:          "OpB",
		Preconditions: []planning.Predicate{"stateA"},
		Effects:       []planning.Predicate{"stateB"},
	})
	registry.Register(planning.Operator{
		Name:    "OpA",
		Effects: []planning.Predicate{"stateA"},
	})

	// Max depth of 2 should not be enough for 4-level chain
	result := (&planning.Solver{Operators: registry, MaxDepth: 2}).Solve(
		planning.GoalCondition{Predicates: []planning.Predicate{"stateD"}},
		&planning.WorldState{},
	)
	if result.Plan == nil {
		t.Fatal("expected plan to be initialized")
	}
	if len(result.Unsatisfied) == 0 {
		t.Fatal("expected unsatisfied predicates when max depth reached")
	}
}
