package otelleg

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

func attrMap(attrs []attribute.KeyValue) map[string]attribute.Value {
	m := make(map[string]attribute.Value, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value
	}
	return m
}

func TestProjectAttributesBlocklist(t *testing.T) {
	blocked := []string{"args", "prompt", "response", "text", "stdout", "stderr"}
	metadata := map[string]any{"text_preview": "clipped preview"}
	for _, key := range blocked {
		metadata[key] = "secret"
	}
	got := attrMap(projectAttributes(telemetry.Event{Metadata: metadata}))
	for _, key := range blocked {
		if _, ok := got[key]; ok {
			t.Errorf("metadata key %q must not be projected", key)
		}
	}
	if got["text_preview"].AsString() != "clipped preview" {
		t.Fatalf("text_preview should project as-is, got %v", got["text_preview"])
	}
}

func TestProjectAttributesTruncation(t *testing.T) {
	ev := telemetry.Event{Metadata: map[string]any{"blob": strings.Repeat("a", maxAttrBytes+1000)}}
	got := attrMap(projectAttributes(ev))
	value := got["blob"].AsString()
	if !strings.HasSuffix(value, attrTruncSuffix) {
		t.Fatalf("truncated value missing suffix: %q", value)
	}
	if len(value) > maxAttrBytes+len(attrTruncSuffix) {
		t.Fatalf("truncated value is %d bytes, want <= %d", len(value), maxAttrBytes+len(attrTruncSuffix))
	}
}

func TestProjectAttributesTruncationKeepsUTF8(t *testing.T) {
	ev := telemetry.Event{Metadata: map[string]any{"blob": strings.Repeat("é", maxAttrBytes)}}
	got := attrMap(projectAttributes(ev))
	value := strings.TrimSuffix(got["blob"].AsString(), attrTruncSuffix)
	if !utf8.ValidString(value) {
		t.Fatalf("truncation split a rune: %q", value)
	}
}

func TestProjectAttributesCap(t *testing.T) {
	metadata := make(map[string]any, 100)
	for i := 0; i < 100; i++ {
		metadata[fmt.Sprintf("k%03d", i)] = i
	}
	ev := telemetry.Event{SessionID: "s", RunID: "r", Metadata: metadata}
	got := projectAttributes(ev)
	if len(got) != maxAttributes {
		t.Fatalf("attribute count = %d, want exactly %d", len(got), maxAttributes)
	}
	keys := attrMap(got)
	for _, fixed := range []string{"session.id", "run.id"} {
		if _, ok := keys[fixed]; !ok {
			t.Fatalf("fixed attribute %q was dropped by the cap", fixed)
		}
	}
}

func TestProjectAttributesKeyNormalization(t *testing.T) {
	ev := telemetry.Event{Metadata: map[string]any{"Bad-Key": 1, "good.key": 2}}
	got := attrMap(projectAttributes(ev))
	if _, ok := got["Bad-Key"]; ok {
		t.Fatal("dirty key projected unnormalized")
	}
	if _, ok := got["relurpify.meta.Bad-Key"]; !ok {
		t.Fatalf("dirty key not prefixed; got %v", got)
	}
	if _, ok := got["good.key"]; !ok {
		t.Fatalf("clean key was altered; got %v", got)
	}
}

func TestProjectAttributesTypedValues(t *testing.T) {
	ev := telemetry.Event{Metadata: map[string]any{
		"i": 42,
		"b": true,
		"f": 1.5,
		"u": uint64(7),
		"s": "plain",
		"n": nil,
	}}
	got := attrMap(projectAttributes(ev))
	if got["i"].Type() != attribute.INT64 || got["i"].AsInt64() != 42 {
		t.Fatalf("int not projected natively: %v", got["i"])
	}
	if got["b"].Type() != attribute.BOOL || !got["b"].AsBool() {
		t.Fatalf("bool not projected natively: %v", got["b"])
	}
	if got["f"].Type() != attribute.FLOAT64 || got["f"].AsFloat64() != 1.5 {
		t.Fatalf("float not projected natively: %v", got["f"])
	}
	if got["u"].AsInt64() != 7 {
		t.Fatalf("uint not projected: %v", got["u"])
	}
	if got["s"].Type() != attribute.STRING || got["s"].AsString() != "plain" {
		t.Fatalf("string not projected: %v", got["s"])
	}
	if _, ok := got["n"]; ok {
		t.Fatal("nil value should be projected as absent")
	}
}

func TestProjectAttributesParamAllowlistPassesThrough(t *testing.T) {
	ev := telemetry.Event{Metadata: map[string]any{
		"param.path": "/tmp/x",
		"param.mode": "ro",
	}}
	got := attrMap(projectAttributes(ev))
	if got["param.path"].AsString() != "/tmp/x" || got["param.mode"].AsString() != "ro" {
		t.Fatalf("param.* allowlisted keys did not pass through: %v", got)
	}
}

func TestProjectAttributesPreservesUnusableTraceID(t *testing.T) {
	ev := telemetry.Event{TraceID: "not-hex", SpanID: "nope"}
	got := attrMap(projectAttributes(ev))
	if got["relurpify.trace_id"].AsString() != "not-hex" {
		t.Fatalf("unusable trace id not preserved: %v", got)
	}
	if got["relurpify.span_id"].AsString() != "nope" {
		t.Fatalf("unusable span id not preserved: %v", got)
	}

	valid := telemetry.Event{TraceID: strings.Repeat("ab", 16), SpanID: strings.Repeat("cd", 8)}
	gotValid := attrMap(projectAttributes(valid))
	if _, ok := gotValid["relurpify.trace_id"]; ok {
		t.Fatal("a usable trace id must not be duplicated as an attribute")
	}
}

func TestStringifyValueTerminatesOnRecursion(t *testing.T) {
	recursive := map[string]any{}
	recursive["self"] = recursive
	if got := stringifyValue(recursive); got == "" {
		t.Fatal("recursive map produced empty rendering")
	}

	nested := map[int]any{1: "a", 2: []any{1, 2, map[string]any{"x": true}}}
	if got := stringifyValue(nested); got == "" {
		t.Fatal("non-string-key map produced empty rendering")
	}
}
