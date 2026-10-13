package otelleg

import (
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
)

const testErrorText = "boom"

func TestSpanStatus(t *testing.T) {
	tests := []struct {
		name string
		meta map[string]any
		want string
	}{
		{name: "no signal", meta: nil, want: statusOK},
		{name: "success true", meta: map[string]any{metaSuccess: true}, want: statusOK},
		{name: "success false", meta: map[string]any{metaSuccess: false}, want: statusError},
		{name: "non-bool success is error", meta: map[string]any{metaSuccess: "yes"}, want: statusError},
		{name: "error key", meta: map[string]any{metaError: testErrorText}, want: statusError},
		{name: "tool error", meta: map[string]any{metaToolError: testErrorText}, want: statusError},
		{name: "capability error", meta: map[string]any{metaCapabilityError: testErrorText}, want: statusError},
		{name: "status ok", meta: map[string]any{metaStatus: statusOK}, want: statusOK},
		{name: "status error", meta: map[string]any{metaStatus: statusError}, want: statusError},
		{name: "status empty is ok", meta: map[string]any{metaStatus: ""}, want: statusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := spanStatus(telemetry.Event{Metadata: tc.meta}); got != tc.want {
				t.Fatalf("spanStatus(%v) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}
}
