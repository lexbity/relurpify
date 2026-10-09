package testhelper

import (
	"context"
	"sync"

	"codeburg.org/lexbit/relurpify/model"
)

// ScriptedModel is a deterministic model.LanguageModel for offline conformance
// runs: every Generate/Chat call returns the same Text, and every
// ChatWithTools call returns the same ToolCalls (or a plain-text decision when
// none are set). Callers can read the invocation count to pin behavior (e.g. a
// react `until` cap bounding the number of model calls).
type ScriptedModel struct {
	mu        sync.Mutex
	Text      string
	ToolCalls []model.ToolCall
	Calls     int
}

// NewScriptedModel creates a model that returns the given text on every call.
func NewScriptedModel(text string) *ScriptedModel {
	return &ScriptedModel{Text: text}
}

// WithToolCalls makes ChatWithTools return the given tool calls.
func (m *ScriptedModel) WithToolCalls(calls ...model.ToolCall) *ScriptedModel {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ToolCalls = calls
	return m
}

// InvocationCount returns how many Generate/Chat/ChatWithTools calls happened.
func (m *ScriptedModel) InvocationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls
}

func (m *ScriptedModel) record() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls++
}

// Generate returns the scripted text.
func (m *ScriptedModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.record()
	return &model.LLMResponse{Text: m.Text, ToolCalls: append([]model.ToolCall(nil), m.ToolCalls...)}, nil
}

// GenerateStream returns an immediately-closed empty token stream.
func (m *ScriptedModel) GenerateStream(ctx context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	m.record()
	ch := make(chan string)
	close(ch)
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return ch, nil
}

// Chat returns the scripted text.
func (m *ScriptedModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.record()
	return &model.LLMResponse{Text: m.Text, ToolCalls: append([]model.ToolCall(nil), m.ToolCalls...)}, nil
}

// ChatWithTools returns the scripted tool calls (or text when none are set).
func (m *ScriptedModel) ChatWithTools(_ context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.record()
	return &model.LLMResponse{Text: m.Text, ToolCalls: append([]model.ToolCall(nil), m.ToolCalls...)}, nil
}

var _ model.LanguageModel = (*ScriptedModel)(nil)
