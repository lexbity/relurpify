package otelleg

import (
	"context"
	"log"
	"sync/atomic"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// exportFailureCounter implements D8's drop accounting for the OTLP network
// leg: every failed export is counted, and the first failure logs exactly one
// warning — a collector outage must degrade to counted drops, never flood the
// workspace log. The SDK's own retry/drop semantics are unchanged: the error
// is passed through to the batch processor untouched.
type exportFailureCounter struct {
	dropped atomic.Uint64
	warned  atomic.Bool
}

func (f *exportFailureCounter) observe(err error) {
	if err == nil {
		return
	}
	f.dropped.Add(1)
	if f.warned.CompareAndSwap(false, true) {
		log.Printf("telemetry: OTLP export failed (continuing; failures accumulate): %v", err)
	}
}

// countedSpanExporter counts failed span exports before propagating the error.
type countedSpanExporter struct {
	inner   sdktrace.SpanExporter
	failure *exportFailureCounter
}

func (e *countedSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := e.inner.ExportSpans(ctx, spans)
	e.failure.observe(err)
	return err
}

func (e *countedSpanExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

// countedLogExporter counts failed log exports before propagating the error.
type countedLogExporter struct {
	inner   sdklog.Exporter
	failure *exportFailureCounter
}

func (e *countedLogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.inner.Export(ctx, records)
	e.failure.observe(err)
	return err
}

func (e *countedLogExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

func (e *countedLogExporter) ForceFlush(ctx context.Context) error {
	return e.inner.ForceFlush(ctx)
}

// countedMetricExporter counts failed metric exports before propagating the
// error. Temporality and aggregation preferences delegate untouched.
type countedMetricExporter struct {
	inner   sdkmetric.Exporter
	failure *exportFailureCounter
}

func (e *countedMetricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	err := e.inner.Export(ctx, data)
	e.failure.observe(err)
	return err
}

func (e *countedMetricExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

func (e *countedMetricExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return e.inner.Temporality(k)
}

func (e *countedMetricExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return e.inner.Aggregation(k)
}

func (e *countedMetricExporter) ForceFlush(ctx context.Context) error {
	return e.inner.ForceFlush(ctx)
}
