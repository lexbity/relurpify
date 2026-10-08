package contextstream

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// recordingSink captures events for assertions.
type recordingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *recordingSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingSink) Events() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telemetry.Event(nil), s.events...)
}

func (s *recordingSink) HasType(eventType telemetry.EventType) bool {
	for _, ev := range s.Events() {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func TestTriggerEmitsCompilerStartedAndCompleted(t *testing.T) {
	sink := &recordingSink{}
	comp := &fakeCompiler{result: &contextports.CompilationResult{}}
	trigger := NewTrigger(comp).SetTelemetry(sink)

	req := Request{
		ID:        "req-telemetry",
		Query:     retrieval.RetrievalQuery{Text: "explain the codebase"},
		MaxTokens: 1024,
		Mode:      ModeBlocking,
	}
	if _, err := trigger.RequestBlocking(context.Background(), req); err != nil {
		t.Fatalf("RequestBlocking: %v", err)
	}

	if !sink.HasType(telemetry.EventCompilerStarted) {
		t.Fatalf("expected compiler.started, got %v", eventTypes(sink))
	}
	if !sink.HasType(telemetry.EventCompilerCompleted) {
		t.Fatalf("expected compiler.completed, got %v", eventTypes(sink))
	}
}

func TestTriggerEmitsCacheHitAndMiss(t *testing.T) {
	t.Run("cache hit", func(t *testing.T) {
		sink := &recordingSink{}
		comp := &fakeCompiler{result: &contextports.CompilationResult{
			Record: contextports.CompilationRecord{CacheHit: true},
		}}
		trigger := NewTrigger(comp).SetTelemetry(sink)
		_, _ = trigger.RequestBlocking(context.Background(), Request{ID: "req-1"})

		if !sink.HasType(telemetry.EventCompilerCacheHit) {
			t.Fatalf("expected compiler.cache_hit, got %v", eventTypes(sink))
		}
		if sink.HasType(telemetry.EventCompilerCacheMiss) {
			t.Fatalf("unexpected compiler.cache_miss for a cache hit: %v", eventTypes(sink))
		}
	})

	t.Run("cache miss", func(t *testing.T) {
		sink := &recordingSink{}
		comp := &fakeCompiler{result: &contextports.CompilationResult{}}
		trigger := NewTrigger(comp).SetTelemetry(sink)
		_, _ = trigger.RequestBlocking(context.Background(), Request{ID: "req-2"})

		if !sink.HasType(telemetry.EventCompilerCacheMiss) {
			t.Fatalf("expected compiler.cache_miss, got %v", eventTypes(sink))
		}
		if sink.HasType(telemetry.EventCompilerCacheHit) {
			t.Fatalf("unexpected compiler.cache_hit for a cache miss: %v", eventTypes(sink))
		}
	})
}

func TestTriggerEmitsBudgetAndSubstitutionEvents(t *testing.T) {
	sink := &recordingSink{}
	comp := &fakeCompiler{result: &contextports.CompilationResult{
		ShortfallTokens: 42,
		StreamedRefs:    []string{"chunk-1"},
		Substitutions: []contextports.SummarySubstitution{
			{Original: "chunk-2", Replaced: "summary-2", ChunkID: "chunk-2"},
		},
	}}
	trigger := NewTrigger(comp).SetTelemetry(sink)
	_, _ = trigger.RequestBlocking(context.Background(), Request{ID: "req-budget", MaxTokens: 64})

	if !sink.HasType(telemetry.EventCompilerBudgetExceeded) {
		t.Fatalf("expected compiler.budget_exceeded, got %v", eventTypes(sink))
	}
	if !sink.HasType(telemetry.EventCompilerSummarySubstituted) {
		t.Fatalf("expected compiler.summary_substituted, got %v", eventTypes(sink))
	}
}

func TestTriggerWithoutTelemetryIsSilent(t *testing.T) {
	comp := &fakeCompiler{result: &contextports.CompilationResult{}}
	trigger := NewTrigger(comp)
	_, err := trigger.RequestBlocking(context.Background(), Request{ID: "req-silent"})
	if err != nil {
		t.Fatalf("RequestBlocking: %v", err)
	}
	// No sink attached; the trigger must not panic and must not emit anywhere.
	if trigger.telemetry != nil {
		t.Fatal("expected nil telemetry on a fresh trigger")
	}
}

func eventTypes(sink *recordingSink) []telemetry.EventType {
	var out []telemetry.EventType
	for _, ev := range sink.Events() {
		out = append(out, ev.Type)
	}
	return out
}
