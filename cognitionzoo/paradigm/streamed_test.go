package paradigm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type streamedTelemetrySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *streamedTelemetrySink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *streamedTelemetrySink) count(eventType telemetry.EventType) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, event := range s.events {
		if event.Type == eventType {
			n++
		}
	}
	return n
}

func seedSentinelSlice(env *contextdata.Envelope, body string, finalTokens int) {
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID:   "req-streamed-test",
		Epoch:       3,
		FinalTokens: finalTokens,
		Chunks: []contextdata.StreamedChunk{
			{ChunkID: "chunk-test", ContentHash: "hash-test", Body: body, TokenEstimate: finalTokens},
		},
	})
}

// TestStreamedSectionTelemetry is NFR-7: the injected, inject_skipped, and
// render_error paths all emit their §5.7 events through the context sink.
func TestStreamedSectionTelemetry(t *testing.T) {
	sink := &streamedTelemetrySink{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	// Non-empty render → injected with chunk/token accounting.
	env := contextdata.NewEnvelope("t", "s")
	seedSentinelSlice(env, "sentinel body", 16)
	section, err := StreamedSection(ctx, env, "react")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(section, "sentinel body") {
		t.Fatalf("section = %q", section)
	}
	if got := sink.count(telemetry.EventContextStreamInjected); got != 1 {
		t.Fatalf("injected events = %d, want 1", got)
	}

	// Empty slice → inject_skipped with the empty_slice reason, zero bytes.
	empty := contextdata.NewEnvelope("t", "s")
	section, err = StreamedSection(ctx, empty, "react")
	if err != nil || section != "" {
		t.Fatalf("empty render = (%q, %v)", section, err)
	}
	if got := sink.count(telemetry.EventContextStreamInjectSkipped); got != 1 {
		t.Fatalf("inject_skipped events = %d, want 1", got)
	}

	// Inconsistent slice → render_error and a failed node (D-10).
	broken := contextdata.NewEnvelope("t", "s")
	broken.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID:   "req-broken",
		Epoch:       1,
		FinalTokens: 1000,
		Chunks: []contextdata.StreamedChunk{
			{ChunkID: "chunk-broken", Body: "short", TokenEstimate: 10},
		},
	})
	section, err = StreamedSection(ctx, broken, "react")
	if !errors.Is(err, contextstream.ErrSliceTokenMismatch) {
		t.Fatalf("err = %v, want ErrSliceTokenMismatch", err)
	}
	if section != "" {
		t.Fatal("failed render must not return a partial section")
	}
	if got := sink.count(telemetry.EventContextStreamRenderError); got != 1 {
		t.Fatalf("render_error events = %d, want 1", got)
	}
}
