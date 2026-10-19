package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Tool calls arrive mid-stream (the terminal chunk is a usage summary). The
// streaming path must collect them from every chunk — dropping them because
// they are not on the final chunk silently disarms native tool calling.
func TestChatWithToolsCollectsMidStreamToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flush := func(s string) {
			_, _ = w.Write([]byte(s))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		flush(`{"model":"m","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"file_list","arguments":{"directory":"."}}}]}}` + "\n")
		flush(`{"model":"m","message":{"role":"assistant","content":""}}` + "\n")
		flush(`{"model":"m","done":true,"done_reason":"stop","eval_count":42}` + "\n")
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "")
	client.Debug = true
	resp, err := client.ChatWithTools(context.Background(),
		[]Message{{Role: "user", Content: "list the files"}},
		[]LLMToolSpec{{Name: "file_list", Description: "lists files"}},
		&LLMOptions{StreamCallback: func(string) {}})
	if err != nil {
		t.Fatalf("ChatWithTools failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call from the mid-stream chunk, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "file_list" {
		t.Errorf("expected tool name file_list, got %q", resp.ToolCalls[0].Name)
	}
}

// Debug logging must carry full payloads (request and streamed response):
// native tool-calling turns are undiagnosable from a truncated tools array.
func TestStreamDebugLogIsNotTruncated(t *testing.T) {
	big := make([]byte, 6000)
	for i := range big {
		big[i] = 'a'
	}
	var logged []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if logged == nil {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			logged = body
		}
		_, _ = w.Write([]byte(`{"model":"m","done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "")
	client.Debug = true
	_, err := client.ChatWithTools(context.Background(),
		[]Message{{Role: "user", Content: string(big)}},
		[]LLMToolSpec{{Name: "file_list", Description: string(big)}},
		nil)
	if err != nil {
		t.Fatalf("ChatWithTools failed: %v", err)
	}
	if len(logged) < len(big) {
		t.Fatalf("request payload reached the server truncated: %d bytes", len(logged))
	}
}
