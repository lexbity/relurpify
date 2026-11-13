// Package conformance contains transport-level gate conformance tests for P03.
package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm/ollama"
)

// TestV2GateTransportConformance verifies that the gate is enforced at the
// physical transport boundary. The loopback server tracks the number of
// request bodies that are actually open at the same time, so a gate that only
// serializes admission without covering the body lifetime fails here.
func TestV2GateTransportConformance(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		active    int
		maxActive int
		requests  int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		active++
		requests++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		// Hold the body open long enough for a competing request to overlap
		// if the gate failed to serialize transport lifetime.
		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":"ok"}`))
	}))
	defer srv.Close()

	gate := model.NewCallGate(model.GateConfig{
		QueueCapacity:           16,
		QueueTimeout:            5 * time.Second,
		ActiveTimeout:           5 * time.Second,
		CancellationGracePeriod: 250 * time.Millisecond,
	})

	const numRequests = 10
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()

			ticket, err := gate.Acquire(
				context.Background(),
				model.CallID(fmt.Sprintf("call_%d", id)),
				model.AttemptID(fmt.Sprintf("attempt_%d", id)),
			)
			if err != nil {
				t.Errorf("acquire %d: %v", id, err)
				return
			}
			defer ticket.Released()

			req, err := http.NewRequestWithContext(ticket.Context, http.MethodPost, srv.URL+"/api/generate", nil)
			if err != nil {
				t.Errorf("request %d: %v", id, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("do %d: %v", id, err)
				return
			}
			_ = resp.Body.Close()
		}(i)
	}

	wg.Wait()

	mu.Lock()
	gotMax := maxActive
	gotRequests := requests
	mu.Unlock()

	if gotMax != 1 {
		t.Fatalf("max concurrent active request bodies = %d, want 1", gotMax)
	}
	if gotRequests != numRequests {
		t.Fatalf("server saw %d requests, want %d", gotRequests, numRequests)
	}
	if got := gate.TotalGranted(); got != numRequests {
		t.Fatalf("gate granted %d tickets, want %d", got, numRequests)
	}
	if got := gate.TotalReclaimed(); got != numRequests {
		t.Fatalf("gate reclaimed %d tickets, want %d", got, numRequests)
	}
}

// TestV2GatePhysicalRequestEnforcement proves that a second physical request
// cannot start while the first ticket is held, even when the caller does not
// touch the network. This is the core D-04 invariant without provider setup.
func TestV2GatePhysicalRequestEnforcement(t *testing.T) {
	t.Parallel()

	gate := model.NewCallGate(model.GateConfig{
		QueueCapacity:           4,
		QueueTimeout:            2 * time.Second,
		ActiveTimeout:           2 * time.Second,
		CancellationGracePeriod: 250 * time.Millisecond,
	})

	first, err := gate.Acquire(context.Background(), "call_first", "attempt_first")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	secondGranted := make(chan struct{})
	go func() {
		ticket, err := gate.Acquire(context.Background(), "call_second", "attempt_second")
		if err != nil {
			return
		}
		close(secondGranted)
		ticket.Released()
	}()

	select {
	case <-secondGranted:
		t.Fatal("second physical request was granted while the first ticket was active")
	case <-time.After(50 * time.Millisecond):
	}

	first.Released()

	select {
	case <-secondGranted:
	case <-time.After(2 * time.Second):
		t.Fatal("second physical request was not granted after the first ticket was released")
	}
}

// TestV2GateQueuedCancellationStartsNoTransport proves that canceling a queued
// caller removes it from the FIFO and never grants it a transport.
func TestV2GateQueuedCancellationStartsNoTransport(t *testing.T) {
	t.Parallel()

	gate := model.NewCallGate(model.GateConfig{
		QueueCapacity:           4,
		QueueTimeout:            2 * time.Second,
		ActiveTimeout:           2 * time.Second,
		CancellationGracePeriod: 250 * time.Millisecond,
	})

	held, err := gate.Acquire(context.Background(), "call_held", "attempt_held")
	if err != nil {
		t.Fatalf("held acquire: %v", err)
	}

	queuedCtx, cancel := context.WithCancel(context.Background())
	queuedErr := make(chan error, 1)
	go func() {
		_, err := gate.Acquire(queuedCtx, "call_queued", "attempt_queued")
		queuedErr <- err
	}()

	// Wait until the call is actually queued.
	deadline := time.Now().Add(time.Second)
	for gate.QueueDepth() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := gate.QueueDepth(); got != 1 {
		t.Fatalf("queue depth = %d, want 1", got)
	}

	cancel()

	select {
	case err := <-queuedErr:
		if err == nil {
			t.Fatal("canceled queued call was granted a ticket")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled queued call did not return")
	}

	held.Released()

	// The canceled caller must not have left a phantom queue entry that
	// consumes the next grant.
	next, err := gate.Acquire(context.Background(), "call_next", "attempt_next")
	if err != nil {
		t.Fatalf("next acquire: %v", err)
	}
	next.Released()

	if got := gate.QueueDepth(); got != 0 {
		t.Fatalf("queue depth after cancellation = %d, want 0", got)
	}
}

// TestV2OllamaBackendUsesSharedLane proves the real Ollama provider wrapper
// acquires the injected gate around the transport request. The loopback server
// counts open /api/chat bodies, so a wrapper that releases before the body is
// drained fails here.
func TestV2OllamaBackendUsesSharedLane(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		active    int
		maxActive int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model_info": map[string]any{"llama.context_length": 4096},
			})
		case "/api/chat":
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gate := model.NewDefaultCallGate()
	backend := ollama.NewBackend(ollama.Config{Endpoint: srv.URL, Model: "test-model"}, "", gate)
	languageModel := backend.Model()

	const calls = 5
	var wg sync.WaitGroup
	wg.Add(calls)
	for i := 0; i < calls; i++ {
		go func() {
			defer wg.Done()
			if _, err := languageModel.Chat(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, nil); err != nil {
				t.Errorf("chat: %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	gotMax := maxActive
	mu.Unlock()
	if gotMax != 1 {
		t.Fatalf("max concurrent Ollama chat bodies = %d, want 1", gotMax)
	}
	if got := gate.TotalGranted(); got != calls {
		t.Fatalf("gate granted %d tickets, want %d", got, calls)
	}
	if got := gate.TotalReclaimed(); got != calls {
		t.Fatalf("gate reclaimed %d tickets, want %d", got, calls)
	}
}
