package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm/toolwire"
)

// FallbackToolModel adapts a model that does not use native tool calling.
// It renders the tool set into the prompt, delegates to Generate, and parses
// tool calls back out of the returned text.
type FallbackToolModel struct {
	inner LanguageModel
}

func NewFallbackToolModel(inner LanguageModel) *FallbackToolModel {
	return &FallbackToolModel{inner: inner}
}

func (m *FallbackToolModel) Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	if m == nil || m.inner == nil {
		return nil, nil
	}
	return m.inner.Generate(ctx, prompt, options)
}

func (m *FallbackToolModel) GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error) {
	if m == nil || m.inner == nil {
		return nil, nil
	}
	return m.inner.GenerateStream(ctx, prompt, options)
}

func (m *FallbackToolModel) Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error) {
	if m == nil || m.inner == nil {
		return nil, nil
	}
	return m.inner.Chat(ctx, messages, options)
}

func (m *FallbackToolModel) ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error) {
	if m == nil || m.inner == nil {
		return nil, nil
	}
	prompt := buildFallbackToolPrompt(messages, tools)
	resp, err := m.inner.Generate(ctx, prompt, options)
	if err != nil || resp == nil {
		return resp, err
	}

	calls := toolwire.ParseToolCallsFromText(resp.Text, m.MaxToolsPerCall())
	if len(calls) == 0 {
		return resp, nil
	}

	resp.ToolCalls = calls
	resp.FinishReason = "tool_calls"
	resp.Text = stripFallbackToolObjects(resp.Text)
	return resp, nil
}

func (m *FallbackToolModel) ToolRepairStrategy() string {
	if m != nil {
		if profiled, ok := m.inner.(model.ProfiledModel); ok {
			return profiled.ToolRepairStrategy()
		}
	}
	return "heuristic-only"
}

func (m *FallbackToolModel) MaxToolsPerCall() int {
	if m != nil {
		if profiled, ok := m.inner.(model.ProfiledModel); ok {
			return profiled.MaxToolsPerCall()
		}
	}
	return 0
}

func (m *FallbackToolModel) UsesNativeToolCalling() bool {
	return false
}

func buildFallbackToolPrompt(messages []Message, tools []LLMToolSpec) string {
	var b strings.Builder
	b.WriteString("Conversation:\n")
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "user"
		}
		fmt.Fprintf(&b, "[%s]\n%s\n\n", role, strings.TrimSpace(msg.Content))
	}
	b.WriteString("Available tools:\n")
	b.WriteString(toolwire.RenderToolsToPrompt(tools))
	b.WriteString("\nConversation policy:\n")
	b.WriteString("Return either prose or one or more fenced tool JSON objects.\n")
	return b.String()
}

func stripFallbackToolObjects(text string) string {
	text = stripFallbackToolFences(text)
	candidates := extractFallbackTopLevelJSONObjects(text)
	if len(candidates) == 0 {
		return strings.TrimSpace(text)
	}

	stripped := text
	for i := len(candidates) - 1; i >= 0; i-- {
		stripped = strings.Replace(stripped, candidates[i], "", 1)
	}
	return strings.TrimSpace(stripped)
}

func stripFallbackToolFences(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		switch {
		case line == "```tool":
			inFence = true
			continue
		case inFence && line == "```":
			inFence = false
			continue
		case inFence:
			continue
		default:
			out = append(out, strings.TrimRight(raw, "\r"))
		}
	}
	return strings.Join(out, "\n")
}

func extractFallbackTopLevelJSONObjects(text string) []string {
	var out []string
	start := -1
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, text[start:i+1])
				start = -1
			}
		}
	}
	return out
}

type callingModeManagedBackend struct {
	inner   ManagedBackend
	mu      sync.RWMutex
	profile *ModelProfile
}

func newCallingModeManagedBackend(inner ManagedBackend) ManagedBackend {
	if inner == nil {
		return nil
	}
	wrapped := &callingModeManagedBackend{inner: inner}
	if _, ok := inner.(PullableBackend); ok {
		return &pullableCallingModeManagedBackend{callingModeManagedBackend: wrapped}
	}
	return wrapped
}

func (b *callingModeManagedBackend) Model() LanguageModel {
	if b == nil || b.inner == nil {
		return nil
	}
	b.mu.RLock()
	profile := b.profile
	b.mu.RUnlock()
	return wrapForCallingMode(b.inner.Model(), b.inner.Capabilities(), profile)
}

func (b *callingModeManagedBackend) Embedder() Embedder {
	if b == nil || b.inner == nil {
		return nil
	}
	return b.inner.Embedder()
}

func (b *callingModeManagedBackend) Capabilities() BackendCapabilities {
	if b == nil || b.inner == nil {
		return BackendCapabilities{}
	}
	return b.inner.Capabilities()
}

func (b *callingModeManagedBackend) ModelContextSize(ctx context.Context) (int, error) {
	if b == nil || b.inner == nil {
		return 0, nil
	}
	return b.inner.ModelContextSize(ctx)
}

func (b *callingModeManagedBackend) Health(ctx context.Context) (*HealthReport, error) {
	if b == nil || b.inner == nil {
		return nil, nil
	}
	return b.inner.Health(ctx)
}

func (b *callingModeManagedBackend) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if b == nil || b.inner == nil {
		return nil, nil
	}
	return b.inner.ListModels(ctx)
}

func (b *callingModeManagedBackend) Warm(ctx context.Context) error {
	if b == nil || b.inner == nil {
		return nil
	}
	return b.inner.Warm(ctx)
}

func (b *callingModeManagedBackend) Close() error {
	if b == nil || b.inner == nil {
		return nil
	}
	return b.inner.Close()
}

func (b *callingModeManagedBackend) SetDebugLogging(enabled bool) {
	if b == nil || b.inner == nil {
		return
	}
	b.inner.SetDebugLogging(enabled)
}

func (b *callingModeManagedBackend) SetProfile(profile *ModelProfile) {
	if b == nil || b.inner == nil {
		return
	}
	clone := profile.Clone()
	b.mu.Lock()
	b.profile = clone
	b.mu.Unlock()
	b.inner.SetProfile(clone)
}

func (b *callingModeManagedBackend) Reset(ctx context.Context, strategy string) error {
	if b == nil || b.inner == nil {
		return nil
	}
	return b.inner.Reset(ctx, strategy)
}

type pullableCallingModeManagedBackend struct {
	*callingModeManagedBackend
}

func (b *pullableCallingModeManagedBackend) Pull(ctx context.Context, model string) error {
	if b == nil || b.inner == nil {
		return nil
	}
	pullable, _ := b.inner.(PullableBackend)
	return pullable.Pull(ctx, model)
}

func wrapForCallingMode(inner LanguageModel, caps BackendCapabilities, profile *ModelProfile) LanguageModel {
	if inner == nil {
		return nil
	}
	if _, ok := inner.(*FallbackToolModel); ok {
		return inner
	}
	if shouldUseNativeToolCalling(caps, profile) {
		return inner
	}
	return &FallbackToolModel{inner: inner}
}

func shouldUseNativeToolCalling(caps BackendCapabilities, profile *ModelProfile) bool {
	if !caps.NativeToolCalling {
		return false
	}
	if profile == nil {
		return true
	}
	return profile.ToolCalling.NativeAPI
}

var _ model.ProfiledModel = (*FallbackToolModel)(nil)
var _ ManagedBackend = (*callingModeManagedBackend)(nil)
var _ ManagedBackend = (*pullableCallingModeManagedBackend)(nil)
var _ PullableBackend = (*pullableCallingModeManagedBackend)(nil)
