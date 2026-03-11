package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/agents/stages"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/pipeline"
	"github.com/stretchr/testify/require"
)

type stubDelegateAgent struct {
	buildGraphResult *graph.Graph
	buildGraphCalls  int
	executeCalls     int
	lastTask         *core.Task
	lastState        *core.Context
}

func (s *stubDelegateAgent) Initialize(config *core.Config) error { return nil }

func (s *stubDelegateAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	s.executeCalls++
	s.lastTask = task
	s.lastState = state
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

func (s *stubDelegateAgent) Capabilities() []core.Capability { return nil }

func (s *stubDelegateAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	s.buildGraphCalls++
	s.lastTask = task
	return s.buildGraphResult, nil
}

func TestCodingAgentBuildGraphUsesTaskMode(t *testing.T) {
	codeGraph := graph.NewGraph()
	architectGraph := graph.NewGraph()
	codeDelegate := &stubDelegateAgent{buildGraphResult: codeGraph}
	architectDelegate := &stubDelegateAgent{buildGraphResult: architectGraph}

	agent := &CodingAgent{
		modeProfiles: map[Mode]ModeProfile{
			ModeCode:      {Name: ModeCode},
			ModeArchitect: {Name: ModeArchitect},
		},
		delegates: map[Mode]graph.Agent{
			ModeCode:      codeDelegate,
			ModeArchitect: architectDelegate,
		},
	}

	task := &core.Task{Context: map[string]any{"mode": "architect"}}
	got, err := agent.BuildGraph(task)
	require.NoError(t, err)
	require.Same(t, architectGraph, got)
	require.Equal(t, 0, codeDelegate.buildGraphCalls)
	require.Equal(t, 1, architectDelegate.buildGraphCalls)
}

func TestCodingAgentExecuteAllocatesStateWhenNil(t *testing.T) {
	delegate := &stubDelegateAgent{}
	agent := &CodingAgent{
		modeProfiles: map[Mode]ModeProfile{
			ModeCode: {Name: ModeCode},
		},
		delegates: map[Mode]graph.Agent{
			ModeCode: delegate,
		},
	}

	task := &core.Task{Instruction: "fix the bug"}
	result, err := agent.Execute(context.Background(), task, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, delegate.lastState)
	require.Equal(t, 1, delegate.executeCalls)
}

type stubCodingPipelineModel struct{}

func (m *stubCodingPipelineModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}

func (m *stubCodingPipelineModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *stubCodingPipelineModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *stubCodingPipelineModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

type stubCodingPipelineStage struct {
	name     string
	output   any
	applyFn  func(*core.Context, any)
	contract pipeline.ContractDescriptor
}

func makeCodingPipelineStage(name string, output any, applyFn func(*core.Context, any)) *stubCodingPipelineStage {
	return &stubCodingPipelineStage{
		name:    name,
		output:  output,
		applyFn: applyFn,
		contract: pipeline.ContractDescriptor{
			Name: name + "-contract",
			Metadata: pipeline.ContractMetadata{
				InputKey:      "pipeline.input",
				OutputKey:     "pipeline.output",
				SchemaVersion: "v1",
			},
		},
	}
}

func (s *stubCodingPipelineStage) Name() string                          { return s.name }
func (s *stubCodingPipelineStage) Contract() pipeline.ContractDescriptor { return s.contract }
func (s *stubCodingPipelineStage) BuildPrompt(ctx *core.Context) (string, error) {
	return s.name + " prompt", nil
}
func (s *stubCodingPipelineStage) Decode(resp *core.LLMResponse) (any, error) { return s.output, nil }
func (s *stubCodingPipelineStage) Validate(output any) error                  { return nil }
func (s *stubCodingPipelineStage) Apply(ctx *core.Context, output any) error {
	if s.applyFn != nil {
		s.applyFn(ctx, output)
	}
	return nil
}

func TestCodingAgentDelegateForModeUsesPipelineControlFlow(t *testing.T) {
	agent := &CodingAgent{
		Model: &stubCodingPipelineModel{},
		modeProfiles: map[Mode]ModeProfile{
			ModeCode: {Name: ModeCode, ControlFlow: ControlFlowPipeline},
		},
		PipelineStages: []pipeline.Stage{
			makeCodingPipelineStage("analyze", map[string]any{"ok": true}, nil),
		},
		delegates: map[Mode]graph.Agent{},
	}

	delegate, err := agent.delegateForMode(ModeCode)
	require.NoError(t, err)
	_, ok := delegate.(*PipelineAgent)
	require.True(t, ok, "expected PipelineAgent delegate")
}

func TestCodingAgentBuildGraphUsesPipelineMode(t *testing.T) {
	agent := &CodingAgent{
		Model: &stubCodingPipelineModel{},
		modeProfiles: map[Mode]ModeProfile{
			ModeCode: {Name: ModeCode, ControlFlow: ControlFlowPipeline},
		},
		PipelineStages: []pipeline.Stage{
			makeCodingPipelineStage("analyze", map[string]any{"ok": true}, nil),
		},
		delegates: map[Mode]graph.Agent{},
	}

	g, err := agent.BuildGraph(&core.Task{Context: map[string]any{"mode": "code"}})
	require.NoError(t, err)
	require.NoError(t, g.Validate())
}

func TestCodingAgentExecuteSurfacesPipelineFinalOutput(t *testing.T) {
	agent := &CodingAgent{
		Model: &stubCodingPipelineModel{},
		modeProfiles: map[Mode]ModeProfile{
			ModeCode: {Name: ModeCode, ControlFlow: ControlFlowPipeline},
		},
		PipelineStages: []pipeline.Stage{
			makeCodingPipelineStage("analyze", map[string]any{"issues": 1}, func(ctx *core.Context, output any) {
				ctx.Set("pipeline.output", output)
			}),
		},
		delegates: map[Mode]graph.Agent{},
	}

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-pipeline",
		Instruction: "run pipeline mode",
		Context:     map[string]any{"mode": "code"},
	}, core.NewContext())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Data)
	_, ok := result.Data["final_output"]
	require.True(t, ok, "expected final_output from pipeline execution")
}

func TestCodingAgentPipelineModeUsesDefaultCodingStageFactory(t *testing.T) {
	agent := &CodingAgent{
		Model: &stubCodingPipelineModel{},
		modeProfiles: map[Mode]ModeProfile{
			ModeCode: {Name: ModeCode, ControlFlow: ControlFlowPipeline},
		},
		delegates: map[Mode]graph.Agent{},
	}

	delegate, err := agent.delegateForMode(ModeCode)
	require.NoError(t, err)

	pipelineDelegate, ok := delegate.(*PipelineAgent)
	require.True(t, ok, "expected PipelineAgent delegate")
	require.IsType(t, stages.CodingStageFactory{}, pipelineDelegate.StageFactory)

	g, err := pipelineDelegate.BuildGraph(&core.Task{Instruction: "fix code"})
	require.NoError(t, err)
	require.NoError(t, g.Validate())
}

func TestCodingAgentOverrideControlFlowClearsCachedDelegate(t *testing.T) {
	agent := &CodingAgent{
		Model: &stubCodingPipelineModel{},
		modeProfiles: map[Mode]ModeProfile{
			ModeCode: {Name: ModeCode, ControlFlow: ControlFlowReAct},
		},
		delegates: map[Mode]graph.Agent{
			ModeCode: &stubDelegateAgent{},
		},
	}

	err := agent.OverrideControlFlow(ModeCode, ControlFlowPipeline)
	require.NoError(t, err)
	require.Empty(t, agent.delegates)

	profile := agent.modeProfiles[ModeCode]
	require.Equal(t, ControlFlowPipeline, profile.ControlFlow)
}
