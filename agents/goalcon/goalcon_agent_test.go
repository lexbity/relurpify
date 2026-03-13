package goalcon_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

type stubAgent struct {
	calls int
}

func (s *stubAgent) Initialize(_ *core.Config) error { return nil }
func (s *stubAgent) Capabilities() []core.Capability { return nil }
func (s *stubAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}
func (s *stubAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	s.calls++
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

func TestWorldState_SatisfyAndQuery(t *testing.T) {
	ws := goalcon.NewWorldState()
	ws.Satisfy("x")
	if !ws.IsSatisfied("x") {
		t.Fatal("expected satisfied")
	}
	clone := ws.Clone()
	clone.Satisfy("y")
	if ws.IsSatisfied("y") {
		t.Fatal("clone mutated source")
	}
}

func TestOperatorRegistry_OperatorsSatisfying(t *testing.T) {
	registry := &goalcon.OperatorRegistry{}
	registry.Register(goalcon.Operator{Name: "a", Effects: []goalcon.Predicate{"x"}})
	registry.Register(goalcon.Operator{Name: "b", Effects: []goalcon.Predicate{"x", "y"}})
	ops := registry.OperatorsSatisfying("x")
	if len(ops) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(ops))
	}
}

func TestSolver_LinearChain(t *testing.T) {
	registry := &goalcon.OperatorRegistry{}
	registry.Register(goalcon.Operator{Name: "A", Effects: []goalcon.Predicate{"x"}})
	registry.Register(goalcon.Operator{Name: "B", Preconditions: []goalcon.Predicate{"x"}, Effects: []goalcon.Predicate{"y"}})
	result := (&goalcon.Solver{Operators: registry, MaxDepth: 10}).Solve(goalcon.GoalCondition{Predicates: []goalcon.Predicate{"y"}}, goalcon.NewWorldState())
	if len(result.Plan.Steps) != 2 || result.Plan.Steps[0].Tool != "A" || result.Plan.Steps[1].Tool != "B" {
		t.Fatalf("unexpected plan: %+v", result.Plan)
	}
}

func TestSolver_AlreadySatisfied(t *testing.T) {
	registry := &goalcon.OperatorRegistry{}
	registry.Register(goalcon.Operator{Name: "A", Effects: []goalcon.Predicate{"x"}})
	ws := goalcon.NewWorldState()
	ws.Satisfy("x")
	result := (&goalcon.Solver{Operators: registry, MaxDepth: 10}).Solve(goalcon.GoalCondition{Predicates: []goalcon.Predicate{"x"}}, ws)
	if len(result.Plan.Steps) != 0 {
		t.Fatalf("expected no steps, got %+v", result.Plan.Steps)
	}
}

func TestSolver_CycleDetection(t *testing.T) {
	registry := &goalcon.OperatorRegistry{}
	registry.Register(goalcon.Operator{Name: "A", Preconditions: []goalcon.Predicate{"y"}, Effects: []goalcon.Predicate{"x"}})
	registry.Register(goalcon.Operator{Name: "B", Preconditions: []goalcon.Predicate{"x"}, Effects: []goalcon.Predicate{"y"}})
	result := (&goalcon.Solver{Operators: registry, MaxDepth: 10}).Solve(goalcon.GoalCondition{Predicates: []goalcon.Predicate{"x"}}, goalcon.NewWorldState())
	if len(result.Unsatisfied) == 0 || result.Unsatisfied[0] != "x" {
		t.Fatalf("expected x unsatisfied, got %+v", result.Unsatisfied)
	}
}

func TestSolver_MaxDepthLimit(t *testing.T) {
	registry := &goalcon.OperatorRegistry{}
	registry.Register(goalcon.Operator{Name: "A", Preconditions: []goalcon.Predicate{"x1"}, Effects: []goalcon.Predicate{"x2"}})
	registry.Register(goalcon.Operator{Name: "B", Preconditions: []goalcon.Predicate{"x0"}, Effects: []goalcon.Predicate{"x1"}})
	registry.Register(goalcon.Operator{Name: "C", Effects: []goalcon.Predicate{"x0"}})
	result := (&goalcon.Solver{Operators: registry, MaxDepth: 1}).Solve(goalcon.GoalCondition{Predicates: []goalcon.Predicate{"x2"}}, goalcon.NewWorldState())
	if len(result.Unsatisfied) == 0 {
		t.Fatal("expected unsatisfied predicates")
	}
}

func TestClassifyGoal_KeywordFix(t *testing.T) {
	goal := goalcon.ClassifyGoal("fix the bug", nil)
	found := false
	for _, pred := range goal.Predicates {
		if pred == "file_modified" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file_modified in %+v", goal.Predicates)
	}
}

func TestGoalConAgent_ImplementsGraphAgent(t *testing.T) {
	agent := &goalcon.GoalConAgent{}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(agent.Capabilities()) == 0 {
		t.Fatal("expected capabilities")
	}
	g, err := agent.BuildGraph(nil)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Fatal("expected graph")
	}
}

func TestGoalConAgent_ExecuteWithNoopExecutor(t *testing.T) {
	registry := &goalcon.OperatorRegistry{}
	registry.Register(goalcon.Operator{Name: "A", Description: "do a", Effects: []goalcon.Predicate{"x"}})
	agent := &goalcon.GoalConAgent{
		Operators: registry,
		GoalOverride: &goalcon.GoalCondition{
			Description: "reach x",
			Predicates:  []goalcon.Predicate{"x"},
		},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	result, err := agent.Execute(context.Background(), &core.Task{Instruction: "do it"}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGoalConAgent_FallbackWhenNoOperators(t *testing.T) {
	exec := &stubAgent{}
	agent := &goalcon.GoalConAgent{
		Operators:    &goalcon.OperatorRegistry{},
		PlanExecutor: exec,
		GoalOverride: &goalcon.GoalCondition{
			Description: "reach x",
			Predicates:  []goalcon.Predicate{"x"},
		},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	result, err := agent.Execute(context.Background(), &core.Task{Instruction: "do it"}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success || exec.calls != 1 {
		t.Fatalf("unexpected result=%+v calls=%d", result, exec.calls)
	}
}
