package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm/offline"
)

type fallbackToolModelStub struct {
	generateCalls   int
	generatePrompt  string
	chatCalls       int
	chatToolCalls   int
	chatToolNames   []string
	generateReply   *model.LLMResponse
	chatToolReply   *model.LLMResponse
	maxToolsPerCall int
	native          bool
}

func (m *fallbackToolModelStub) Generate(_ context.Context, prompt string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.generateCalls++
	m.generatePrompt = prompt
	if m.generateReply != nil {
		return m.generateReply, nil
	}
	return &model.LLMResponse{Text: "plain answer", FinishReason: "stop"}, nil
}

func (m *fallbackToolModelStub) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *fallbackToolModelStub) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.chatCalls++
	return &model.LLMResponse{Text: "chat answer", FinishReason: "stop"}, nil
}

func (m *fallbackToolModelStub) ChatWithTools(_ context.Context, _ []model.Message, tools []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.chatToolCalls++
	m.chatToolNames = m.chatToolNames[:0]
	for _, tool := range tools {
		m.chatToolNames = append(m.chatToolNames, tool.Name)
	}
	if m.chatToolReply != nil {
		return m.chatToolReply, nil
	}
	return &model.LLMResponse{Text: "tool answer", FinishReason: "tool_calls"}, nil
}

func (m *fallbackToolModelStub) ToolRepairStrategy() string {
	return "llm"
}

func (m *fallbackToolModelStub) MaxToolsPerCall() int {
	return m.maxToolsPerCall
}

func (m *fallbackToolModelStub) UsesNativeToolCalling() bool {
	return m.native
}

func TestFallbackToolModel_ChatWithToolsParsesAndStrips(t *testing.T) {
	inner := &fallbackToolModelStub{
		generateReply: &model.LLMResponse{
			Text: "before\n```tool\n{\"tool\":\"file_read\",\"arguments\":{\"path\":\"main.go\"}}\n```\nafter",
		},
		maxToolsPerCall: 1,
	}
	m := &FallbackToolModel{inner: inner}

	resp, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "read"}}, []model.LLMToolSpec{
		{Name: "visible_tool"},
	}, &model.LLMOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, inner.generateCalls)
	require.Equal(t, 0, inner.chatToolCalls)
	require.Contains(t, inner.generatePrompt, "visible_tool")
	require.NotContains(t, inner.generatePrompt, "hidden_tool")
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "file_read", resp.ToolCalls[0].Name)
	require.Equal(t, map[string]any{"path": "main.go"}, resp.ToolCalls[0].Args)
	require.Equal(t, "tool_calls", resp.FinishReason)
	require.Contains(t, resp.Text, "before")
	require.Contains(t, resp.Text, "after")
	require.NotContains(t, resp.Text, "```tool")
}

func TestFallbackToolModel_RendersOnlyProvidedTools(t *testing.T) {
	inner := &fallbackToolModelStub{}
	m := &FallbackToolModel{inner: inner}

	_, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "read"}}, []model.LLMToolSpec{
		{Name: "file_read"},
	}, &model.LLMOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, inner.generateCalls)
	require.Contains(t, inner.generatePrompt, "file_read")
	require.NotContains(t, inner.generatePrompt, "file_edit")
}

func TestFallbackToolModel_RespectsMaxToolsPerCall(t *testing.T) {
	inner := &fallbackToolModelStub{
		generateReply: &model.LLMResponse{
			Text: strings.Join([]string{
				`{"tool":"one","arguments":{"a":1}}`,
				`{"tool":"two","arguments":{"b":2}}`,
			}, "\n"),
		},
		maxToolsPerCall: 1,
	}
	m := &FallbackToolModel{inner: inner}

	resp, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "run"}}, []model.LLMToolSpec{{Name: "one"}, {Name: "two"}}, &model.LLMOptions{})
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "one", resp.ToolCalls[0].Name)
}

func TestFallbackToolModel_ZeroCallsReturnsChatAnswer(t *testing.T) {
	inner := &fallbackToolModelStub{
		generateReply: &model.LLMResponse{Text: "plain answer", FinishReason: "stop"},
	}
	m := &FallbackToolModel{inner: inner}

	resp, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "run"}}, []model.LLMToolSpec{{Name: "one"}}, &model.LLMOptions{})
	require.NoError(t, err)
	require.Empty(t, resp.ToolCalls)
	require.Equal(t, "plain answer", resp.Text)
	require.Equal(t, "stop", resp.FinishReason)
}

func TestFallbackToolModel_FileListTraversalStopsAfterRead(t *testing.T) {
	inner := offline.NewModel()
	m := NewFallbackToolModel(inner)
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_list:docs"}}

	first, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "list docs"}}, []model.LLMToolSpec{{Name: "file_list"}, {Name: "file_read"}}, opts)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "file_list", first.ToolCalls[0].Name)

	second, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "list docs"},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "1", Name: "file_list", Args: map[string]any{"directory": "docs", "pattern": "*"}}}},
		{Role: "tool", Name: "file_list", Content: "success=true Listed files: [/tmp/ws/docs/other.txt /tmp/ws/docs/target.txt]", ToolCallID: "1"},
	}, []model.LLMToolSpec{{Name: "file_list"}, {Name: "file_read"}}, opts)
	require.NoError(t, err)
	require.Len(t, second.ToolCalls, 1)
	require.Equal(t, "file_read", second.ToolCalls[0].Name)

	third, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "list docs"},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "1", Name: "file_list", Args: map[string]any{"directory": "docs", "pattern": "*"}}}},
		{Role: "tool", Name: "file_list", Content: "success=true Listed files: [/tmp/ws/docs/other.txt /tmp/ws/docs/target.txt]", ToolCallID: "1"},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "2", Name: "file_read", Args: map[string]any{"path": "/tmp/ws/docs/target.txt"}}}},
		{Role: "tool", Name: "file_read", Content: "success=true Read /tmp/ws/docs/target.txt target payload", ToolCallID: "2"},
	}, []model.LLMToolSpec{{Name: "file_list"}, {Name: "file_read"}}, opts)
	require.NoError(t, err)
	require.Empty(t, third.ToolCalls)
	require.Contains(t, third.Text, "offline file_list result:")
	require.Contains(t, third.Text, "target payload")
}

type callingModeBackendStub struct {
	model   model.LanguageModel
	caps    BackendCapabilities
	profile *ModelProfile
	pulls   int
}

func (b *callingModeBackendStub) Model() LanguageModel {
	return b.model
}

func (b *callingModeBackendStub) Embedder() Embedder {
	return nil
}

func (b *callingModeBackendStub) Capabilities() BackendCapabilities {
	return b.caps
}

func (b *callingModeBackendStub) ModelContextSize(context.Context) (int, error) {
	return 0, nil
}

func (b *callingModeBackendStub) Health(context.Context) (*HealthReport, error) {
	return &HealthReport{State: BackendHealthReady}, nil
}

func (b *callingModeBackendStub) ListModels(context.Context) ([]ModelInfo, error) {
	return nil, nil
}

func (b *callingModeBackendStub) Warm(context.Context) error {
	return nil
}

func (b *callingModeBackendStub) Close() error {
	return nil
}

func (b *callingModeBackendStub) SetDebugLogging(bool) {}

func (b *callingModeBackendStub) SetProfile(profile *ModelProfile) {
	b.profile = profile.Clone()
}

func (b *callingModeBackendStub) Reset(context.Context, string) error {
	return nil
}

func (b *callingModeBackendStub) Pull(context.Context, string) error {
	b.pulls++
	return nil
}

type callingModeNativeModelStub struct {
	generateCalls  int
	chatCalls      int
	chatToolCalls  int
	generatePrompt string
	generateText   string
}

func (m *callingModeNativeModelStub) Generate(_ context.Context, prompt string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.generateCalls++
	m.generatePrompt = prompt
	if m.generateText != "" {
		return &model.LLMResponse{Text: m.generateText}, nil
	}
	return &model.LLMResponse{Text: "native generate"}, nil
}

func (m *callingModeNativeModelStub) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *callingModeNativeModelStub) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.chatCalls++
	return &model.LLMResponse{Text: "native chat"}, nil
}

func (m *callingModeNativeModelStub) ChatWithTools(_ context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.chatToolCalls++
	return &model.LLMResponse{
		Text:         "native tool answer",
		FinishReason: "tool_calls",
		ToolCalls:    []model.ToolCall{{Name: "native_tool", Args: map[string]any{"ok": true}}},
	}, nil
}

func TestFactoryWrapsFallbackModels(t *testing.T) {
	const providerName = "fallback-wrap-test"
	inner := &callingModeNativeModelStub{
		generateText: "before\n```tool\n{\"tool\":\"file_read\",\"arguments\":{\"path\":\"main.go\"}}\n```\nafter",
	}
	backend := &callingModeBackendStub{
		model: inner,
		caps:  BackendCapabilities{NativeToolCalling: false},
	}
	RegisterProvider(providerName, func(ProviderConfig, ProviderSecrets) (ManagedBackend, error) {
		return backend, nil
	})

	managed, err := New(ProviderConfig{Provider: providerName, Model: "test-model"}, ProviderSecrets{})
	require.NoError(t, err)
	managed.SetProfile(&ModelProfile{ToolCalling: model.ModelToolCalling{NativeAPI: false, MaxToolsPerCall: 1}})

	lm := managed.Model()
	_, ok := lm.(*FallbackToolModel)
	require.True(t, ok)

	resp, err := lm.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "read"}}, []model.LLMToolSpec{{Name: "visible_tool"}}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, inner.generateCalls)
	require.Equal(t, 0, inner.chatToolCalls)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "file_read", resp.ToolCalls[0].Name)
}

func TestFactoryLeavesNativeModelsUnwrapped(t *testing.T) {
	const providerName = "native-wrap-test"
	inner := &callingModeNativeModelStub{}
	backend := &callingModeBackendStub{
		model: inner,
		caps:  BackendCapabilities{NativeToolCalling: true},
	}
	RegisterProvider(providerName, func(ProviderConfig, ProviderSecrets) (ManagedBackend, error) {
		return backend, nil
	})

	managed, err := New(ProviderConfig{Provider: providerName, Model: "test-model"}, ProviderSecrets{})
	require.NoError(t, err)
	managed.SetProfile(&ModelProfile{ToolCalling: model.ModelToolCalling{NativeAPI: true, MaxToolsPerCall: 1}})

	lm := managed.Model()
	_, ok := lm.(*FallbackToolModel)
	require.False(t, ok)

	resp, err := lm.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "read"}}, []model.LLMToolSpec{{Name: "visible_tool"}}, nil)
	require.NoError(t, err)
	require.Equal(t, 0, inner.generateCalls)
	require.Equal(t, 1, inner.chatToolCalls)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "native_tool", resp.ToolCalls[0].Name)
}
