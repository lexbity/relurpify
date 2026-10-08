package agenttest

import (
	"sync"
	"testing"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestRecordingTelemetrySinkCapturesEvents(t *testing.T) {
	sink := newRecordingTelemetrySink()
	if sink.Len() != 0 {
		t.Fatalf("expected empty sink, got %d", sink.Len())
	}
	sink.Emit(telemetry.Event{Type: telemetry.EventToolCall, Message: "one"})
	sink.Emit(telemetry.Event{Type: telemetry.EventToolResult, Message: "two"})

	if sink.Len() != 2 {
		t.Fatalf("expected 2 captured events, got %d", sink.Len())
	}
	events := sink.Events()
	if len(events) != 2 || events[0].Type != telemetry.EventToolCall || events[1].Type != telemetry.EventToolResult {
		t.Fatalf("unexpected captured events: %#v", events)
	}
}

func TestRecordingTelemetrySinkEventsReturnsCopy(t *testing.T) {
	sink := newRecordingTelemetrySink()
	sink.Emit(telemetry.Event{Type: telemetry.EventAgentStart})

	first := sink.Events()
	first[0].Type = telemetry.EventAgentFinish

	if got := sink.Events()[0].Type; got != telemetry.EventAgentStart {
		t.Fatalf("Events() mutated the sink contents: got type %q", got)
	}
}

func TestRecordingTelemetrySinkResetDropsEvents(t *testing.T) {
	sink := newRecordingTelemetrySink()
	sink.Emit(telemetry.Event{Type: telemetry.EventAgentStart})
	sink.Reset()
	if sink.Len() != 0 {
		t.Fatalf("expected empty sink after reset, got %d", sink.Len())
	}
}

func TestRecordingTelemetrySinkIsThreadSafe(t *testing.T) {
	sink := newRecordingTelemetrySink()
	const goroutines, perGoroutine = 8, 100
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				sink.Emit(telemetry.Event{Type: telemetry.EventToolCall})
			}
		}()
	}
	wg.Wait()
	if sink.Len() != goroutines*perGoroutine {
		t.Fatalf("expected %d events, got %d", goroutines*perGoroutine, sink.Len())
	}
}

func TestRecordingTelemetrySinkNilSafe(t *testing.T) {
	var sink *recordingTelemetrySink
	sink.Emit(telemetry.Event{Type: telemetry.EventAgentStart}) // must not panic
	if sink.Len() != 0 {
		t.Fatal("nil sink should not capture")
	}
	if events := sink.Events(); events != nil {
		t.Fatal("nil sink should return nil events")
	}
}
