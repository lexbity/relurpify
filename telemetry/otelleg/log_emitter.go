package otelleg

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

// instrumentationScopeName identifies relurpify's records and spans in the
// backend's scope taxonomy.
const instrumentationScopeName = "codeburg.org/lexbit/relurpify/telemetry"

// emitLog converts one point event into an OTel log record (spec D3/§6.3):
// event name, severity, timestamp, synthetic trace context, body, and the
// projected attributes of §6.4.
func emitLog(logger otellog.Logger, ev telemetry.Event) {
	if logger == nil {
		return
	}
	severity := SeverityOf(ev.Type)
	var rec otellog.Record
	rec.SetTimestamp(eventTimestamp(ev))
	rec.SetEventName(string(ev.Type))
	rec.SetSeverity(severity)
	rec.SetSeverityText(severity.String())
	if ev.Message != "" {
		rec.SetBody(otellog.StringValue(ev.Message))
	}
	attrs := projectAttributes(ev)
	kvs := make([]otellog.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		kvs = append(kvs, otellog.KeyValueFromAttribute(a))
	}
	rec.AddAttributes(kvs...)
	// The OTel log API carries trace context on the context, not the record: the
	// SDK logger copies the span context into the record. The Telemetry
	// interface carries no context (it predates and is independent of OTel), so
	// the synthetic remote context is attached here, fire-and-forget.
	ctx := trace.ContextWithSpanContext(context.Background(), syntheticSpanContext(ev))
	logger.Emit(ctx, rec)
}

// eventTimestamp returns the event's timestamp, or the emit time when unset.
func eventTimestamp(ev telemetry.Event) time.Time {
	if ev.Timestamp.IsZero() {
		return time.Now()
	}
	return ev.Timestamp
}

// emitSelfLog writes a telemetry self-record (assembler orphan/overflow/panic
// notices). Self-records are not part of the event taxonomy, so their name and
// severity are explicit rather than resolved through SeverityOf.
func emitSelfLog(logger otellog.Logger, name string, severity otellog.Severity, body string, attrs ...attribute.KeyValue) {
	if logger == nil {
		return
	}
	var rec otellog.Record
	rec.SetTimestamp(time.Now())
	rec.SetEventName(name)
	rec.SetSeverity(severity)
	rec.SetSeverityText(severity.String())
	if body != "" {
		rec.SetBody(otellog.StringValue(body))
	}
	if len(attrs) > 0 {
		kvs := make([]otellog.KeyValue, 0, len(attrs))
		for _, a := range attrs {
			kvs = append(kvs, otellog.KeyValueFromAttribute(a))
		}
		rec.AddAttributes(kvs...)
	}
	logger.Emit(context.Background(), rec)
}

// syntheticSpanContext rebuilds an OTel span context from the correlation ids
// stamped by StampCorrelation (spec D5). A non-hex or absent TraceID yields a
// zero (trace-less) context; the raw ids are still projected as attributes by
// projectAttributes, so nothing is lost.
func syntheticSpanContext(ev telemetry.Event) trace.SpanContext {
	traceID, err := trace.TraceIDFromHex(ev.TraceID)
	if err != nil || !traceID.IsValid() {
		return trace.SpanContext{}
	}
	cfg := trace.SpanContextConfig{
		TraceID:    traceID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}
	if spanID, err := trace.SpanIDFromHex(ev.SpanID); err == nil && spanID.IsValid() {
		cfg.SpanID = spanID
	}
	return trace.NewSpanContext(cfg)
}
