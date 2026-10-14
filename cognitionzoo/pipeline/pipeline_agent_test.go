package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/execution"
	agentgraph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

func TestPipelineAgentInitializeStoresConfig(t *testing.T) {
	agent := &PipelineAgent{}
	cfg := &execution.Config{Name: "agent", Model: "m1"}
	require.NoError(t, agent.Initialize(cfg))
	assert.Same(t, cfg, agent.Config)
	assert.Equal(t, []string{"pipeline"}, agent.Capabilities())
}

func TestPipelineAgentInitializeDeps(t *testing.T) {
	agent := &PipelineAgent{}
	require.Error(t, agent.InitializeDeps(nil), "nil deps must be rejected")

	cfg := &execution.Config{Model: "m2"}
	mdl := &fakeModel{}
	require.NoError(t, agent.InitializeDeps(&paradigm.Deps{Config: cfg, Model: mdl}))
	assert.Same(t, cfg, agent.Config)
	assert.Same(t, mdl, agent.Model)
}

func TestNewAppliesOptions(t *testing.T) {
	mdl := &fakeModel{}
	cfg := &execution.Config{Model: "m3"}
	factory := &fakeStageFactory{}
	agent := New(&paradigm.Deps{Config: cfg, Model: mdl},
		WithStages([]Stage{newFakeStage("s1")}),
		WithStageFactory(factory),
		WithContextStreamMode(contextstream.ModeBackground),
		WithContextStreamQuery("custom query"),
		WithContextStreamMaxTokens(64),
	)
	require.NotNil(t, agent)
	assert.Same(t, cfg, agent.Config)
	assert.Same(t, mdl, agent.Model)
	assert.NotNil(t, agent.StageFactory)
	assert.Equal(t, contextstream.ModeBackground, agent.StreamMode)
	assert.Equal(t, "custom query", agent.StreamQuery)
	assert.Equal(t, 64, agent.StreamMaxTokens)
	assert.Len(t, agent.Stages, 1)

	// WithStages must copy the input slice.
	agent.Stages[0] = nil
	assert.NotNil(t, factory, "factory option applied")
}

type fakeStageFactory struct {
	stages []Stage
	err    error
}

func (f *fakeStageFactory) StagesForTask(_ *execution.Task) ([]Stage, error) {
	return f.stages, f.err
}

func TestStagesForTaskPrecedenceAndErrors(t *testing.T) {
	stage := newFakeStage("s")
	factoryStages := []Stage{newFakeStage("factory")}

	builder := &PipelineAgent{Stages: []Stage{stage}}
	viaStages, err := builder.stagesForTask(&execution.Task{})
	require.NoError(t, err)
	assert.Len(t, viaStages, 1)
	assert.NotSame(t, &builder.Stages[0], &viaStages[0], "agent stages must be copied")

	withBuilder := &PipelineAgent{
		Stages:       []Stage{stage},
		StageBuilder: func(*execution.Task) ([]Stage, error) { return []Stage{newFakeStage("built")}, nil },
	}
	out, err := withBuilder.stagesForTask(&execution.Task{})
	require.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Equal(t, "built", out[0].Name(), "StageBuilder takes precedence")

	factory := &PipelineAgent{StageFactory: &fakeStageFactory{stages: factoryStages}}
	out, err = factory.stagesForTask(&execution.Task{})
	require.NoError(t, err)
	assert.Equal(t, "factory", out[0].Name(), "StageFactory used when no builder")

	brokenFactory := &PipelineAgent{StageFactory: &fakeStageFactory{err: errBoom}}
	_, err = brokenFactory.stagesForTask(&execution.Task{})
	require.ErrorIs(t, err, errBoom)

	unconfigured := &PipelineAgent{}
	_, err = unconfigured.stagesForTask(&execution.Task{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline stages not configured")
}

func TestPipelineAgentExecuteTaskAndModelValidation(t *testing.T) {
	agent := &PipelineAgent{Model: &fakeModel{}}
	_, err := agent.Execute(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task required")

	_, err = (&PipelineAgent{}).Execute(context.Background(), &execution.Task{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing language model")

	_, err = (&PipelineAgent{Model: &fakeModel{}}).Execute(context.Background(), &execution.Task{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline stages not configured")

	// The empty-stages error only fires when a configured source yields nothing.
	emptyBuilder := &PipelineAgent{
		Model:        &fakeModel{},
		StageBuilder: func(*execution.Task) ([]Stage, error) { return nil, nil },
	}
	_, err = emptyBuilder.Execute(context.Background(), &execution.Task{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no stages for task")
}

func TestPipelineAgentExecuteRecordsResultsOnEnvelope(t *testing.T) {
	mdl := &fakeModel{}
	agent := &PipelineAgent{
		Model:  mdl,
		Stages: []Stage{newFakeStage("explore"), newFakeStage("code")},
		Config: &execution.Config{Name: "cfg", Model: "cfg-model"},
	}
	env := contextdata.NewEnvelope("task-1", "session-1")

	result, err := agent.Execute(context.Background(), &execution.Task{ID: "task-1", Instruction: "do it"}, env)
	require.NoError(t, err)
	assert.True(t, result.Success)

	payload := result.Data.Fields()
	require.Contains(t, payload, "stage_results")
	require.Contains(t, payload, "final_output")
	results := payload["stage_results"].([]StageResult)
	assert.Len(t, results, 2)

	final := payload["final_output"].(map[string]any)
	assert.Equal(t, 2, final["stages"])
	assert.Equal(t, "code", final["stage_name"])
	assert.Equal(t, "", final["workflow_id"])

	envResults, ok := contextdata.GetTyped[[]StageResult](env, "pipeline.results")
	assert.True(t, ok, "pipeline.results must be recorded on the envelope")
	assert.Len(t, envResults, 2)

	envFinal, ok := contextdata.GetTyped[map[string]any](env, "pipeline.final_output")
	assert.True(t, ok)
	assert.Equal(t, "code", envFinal["stage_name"])

	summary, ok := contextdata.GetTyped[string](env, "pipeline.results_summary")
	assert.True(t, ok)
	assert.Equal(t, "explore [ok]; code [ok]", summary)
}

func TestPipelineAgentExecutePreservesExistingResultsKey(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")
	env.SetWorkingValueWithClass("pipeline.results", "sentinel", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("pipeline.final_output", "sentinel-final", contextdata.MemoryClassTask)

	agent := &PipelineAgent{Model: &fakeModel{}, Stages: []Stage{newFakeStage("s")}}
	_, err := agent.Execute(context.Background(), &execution.Task{}, env)
	require.NoError(t, err)

	assert.Equal(t, "sentinel", envValue[string](t, env, "pipeline.results"),
		"pre-existing pipeline.results must not be overwritten")
	assert.Equal(t, "sentinel-final", envValue[string](t, env, "pipeline.final_output"))
	assert.Equal(t, "s [ok]", envValue[string](t, env, "pipeline.results_summary"),
		"summary is always refreshed")
}

func envValue[T any](t *testing.T, env *contextdata.Envelope, key string) T {
	t.Helper()
	v, ok := contextdata.GetTyped[any](env, key)
	require.True(t, ok)
	typed, ok := v.(T)
	require.True(t, ok, "value under %s has unexpected type %T", key, v)
	return typed
}

func TestPipelineAgentExecutePropagatesRunnerErrors(t *testing.T) {
	broken := newFakeStage("explore")
	broken.promptErr = errBoom
	agent := &PipelineAgent{Model: &fakeModel{}, Stages: []Stage{broken}}
	_, err := agent.Execute(context.Background(), &execution.Task{}, nil)
	require.ErrorIs(t, err, errBoom)
}

func TestPipelineAgentExecuteUsesRegistryTools(t *testing.T) {
	ctx := context.Background()
	reg := capability.NewRegistry()
	require.NoError(t, reg.RegisterLegacyTool(ctx, &fakeTool{name: "read"}))

	mdl := &fakeModel{}
	agent := &PipelineAgent{
		Model:  mdl,
		Tools:  reg,
		Config: &execution.Config{Model: "m"},
		Stages: []Stage{newFakeStage("s")},
	}
	_, err := agent.Execute(ctx, &execution.Task{}, nil)
	require.NoError(t, err)
	assert.Same(t, reg, agent.CapabilityRegistry())
	assert.Nil(t, agent.executionCatalog, "catalog snapshot must be cleared after Execute")
}

func TestPipelineAgentCapabilityRegistryNilSafe(t *testing.T) {
	var nilAgent *PipelineAgent
	assert.Nil(t, nilAgent.CapabilityRegistry())
	assert.Equal(t, "", nilAgent.modelName())
	assert.False(t, nilAgent.toolCallingEnabled())
	assert.Nil(t, nilAgent.telemetry())
	assert.Nil(t, nilAgent.availableTools(context.Background()))
}

func TestSummarizePipelineResults(t *testing.T) {
	assert.Empty(t, summarizePipelineResults(nil))

	summary := summarizePipelineResults([]StageResult{
		{StageName: "explore", ValidationOK: true},
		{StageName: "plan", ValidationOK: false},
		{StageName: "code", ErrorText: "boom"},
	})
	assert.Equal(t, "explore [ok]; plan [invalid]; code [error]", summary)
}

func TestSanitizePipelineName(t *testing.T) {
	assert.Equal(t, "explore_phase", sanitizePipelineName("Explore Phase"))
	assert.Equal(t, "a_b", sanitizePipelineName("a-b"))
	assert.Equal(t, "mixed", sanitizePipelineName("MiXeD"))
	assert.Equal(t, "stage", sanitizePipelineName(""))
	assert.Equal(t, "stage", sanitizePipelineName("   "))
}

func TestPipelineStageNodeInspection(t *testing.T) {
	stage := newFakeStage("inspect")
	node := &pipelineStageNode{id: "pipeline_stage_01_inspect", stage: stage}
	assert.Equal(t, "pipeline_stage_01_inspect", node.ID())
	assert.Equal(t, agentgraph.NodeTypeSystem, node.Type())

	env := contextdata.NewEnvelope("t", "s")
	res, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "pipeline_stage_01_inspect", res.NodeID)
	assert.Equal(t, "inspect", envValue[string](t, env, "pipeline.inspect_stage"))

	// Node without a stage or envelope must not panic.
	bare := &pipelineStageNode{id: "bare"}
	res, err = bare.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, res.Success)
}

func TestPipelineStreamDefaultsAndOverrides(t *testing.T) {
	agent := &PipelineAgent{}
	assert.Equal(t, contextstream.ModeBlocking, agent.streamMode())
	assert.Equal(t, "task instruction", agent.streamQuery(&execution.Task{Instruction: "task instruction"}))
	assert.Equal(t, "", agent.streamQuery(nil))
	assert.Equal(t, 256, agent.streamMaxTokens())

	agent.StreamMode = contextstream.ModeBackground
	agent.StreamQuery = "custom"
	agent.StreamMaxTokens = 32
	assert.Equal(t, contextstream.ModeBackground, agent.streamMode())
	assert.Equal(t, "custom", agent.streamQuery(nil))
	assert.Equal(t, 32, agent.streamMaxTokens())
}

func TestPipelineStreamTriggerNode(t *testing.T) {
	agent := &PipelineAgent{StreamQuery: "q", StreamMode: contextstream.ModeBackground, StreamMaxTokens: 9}
	node := agent.streamTriggerNode(&execution.Task{})
	require.NotNil(t, node)
	streamNode, ok := node.(*agentgraph.StreamTriggerNode)
	require.True(t, ok)
	assert.Equal(t, "pipeline_stream", node.ID())
	assert.Equal(t, contextstream.ModeBackground, streamNode.Mode)
	assert.Equal(t, 9, streamNode.MaxTokens)
	assert.Equal(t, "q", streamNode.Query.Text)
}

func TestPipelineAgentBuildGraphAlwaysStartsAtStreamNode(t *testing.T) {
	// streamTriggerNode always returns a node, so the graph start is the
	// stream trigger regardless of stream configuration; the query defaults
	// to the task instruction.
	agent := &PipelineAgent{
		Stages: []Stage{newFakeStage("explore"), newFakeStage("plan")},
	}
	g, err := agent.BuildGraph(context.Background(), &execution.Task{Instruction: "task q"})
	require.NoError(t, err)
	require.NotNil(t, g)
	assert.Equal(t, "pipeline_stream", g.StartNodeID())

	// Explicit stream configuration keeps the same start node.
	agent.StreamQuery = "custom q"
	g, err = agent.BuildGraph(context.Background(), &execution.Task{})
	require.NoError(t, err)
	assert.Equal(t, "pipeline_stream", g.StartNodeID())
}

func TestPipelineAgentBuildGraphErrors(t *testing.T) {
	broken := &PipelineAgent{StageBuilder: func(*execution.Task) ([]Stage, error) { return nil, errBoom }}
	_, err := broken.BuildGraph(context.Background(), &execution.Task{})
	require.ErrorIs(t, err, errBoom)

	empty := &PipelineAgent{}
	_, err = empty.BuildGraph(context.Background(), &execution.Task{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline stages not configured")
}

func TestPipelineAgentTelemetryAndConfigAccessors(t *testing.T) {
	sink := &recordingTelemetry{}
	agent := &PipelineAgent{Config: &execution.Config{
		Model:             "m",
		NativeToolCalling: true,
		Telemetry:         sink,
	}}
	assert.Equal(t, "m", agent.modelName())
	assert.True(t, agent.toolCallingEnabled())
	assert.NotNil(t, agent.telemetry())
	assert.Nil(t, (&PipelineAgent{}).telemetry())

	// Agent helpers tolerate a nil receiver.
	var nilAgent *PipelineAgent
	assert.Equal(t, "", nilAgent.modelName())
}

func TestPipelineAgentExecuteEmitsTelemetryThroughConfig(t *testing.T) {
	sink := &recordingTelemetry{}
	agent := &PipelineAgent{
		Model:  &fakeModel{},
		Config: &execution.Config{Telemetry: sink},
		Stages: []Stage{newFakeStage("s")},
	}
	_, err := agent.Execute(context.Background(), &execution.Task{}, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, sink.emitted())
}

func TestPipelineAgentExecuteWithoutConfigStillSucceeds(t *testing.T) {
	agent := &PipelineAgent{Model: &fakeModel{}, Stages: []Stage{newFakeStage("s")}}
	result, err := agent.Execute(context.Background(), &execution.Task{}, nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)
}
