package ollama

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
)

// TestChatClassifiesContextLength pins the ollama provider-boundary
// classifier: a 413/context-length response is wrapped with
// model.ErrContextLength (FR-10 producer).
func TestChatClassifiesContextLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte("context length exceeded"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-model", "")
	_, err := client.Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, &model.LLMOptions{})
	if err == nil || !errors.Is(err, model.ErrContextLength) {
		t.Fatalf("Chat error = %v, want errors.Is ErrContextLength", err)
	}
}
