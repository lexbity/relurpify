package blackboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
)

// bbScriptedModel is a minimal fake language model for authored-source
// specialist calls.
type bbScriptedModel struct {
	mu    sync.Mutex
	text  string
	calls int
}

func (m *bbScriptedModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.calls++
	text := m.text
	m.mu.Unlock()
	return &model.LLMResponse{Text: text}, nil
}

func (m *bbScriptedModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(context.Background(), nil, nil)
}

func (m *bbScriptedModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *bbScriptedModel) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

// recordingCapability records invocation order, optionally writes state keys
// before returning (the episode re-arm actuator), and reports its id.
type recordingCapability struct {
	id      string
	order   *[]string
	writes  map[string]string
	failure error
}

func (h *recordingCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *recordingCapability) Invoke(_ context.Context, state ports.State, _ map[string]any) (*ports.ToolResult, error) {
	*h.order = append(*h.order, h.id)
	if h.failure != nil {
		return nil, h.failure
	}
	for key, value := range h.writes {
		state.SetWorkingValue(key, value)
	}
	return &ports.ToolResult{Success: true, Data: map[string]any{"capability_id": h.id}}, nil
}

func bbTestDeps(mdl model.LanguageModel, reg *registry.CapabilityRegistry, grounder *knowledge.GroundingService) *paradigm.Deps {
	return &paradigm.Deps{
		Model:    mdl,
		Registry: reg,
		Config:   &execution.Config{Name: "blackboard-authored-test"},
		Grounder: grounder,
	}
}

func newRecordingRegistry(t *testing.T, order *[]string, caps ...*recordingCapability) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	for _, capHandler := range caps {
		capHandler.order = order
		if err := reg.RegisterInvocableCapability(context.Background(), capHandler); err != nil {
			t.Fatalf("register %s: %v", capHandler.id, err)
		}
	}
	return reg
}

func sourceIDs(names ...string) []AuthoredSource {
	out := make([]AuthoredSource, 0, len(names))
	for _, name := range names {
		out = append(out, AuthoredSource{Name: name})
	}
	return out
}

func TestDeclarationOrderAndGating(t *testing.T) {
	order := []string{}
	reg := newRecordingRegistry(t, &order,
		&recordingCapability{id: "euclo:cap.alpha"},
		&recordingCapability{id: "euclo:cap.beta"},
	)
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{Name: "alpha", Capability: "euclo:cap.alpha", Write: "state.alpha_out"},
		{
			Name:       "beta",
			When:       func(env *contextdata.Envelope) bool { return envString(env, "state.gate") == "ready" },
			WhenExpr:   "state.gate is ready",
			Read:       []string{"state.seed"},
			Capability: "euclo:cap.beta",
			Write:      "state.beta_out",
		},
	}))
	env := contextdata.NewEnvelope("task-bb-order", "session-bb")
	env.SetWorkingValueWithClass("state.gate", "ready", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("state.seed", "s1", contextdata.MemoryClassTask)

	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "board goal"}, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantOrder := []string{"euclo:cap.alpha", "euclo:cap.beta"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("execution order = %v, want %v (declaration order, gated)", order, wantOrder)
	}
	gotExecuted, _ := contextdata.GetTyped[[]string](env, "blackboard.sources_executed")
	if strings.Join(gotExecuted, ",") != "alpha,beta" {
		t.Fatalf("blackboard.sources_executed = %v, want [alpha beta]", gotExecuted)
	}
	if cycles, _ := contextdata.GetTyped[int](env, "blackboard.cycles"); cycles != 1 {
		t.Fatalf("blackboard.cycles = %d, want 1 (both sources ran in cycle 1)", cycles)
	}
	for _, key := range []string{"state.alpha_out", "state.beta_out"} {
		if value, ok := envelopeGet(env, key); !ok || fmt.Sprint(value) == "" {
			t.Fatalf("write target %q missing after run", key)
		}
	}
}

func TestGatedSourceIneligibleSkipsCycle(t *testing.T) {
	order := []string{}
	reg := newRecordingRegistry(t, &order, &recordingCapability{id: "euclo:cap.beta"})
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{
			Name:       "beta",
			When:       func(env *contextdata.Envelope) bool { return envString(env, "state.gate") == "ready" },
			Capability: "euclo:cap.beta",
			Write:      "state.beta_out",
		},
	}))
	env := contextdata.NewEnvelope("task-bb-gate", "session-bb")
	env.SetWorkingValueWithClass("state.gate", "missing", contextdata.MemoryClassTask)

	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "goal"}, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(order) != 0 {
		t.Fatalf("gated source executed with a false predicate: %v", order)
	}
	if executed, _ := contextdata.GetTyped[[]string](env, "blackboard.sources_executed"); len(executed) != 0 {
		t.Fatalf("sources_executed = %v, want empty", executed)
	}
	if termination, _ := contextdata.GetTyped[string](env, "blackboard.termination"); termination != "goal_satisfied" {
		t.Fatalf("termination = %q, want goal_satisfied", termination)
	}
}

func TestEpisodeRearm(t *testing.T) {
	order := []string{}
	reg := newRecordingRegistry(t, &order,
		&recordingCapability{id: "euclo:cap.probe"},
		&recordingCapability{id: "euclo:cap.flip", writes: map[string]string{"state.gate": "ready"}},
		&recordingCapability{id: "euclo:cap.break", writes: map[string]string{"state.gate": "missing"}},
	)
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{
			Name:       "probe",
			When:       func(env *contextdata.Envelope) bool { return envString(env, "state.gate") == "ready" },
			Capability: "euclo:cap.probe",
			Write:      "state.probe_out",
		},
		{
			Name:       "flip",
			When:       func(env *contextdata.Envelope) bool { return envString(env, "state.gate") == "missing" },
			Capability: "euclo:cap.flip",
			Write:      "state.flip_out",
		},
		{
			Name:       "break",
			Capability: "euclo:cap.break",
			Write:      "state.break_out",
		},
	}))
	env := contextdata.NewEnvelope("task-bb-rearm", "session-bb")
	env.SetWorkingValueWithClass("state.gate", "ready", contextdata.MemoryClassTask)

	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "goal"}, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// c1: probe (gate ready) + break (unconditional, gate→missing);
	// c2: flip (gate missing → ready); c3: probe re-armed (false→true
	// transition); c4: nothing eligible → goal_satisfied.
	wantOrder := []string{"euclo:cap.probe", "euclo:cap.break", "euclo:cap.flip", "euclo:cap.probe"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("execution order = %v, want %v", order, wantOrder)
	}
	if cycles, _ := contextdata.GetTyped[int](env, "blackboard.cycles"); cycles != 3 {
		t.Fatalf("blackboard.cycles = %d, want 3", cycles)
	}
	if termination, _ := contextdata.GetTyped[string](env, "blackboard.termination"); termination != "goal_satisfied" {
		t.Fatalf("termination = %q, want goal_satisfied", termination)
	}
}

func TestContinuouslyTruePredicateRunsOnce(t *testing.T) {
	order := []string{}
	reg := newRecordingRegistry(t, &order, &recordingCapability{id: "euclo:cap.steady"})
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{
			Name:       "steady",
			When:       func(env *contextdata.Envelope) bool { return true },
			Capability: "euclo:cap.steady",
			Write:      "state.steady_out",
		},
	}))
	env := contextdata.NewEnvelope("task-bb-steady", "session-bb")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "goal"}, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(order) != 1 {
		t.Fatalf("continuously-true source executed %d times, want exactly 1 (episode semantics)", len(order))
	}
}

func TestWriteGroundsWithProvenance(t *testing.T) {
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	store := &knowledge.ChunkStore{Graph: engine}
	grounder := knowledge.NewGroundingService(store, nil, nil, nil)
	grounder.SetClock(func() time.Time { return time.Unix(1700000000, 0).UTC() })

	// Seed the read input: state.note holds a grounded value.
	if _, err := grounder.Ground(context.Background(), []knowledge.GroundingItem{{
		Value:      "seed-note-1",
		Epistemics: knowledge.EpistemicClaimed,
		Origin:     contextdata.OriginLLM,
		StateKey:   "state.note",
		NodeID:     "seed",
		TaskID:     "task-bb-ground",
		Kind:       knowledge.ChunkKindCapture,
	}}); err != nil {
		t.Fatalf("seed grounding: %v", err)
	}

	order := []string{}
	reg := newRecordingRegistry(t, &order, &recordingCapability{id: "euclo:cap.echo"})
	agent := New(bbTestDeps(nil, reg, grounder), WithAuthoredSources([]AuthoredSource{
		{
			Name:       "echo",
			Read:       []string{"state.note"},
			Capability: "euclo:cap.echo",
			Write:      "state.echo_out",
		},
	}))
	env := contextdata.NewEnvelope("task-bb-ground", "session-bb")
	env.SetWorkingValueWithClass("state.note", "seed-note-1", contextdata.MemoryClassTask)

	ctx := context.Background()
	coord := agentgraph.NewEpochCoordinator(ctx, grounder, nil)
	ctx = agentgraph.WithEpochCoordinator(ctx, coord)
	if _, err := agent.Execute(ctx, &execution.Task{ID: "t", Instruction: "goal"}, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// The envelope write is synchronous (read-your-writes within the run).
	if value, ok := envelopeGet(env, "state.echo_out"); !ok {
		t.Fatal("write target state.echo_out missing from the envelope after the cycle")
	} else if fmt.Sprint(value) == "" {
		t.Fatal("write target state.echo_out empty")
	}
	// The durable leg landed at the cycle barrier: the chunk exists with the
	// write's state key and a derives_from edge to the read input.
	all, err := store.FindAll()
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	var outChunkID knowledge.ChunkID
	for _, chunk := range all {
		if key, _ := chunk.Body.Fields["state_key"].(string); key == "state.echo_out" {
			outChunkID = chunk.ID
		}
	}
	if outChunkID == "" {
		t.Fatalf("no grounded chunk for state.echo_out; chunks: %d", len(all))
	}
	edges, err := store.LoadEdgesFrom(outChunkID, knowledge.EdgeKindDerivesFrom)
	if err != nil {
		t.Fatalf("LoadEdgesFrom: %v", err)
	}
	noteID, ok, err := grounder.ChunkIDForCaptureValue("seed-note-1", "")
	if err != nil || !ok {
		t.Fatalf("read input chunk unresolved: %v (ok=%v)", err, ok)
	}
	found := false
	for _, edge := range edges {
		if edge.ToChunk == noteID {
			found = true
		}
	}
	if !found {
		t.Fatalf("grounded write carries no derives_from edge to the read input %s; edges: %v", noteID, edges)
	}
}

func TestGroundingFailureFailsCycle(t *testing.T) {
	order := []string{}
	reg := newRecordingRegistry(t, &order, &recordingCapability{id: "euclo:cap.echo"})
	agent := New(bbTestDeps(nil, reg, nil), WithAuthoredSources([]AuthoredSource{
		{Name: "echo", Capability: "euclo:cap.echo", Write: "state.echo_out"},
	}))
	env := contextdata.NewEnvelope("task-bb-fail", "session-bb")

	ctx := context.Background()
	coord := agentgraph.NewEpochCoordinator(ctx, &failingGrounder{}, nil)
	ctx = agentgraph.WithEpochCoordinator(ctx, coord)
	_, err := agent.Execute(ctx, &execution.Task{ID: "t", Instruction: "goal"}, env)
	if err == nil {
		t.Fatal("expected the grounding admission failure to fail the cycle")
	}
	if !errors.Is(err, knowledge.ErrGroundingFailed) {
		t.Fatalf("error = %v, want grounding_failed classification", err)
	}
}

// failingGrounder always rejects the batch (fault-injected admission).
type failingGrounder struct{}

func (f *failingGrounder) Ground(context.Context, []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	return knowledge.GroundingReport{}, fmt.Errorf("injected admission failure")
}

func TestBlackboardZeroOptionsUnchanged(t *testing.T) {
	mdl := &bbScriptedModel{text: "specialist output"}
	agent := New(bbTestDeps(mdl, registry.NewRegistry(), nil))
	if agent.authoredMode() {
		t.Fatal("zero options must keep the built-in specialist loop (FR-9)")
	}
	if len(agent.Sources) == 0 {
		if err := agent.Initialize(nil); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
	}
	if len(agent.Sources) != len(DefaultKnowledgeSources()) {
		t.Fatalf("built-in specialist set = %d sources, want the default set", len(agent.Sources))
	}
	env := contextdata.NewEnvelope("task-bb-zero", "session-bb")
	// The built-in path routes through the graph (bb_load/evaluate/dispatch):
	// whatever its outcome, the authored episode bookkeeping must be absent.
	_, _ = agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "explore the board"}, env)
	if _, ok := envelopeGet(env, episodeGenerationKey); ok {
		t.Fatal("authored episode state written on the built-in path")
	}
}

func envString(env *contextdata.Envelope, key string) string {
	value, ok := envelopeGet(env, key)
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}
