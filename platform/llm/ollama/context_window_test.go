package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
)

// chatPayloadCapture is an /api/chat handler that records the request body so
// tests can assert the transport options the client actually sent.
type chatPayloadCapture struct {
	payload map[string]any
}

func (c *chatPayloadCapture) handler(w http.ResponseWriter, r *http.Request) {
	// Decode and keep the request body: tests assert the transport options
	// the client actually sent.
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	c.payload = body
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": map[string]any{"role": "assistant", "content": "ok"},
	})
}

func contextWindowServer(t *testing.T, contextLength int) (*Backend, *chatPayloadCapture) {
	t.Helper()
	capture := &chatPayloadCapture{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model_info": map[string]any{
				"gemma4.context_length": contextLength,
			},
		})
	})
	mux.HandleFunc("/api/chat", capture.handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewBackend(Config{Endpoint: srv.URL, Model: "test-model"}, ""), capture
}

func decodedOptions(t *testing.T, capture *chatPayloadCapture) map[string]any {
	t.Helper()
	require.NotNil(t, capture.payload, "the chat request must reach the server")
	raw, err := json.Marshal(capture.payload)
	require.NoError(t, err)
	var body struct {
		Options map[string]any `json:"options"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	return body.Options
}

// TestModelDerivesNumCtxFromServerCapability proves the served window is
// derived from the live server: with no profile budget, num_ctx equals the
// capability the server reports — never the server's silent default window.
func TestModelDerivesNumCtxFromServerCapability(t *testing.T) {
	backend, capture := contextWindowServer(t, 131072)

	_, err := backend.Model().Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)

	opts := decodedOptions(t, capture)
	require.Equal(t, float64(131072), opts["num_ctx"], "the discovered capability becomes the served window")
}

// TestModelClampsWindowToProfileBudget proves the profile budget bounds the
// served window: budget 8192 against a 131072-capable server serves 8192 —
// the configured budget is what the server attends to, no more (KV-cache
// memory stays bounded by the configured posture).
func TestModelClampsWindowToProfileBudget(t *testing.T) {
	backend, capture := contextWindowServer(t, 131072)
	backend.SetProfile(&ModelProfile{ContextSize: 8192})

	_, err := backend.Model().Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)

	opts := decodedOptions(t, capture)
	require.Equal(t, float64(8192), opts["num_ctx"], "the profile budget clamps the served window")
}

// TestModelClampsBudgetToServerCapability proves the direction that protects
// the model: a profile budget LARGER than the server's capability is clamped
// down — a budget the server cannot attend to must never be requested.
func TestModelClampsBudgetToServerCapability(t *testing.T) {
	backend, capture := contextWindowServer(t, 4096)
	backend.SetProfile(&ModelProfile{ContextSize: 8192})

	_, err := backend.Model().Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)

	opts := decodedOptions(t, capture)
	require.Equal(t, float64(4096), opts["num_ctx"], "the server capability clamps an oversized budget")
}

// TestExplicitCallerWindowWins proves an explicit caller ContextWindow is
// never overridden by the derivation.
func TestExplicitCallerWindowWins(t *testing.T) {
	backend, capture := contextWindowServer(t, 131072)

	_, err := backend.Model().Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}},
		&LLMOptions{ContextWindow: 2048})
	require.NoError(t, err)

	opts := decodedOptions(t, capture)
	require.Equal(t, float64(2048), opts["num_ctx"])
}

// TestUnreachableServerLeavesDefault proves derivation never guesses: when
// the server reports nothing, no num_ctx is sent and the transport default
// applies (the capability is retried on the next call).
func TestUnreachableServerLeavesDefault(t *testing.T) {
	capture := &chatPayloadCapture{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/api/chat", capture.handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"}, "")
	_, err := backend.Model().Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)

	opts := decodedOptions(t, capture)
	require.NotContains(t, opts, "num_ctx", "no capability discovered — no window guessed")
}
