package core

import (
	"context"
	"testing"
)

type telemetrySinkStub struct {
	events []Event
}

func (s *telemetrySinkStub) Emit(event Event) {
	s.events = append(s.events, event)
}

func TestWithTelemetry_roundtrip(t *testing.T) {
	sink := &telemetrySinkStub{}
	ctx := WithTelemetry(context.Background(), sink)

	got := TelemetryFromContext(ctx)
	if got == nil {
		t.Fatalf("expected telemetry sink, got nil")
	}
	if got != sink {
		t.Fatalf("expected same sink instance, got %#v", got)
	}
}

func TestTelemetryFromContext_absent(t *testing.T) {
	if got := TelemetryFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil telemetry, got %#v", got)
	}
}

func TestTelemetryFromContext_nilContext(t *testing.T) {
	if got := TelemetryFromContext(nil); got != nil {
		t.Fatalf("expected nil telemetry, got %#v", got)
	}
}
