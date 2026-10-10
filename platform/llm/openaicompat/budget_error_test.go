package openaicompat

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
)

// TestReadHTTPErrorClassifiesContextLength pins the provider-boundary
// classifier: an openai-compatible context-length error body is wrapped with
// model.ErrContextLength, so failure classification reaches budget_exhausted
// without string sniffing outside platform/llm (FR-10).
func TestReadHTTPErrorClassifiesContextLength(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"this model's maximum context length is 8192 tokens"}}`)),
	}
	err := readHTTPError(resp)
	if err == nil || !errors.Is(err, model.ErrContextLength) {
		t.Fatalf("readHTTPError = %v, want errors.Is ErrContextLength", err)
	}

	plain := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader("bad request")),
	}
	if err := readHTTPError(plain); err != nil && errors.Is(err, model.ErrContextLength) {
		t.Fatalf("ordinary 400 must not classify as context length: %v", err)
	}
}
