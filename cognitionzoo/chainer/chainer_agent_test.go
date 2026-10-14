package chainer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/execution"
	agentgraph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

func TestChainerInitializeCreatesDefaultRegistry(t *testing.T) {
	agent := &ChainerAgent{}
	require.NoError(t, agent.Initialize(&execution.Config{Name: "cfg"}))
	assert.NotNil(t, agent.Tools, "Initialize must provision an empty capability registry")
	assert.True(t, agent.initialised)
	assert.Equal(t, []string{"plan", "execute", "explain"}, agent.Capabilities())

	// An injected registry is preserved.
	injected := capability.NewRegistry()
	other := &ChainerAgent{Tools: injected}
	require.NoError(t, other.Initialize(nil))
	assert.Same(t, injected, other.Tools)
}

func TestChainerInitializeDeps(t *testing.T) {
	agent := &ChainerAgent{}
	require.EqualError(t, agent.InitializeDeps(nil), "chainer dependencies unavailable")

	mdl := &scriptChatModel{}
	cfg := &execution.Config{Model: "m"}
	reg := capability.NewRegistry()
	require.NoError(t, agent.InitializeDeps(&paradigm.Deps{
		Config:         cfg,
		Model:          mdl,
		Registry:       reg,
		PromptRegistry: prompt.NewRegistry(),
	}))
	assert.Same(t, mdl, agent.Model)
	assert.Same(t, reg, agent.Tools)
	assert.Same(t, cfg, agent.Config)
	assert.NotNil(t, agent.PromptRegistry)
}

func TestChainerNewDefaultsAndOptions(t *testing.T) {
	agent := New(nil)
	require.NotNil(t, agent)
	require.NotNil(t, agent.Chain, "New must provide a default summarize chain")
	require.Len(t, agent.Chain.Links, 1)
	assert.Equal(t, "default", agent.Chain.Links[0].Name)
	assert.Equal(t, "chainer.output", agent.Chain.Links[0].OutputKey)
	assert.Nil(t, agent.Model, "nil deps leave the model unset")

	chain := &Chain{Links: []Link{NewSummarizeLink("custom", nil, "custom.out")}}
	agent = New(nil,
		WithChain(chain),
		WithContextStreamMode(contextstream.ModeBackground),
		WithContextStreamQuery("custom query"),
		WithContextStreamMaxTokens(77),
	)
	assert.Equal(t, "custom", agent.Chain.Links[0].Name, "WithChain replaces the default chain")
	assert.Equal(t, contextstream.ModeBackground, agent.StreamMode)
	assert.Equal(t, "custom query", agent.StreamQuery)
	assert.Equal(t, 77, agent.StreamMaxTokens)

	// A nil option must be tolerated.
	assert.NotNil(t, New(nil, nil))

	// WithChainBuilder wires the task-driven builder; resolveChain prefers it
	// without replacing the agent's default Chain field.
	built := &Chain{Links: []Link{NewSummarizeLink("built", nil, "out")}}
	agent = New(nil, WithChainBuilder(func(*execution.Task) (*Chain, error) { return built, nil }))
	agent.Model = newScriptChatModel("out")
	result, err := agent.Execute(context.Background(), taskWith("goal"), nil)
	require.NoError(t, err)
	assert.Equal(t, "default", agent.Chain.Links[0].Name)
	resolved, err := agent.resolveChain(taskWith("goal"))
	require.NoError(t, err)
	assert.Equal(t, "built", resolved.Links[0].Name)
	assert.Equal(t, "out", result.Data.Fields()["out"])
}

func TestChainerResolveChain(t *testing.T) {
	chain := &Chain{Links: []Link{NewSummarizeLink("l", nil, "out")}}

	viaField := &ChainerAgent{Chain: chain}
	resolved, err := viaField.resolveChain(taskWith("t"))
	require.NoError(t, err)
	assert.Same(t, chain, resolved)

	builderChain := &Chain{Links: []Link{NewSummarizeLink("built", nil, "out")}}
	viaBuilder := &ChainerAgent{Chain: chain, ChainBuilder: func(*execution.Task) (*Chain, error) {
		return builderChain, nil
	}}
	resolved, err = viaBuilder.resolveChain(taskWith("t"))
	require.NoError(t, err)
	assert.Same(t, builderChain, resolved, "ChainBuilder takes precedence over Chain")

	broken := &ChainerAgent{ChainBuilder: func(*execution.Task) (*Chain, error) { return nil, errBoomChainer }}
	_, err = broken.resolveChain(taskWith("t"))
	require.ErrorIs(t, err, errBoomChainer)

	unconfigured := &ChainerAgent{}
	_, err = unconfigured.resolveChain(taskWith("t"))
	require.EqualError(t, err, "chainer: chain not configured")
}

func TestChainerExecuteRunsChainAndRecordsOutputs(t *testing.T) {
	agent := New(&paradigm.Deps{Model: newScriptChatModel("first out", "second out")},
		WithChain(&Chain{Links: []Link{
			NewSummarizeLink("l1", nil, "chainer.first"),
			NewSummarizeLink("l2", nil, "chainer.second"),
		}}))

	result, err := agent.Execute(context.Background(), taskWith("goal"), newTestEnvelope(t))
	require.NoError(t, err)
	assert.True(t, result.Success)

	fields := result.Data.Fields()
	assert.Equal(t, 2, fields["links_executed"])
	assert.Equal(t, "first out", fields["chainer.first"])
	assert.Equal(t, "second out", fields["chainer.second"])
}

func TestChainerExecuteWritesEnvelopeKeys(t *testing.T) {
	agent := New(&paradigm.Deps{Model: newScriptChatModel("summary text")},
		WithChain(&Chain{Links: []Link{NewSummarizeLink("l1", nil, "chainer.output")}}))
	env := newTestEnvelope(t)

	_, err := agent.Execute(context.Background(), taskWith("goal"), env)
	require.NoError(t, err)

	assert.Equal(t, "summary text", envValue[string](t, env, "chainer.output"))
	assert.Equal(t, 1, envValue[int](t, env, "chainer.links_executed"))
}

func TestChainerExecuteCreatesEnvelopeWhenNil(t *testing.T) {
	agent := New(&paradigm.Deps{Model: newScriptChatModel("out")},
		WithChain(&Chain{Links: []Link{NewSummarizeLink("l1", nil, "chainer.output")}}))

	result, err := agent.Execute(context.Background(), taskWith("goal"), nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "out", result.Data.Fields()["chainer.output"])
}

func TestChainerExecuteAutoInitializes(t *testing.T) {
	agent := &ChainerAgent{
		Model: newScriptChatModel("out"),
		Chain: &Chain{Links: []Link{NewSummarizeLink("l1", nil, "chainer.output")}},
	}
	require.False(t, agent.initialised)

	result, err := agent.Execute(context.Background(), taskWith("goal"), nil)
	require.NoError(t, err, "Execute must lazily initialize")
	assert.True(t, result.Success)
	assert.True(t, agent.initialised)
	assert.NotNil(t, agent.Tools)
}

func TestChainerExecuteValidatesAndPropagatesErrors(t *testing.T) {
	invalid := New(&paradigm.Deps{Model: newScriptChatModel("x")},
		WithChain(&Chain{Links: []Link{{Name: "l", OutputKey: ""}}}))
	_, err := invalid.Execute(context.Background(), taskWith("goal"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output key required")

	unconfigured := &ChainerAgent{Model: newScriptChatModel("x")}
	_, err = unconfigured.Execute(context.Background(), taskWith("goal"), nil)
	require.EqualError(t, err, "chainer: chain not configured")

	builderBroken := &ChainerAgent{ChainBuilder: func(*execution.Task) (*Chain, error) { return nil, errBoomChainer }}
	_, err = builderBroken.Execute(context.Background(), taskWith("goal"), nil)
	require.ErrorIs(t, err, errBoomChainer)

	runnerFails := New(&paradigm.Deps{Model: nil},
		WithChain(&Chain{Links: []Link{NewSummarizeLink("l", nil, "out")}}))
	_, err = runnerFails.Execute(context.Background(), taskWith("goal"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model unavailable")
}

func TestChainerStreamDefaultsAndOverrides(t *testing.T) {
	agent := &ChainerAgent{}
	assert.Equal(t, contextstream.ModeBlocking, agent.streamMode())
	assert.Equal(t, "instruction", agent.streamQuery(taskWith("instruction")))
	assert.Equal(t, "", agent.streamQuery(nil))
	assert.Equal(t, 256, agent.streamMaxTokens())

	agent.StreamMode = contextstream.ModeBackground
	agent.StreamQuery = "  padded query  "
	agent.StreamMaxTokens = 12
	assert.Equal(t, contextstream.ModeBackground, agent.streamMode())
	assert.Equal(t, "padded query", agent.streamQuery(nil), "stream query is trimmed")
	assert.Equal(t, 12, agent.streamMaxTokens())

	agent.StreamQuery = "   "
	assert.Equal(t, "task instruction", agent.streamQuery(taskWith("task instruction")),
		"whitespace-only override falls back to the task instruction")
}

func TestChainerStreamTriggerNodeRequiresQuery(t *testing.T) {
	agent := &ChainerAgent{}
	assert.Nil(t, agent.streamTriggerNode(nil), "no query means no stream node")
	assert.Nil(t, agent.streamTriggerNode(taskWith("")), "empty instruction means no stream node")

	agent.StreamQuery = "q"
	node := agent.streamTriggerNode(taskWith("goal"))
	require.NotNil(t, node)
	streamNode, ok := node.(*agentgraph.StreamTriggerNode)
	require.True(t, ok)
	assert.Equal(t, "chainer_stream", node.ID())
	assert.Equal(t, contextstream.ModeBlocking, streamNode.Mode)
	assert.Equal(t, "emit_partial", streamNode.BudgetShortfallPolicy)
	assert.Equal(t, map[string]any{"agent": "chainer"}, streamNode.Metadata)
	assert.Equal(t, "q", streamNode.Query.Text)
}

func TestChainerExecuteStreamingTriggerSkipsWithoutQueryOrTrigger(t *testing.T) {
	agent := &ChainerAgent{StreamQuery: "q"}
	// No trigger in the context: the trigger step is a no-op, not an error.
	require.NoError(t, agent.executeStreamingTrigger(context.Background(), taskWith("t"), newTestEnvelope(t)))

	quiet := &ChainerAgent{}
	require.NoError(t, quiet.executeStreamingTrigger(context.Background(), taskWith("t"), newTestEnvelope(t)))
}

func TestChainerBuildGraph(t *testing.T) {
	agent := New(nil, WithChain(&Chain{Links: []Link{
		NewSummarizeLink("Explore Phase", nil, "out"),
	}}))

	g, err := agent.BuildGraph(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "chainer_link_00_explore_phase", g.StartNodeID(),
		"with an empty query (no stream config, no instruction) the first link starts the graph")

	// Any non-empty task instruction seeds the stream trigger node.
	g, err = agent.BuildGraph(context.Background(), taskWith("goal"))
	require.NoError(t, err)
	assert.Equal(t, "chainer_stream", g.StartNodeID(),
		"with a task instruction the stream node starts the graph")

	_, err = (&ChainerAgent{}).BuildGraph(context.Background(), taskWith("goal"))
	require.EqualError(t, err, "chainer: chain not configured")

	_, err = (&ChainerAgent{ChainBuilder: func(*execution.Task) (*Chain, error) {
		return nil, errBoomChainer
	}}).BuildGraph(context.Background(), taskWith("goal"))
	require.ErrorIs(t, err, errBoomChainer)
}

func TestChainerLinkNode(t *testing.T) {
	node := &chainerLinkNode{id: "chainer_link_00_l1", name: "l1"}
	assert.Equal(t, "chainer_link_00_l1", node.ID())
	assert.Equal(t, agentgraph.NodeTypeSystem, node.Type())

	env := newTestEnvelope(t)
	res, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "chainer_link_00_l1", res.NodeID)
	assert.Equal(t, "l1", envValue[string](t, env, "chainer.inspect_link"))

	// Nil envelope must not panic.
	res, err = (&chainerLinkNode{id: "x"}).Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, res.Success)
}

func TestSanitizeLinkName(t *testing.T) {
	assert.Equal(t, "explore_phase", sanitizeLinkName("Explore Phase"))
	assert.Equal(t, "a_b", sanitizeLinkName("a-b"))
	assert.Equal(t, "link", sanitizeLinkName(""))
	assert.Equal(t, "link", sanitizeLinkName("  "))
}

func TestChainerCapabilityRegistry(t *testing.T) {
	var nilAgent *ChainerAgent
	assert.Nil(t, nilAgent.CapabilityRegistry())

	reg := capability.NewRegistry()
	agent := &ChainerAgent{Tools: reg}
	assert.Same(t, reg, agent.CapabilityRegistry())
}

var errBoomChainer = errors.New("boom")
