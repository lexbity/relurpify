package llm

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
)

func TestContextLengthErrorClassifiesStatusAndBody(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		detail     string
		want       bool
	}{
		{"http 413", 413, "payload too large", true},
		{"ollama phrasing", 400, "error: context length exceeded", true},
		{"openai phrasing", 400, `{"error":{"code":"context_length_exceeded"}}`, true},
		{"maximum context", 400, "maximum context length is 8192 tokens", true},
		{"unrelated 500", 500, "internal server error", false},
		{"unrelated 400", 400, "bad request", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ContextLengthError(tc.statusCode, tc.detail)
			if tc.want {
				if err == nil {
					t.Fatalf("ContextLengthError(%d, %q) = nil, want ErrContextLength", tc.statusCode, tc.detail)
				}
				if !errors.Is(err, model.ErrContextLength) {
					t.Fatalf("ContextLengthError(%d, %q) = %v, want errors.Is ErrContextLength", tc.statusCode, tc.detail, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ContextLengthError(%d, %q) = %v, want nil", tc.statusCode, tc.detail, err)
			}
		})
	}
}

func TestRecordedBudgetErrorReplaysAsTokenBudget(t *testing.T) {
	err := RecordedBudgetError("ollama error: 400 Bad Request: context length exceeded")
	if err == nil || !errors.Is(err, model.ErrTokenBudget) {
		t.Fatalf("RecordedBudgetError = %v, want errors.Is ErrTokenBudget", err)
	}
	if err := RecordedBudgetError("model offline"); err != nil {
		t.Fatalf("RecordedBudgetError(non-budget) = %v, want nil", err)
	}
}
