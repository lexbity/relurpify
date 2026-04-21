package testutil

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type ScenarioModelTurn struct {
	Method         string
	Response       *core.LLMResponse
	Err            error
	PromptContains []string
}

type ScenarioStubModel struct {
	mu    sync.Mutex
	turns []ScenarioModelTurn

	Calls []ScenarioModelCall
}

type ScenarioModelCall struct {
	Method       string
	Prompt       string
	ToolNames    []string
	MessageCount int
}

func NewScenarioStubModel(turns ...ScenarioModelTurn) *ScenarioStubModel {
	return &ScenarioStubModel{turns: append([]ScenarioModelTurn(nil), turns...)}
}

func (m *ScenarioStubModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return m.consume("generate", prompt, nil, 0)
}

func (m *ScenarioStubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *ScenarioStubModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return m.consume("chat", joinMessages(messages), nil, len(messages))
}

func (m *ScenarioStubModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return m.consume("chat_with_tools", joinMessages(messages), names, len(messages))
}

func (m *ScenarioStubModel) AssertExhausted(tb testing.TB) {
	tb.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.turns) != 0 {
		tb.Fatalf("scenario stub model has %d unconsumed turn(s)", len(m.turns))
	}
}

func (m *ScenarioStubModel) Turns() []ScenarioModelTurn {
	m.mu.Lock()
	defer m.mu.Unlock()

	turns := make([]ScenarioModelTurn, 0, len(m.turns))
	for _, turn := range m.turns {
		cloned := turn
		cloned.PromptContains = append([]string(nil), turn.PromptContains...)
		if turn.Response != nil {
			resp := *turn.Response
			resp.ToolCalls = append([]core.ToolCall(nil), turn.Response.ToolCalls...)
			cloned.Response = &resp
		}
		turns = append(turns, cloned)
	}
	return turns
}

func (m *ScenarioStubModel) consume(method, prompt string, toolNames []string, messageCount int) (*core.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, ScenarioModelCall{
		Method:       method,
		Prompt:       prompt,
		ToolNames:    append([]string(nil), toolNames...),
		MessageCount: messageCount,
	})
	if len(m.turns) == 0 {
		return nil, fmt.Errorf("scenario stub model received unexpected %s call", method)
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	if want := strings.TrimSpace(turn.Method); want != "" && want != method {
		return nil, fmt.Errorf("scenario stub model expected %s call, got %s", want, method)
	}
	for _, fragment := range turn.PromptContains {
		if strings.TrimSpace(fragment) == "" {
			continue
		}
		if !strings.Contains(prompt, fragment) {
			return nil, fmt.Errorf("scenario stub model expected prompt fragment %q in %s call", fragment, method)
		}
	}
	if turn.Err != nil {
		return nil, turn.Err
	}
	if turn.Response == nil {
		return &core.LLMResponse{Text: "{}"}, nil
	}
	resp := *turn.Response
	resp.ToolCalls = append([]core.ToolCall(nil), turn.Response.ToolCalls...)
	return &resp, nil
}

func joinMessages(messages []core.Message) string {
	if len(messages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		part := strings.TrimSpace(message.Role)
		if content := strings.TrimSpace(message.Content); content != "" {
			if part != "" {
				part += ": "
			}
			part += content
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n")
}
