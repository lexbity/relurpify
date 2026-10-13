package otelleg

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

// Attribute projection rules (spec §6.4): a metadata blocklist, byte/attribute
// caps, key normalization, and native scalar projection. The same projection is
// used for log records (slice 2) and spans (slice 3).
const (
	maxAttributes   = 64
	maxAttrBytes    = 4096
	attrTruncSuffix = "…[truncated]"
	maxValueDepth   = 6
	metadataKeyPfx  = "relurpify.meta."
)

// blockedMetadataKeys are never projected: raw payloads that reach spans only
// through the ToolSpanAttrs allowlist (param.*), never as free-form metadata.
// "text_preview" is deliberately not blocked — it is a clipped preview by
// construction.
var blockedMetadataKeys = map[string]struct{}{ //nolint:gochecknoglobals // immutable privacy blocklist
	"args":     {},
	"prompt":   {},
	"response": {},
	"text":     {},
	"stdout":   {},
	"stderr":   {},
}

// projectAttributes builds the ordered attribute set for one event: fixed
// correlation attributes first, then metadata in sorted key order, stopping at
// maxAttributes. Fixed attributes win the cap.
func projectAttributes(ev telemetry.Event) []attribute.KeyValue {
	return projectEventAttributes(ev, nil)
}

// projectEventAttributes is projectAttributes with a metadata exclusion set
// (used by spans to skip keys they handle explicitly).
func projectEventAttributes(ev telemetry.Event, excluded map[string]struct{}) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, maxAttributes)
	appendString := func(key, value string) {
		if value == "" || len(attrs) >= maxAttributes {
			return
		}
		attrs = append(attrs, attribute.String(key, value))
	}
	appendString("session.id", ev.SessionID)
	appendString("run.id", ev.RunID)
	appendString("agent.id", ev.AgentID)
	appendString("node.id", ev.NodeID)
	appendString("task.id", ev.TaskID)
	if ev.Seq != 0 && len(attrs) < maxAttributes {
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key("relurpify.seq"),
			Value: uintAttrValue(ev.Seq),
		})
	}
	// A trace id that cannot be turned into a trace context is preserved as an
	// attribute so nothing is lost (spec D5).
	if ev.TraceID != "" && !syntheticSpanContext(ev).TraceID().IsValid() {
		appendString("relurpify.trace_id", ev.TraceID)
		appendString("relurpify.span_id", ev.SpanID)
	}

	keys := make([]string, 0, len(ev.Metadata))
	for k := range ev.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(attrs) >= maxAttributes {
			break
		}
		if _, blocked := blockedMetadataKeys[key]; blocked {
			continue
		}
		if excluded != nil {
			if _, skip := excluded[key]; skip {
				continue
			}
		}
		value, ok := projectMetadataValue(ev.Metadata[key])
		if !ok {
			continue
		}
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(normalizeAttrKey(key)),
			Value: value,
		})
	}
	return attrs
}

// projectMetadataValue converts one metadata value to a native OTel value. The
// boolean is false when the value is nil (projected as absent, not "<nil>").
func projectMetadataValue(v any) (attribute.Value, bool) {
	switch t := v.(type) {
	case nil:
		return attribute.Value{}, false
	case string:
		return attribute.StringValue(truncateAttr(t)), true
	case bool:
		return attribute.BoolValue(t), true
	case int:
		return attribute.Int64Value(int64(t)), true
	case int8:
		return attribute.Int64Value(int64(t)), true
	case int16:
		return attribute.Int64Value(int64(t)), true
	case int32:
		return attribute.Int64Value(int64(t)), true
	case int64:
		return attribute.Int64Value(t), true
	case uint:
		return uintAttrValue(uint64(t)), true
	case uint8:
		return uintAttrValue(uint64(t)), true
	case uint16:
		return uintAttrValue(uint64(t)), true
	case uint32:
		return uintAttrValue(uint64(t)), true
	case uint64:
		return uintAttrValue(t), true
	case float32:
		return attribute.Float64Value(float64(t)), true
	case float64:
		return attribute.Float64Value(t), true
	default:
		return attribute.StringValue(truncateAttr(stringifyValue(v))), true
	}
}

func uintAttrValue(u uint64) attribute.Value {
	if u <= math.MaxInt64 {
		return attribute.Int64Value(int64(u))
	}
	return attribute.StringValue(strconv.FormatUint(u, 10))
}

// normalizeAttrKey enforces ^[a-z0-9_.]+$ by prefixing any offending key with
// relurpify.meta. so it stays queryable without polluting backend indexes.
func normalizeAttrKey(key string) string {
	if isCleanAttrKey(key) {
		return key
	}
	return metadataKeyPfx + key
}

func isCleanAttrKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '.':
		default:
			return false
		}
	}
	return true
}

// truncateAttr caps a value at maxAttrBytes on a UTF-8 boundary and marks the
// truncation. Short values are returned unchanged.
func truncateAttr(s string) string {
	if len(s) <= maxAttrBytes {
		return s
	}
	cut := maxAttrBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + attrTruncSuffix
}

// stringifyValue renders an arbitrary metadata value. Scalars use fmt.Sprint
// (which already tolerates pointer cycles and panicking String methods); maps,
// slices, arrays, and pointers are walked with a depth cap so a self-referential
// container can never recurse without bound.
func stringifyValue(v any) string {
	var b strings.Builder
	writeReflectValue(&b, reflect.ValueOf(v), 0)
	return b.String()
}

func writeReflectValue(b *strings.Builder, rv reflect.Value, depth int) {
	if !rv.IsValid() {
		b.WriteString("null")
		return
	}
	if depth >= maxValueDepth {
		b.WriteString("...")
		return
	}
	switch rv.Kind() {
	case reflect.Interface, reflect.Pointer:
		if rv.IsNil() {
			b.WriteString("null")
			return
		}
		writeReflectValue(b, rv.Elem(), depth+1)
	case reflect.Map:
		writeMapValue(b, rv, depth)
	case reflect.Slice, reflect.Array:
		writeSliceValue(b, rv, depth)
	case reflect.Struct:
		writeStructValue(b, rv, depth)
	default:
		_, _ = fmt.Fprint(b, rv.Interface())
	}
}

func writeMapValue(b *strings.Builder, rv reflect.Value, depth int) {
	keys := rv.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
	})
	b.WriteString("map[")
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		writeReflectValue(b, key, depth+1)
		b.WriteByte(':')
		writeReflectValue(b, rv.MapIndex(key), depth+1)
	}
	b.WriteByte(']')
}

func writeSliceValue(b *strings.Builder, rv reflect.Value, depth int) {
	b.WriteByte('[')
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		writeReflectValue(b, rv.Index(i), depth+1)
	}
	b.WriteByte(']')
}

// writeStructValue walks struct fields under the same depth cap. fmt.Sprint is
// safe for pointer cycles but not for a struct that reaches back to itself
// through a map, so structs are rendered field-by-field instead. Unexported
// fields are shown as "?" because they cannot be reflected on.
func writeStructValue(b *strings.Builder, rv reflect.Value, depth int) {
	t := rv.Type()
	b.WriteString(t.Name())
	b.WriteByte('{')
	for i := 0; i < rv.NumField(); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.Field(i).Name)
		b.WriteByte(':')
		field := rv.Field(i)
		if field.CanInterface() {
			writeReflectValue(b, field, depth+1)
		} else {
			b.WriteByte('?')
		}
	}
	b.WriteByte('}')
}
