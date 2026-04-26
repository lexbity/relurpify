package chainer_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer"
	chainctx "codeburg.org/lexbit/relurpify/agents/chainer/context"
	"codeburg.org/lexbit/relurpify/agents/chainer/telemetry"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

type captureModel struct {
	responses []string
	calls     int
	messages  [][]core.Message
}

func (m *captureModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *captureModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}
func (m *captureModel) Chat(_ context.Context, messages []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	copied := make([]core.Message, len(messages))
	copy(copied, messages)
	m.messages = append(m.messages, copied)
	text := ""
	if m.calls < len(m.responses) {
		text = m.responses[m.calls]
	}
	m.calls++
	return &core.LLMResponse{Text: text}, nil
}
func (m *captureModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	// For pipeline tests, just return the next response without tool calling
	text := ""
	if m.calls < len(m.responses) {
		text = m.responses[m.calls]
	}
	m.calls++
	return &core.LLMResponse{Text: text}, nil
}

func TestChain_Validate_EmptyName(t *testing.T) {
	chain := &chainer.Chain{Links: []chainer.Link{{OutputKey: "x"}}}
	if err := chain.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestChain_Validate_EmptyOutputKey(t *testing.T) {
	chain := &chainer.Chain{Links: []chainer.Link{{Name: "x"}}}
	if err := chain.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterState_OnlyDeclaredKeys(t *testing.T) {
	state := core.NewContext()
	state.Set("a", 1)
	state.Set("b", 2)
	state.Set("c", 3)
	filtered := chainer.FilterState(state, []string{"a", "b"})
	if len(filtered) != 2 || filtered["a"] != 1 || filtered["b"] != 2 {
		t.Fatalf("unexpected filtered state: %+v", filtered)
	}
	if _, ok := filtered["c"]; ok {
		t.Fatal("unexpected key c")
	}
}

func TestChainRunner_WritesOutputKey(t *testing.T) {
	model := &captureModel{responses: []string{"hello"}}
	state := core.NewContext()
	chain := &chainer.Chain{Links: []chainer.Link{
		chainer.NewLink("one", "Prompt", nil, "out", nil),
	}}
	if err := chainer.RunChain(context.Background(), model, &core.Task{Instruction: "go"}, chain, state); err != nil {
		t.Fatalf("RunChain: %v", err)
	}
	if got := state.GetString("out"); got != "hello" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestChainRunner_RetryOnParseFailure(t *testing.T) {
	model := &captureModel{responses: []string{"bad", "2"}}
	state := core.NewContext()
	parseCalls := 0
	chain := &chainer.Chain{Links: []chainer.Link{{
		Name:         "one",
		SystemPrompt: "Prompt",
		OutputKey:    "out",
		OnFailure:    chainer.FailurePolicyRetry,
		MaxRetries:   1,
		Parse: func(text string) (any, error) {
			parseCalls++
			if parseCalls == 1 {
				return nil, errors.New("bad parse")
			}
			return strconv.Atoi(text)
		},
	}}}
	if err := chainer.RunChain(context.Background(), model, &core.Task{Instruction: "go"}, chain, state); err != nil {
		t.Fatalf("RunChain: %v", err)
	}
	if model.calls != 2 {
		t.Fatalf("expected 2 model calls, got %d", model.calls)
	}
	if value, _ := state.Get("out"); value != 2 {
		t.Fatalf("unexpected parsed output: %+v", value)
	}
}

func TestChainRunner_FailFastOnParseFailure(t *testing.T) {
	model := &captureModel{responses: []string{"bad"}}
	chain := &chainer.Chain{Links: []chainer.Link{{
		Name:         "one",
		SystemPrompt: "Prompt",
		OutputKey:    "out",
		OnFailure:    chainer.FailurePolicyFailFast,
		Parse: func(text string) (any, error) {
			return nil, errors.New("bad parse")
		},
	}}}
	err := chainer.RunChain(context.Background(), model, &core.Task{Instruction: "go"}, chain, core.NewContext())
	if !errors.Is(err, chainer.ErrLinkParseFailure) {
		t.Fatalf("expected ErrLinkParseFailure, got %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("expected 1 model call, got %d", model.calls)
	}
}

func TestChainerAgent_ImplementsGraphAgent(t *testing.T) {
	agent := &chainer.ChainerAgent{
		Chain: &chainer.Chain{Links: []chainer.Link{
			chainer.NewLink("one", "Prompt", nil, "out", nil),
		}},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(agent.Capabilities()) == 0 {
		t.Fatal("expected capabilities")
	}
	g, err := agent.BuildGraph(&core.Task{Instruction: "go"})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Fatal("expected graph")
	}
}

func TestChainerAgent_SequentialLinks(t *testing.T) {
	model := &captureModel{responses: []string{"first", "second"}}
	agent := &chainer.ChainerAgent{
		Model: model,
		Chain: &chainer.Chain{Links: []chainer.Link{
			chainer.NewLink("one", "first", nil, "out.one", nil),
			chainer.NewLink("two", "second {{.Input.out.one}}", []string{"out.one"}, "out.two", nil),
		}},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{Instruction: "go"}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if state.GetString("out.one") != "first" || state.GetString("out.two") != "second" {
		t.Fatalf("missing outputs: %+v", state.StateSnapshot())
	}
}

func TestChainerAgent_InputKeyIsolation(t *testing.T) {
	model := &captureModel{responses: []string{"first", "second"}}
	agent := &chainer.ChainerAgent{
		Model: model,
		Chain: &chainer.Chain{Links: []chainer.Link{
			chainer.NewLink("one", "visible {{.Instruction}}", nil, "out.one", nil),
			chainer.NewLink("two", "only instruction {{.Instruction}}", nil, "out.two", nil),
		}},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := agent.Execute(context.Background(), &core.Task{Instruction: "go"}, core.NewContext()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(model.messages) < 2 {
		t.Fatalf("expected captured messages")
	}
	if strings.Contains(model.messages[1][0].Content, "first") {
		t.Fatalf("second link prompt leaked prior output: %q", model.messages[1][0].Content)
	}
}

// Phase 2: Pipeline-based execution tests (integration)
// NOTE: Full pipeline integration tests require Ollama and will be added in Phase 2 follow-up.
// The checkpoint infrastructure (Store, RecoveryManager) is fully tested in checkpoint tests.

func TestChainerAgent_CheckpointStoreConfigurable(t *testing.T) {
	// Verify that CheckpointStore can be configured on the agent
	model := &captureModel{responses: []string{"first", "second"}}
	store := &testCheckpointStore{checkpoints: make(map[string]*pipeline.Checkpoint)}

	agent := &chainer.ChainerAgent{
		Model:                model,
		CheckpointStore:      store,
		CheckpointAfterStage: true,
	}

	if agent.CheckpointStore == nil {
		t.Fatal("CheckpointStore not set")
	}

	if !agent.CheckpointAfterStage {
		t.Fatal("CheckpointAfterStage not set")
	}
}

func TestChainerAgent_RecoveryManagerInitialized(t *testing.T) {
	// Verify that RecoveryManager is initialized when CheckpointStore is configured
	store := &testCheckpointStore{checkpoints: make(map[string]*pipeline.Checkpoint)}

	agent := &chainer.ChainerAgent{
		CheckpointStore: store,
	}

	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if agent.RecoveryManager == nil {
		t.Fatal("RecoveryManager not initialized")
	}
}

// Test checkpoint store implementation

type testCheckpointStore struct {
	checkpoints map[string]*pipeline.Checkpoint
}

func (s *testCheckpointStore) Save(cp *pipeline.Checkpoint) error {
	if cp == nil || cp.TaskID == "" || cp.CheckpointID == "" {
		return strconv.ErrSyntax
	}
	key := cp.TaskID + ":" + cp.CheckpointID
	s.checkpoints[key] = cp
	return nil
}

func (s *testCheckpointStore) Load(taskID, checkpointID string) (*pipeline.Checkpoint, error) {
	key := taskID + ":" + checkpointID
	cp, ok := s.checkpoints[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return cp, nil
}

func (s *testCheckpointStore) Count(taskID string) int {
	count := 0
	for key := range s.checkpoints {
		if strings.HasPrefix(key, taskID+":") {
			count++
		}
	}
	return count
}

// Phase 3: Budget Manager Integration Tests

func TestChainerAgent_BudgetManagerConfigurable(t *testing.T) {
	// Verify that BudgetManager can be configured on ChainerAgent
	budgetManager := chainctx.NewBudgetManager(1000)

	agent := &chainer.ChainerAgent{
		BudgetManager: budgetManager,
	}

	if agent.BudgetManager == nil {
		t.Fatal("BudgetManager not configured")
	}

	if agent.BudgetManager != budgetManager {
		t.Fatal("BudgetManager not properly assigned")
	}
}

func TestChainerAgent_BudgetMetricsInContext(t *testing.T) {
	// Verify that budget metrics can be retrieved from BudgetManager
	budgetManager := chainctx.NewBudgetManager(1000)

	// Track some tokens
	_ = budgetManager.Track("llm", 500)

	// Get metrics
	metrics := budgetManager.Budget()

	if metrics == nil {
		t.Fatal("expected budget metrics")
	}

	// Verify metric structure
	total, hasTotal := metrics["total"]
	used, hasUsed := metrics["used"]
	remaining, hasRemaining := metrics["remaining"]

	if !hasTotal || !hasUsed || !hasRemaining {
		t.Fatal("expected total, used, and remaining in metrics")
	}

	if total.(int) != 1000 {
		t.Errorf("expected total 1000, got %d", total)
	}

	if used.(int) != 500 {
		t.Errorf("expected used 500, got %d", used)
	}

	if remaining.(int) != 500 {
		t.Errorf("expected remaining 500, got %d", remaining)
	}
}

// Phase 4: Telemetry & Observability Tests

func TestChainerAgent_EventRecorderConfigurable(t *testing.T) {
	// Verify that EventRecorder can be configured on ChainerAgent
	recorder := telemetry.NewEventRecorder()

	agent := &chainer.ChainerAgent{
		EventRecorder: recorder,
	}

	if agent.EventRecorder == nil {
		t.Fatal("EventRecorder not configured")
	}

	if agent.EventRecorder != recorder {
		t.Fatal("EventRecorder not properly assigned")
	}
}

func TestChainerAgent_ExecutionEventsQuery(t *testing.T) {
	// Verify that execution events can be queried
	recorder := telemetry.NewEventRecorder()

	event1 := telemetry.LinkStartEvent("task-1", "step1", 0, []string{"input"}, "output")
	event2 := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result")

	recorder.Record(event1)
	recorder.Record(event2)

	agent := &chainer.ChainerAgent{
		EventRecorder: recorder,
	}

	events := agent.ExecutionEvents("task-1")
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Verify events are in order
	if events[0].Kind != telemetry.KindLinkStart {
		t.Errorf("expected first event to be LinkStart")
	}

	if events[1].Kind != telemetry.KindLinkFinish {
		t.Errorf("expected second event to be LinkFinish")
	}
}

func TestChainerAgent_LinkEventsQuery(t *testing.T) {
	// Verify that link-specific events can be queried
	recorder := telemetry.NewEventRecorder()

	recorder.Record(telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output1"))
	recorder.Record(telemetry.LinkFinishEvent("task-1", "step1", 0, "output1", "result1"))
	recorder.Record(telemetry.LinkStartEvent("task-1", "step2", 1, nil, "output2"))

	agent := &chainer.ChainerAgent{
		EventRecorder: recorder,
	}

	step1Events := agent.LinkEvents("task-1", "step1")
	if len(step1Events) != 2 {
		t.Errorf("expected 2 events for step1, got %d", len(step1Events))
	}

	step2Events := agent.LinkEvents("task-1", "step2")
	if len(step2Events) != 1 {
		t.Errorf("expected 1 event for step2, got %d", len(step2Events))
	}
}

func TestChainerAgent_ExecutionSummaryQuery(t *testing.T) {
	// Verify that execution summary can be generated
	recorder := telemetry.NewEventRecorder()

	recorder.Record(telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output1"))
	recorder.Record(telemetry.LinkFinishEvent("task-1", "step1", 0, "output1", "result1"))
	recorder.Record(telemetry.LinkStartEvent("task-1", "step2", 1, nil, "output2"))
	recorder.Record(telemetry.LinkErrorEvent("task-1", "step2", 1, "error", "NetworkError"))

	agent := &chainer.ChainerAgent{
		EventRecorder: recorder,
	}

	summary := agent.ExecutionSummary("task-1")
	if summary == nil {
		t.Fatal("expected summary")
	}

	if summary.TaskID != "task-1" {
		t.Errorf("expected taskID task-1")
	}

	if summary.SuccessfulLinks != 1 {
		t.Errorf("expected 1 successful link, got %d", summary.SuccessfulLinks)
	}

	if summary.FailedLinks != 1 {
		t.Errorf("expected 1 failed link, got %d", summary.FailedLinks)
	}
}

func TestChainerAgent_EventRecorderOptional(t *testing.T) {
	// Verify that EventRecorder is optional (nil-safe)
	agent := &chainer.ChainerAgent{
		// No EventRecorder configured
	}

	events := agent.ExecutionEvents("task-1")
	if events != nil {
		t.Fatal("expected nil for unconfigured EventRecorder")
	}

	summary := agent.ExecutionSummary("task-1")
	if summary != nil {
		t.Fatal("expected nil for unconfigured EventRecorder")
	}

	linkEvents := agent.LinkEvents("task-1", "step1")
	if linkEvents != nil {
		t.Fatal("expected nil for unconfigured EventRecorder")
	}
}
