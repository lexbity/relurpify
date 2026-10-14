package chainer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

// scriptChatModel answers Chat calls from a scripted queue.
type scriptChatModel struct {
	mu        sync.Mutex
	responses []chatResponse
	calls     [][]model.Message
}

type chatResponse struct {
	text string
	err  error
}

func newScriptChatModel(texts ...string) *scriptChatModel {
	m := &scriptChatModel{}
	for _, text := range texts {
		m.responses = append(m.responses, chatResponse{text: text})
	}
	return m
}

func (m *scriptChatModel) Chat(_ context.Context, messages []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, append([]model.Message(nil), messages...))
	if len(m.responses) == 0 {
		return nil, errors.New("script exhausted")
	}
	next := m.responses[0]
	m.responses = m.responses[1:]
	if next.err != nil {
		return nil, next.err
	}
	return &model.LLMResponse{Text: next.text}, nil
}

func (m *scriptChatModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, errors.New("Generate not scripted")
}

func (m *scriptChatModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	return nil, errors.New("GenerateStream not scripted")
}

func (m *scriptChatModel) ChatWithTools(_ context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, errors.New("ChatWithTools not scripted")
}

func (m *scriptChatModel) chatCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *scriptChatModel) lastMessages() []model.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return m.calls[len(m.calls)-1]
}

// resolvingRegistry matches the structural interface resolveSystemPrompt
// type-asserts on: Resolve(id string, ctx any) (string, error).
type resolvingRegistry struct {
	resolved map[string]string
	err      error
	seenIDs  []string
}

func (r *resolvingRegistry) Resolve(id string, _ any) (string, error) {
	r.seenIDs = append(r.seenIDs, id)
	if r.err != nil {
		return "", r.err
	}
	if text, ok := r.resolved[id]; ok {
		return text, nil
	}
	return "", errors.New("unknown prompt id")
}

func newTestEnvelope(t *testing.T) *contextdata.Envelope {
	t.Helper()
	return contextdata.NewEnvelope("task", "session")
}

func taskWith(instruction string) *execution.Task {
	return &execution.Task{ID: "task", Instruction: instruction}
}
