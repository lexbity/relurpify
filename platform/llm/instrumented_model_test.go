package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type llmEventSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *llmEventSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *llmEventSink) Snapshot() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]telemetry.Event, len(s.events))
	copy(out, s.events)
	return out
}

type profileAwareStubModel struct {
	profile *ModelProfile
}

func (m *profileAwareStubModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *profileAwareStubModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) SetProfile(profile *ModelProfile) {
	m.profile = profile
}

func (m *profileAwareStubModel) ToolRepairStrategy() string {
	if m.profile == nil {
		return "heuristic-only"
	}
	return m.profile.Repair.Strategy
}

func (m *profileAwareStubModel) MaxToolsPerCall() int {
	if m.profile == nil {
		return 0
	}
	return m.profile.ToolCalling.MaxToolsPerCall
}

func (m *profileAwareStubModel) UsesNativeToolCalling() bool {
	return m.profile != nil && m.profile.ToolCalling.NativeAPI
}

func TestInstrumentedModel_ProxiesProfileAwareBehavior(t *testing.T) {
	inner := &profileAwareStubModel{}
	instrumented := NewInstrumentedModel(inner, nil, false)

	profile := &ModelProfile{}
	profile.ToolCalling.NativeAPI = true
	profile.ToolCalling.MaxToolsPerCall = 2
	profile.Repair.Strategy = "llm"

	instrumented.SetProfile(profile)

	require.NotNil(t, inner.profile)
	require.True(t, instrumented.UsesNativeToolCalling())
	require.Equal(t, "llm", instrumented.ToolRepairStrategy())
	require.Equal(t, 2, instrumented.MaxToolsPerCall())

	_, ok := any(instrumented).(model.ProfiledModel)
	require.True(t, ok)
}

func TestInstrumentedModel_IngestsLLMResponse(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	store := &knowledge.ChunkStore{Graph: engine}
	ing := knowledge.NewOutputIngester(store, &knowledge.EventBus{})
	env := contextdata.NewEnvelope("task-1", "session-1")
	ctx := knowledge.WithOutputIngester(contextdata.WithEnvelope(context.Background(), env), ing)

	instrumented := NewInstrumentedModel(stubResponseModel{}, nil, false)
	resp, err := instrumented.Chat(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Text)

	require.Eventually(t, func() bool {
		chunks, err := store.FindByContentHash(hashText("hello"))
		return err == nil && len(chunks) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestInstrumentedModel_IngestsLLMResponse_NonBlocking(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	store := &knowledge.ChunkStore{Graph: engine}
	ing := knowledge.NewOutputIngester(store, &knowledge.EventBus{})
	env := contextdata.NewEnvelope("task-1", "session-1")
	ctx := knowledge.WithOutputIngester(contextdata.WithEnvelope(context.Background(), env), ing)

	instrumented := NewInstrumentedModel(stubResponseModel{}, nil, false)
	start := time.Now()
	resp, err := instrumented.Chat(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Text)
	require.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestInstrumentedModel_EmitsSessionResetRequired(t *testing.T) {
	advisor := &telemetry.ContextBudgetAdvisor{ModelContextSize: 1024}
	sink := &llmEventSink{}
	instrumented := NewInstrumentedModel(stubUsageResponseModel{}, sink, false)
	ctx := telemetry.WithAdvisor(context.Background(), advisor)
	ctx = telemetry.WithSnapshotEmitter(ctx, telemetry.NewSnapshotEmitter(advisor, sink, 1))

	_, err := instrumented.Chat(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		events := sink.Snapshot()
		for _, event := range events {
			if event.Type == telemetry.EventSessionResetRequired {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
	events := sink.Snapshot()
	var resetEvent *telemetry.Event
	for i := range events {
		if events[i].Type == telemetry.EventSessionResetRequired {
			resetEvent = &events[i]
			break
		}
	}
	require.NotNil(t, resetEvent)
	snapshot, ok := resetEvent.Metadata["budget_snapshot"].(telemetry.BudgetSnapshot)
	require.True(t, ok)
	require.True(t, snapshot.ShouldReset)
}

type stubResponseModel struct{}

func (stubResponseModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "hello", FinishReason: "stop"}, nil
}

func (stubResponseModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubResponseModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "hello", FinishReason: "stop"}, nil
}

func (stubResponseModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "hello", FinishReason: "stop"}, nil
}

type stubUsageResponseModel struct{}

func (stubUsageResponseModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        telemetry.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (stubUsageResponseModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubUsageResponseModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        telemetry.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (stubUsageResponseModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        telemetry.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}
