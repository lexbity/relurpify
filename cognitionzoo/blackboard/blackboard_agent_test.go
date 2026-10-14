package blackboard

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
	"codeburg.org/lexbit/relurpify/model"
)

// streamCompilerStub is a minimal contextstream.CompilerInvoker returning a
// canned compilation result.
type streamCompilerStub struct {
	result *contextports.CompilationResult
}

func (s *streamCompilerStub) Compile(_ context.Context, _ contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return s.result, nil
}

// bbStreamContext carries a streaming trigger so the built-in graph's stream
// trigger node can run without network access.
func bbStreamContext(ctx context.Context) context.Context {
	return contextstream.WithTrigger(ctx, contextstream.NewTrigger(&streamCompilerStub{
		result: &contextports.CompilationResult{},
	}))
}

// passthroughModel is a fake language model satisfying model.LanguageModel.
type passthroughModel struct {
	text  string
	calls int
}

func (m *passthroughModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.calls++
	return &model.LLMResponse{Text: m.text}, nil
}

func (m *passthroughModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, []model.Message{{Role: "user", Content: prompt}}, options)
}

func (m *passthroughModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- m.text
	close(ch)
	return ch, nil
}

func (m *passthroughModel) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

func TestNewWithoutDepsInitializesDefaults(t *testing.T) {
	agent := New(nil)
	require.NotNil(t, agent.Tools, "Initialize with nil deps still provisions a registry")
	assert.True(t, agent.initialised)
	assert.Equal(t, []string{"blackboard"}, agent.Capabilities())
	require.Len(t, agent.Sources, len(DefaultKnowledgeSources()),
		"Initialize populates the built-in specialist set immediately")
}

func TestNewWiresDeps(t *testing.T) {
	mdl := &passthroughModel{text: "ok"}
	reg := registry.NewRegistry()
	agent := New(&paradigm.Deps{
		Model:    mdl,
		Registry: reg,
		Config:   &execution.Config{Name: "unit"},
	})
	assert.Same(t, mdl, agent.Model)
	assert.Same(t, reg, agent.CapabilityRegistry())
	require.NotNil(t, agent.Config)
	assert.Equal(t, "unit", agent.Config.Name)
}

func TestInitializeDepsNilFallsBackToInitialize(t *testing.T) {
	agent := &BlackboardAgent{}
	require.NoError(t, agent.InitializeDeps(nil))
	assert.True(t, agent.initialised)
	require.NotNil(t, agent.Tools)
}

func TestOptions(t *testing.T) {
	sources := []KnowledgeSource{&ExplorerKS{}}
	streamModel := &passthroughModel{text: "x"}
	agent := New(
		&paradigm.Deps{Model: streamModel, Registry: registry.NewRegistry()},
		WithSources(sources),
		WithMaxCycles(9),
		WithContextStreamMode(contextstream.ModeBackground),
		WithContextStreamQuery("find flaky tests"),
		WithContextStreamMaxTokens(64),
	)
	assert.Len(t, agent.Sources, 1)
	assert.Equal(t, 9, agent.MaxCycles)
	assert.Equal(t, contextstream.ModeBackground, agent.streamMode())
	assert.Equal(t, "find flaky tests", agent.streamQuery(nil))
	assert.Equal(t, 64, agent.streamMaxTokens())

	// WithSources copies the slice.
	sources[0] = nil
	assert.NotNil(t, agent.Sources[0])
}

func TestStreamDefaults(t *testing.T) {
	agent := &BlackboardAgent{SemanticContext: execctx.AgentSemanticContext{}}
	assert.Equal(t, contextstream.ModeBlocking, agent.streamMode())
	assert.Equal(t, "task instruction", agent.streamQuery(&execution.Task{Instruction: "task instruction"}))
	assert.Equal(t, "", agent.streamQuery(nil))
	assert.Equal(t, 256, agent.streamMaxTokens())
}

func TestStreamTriggerNode(t *testing.T) {
	agent := &BlackboardAgent{}
	assert.Nil(t, agent.streamTriggerNode(&execution.Task{Instruction: "   "}), "blank query yields no stream node")
	assert.Nil(t, agent.streamTriggerNode(nil))

	agent.StreamQuery = "the query"
	agent.StreamMode = contextstream.ModeBackground
	agent.StreamMaxTokens = 32
	node := agent.streamTriggerNode(nil)
	require.NotNil(t, node)
	assert.Equal(t, "blackboard_stream", node.ID())
}

func TestWithAuthoredSourcesEmptyIsNoOp(t *testing.T) {
	agent := New(nil, WithAuthoredSources(nil))
	assert.False(t, agent.authoredMode())
	assert.Empty(t, agent.AuthoredSources())

	agent = New(nil, WithAuthoredSources([]AuthoredSource{{Name: "a", Write: "state.out"}}))
	assert.True(t, agent.authoredMode())
	authored := agent.AuthoredSources()
	require.Len(t, authored, 1)
	assert.Equal(t, "a", authored[0].Name)

	// Returned slice is detached.
	authored[0].Name = "mutated"
	assert.Equal(t, "a", agent.AuthoredSources()[0].Name)

	var nilAgent *BlackboardAgent
	assert.False(t, nilAgent.authoredMode())
	assert.Nil(t, nilAgent.AuthoredSources())
	assert.Nil(t, (*BlackboardAgent)(nil).CapabilityRegistry())
}

func TestMaxCyclesDefault(t *testing.T) {
	assert.Equal(t, defaultMaxCycles, maxCycles(0))
	assert.Equal(t, defaultMaxCycles, maxCycles(-3))
	assert.Equal(t, 4, maxCycles(4))
}

func TestBuildGraphWithoutInitialize(t *testing.T) {
	agent := New(&paradigm.Deps{Registry: registry.NewRegistry()}, WithMaxCycles(3))
	tool := &scriptedCapability{id: "unittest.tool", name: "unittest.tool"}
	require.NoError(t, agent.Tools.RegisterInvocableCapability(context.Background(), tool))

	g, err := agent.BuildGraph(context.Background(), &execution.Task{ID: "t1", Instruction: "goal"})
	require.NoError(t, err)
	require.NotNil(t, g)
	assert.True(t, agent.initialised, "BuildGraph lazily initializes")
}

func TestExecuteBuiltInSatisfiedFromPrepopulatedContext(t *testing.T) {
	agent := New(&paradigm.Deps{
		Config:   &execution.Config{Name: "unit"},
		Registry: registry.NewRegistry(),
	}, WithSources([]KnowledgeSource{&stubKS{name: "noop", canAct: func(*Blackboard) bool { return false }}}))

	env := contextdata.NewEnvelope("task-bb-ok", "session-1")
	bb := NewBlackboard("the goal")
	require.NoError(t, bb.AddArtifact("art", "report", "done body", "KS"))
	bb.VerifyArtifact("art")
	PublishToContext(env, bb, ControllerState{Cycle: 1, MaxCycles: 5, Termination: "running"})
	envelopeSet(env, contextKeyControllerCycle, 1)

	result, err := agent.Execute(bbStreamContext(context.Background()), &execution.Task{ID: "task-bb-ok", Instruction: "the goal"}, env)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestExecuteBuiltInStuck(t *testing.T) {
	agent := New(&paradigm.Deps{
		Config:   &execution.Config{Name: "unit"},
		Registry: registry.NewRegistry(),
	}, WithSources([]KnowledgeSource{&stubKS{name: "inert", canAct: func(*Blackboard) bool { return false }}}), WithMaxCycles(3))

	_, err := agent.Execute(bbStreamContext(context.Background()), &execution.Task{ID: "t", Instruction: "impossible"}, contextdata.NewEnvelope("t", "s"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no knowledge source can activate")
}

func TestExecuteBuiltInCycleLimit(t *testing.T) {
	agent := New(&paradigm.Deps{
		Config:   &execution.Config{Name: "unit"},
		Registry: registry.NewRegistry(),
	}, WithSources([]KnowledgeSource{&stubKS{
		name: "spin",
		act:  func(bb *Blackboard) { bb.AddFact(fmt.Sprintf("spin.%d", len(bb.Facts)), "1", "spin") },
	}}), WithMaxCycles(2))

	_, err := agent.Execute(bbStreamContext(context.Background()), &execution.Task{ID: "t", Instruction: "loop forever"}, contextdata.NewEnvelope("t", "s"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reached cycle limit (2)")
}

func TestExecuteBuiltInDispatchError(t *testing.T) {
	agent := New(&paradigm.Deps{
		Config:   &execution.Config{Name: "unit"},
		Registry: registry.NewRegistry(),
	}, WithSources([]KnowledgeSource{&stubKS{name: "boom", execErr: assert.AnError}}), WithMaxCycles(3))

	_, err := agent.Execute(bbStreamContext(context.Background()), &execution.Task{ID: "t", Instruction: "goal"}, contextdata.NewEnvelope("t", "s"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "graph execution failed")
}

func TestExecuteBuiltInFullSpecialistLoopSatisfiesGoal(t *testing.T) {
	// The default specialist set drives a board from exploration to a verified
	// artifact using only in-process behavior (no model, no capabilities).
	agent := New(&paradigm.Deps{
		Config:   &execution.Config{Name: "unit"},
		Registry: registry.NewRegistry(),
	})
	env := contextdata.NewEnvelope("task-loop", "session-1")
	result, err := agent.Execute(bbStreamContext(context.Background()), &execution.Task{ID: "task-loop", Instruction: "produce a result"}, env)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestExecuteNilEnvelopeAndTask(t *testing.T) {
	agent := New(&paradigm.Deps{Registry: registry.NewRegistry()}, WithSources([]KnowledgeSource{
		&stubKS{name: "inert", canAct: func(*Blackboard) bool { return false }},
	}))
	_, err := agent.Execute(bbStreamContext(context.Background()), nil, nil)
	require.Error(t, err, "no sources can activate on an empty board")
}

func TestExecuteAuthoredSetsTaskEnvelopeKeys(t *testing.T) {
	order := []string{}
	reg := newRecordingRegistry(t, &order, &recordingCapability{id: "euclo:cap.echo"})
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{Name: "echo", Capability: "euclo:cap.echo", Write: "state.out"},
	}))
	env := contextdata.NewEnvelope("task-authored", "session-1")
	task := &execution.Task{ID: "task-authored", Type: "review", Instruction: "review it"}
	_, err := agent.Execute(context.Background(), task, env)
	require.NoError(t, err)
	assert.Equal(t, "task-authored", envelopeGetString(env, "task.id"))
	assert.Equal(t, "review", envelopeGetString(env, "task.type"))
	assert.Equal(t, "review it", envelopeGetString(env, "task.instruction"))
}

func TestAuthoredSourceUnknownCapabilityFails(t *testing.T) {
	// Initialize always provisions a registry when none is configured, so a
	// pinned-but-unregistered capability surfaces as a lookup failure.
	agent := New(bbTestDeps(nil, nil, nil), WithAuthoredSources([]AuthoredSource{
		{Name: "echo", Capability: "euclo:cap.missing", Write: "state.out"},
	}))
	require.NotNil(t, agent.CapabilityRegistry())
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "goal"}, contextdata.NewEnvelope("t", "s"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability euclo:cap.missing not found")
}

func TestAuthoredSourceSpecialistWithoutModelFails(t *testing.T) {
	agent := New(bbTestDeps(nil, registry.NewRegistry(), nil), WithAuthoredSources([]AuthoredSource{
		{Name: "solo", Write: "state.out"},
	}))
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "goal"}, contextdata.NewEnvelope("t", "s"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a model")
}

func TestAuthoredSourceSpecialistUsesModel(t *testing.T) {
	mdl := &passthroughModel{text: "generated content"}
	agent := New(bbTestDeps(mdl, registry.NewRegistry(), nil), WithAuthoredSources([]AuthoredSource{
		{
			Name:  "writer",
			Read:  []string{"state.note"},
			Write: "state.out",
		},
	}))
	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.note", "the note", contextdata.MemoryClassTask)
	result, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "write it"}, env)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "generated content", envelopeGetString(env, "state.out"))
	require.Equal(t, 1, mdl.calls)
}

func TestAuthoredSourceCapabilityErrorFailsCycle(t *testing.T) {
	reg := newRegistryWith(t, &scriptedCapability{
		id:   "euclo:cap.failing",
		name: "failing",
		result: &ports.ToolResult{
			Success: false,
			Error:   "capability refused",
		},
	})
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{Name: "failing", Capability: "euclo:cap.failing", Write: "state.out"},
	}))
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "goal"}, contextdata.NewEnvelope("t", "s"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability refused")
}

func TestWorkspaceIDAndCappedChunkIDs(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	assert.Equal(t, "", workspaceID(env))
	env.SetWorkingValueWithClass("input.workspace", "ws-1", contextdata.MemoryClassTask)
	assert.Equal(t, "ws-1", workspaceID(env))

	assert.Empty(t, cappedChunkIDs(nil))
	ids := cappedChunkIDs([]contextdata.ChunkID{"a", "", "a", "b", "c"})
	require.Len(t, ids, 3)
	assert.Equal(t, knowledge.ChunkID("a"), ids[0])
	assert.Equal(t, knowledge.ChunkID("b"), ids[1])
	assert.Equal(t, knowledge.ChunkID("c"), ids[2])
}

func TestEpisodeCounting(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	_, ok := episodeCount(env, "src", episodeRanSuffix)
	assert.False(t, ok)

	setEpisodeCount(env, "src", episodeRanSuffix, 2)
	value, ok := episodeCount(env, "src", episodeRanSuffix)
	require.True(t, ok)
	assert.Equal(t, 2, value)

	// Wrong type is treated as absent.
	envelopeSet(env, episodeKey("other", episodeRanSuffix), "not-int")
	_, ok = episodeCount(env, "other", episodeRanSuffix)
	assert.False(t, ok)

	assert.Equal(t, "blackboard.episode.src.ran", episodeKey("src", episodeRanSuffix))
}

func TestTaskInstruction(t *testing.T) {
	assert.Equal(t, "", taskInstruction(nil))
	assert.Equal(t, "do it", taskInstruction(&execution.Task{Instruction: "do it"}))
}

// FuzzLoadFromContext exercises the blackboard state decode path with
// arbitrary untrusted JSON payloads stuffed into the namespaced envelope keys.
func FuzzLoadFromContext(f *testing.F) {
	seeds := []string{
		`{}`,
		`{"blackboard.goals":[]}`,
		`{"blackboard.goals":["a","b"]}`,
		`{"blackboard.facts":[{"Key":"k","Value":"v","Source":"s"}]}`,
		`{"blackboard.facts":[{"Key":1}]}`,
		`{"blackboard.facts":"scalar"}`,
		`{"blackboard.hypotheses":[{"ID":"h","Confidence":0.5}]}`,
		`{"blackboard.hypotheses":[{"Confidence":12}]}`,
		`{"blackboard.issues":[{"ID":"i","Severity":"high"}]}`,
		`{"blackboard.issues":[{"ID":"","Severity":"bogus"},{"ID":"dup"},{"ID":"dup"}]}`,
		`{"blackboard.pending_actions":[{"ID":"a","ToolOrAgent":"t","Args":{"x":1}}]}`,
		`{"blackboard.completed_actions":[{"RequestID":"r"}]}`,
		`{"blackboard.artifacts":[{"ID":"a","Kind":"k"},{"ID":"a","Kind":"k"}]}`,
		`{"blackboard.facts":[],"blackboard.hypotheses":[],"blackboard.issues":[],"blackboard.pending_actions":[],"blackboard.completed_actions":[],"blackboard.artifacts":[]}`,
		`{"blackboard.goals":null,"blackboard.facts":null}`,
		`{"blackboard.facts":[{"Derivation":{"steps":[{"id":"x"}]}}]}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Skip()
		}
		env := contextdata.NewEnvelope("fuzz", "fuzz")
		for key, value := range payload {
			envelopeSet(env, key, value)
		}
		bb := LoadFromContext(env, "goal")
		if bb == nil {
			t.Fatal("LoadFromContext must always return a board for non-nil state")
		}
		_ = bb.Validate()
		_ = bb.Metrics()
		_ = bb.Clone()
		// Publishing must either succeed or panic deterministically on invalid
		// invariants; it must never corrupt the process.
		if err := bb.Validate(); err == nil {
			out := contextdata.NewEnvelope("fuzz", "fuzz")
			PublishToContext(out, bb, ControllerState{})
		}
	})
}
