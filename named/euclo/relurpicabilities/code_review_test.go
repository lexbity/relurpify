package relurpicabilities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type mockReviewModel struct {
	err   error
	reply func(prompt string) string
}

func (m *mockReviewModel) Generate(ctx context.Context, prompt string, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.reply != nil {
		return &contracts.LLMResponse{Text: m.reply(prompt)}, nil
	}
	return &contracts.LLMResponse{Text: `{"thought":"complete","complete":true,"summary":"ok"}`}, nil
}

func (m *mockReviewModel) GenerateStream(ctx context.Context, prompt string, options *contracts.LLMOptions) (<-chan string, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan string, 1)
	if m.reply != nil {
		ch <- m.reply(prompt)
	} else {
		ch <- `{"thought":"complete","complete":true,"summary":"ok"}`
	}
	close(ch)
	return ch, nil
}

func (m *mockReviewModel) Chat(ctx context.Context, messages []contracts.Message, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return m.Generate(ctx, "", options)
}

func (m *mockReviewModel) ChatWithTools(ctx context.Context, messages []contracts.Message, tools []contracts.LLMToolSpec, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return m.Generate(ctx, "", options)
}

func TestCodeReviewHandlerStructuredModelResponse(t *testing.T) {
	model := &mockReviewModel{
		reply: func(prompt string) string {
			switch {
			case strings.Contains(prompt, "Review the following result"):
				return `{"issues":[{"severity":"warning","description":"placeholder remains","suggestion":"remove the stub"}],"approve":false}`
			case strings.Contains(prompt, "Perform a code review"):
				return `{"thought":"reviewed workspace","complete":true,"summary":"delegate complete"}`
			default:
				return `{"thought":"complete","complete":true,"summary":"ok"}`
			}
		},
	}
	handler := NewCodeReviewHandler(agentenv.WorkspaceEnvironment{Model: model})

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("code", "todo: remove stub", contextdata.MemoryClassTask)

	result, err := handler.Invoke(context.Background(), env, map[string]interface{}{"focus": "style"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Fatalf("result.Success = false")
	}

	findings, ok := result.Data["findings"].([]interface{})
	if !ok {
		t.Fatalf("findings has unexpected type %T", result.Data["findings"])
	}
	if len(findings) != 1 {
		t.Fatalf("findings length = %d, want 1", len(findings))
	}

	summary, ok := result.Data["summary"].(string)
	if !ok {
		t.Fatalf("summary has unexpected type %T", result.Data["summary"])
	}
	if summary == "" {
		t.Fatalf("summary is empty")
	}
}

func TestCodeReviewHandlerModelErrorFallsBack(t *testing.T) {
	handler := NewCodeReviewHandler(agentenv.WorkspaceEnvironment{
		Model: &mockReviewModel{err: errors.New("model unavailable")},
	})

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("code", "TODO: refactor this stub", contextdata.MemoryClassTask)

	result, err := handler.Invoke(context.Background(), env, map[string]interface{}{"focus": "all"})
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Fatalf("result.Success = false")
	}

	findings, ok := result.Data["findings"].([]interface{})
	if !ok {
		t.Fatalf("findings has unexpected type %T", result.Data["findings"])
	}
	if len(findings) == 0 {
		t.Fatalf("expected fallback findings, got none")
	}
}
