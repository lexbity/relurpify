package thoughtrecipes

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon"
	"github.com/lexcodex/relurpify/agents/htn"
	"github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// createTestEnvironment creates a minimal test environment
func createTestEnvironment(t *testing.T) agentenv.AgentEnvironment {
	t.Helper()
	return agentenv.AgentEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{MaxIterations: 10},
	}
}

func TestBuildParadigmAgentReact(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("react", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil react agent")
	}

	// Verify it's actually a react agent
	if _, ok := agent.(*react.ReActAgent); !ok {
		t.Errorf("expected *react.ReActAgent, got %T", agent)
	}
}

func TestBuildParadigmAgentPlanner(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("planner", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil planner agent")
	}
}

func TestBuildParadigmAgentReWOO(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("rewoo", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil rewoo agent")
	}
}

func TestBuildParadigmAgentBlackboard(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("blackboard", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil blackboard agent")
	}
}

func TestBuildParadigmAgentChainer(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("chainer", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil chainer agent")
	}
}

func TestBuildParadigmAgentArchitect(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("architect", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil architect agent")
	}
}

func TestBuildParadigmAgentHTN(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("htn", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil htn agent")
	}

	// Verify it's an HTN agent with default react primitive exec
	htnAgent, ok := agent.(*htn.HTNAgent)
	if !ok {
		t.Fatalf("expected *htn.HTNAgent, got %T", agent)
	}

	// PrimitiveExec should be set to a react agent by default
	if htnAgent.PrimitiveExec == nil {
		t.Error("expected PrimitiveExec to be set (default react)")
	}
}

func TestBuildParadigmAgentHTNWithChild(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	// Create a child agent (planner)
	child, err := factory.BuildParadigmAgent("planner", nil, env, reg)
	if err != nil {
		t.Fatalf("failed to build child: %v", err)
	}

	// Build HTN with the child wired in
	agent, err := factory.BuildParadigmAgent("htn", child, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	htnAgent, ok := agent.(*htn.HTNAgent)
	if !ok {
		t.Fatalf("expected *htn.HTNAgent, got %T", agent)
	}

	if htnAgent.PrimitiveExec == nil {
		t.Error("expected PrimitiveExec to be set to the child")
	}
}

func TestBuildParadigmAgentReflection(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("reflection", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil reflection agent")
	}

	// Verify it's a reflection agent with default react delegate
	reflAgent, ok := agent.(*reflection.ReflectionAgent)
	if !ok {
		t.Fatalf("expected *reflection.ReflectionAgent, got %T", agent)
	}

	if reflAgent.Delegate == nil {
		t.Error("expected Delegate to be set (default react)")
	}
}

func TestBuildParadigmAgentReflectionWithChild(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	// Create a child agent (react)
	child, err := factory.BuildParadigmAgent("react", nil, env, reg)
	if err != nil {
		t.Fatalf("failed to build child: %v", err)
	}

	// Build reflection with the child wired in as delegate
	agent, err := factory.BuildParadigmAgent("reflection", child, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	reflAgent, ok := agent.(*reflection.ReflectionAgent)
	if !ok {
		t.Fatalf("expected *reflection.ReflectionAgent, got %T", agent)
	}

	if reflAgent.Delegate == nil {
		t.Error("expected Delegate to be set to the child")
	}
}

func TestBuildParadigmAgentGoalcon(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("goalcon", nil, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil goalcon agent")
	}

	// Verify it's a goalcon agent
	goalconAgent, ok := agent.(*goalcon.GoalConAgent)
	if !ok {
		t.Fatalf("expected *goalcon.GoalConAgent, got %T", agent)
	}

	// Operators should be set
	if goalconAgent.Operators == nil {
		t.Error("expected Operators to be set")
	}
}

func TestBuildParadigmAgentGoalconWithChild(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	// Create a child agent (react)
	child, err := factory.BuildParadigmAgent("react", nil, env, reg)
	if err != nil {
		t.Fatalf("failed to build child: %v", err)
	}

	// Build goalcon with the child wired in as PlanExecutor
	agent, err := factory.BuildParadigmAgent("goalcon", child, env, reg)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	goalconAgent, ok := agent.(*goalcon.GoalConAgent)
	if !ok {
		t.Fatalf("expected *goalcon.GoalConAgent, got %T", agent)
	}

	if goalconAgent.PlanExecutor == nil {
		t.Error("expected PlanExecutor to be set to the child")
	}
}

func TestBuildParadigmAgentUnknown(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	agent, err := factory.BuildParadigmAgent("unknown-paradigm", nil, env, reg)
	if err == nil {
		t.Error("expected error for unknown paradigm")
	}
	if agent != nil {
		t.Error("expected nil agent for unknown paradigm")
	}
}

func TestBuildParadigmAgentNilRegistry(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)

	agent, err := factory.BuildParadigmAgent("react", nil, env, nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}
	if agent != nil {
		t.Error("expected nil agent for nil registry")
	}
}

func TestBuildStepAgentWithChild(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	step := ExecutionStep{
		ID: "step1",
		Parent: ExecutionStepAgent{
			Paradigm:     "htn",
			Capabilities: []string{},
		},
		Child: &ExecutionStepAgent{
			Paradigm:     "react",
			Capabilities: []string{},
		},
	}

	parent, child, warnings, err := factory.BuildStepAgent(step, reg, env)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if parent == nil {
		t.Error("expected non-nil parent")
	}
	if child == nil {
		t.Error("expected non-nil child")
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid child, got %v", warnings)
	}

	// Verify HTN has the react child wired
	htnAgent, ok := parent.(*htn.HTNAgent)
	if !ok {
		t.Fatalf("expected *htn.HTNAgent, got %T", parent)
	}
	if htnAgent.PrimitiveExec == nil {
		t.Error("expected HTN PrimitiveExec to be set")
	}
}

func TestBuildStepAgentChildNoDelegation(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	// React does not support delegation, so child should be ignored with warning
	step := ExecutionStep{
		ID: "step1",
		Parent: ExecutionStepAgent{
			Paradigm:     "react",
			Capabilities: []string{},
		},
		Child: &ExecutionStepAgent{
			Paradigm:     "planner",
			Capabilities: []string{},
		},
	}

	parent, child, warnings, err := factory.BuildStepAgent(step, reg, env)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if parent == nil {
		t.Error("expected non-nil parent")
	}
	if child == nil {
		t.Error("expected non-nil child (still built even if not wired)")
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for child on non-delegating paradigm, got %v", warnings)
	}
}

func TestBuildStepAgentCapabilityFiltering(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	step := ExecutionStep{
		ID: "step1",
		Parent: ExecutionStepAgent{
			Paradigm:     "react",
			Capabilities: []string{"cap:file_read", "cap:file_write"},
		},
	}

	parent, _, _, err := factory.BuildStepAgent(step, reg, env)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if parent == nil {
		t.Error("expected non-nil parent")
	}
}

func TestBuildFallbackAgent(t *testing.T) {
	factory := NewParadigmFactory()
	env := createTestEnvironment(t)
	reg := capability.NewRegistry()

	fallback := ExecutionStepAgent{
		Paradigm:     "react",
		Capabilities: []string{},
	}

	agent, err := factory.BuildFallbackAgent(fallback, reg, env)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if agent == nil {
		t.Error("expected non-nil fallback agent")
	}

	// Verify it's a react agent
	if _, ok := agent.(*react.ReActAgent); !ok {
		t.Errorf("expected *react.ReActAgent, got %T", agent)
	}
}
