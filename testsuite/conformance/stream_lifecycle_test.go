package conformance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/platform/llm/offline"
	"codeburg.org/lexbit/relurpify/platform/llm/openaicompat"
	"codeburg.org/lexbit/relurpify/platform/observability"
)

// The stream lifecycle contract (R1-R6, stated normatively on
// model.LanguageModel.GenerateStream) is enforced here for every provider
// kind constructible offline. A future provider must be added to
// streamLifecycleKinds the moment it lands, or this suite will not cover it.

const streamCancelGrace = 250 * time.Millisecond

// gatedStreamModel is a LanguageModel whose stream emits tokens only when
// poked via emit and stops on its own ctx, giving the suite deterministic
// mid-stream control for cancel and slow-consumer scenarios.
type gatedStreamModel struct {
	tokens chan string
}

func newGatedStreamModel() *gatedStreamModel {
	return &gatedStreamModel{tokens: make(chan string)}
}

func (g *gatedStreamModel) emit(tok string) { g.tokens <- tok }

func (g *gatedStreamModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (g *gatedStreamModel) GenerateStream(ctx context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	out := make(chan string)
	go func() {
		defer close(out)
		for {
			select {
			case tok := <-g.tokens:
				select {
				case out <- tok:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (g *gatedStreamModel) Chat(ctx context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return g.Generate(ctx, "chat", nil)
}

func (g *gatedStreamModel) ChatWithTools(ctx context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return g.Generate(ctx, "tools", nil)
}

type capturedTelemetry struct {
	events []observability.Event
}

func (c *capturedTelemetry) Emit(ev observability.Event) {
	c.events = append(c.events, ev)
}

func (c *capturedTelemetry) countOfType(t observability.EventType) int {
	n := 0
	for _, ev := range c.events {
		if ev.Type == t {
			n++
		}
	}
	return n
}

// streamLifecycleKind names one offline-constructible provider shape. Each
// instance is built per scenario so goleak verification sees only that
// scenario's goroutines (an httptest server shared across subtests would
// leak its accept loop into every sibling's check).
type streamLifecycleKind struct {
	name string
	// build returns the model under test, an emitter for models whose
	// stream must be poked manually (nil otherwise), and the teardown that
	// releases provider-owned resources (nil when none).
	build func(t *testing.T) (model.LanguageModel, *gatedStreamModel, func())
}

func streamLifecycleKinds() []streamLifecycleKind {
	return []streamLifecycleKind{
		{
			name: "tape(record)",
			build: func(t *testing.T) (model.LanguageModel, *gatedStreamModel, func()) {
				inner := newGatedStreamModel()
				tm, err := llm.NewTapeModel(inner, filepath.Join(t.TempDir(), "tape.jsonl"), "record")
				if err != nil {
					t.Fatal(err)
				}
				return tm, inner, func() { _ = tm.Close() }
			},
		},
		{
			name: "offline",
			build: func(t *testing.T) (model.LanguageModel, *gatedStreamModel, func()) {
				return offline.Model{}, nil, nil
			},
		},
		{
			name: "openaicompat(httptest)",
			build: func(t *testing.T) (model.LanguageModel, *gatedStreamModel, func()) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					flusher := w.(http.Flusher)
					for i := 0; i < 100; i++ {
						_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"t\"}}]}\n\n"))
						flusher.Flush()
						select {
						case <-time.After(5 * time.Millisecond):
						case <-r.Context().Done():
							return
						}
					}
					_, _ = w.Write([]byte("data: [DONE]\n\n"))
				}))
				return openaicompat.NewClient(openaicompat.OpenAICompatConfig{Endpoint: srv.URL}, ""), nil, srv.Close
			},
		},
	}
}

func TestStreamLifecycle(t *testing.T) {
	for _, kind := range streamLifecycleKinds() {
		kind := kind
		t.Run(kind.name+"/cancel-mid-stream", func(t *testing.T) {
			defer goleak.VerifyNone(t)
			m, emitter, teardown := kind.build(t)
			if teardown != nil {
				defer teardown()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := m.GenerateStream(ctx, "prompt", nil)
			if err != nil {
				t.Fatal(err)
			}
			if emitter != nil {
				emitter.emit("t1")
				<-stream
			} else {
				select {
				case _, ok := <-stream:
					if !ok {
						// Short stream already closed; cancellation is
						// trivially satisfied below.
					}
				case <-time.After(2 * time.Second):
					t.Fatal("no first token within 2s")
				}
			}
			cancel()
			assertStreamClosesWithin(t, stream, streamCancelGrace)
		})

		t.Run(kind.name+"/slow-consumer", func(t *testing.T) {
			defer goleak.VerifyNone(t)
			m, emitter, teardown := kind.build(t)
			if teardown != nil {
				defer teardown()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := m.GenerateStream(ctx, "prompt", nil)
			if err != nil {
				t.Fatal(err)
			}
			if emitter != nil {
				emitter.emit("t1")
			}
			// Read at most one token, then stop reading entirely;
			// cancellation is the only stop signal the pump may rely on.
			select {
			case _, _ = <-stream:
			case <-time.After(2 * time.Second):
			}
			cancel()
			assertStreamClosesWithin(t, stream, streamCancelGrace)
		})
	}

	t.Run("tape/recorder-fault", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("/dev/full fault injection is linux-only")
		}
		defer goleak.VerifyNone(t)
		inner := newGatedStreamModel()
		tm, err := llm.NewTapeModel(inner, "/dev/full", "record")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tm.Close() }()
		tel := &capturedTelemetry{}
		tm.Telemetry = tel

		resp, err := tm.Generate(context.Background(), "prompt", nil)
		if err != nil {
			t.Fatalf("response must succeed despite record failure: %v", err)
		}
		if resp.Text != "ok" {
			t.Fatalf("unexpected response %q", resp.Text)
		}
		if !tm.Degraded() {
			t.Fatal("recorder must degrade on write failure")
		}
		if got := tel.countOfType(observability.EventTapeRecordFailed); got != 1 {
			t.Fatalf("tape.record_failed must fire exactly once, got %d", got)
		}
		if _, err := tm.Chat(context.Background(), []model.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
			t.Fatal(err)
		}
		if got := tm.DroppedRecords(); got != 1 {
			t.Fatalf("post-degradation entries must be dropped and counted, got %d", got)
		}
		if got := tel.countOfType(observability.EventTapeRecordFailed); got != 1 {
			t.Fatalf("tape.record_failed must stay one-shot, got %d", got)
		}
	})
}

// TestStreamContractSurfacesR6Counters pins the tape model's R6 drop counters
// so the counter contract cannot silently disappear.
func TestStreamContractSurfacesR6Counters(t *testing.T) {
	defer goleak.VerifyNone(t)
	tm, err := llm.NewTapeModel(newGatedStreamModel(), filepath.Join(t.TempDir(), "tape.jsonl"), "record")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tm.Close() }()
	if tm.Degraded() {
		t.Fatal("fresh recorder must not be degraded")
	}
	if tm.DroppedRecords() != 0 || tm.DroppedStreamTokens() != 0 {
		t.Fatal("fresh recorder counters must be zero")
	}
}

func assertStreamClosesWithin(t *testing.T, stream <-chan string, grace time.Duration) {
	t.Helper()
	deadline := time.After(grace)
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("expected channel close after cancellation, received a token instead")
		}
	case <-deadline:
		t.Fatalf("stream not closed within %v of cancellation", grace)
	}
}
