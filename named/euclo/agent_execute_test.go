package euclo

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

// cancelOnGenerateModel blocks in Generate until the context is canceled, so
// Agent.Execute exercises cancellation on the LLM path without tight timing.
type cancelOnGenerateModel struct{}

func (cancelOnGenerateModel) Generate(ctx context.Context, _ string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (cancelOnGenerateModel) GenerateStream(ctx context.Context, _ string, _ *core.LLMOptions) (<-chan string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (cancelOnGenerateModel) Chat(ctx context.Context, _ []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (cancelOnGenerateModel) ChatWithTools(ctx context.Context, _ []core.Message, _ []core.LLMToolSpec, _ *core.LLMOptions) (*core.LLMResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAgentExecute_ChatAskCompletesSuccessfully(t *testing.T) {
	env := testutil.WorkspaceEnv(t)
	agent := New(env)
	task := &core.Task{
		ID:          "task-chat-ask",
		Type:        core.TaskTypeAnalysis,
		Instruction: "What does this code do?",
		Context: map[string]any{
			"workspace": t.TempDir(),
		},
	}
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	raw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok {
		t.Fatal("expected behavior trace in state")
	}
	trace, ok := raw.(execution.Trace)
	if !ok {
		t.Fatalf("expected trace type, got %#v", raw)
	}
	if trace.PrimaryCapabilityID != euclorelurpic.CapabilityChatAsk {
		t.Fatalf("expected chat ask primary, got %q", trace.PrimaryCapabilityID)
	}
}

func TestAgentExecute_DebugInvestigateCompletesSuccessfully(t *testing.T) {
	env := testutil.WorkspaceEnv(t)
	agent := New(env)
	task := &core.Task{
		ID:   "task-debug",
		Type: core.TaskTypeAnalysis,
		Instruction: `Investigate failing test

panic: index out of range
goroutine 1 [running]:
main.handler(0x0)
    service.go:42 +0x1a`,
		Context: map[string]any{
			"workspace": t.TempDir(),
			"mode":      "debug",
		},
	}
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	raw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok {
		t.Fatal("expected behavior trace")
	}
	trace, ok := raw.(execution.Trace)
	if !ok {
		t.Fatalf("expected trace type, got %#v", raw)
	}
	if trace.PrimaryCapabilityID != euclorelurpic.CapabilityDebugInvestigateRepair {
		t.Fatalf("expected debug investigate primary, got %q", trace.PrimaryCapabilityID)
	}
}

func TestAgentExecute_PlanningExploreCompletesSuccessfully(t *testing.T) {
	// Scenario aligned with relurpicabilities/archaeology.TestExploreBehaviorExecutesOfflineWithScenarioStubModel
	model := testutil.NewScenarioStubModel(
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:patterns","value":[{"name":"Adapter","summary":"wraps external behavior","files":["pkg/service.go"],"relevance":0.8}]}]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:prospectives","value":[{"title":"Split transport boundary","summary":"separate transport and domain coordination","tradeoffs":["more files"],"confidence":0.7}]}]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:coherence_assessment","value":{"status":"coherent","notes":["patterns align"],"risks":["migration cost"]}}]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"facts":[{"key":"archaeology:convergence_assessment","value":{"status":"ready","recommended_direction":"split transport boundary","open_questions":["How to migrate callers?"]}}]}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Shape the exploration findings into candidate engineering directions"},
			Response:       &core.LLMResponse{Text: `{"goal":"explore","steps":[{"id":"step-1","description":"Investigate migration seams","tool":"file_read","params":{"path":"pkg/service.go"},"expected":"candidate direction identified","verification":"review the candidate","files":["pkg/service.go"]}],"dependencies":{},"files":["pkg/service.go"]}`},
		},
		testutil.ScenarioModelTurn{
			Response: &core.LLMResponse{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"review delegate finished"}`},
		},
		testutil.ScenarioModelTurn{
			Method:         "generate",
			PromptContains: []string{"Respond JSON"},
			Response:       &core.LLMResponse{Text: `{"issues":[],"approve":true}`},
		},
	)
	env := testutil.WorkspaceEnv(t)
	env.Model = model
	env.Config.Model = "scenario-stub"
	env.Config.MaxIterations = 1

	agent := New(env)
	state := core.NewContext()
	task := &core.Task{
		ID:          "task-agent-planning-explore",
		Type:        core.TaskTypePlanning,
		Instruction: "Explore archaeology-grounded candidate directions",
		Context: map[string]any{
			"workspace":    t.TempDir(),
			"mode":         "planning",
			"workflow_id":  "wf-agent-planning-explore",
			"corpus_scope": "workspace",
		},
	}
	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	raw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok {
		t.Fatal("expected behavior trace")
	}
	trace, ok := raw.(execution.Trace)
	if !ok {
		t.Fatalf("expected trace type, got %#v", raw)
	}
	if trace.PrimaryCapabilityID != euclorelurpic.CapabilityArchaeologyExplore {
		t.Fatalf("expected explore primary, got %q", trace.PrimaryCapabilityID)
	}
}

func TestAgentExecute_UnknownInteractiveModeReturnsError(t *testing.T) {
	env := testutil.WorkspaceEnv(t)
	agent := New(env)
	agent.InteractionRegistry = interaction.NewModeMachineRegistry()

	task := &core.Task{
		ID:   "task-interactive-missing",
		Type: core.TaskTypeAnalysis,
		// Avoid summary/status fast path (plan_stage_execute + "summarize") which
		// short-circuits before the interactive phase machine runs.
		Instruction: "What does this code do?",
		Context: map[string]any{
			"workspace": t.TempDir(),
		},
	}
	state := core.NewContext()
	_, err := agent.Execute(context.Background(), task, state)
	if err == nil {
		t.Fatal("expected error when no interactive mode is registered for resolved mode")
	}
}

func TestAgentExecute_ContextCanceledPropagatesFromModel(t *testing.T) {
	env := testutil.WorkspaceEnv(t)
	env.Model = cancelOnGenerateModel{}
	agent := New(env)

	task := &core.Task{
		ID:          "task-cancel",
		Type:        core.TaskTypeAnalysis,
		Instruction: "What does this function return?",
		Context: map[string]any{
			"workspace": t.TempDir(),
		},
	}
	state := core.NewContext()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.Execute(ctx, task, state)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
