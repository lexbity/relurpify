package delegates

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	rexroute "codeburg.org/lexbit/relurpify/named/rex/route"
)

type stubModel struct{}

func (stubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
func (stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (stubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}
func (stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func testEnv(t *testing.T) *agentenv.WorkspaceEnvironment {
	t.Helper()
	memStore := memory.NewWorkingMemoryStore()
	return &agentenv.WorkspaceEnvironment{
		Model:         stubModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memStore,
		Config: &core.Config{
			Name:          "rex-test",
			Model:         "stub",
			MaxIterations: 1,
		},
	}
}

func TestResolveReturnsPrimaryDelegate(t *testing.T) {
	registry := NewRegistry(testEnv(t), t.TempDir())
	delegate, err := registry.Resolve(rexroute.ExecutionPlan{PrimaryFamily: rexroute.FamilyReAct})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if delegate.Family() != rexroute.FamilyReAct {
		t.Fatalf("family = %q", delegate.Family())
	}
}

func TestResolveFallsBackAndErrorsWhenUnavailable(t *testing.T) {
	registry := NewRegistry(testEnv(t), t.TempDir())
	delegate, err := registry.Resolve(rexroute.ExecutionPlan{
		PrimaryFamily: "missing",
		Fallbacks:     []string{"also-missing", rexroute.FamilyPipeline},
	})
	if err != nil {
		t.Fatalf("Resolve fallback: %v", err)
	}
	if delegate.Family() != rexroute.FamilyPipeline {
		t.Fatalf("fallback family = %q", delegate.Family())
	}
	if _, err := registry.Resolve(rexroute.ExecutionPlan{PrimaryFamily: "missing"}); err == nil {
		t.Fatalf("expected error for unavailable delegate")
	}
}

type stubExecutor struct {
	buildGraphFn func(*core.Task) (*agentgraph.Graph, error)
	executeFn    func(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error)
}

func (s stubExecutor) Initialize(*core.Config) error   { return nil }
func (s stubExecutor) Capabilities() []string          { return nil }
func (s stubExecutor) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
	if s.buildGraphFn != nil {
		return s.buildGraphFn(task)
	}
	return &agentgraph.Graph{}, nil
}
func (s stubExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if s.executeFn != nil {
		return s.executeFn(ctx, task, env)
	}
	return &core.Result{Success: true}, nil
}

func TestAgentDelegatePassesThroughGraphAndExecution(t *testing.T) {
	var buildTaskID string
	var executed bool
	delegate := agentDelegate{
		family: "stub",
		agent: stubExecutor{
			buildGraphFn: func(task *core.Task) (*agentgraph.Graph, error) {
				buildTaskID = task.ID
				return &agentgraph.Graph{}, nil
			},
			executeFn: func(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error) {
				executed = true
				return &core.Result{NodeID: "node-1", Success: true}, nil
			},
		},
	}
	graphResult, err := delegate.BuildGraph(&core.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if buildTaskID != "task-1" || graphResult == nil {
		t.Fatalf("unexpected build graph result: task=%q graph=%v", buildTaskID, graphResult)
	}
	env := contextdata.NewEnvelope("task-2", "")
	result, err := delegate.Execute(context.Background(), &core.Task{ID: "task-2"}, env)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !executed || result.NodeID != "node-1" {
		t.Fatalf("unexpected execute result: executed=%v result=%+v", executed, result)
	}
}
