package goalcon

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

type goalconStubExecutor struct {
	calls int
}

func (s *goalconStubExecutor) Initialize(_ *core.Config) error { return nil }
func (s *goalconStubExecutor) Capabilities() []core.Capability { return nil }
func (s *goalconStubExecutor) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("goalcon_stub_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}
func (s *goalconStubExecutor) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	s.calls++
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

func TestCapabilityRegistry(t *testing.T) {
	var agent *GoalConAgent
	if got := agent.CapabilityRegistry(); got != nil {
		t.Fatalf("expected nil registry on nil agent, got %#v", got)
	}

	registry := capability.NewRegistry()
	agent = &GoalConAgent{Tools: registry}
	if got := agent.CapabilityRegistry(); got != registry {
		t.Fatalf("expected registry to round-trip, got %#v", got)
	}
}

func TestNewAndInitializeEnvironment(t *testing.T) {
	env := agentenv.AgentEnvironment{
		Config:   &core.Config{Name: "goalcon"},
		Registry: capability.NewRegistry(),
	}
	agent := New(env, nil)
	if agent == nil {
		t.Fatal("expected agent")
	}
	if agent.Config != env.Config {
		t.Fatalf("expected config from env")
	}
	if agent.Tools == nil {
		t.Fatal("expected tools to be initialized")
	}
	if agent.Operators == nil {
		t.Fatal("expected default operator registry")
	}
	if agent.MetricsRecorder == nil {
		t.Fatal("expected metrics recorder")
	}

	other := &GoalConAgent{}
	if err := other.InitializeEnvironment(env); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if other.Config != env.Config || other.Tools == nil || other.Operators == nil {
		t.Fatalf("unexpected initialized agent: %+v", other)
	}
}

func TestGoalAndDepthHelpers(t *testing.T) {
	agent := &GoalConAgent{
		GoalOverride: &GoalCondition{Description: "override", Predicates: []Predicate{"x"}},
	}
	if got := agent.goal(nil); got.Description != "override" {
		t.Fatalf("expected override goal, got %+v", got)
	}

	agent.GoalOverride = nil
	if got := agent.goal(nil); len(got.Predicates) != 0 || got.Description != "" {
		t.Fatalf("expected zero goal for nil task, got %+v", got)
	}

	if got := (&GoalConAgent{}).maxDepth(); got != 10 {
		t.Fatalf("expected default max depth, got %d", got)
	}
	if got := (&GoalConAgent{MaxDepth: 3}).maxDepth(); got != 3 {
		t.Fatalf("expected custom max depth, got %d", got)
	}

	exec := &goalconStubExecutor{}
	if got := (&GoalConAgent{PlanExecutor: exec}).planExecutorAgent(); got != exec {
		t.Fatalf("expected custom executor, got %#v", got)
	}
	if got := (&GoalConAgent{}).planExecutorAgent(); got == nil {
		t.Fatal("expected noop fallback executor")
	}
}

func TestGoalconNodeAndNoopAgent(t *testing.T) {
	node := &goalconNode{id: "goalcon-test"}
	if node.ID() != "goalcon-test" || node.Type() != graph.NodeTypeSystem {
		t.Fatalf("unexpected node identity: %+v", node)
	}
	if result, err := node.Execute(context.Background(), core.NewContext()); err != nil || result == nil || !result.Success {
		t.Fatalf("unexpected node execution: result=%+v err=%v", result, err)
	}

	noop := &noopAgent{}
	if err := noop.Initialize(&core.Config{}); err != nil {
		t.Fatalf("noop initialize: %v", err)
	}
	if len(noop.Capabilities()) != 0 {
		t.Fatalf("expected no capabilities")
	}
	graph, err := noop.BuildGraph(nil)
	if err != nil || graph == nil {
		t.Fatalf("noop build graph: graph=%#v err=%v", graph, err)
	}
	if result, err := noop.Execute(context.Background(), nil, nil); err != nil || result == nil || !result.Success {
		t.Fatalf("noop execute: result=%+v err=%v", result, err)
	}
}
