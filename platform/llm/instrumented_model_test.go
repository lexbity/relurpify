package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextbudget"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

type llmEventSink struct {
	mu     sync.Mutex
	events []contracts.Event
}

func (s *llmEventSink) Emit(event contracts.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *llmEventSink) Snapshot() []contracts.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contracts.Event, len(s.events))
	copy(out, s.events)
	return out
}

type profileAwareStubModel struct {
	profile *ModelProfile
}

func (m *profileAwareStubModel) Generate(context.Context, string, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *profileAwareStubModel) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "ok"}, nil
}

func (m *profileAwareStubModel) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "ok"}, nil
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
	model := NewInstrumentedModel(inner, nil, false)

	profile := &ModelProfile{}
	profile.ToolCalling.NativeAPI = true
	profile.ToolCalling.MaxToolsPerCall = 2
	profile.Repair.Strategy = "llm"

	model.SetProfile(profile)

	require.NotNil(t, inner.profile)
	require.True(t, model.UsesNativeToolCalling())
	require.Equal(t, "llm", model.ToolRepairStrategy())
	require.Equal(t, 2, model.MaxToolsPerCall())

	_, ok := any(model).(contracts.ProfiledModel)
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

	model := NewInstrumentedModel(stubResponseModel{}, nil, false)
	resp, err := model.Chat(ctx, []contracts.Message{{Role: "user", Content: "ping"}}, nil)
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

	model := NewInstrumentedModel(stubResponseModel{}, nil, false)
	start := time.Now()
	resp, err := model.Chat(ctx, []contracts.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Text)
	require.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestInstrumentedModel_EmitsSessionResetRequired(t *testing.T) {
	advisor := &contextbudget.ContextBudgetAdvisor{ModelContextSize: 1024}
	sink := &llmEventSink{}
	model := NewInstrumentedModel(stubUsageResponseModel{}, sink, false)
	ctx := contextbudget.WithAdvisor(context.Background(), advisor)
	ctx = contextbudget.WithSnapshotEmitter(ctx, contextbudget.NewSnapshotEmitter(advisor, sink, 1))

	_, err := model.Chat(ctx, []contracts.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		events := sink.Snapshot()
		for _, event := range events {
			if event.Type == contracts.EventSessionResetRequired {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
	events := sink.Snapshot()
	var resetEvent *contracts.Event
	for i := range events {
		if events[i].Type == contracts.EventSessionResetRequired {
			resetEvent = &events[i]
			break
		}
	}
	require.NotNil(t, resetEvent)
	snapshot, ok := resetEvent.Metadata["budget_snapshot"].(contextbudget.BudgetSnapshot)
	require.True(t, ok)
	require.True(t, snapshot.ShouldReset)
}

type stubResponseModel struct{}

func (stubResponseModel) Generate(context.Context, string, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "hello", FinishReason: "stop"}, nil
}

func (stubResponseModel) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubResponseModel) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "hello", FinishReason: "stop"}, nil
}

func (stubResponseModel) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "hello", FinishReason: "stop"}, nil
}

type stubUsageResponseModel struct{}

func (stubUsageResponseModel) Generate(context.Context, string, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        contracts.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (stubUsageResponseModel) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubUsageResponseModel) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        contracts.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (stubUsageResponseModel) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        contracts.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}
