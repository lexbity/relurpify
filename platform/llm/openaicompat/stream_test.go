package openaicompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.uber.org/goleak"
)

// sseStreamHandler serves count content deltas, pacing one per tick so the
// client sees a live stream rather than a buffered dump.
func sseStreamHandler(count int, tick time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for i := 0; i < count; i++ {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"t\"}}]}\n\n"))
			flusher.Flush()
			if tick > 0 {
				select {
				case <-time.After(tick):
				case <-r.Context().Done():
					return
				}
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
}

// TestGenerateStreamCancelMidStream: cancelling the call-time ctx closes the
// token channel within the contract grace and leaks nothing (R1-R4).
func TestGenerateStreamCancelMidStream(t *testing.T) {
	defer goleak.VerifyNone(t)
	srv := httptest.NewServer(sseStreamHandler(100, 5*time.Millisecond))
	defer srv.Close()
	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.GenerateStream(ctx, "hello", nil)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		<-stream
	}
	cancel()
	deadline := time.After(250 * time.Millisecond)
	select {
	case _, ok := <-stream:
		require.False(t, ok, "expected channel close, got token after cancel")
	case <-deadline:
		t.Fatal("stream channel not closed within 250ms of cancellation")
	}
}

// TestChatStreamSlowConsumerCancel: a consumer that stops reading must not
// wedge the pump; cancellation alone ends the stream and the response body
// is released (R1, R4, R6).
func TestChatStreamSlowConsumerCancel(t *testing.T) {
	defer goleak.VerifyNone(t)
	bodyClosed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(bodyClosed)
		sseStreamHandler(100, time.Millisecond).ServeHTTP(w, r)
	}))
	defer srv.Close()
	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.ChatStream(ctx, []Message{{Role: "user", Content: "hi"}}, nil, nil)
	require.NoError(t, err)
	<-stream
	// Stop reading entirely; cancel is the only stop signal.
	cancel()
	deadline := time.After(250 * time.Millisecond)
	select {
	case _, ok := <-stream:
		require.False(t, ok, "expected channel close, got token after cancel")
	case <-deadline:
		t.Fatal("pump did not exit on cancellation with a stalled consumer")
	}
	select {
	case <-bodyClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("response body was not released after cancellation")
	}
}

// TestGenerateStreamCompletes: an uninterrupted stream delivers every token
// and closes cleanly (R3).
func TestGenerateStreamCompletes(t *testing.T) {
	defer goleak.VerifyNone(t)
	srv := httptest.NewServer(sseStreamHandler(4, 0))
	defer srv.Close()
	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")

	stream, err := client.GenerateStream(context.Background(), "hello", nil)
	require.NoError(t, err)
	tokens := 0
	deadline := time.After(2 * time.Second)
	reading := true
	for reading {
		select {
		case _, ok := <-stream:
			if !ok {
				reading = false
				break
			}
			tokens++
		case <-deadline:
			t.Fatal("stream did not complete")
		}
	}
	require.Equal(t, 4, tokens)
}
