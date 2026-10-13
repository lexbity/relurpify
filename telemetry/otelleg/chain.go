package otelleg

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	// Batch-queue bounds (D8): absorb tool-result storms, flush often enough to
	// keep the file leg fresh.
	batchQueueSize    = 8192
	batchFlushTimeout = 5 * time.Second
	// closeFlushTimeout bounds each provider flush/shutdown during Close (D9).
	closeFlushTimeout = 10 * time.Second
	// closeTotalTimeout bounds the whole close sequence (NFR-5).
	closeTotalTimeout = 30 * time.Second

	serviceName = "relurpify"
)

// SelfSnapshot is the chain's own health, for tests and the degraded log. It
// grows with the metric bridge (slice 4).
type SelfSnapshot struct {
	Received    uint64
	Dropped     uint64
	FileDropped uint64
}

// Chain is the OTel export leg. It implements telemetry.Telemetry and io.Closer.
// It is safe for concurrent use and its Emit never panics and never blocks.
type Chain struct {
	settings Settings
	enabled  bool

	tracerProvider *sdktrace.TracerProvider
	loggerProvider *sdklog.LoggerProvider
	meterProvider  *sdkmetric.MeterProvider
	segment        *segmentWriter
	logger         otellog.Logger

	closed   atomic.Bool
	received atomic.Uint64
	dropped  atomic.Uint64
}

// Option configures a Chain at construction.
type Option func(*options)

type options struct {
	clock func() time.Time
}

// WithClock injects the time source used for segment rotation and pruning. It
// exists for deterministic tests; production uses time.Now.
func WithClock(clock func() time.Time) Option {
	return func(o *options) {
		if clock != nil {
			o.clock = clock
		}
	}
}

// New builds the export leg: resource, providers, processors, and exporters.
// A returned error is a configuration error (fail-closed, D9). A disabled
// Settings returns an inert chain that emits nothing and owns no resources.
func New(ctx context.Context, s Settings, opts ...Option) (*Chain, error) {
	resolved, err := s.resolve()
	if err != nil {
		return nil, err
	}
	cfg := options{clock: time.Now}
	for _, opt := range opts {
		opt(&cfg)
	}
	if !resolved.Enabled {
		return &Chain{settings: resolved}, nil
	}

	chain := &Chain{settings: resolved, enabled: true}
	res := newResource()

	var spanProcessors []sdktrace.SpanProcessor
	var logProcessors []sdklog.Processor
	var metricReaders []sdkmetric.Reader

	if resolved.FileEnabled {
		chain.segment = newSegmentWriter(resolved.FileDir, segmentPrefix, resolved.FileRetention, resolved.FileMaxSizeMB, cfg.clock)
		traceExp, err := newFileTraceExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError()
			return nil, fmt.Errorf("telemetry file trace exporter: %w", err)
		}
		metricExp, err := newFileMetricExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError()
			return nil, fmt.Errorf("telemetry file metric exporter: %w", err)
		}
		logExp, err := newFileLogExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError()
			return nil, fmt.Errorf("telemetry file log exporter: %w", err)
		}
		spanProcessors = append(spanProcessors, newBatchSpanProcessor(traceExp))
		metricReaders = append(metricReaders, newPeriodicReader(metricExp, resolved.MetricsInterval))
		logProcessors = append(logProcessors, newBatchLogProcessor(logExp))
	}

	if resolved.Endpoint != "" {
		netCfg := endpointConfig{
			endpoint: resolved.Endpoint,
			headers:  resolved.Headers,
			timeout:  resolved.Timeout,
			insecure: resolved.TLSInsecure,
			caFile:   resolved.CAFile,
		}
		traceExp, err := newNetworkTraceExporter(ctx, netCfg)
		if err != nil {
			chain.closeOnError()
			return nil, fmt.Errorf("telemetry OTLP trace exporter: %w", err)
		}
		metricExp, err := newNetworkMetricExporter(ctx, netCfg)
		if err != nil {
			chain.closeOnError()
			return nil, fmt.Errorf("telemetry OTLP metric exporter: %w", err)
		}
		logExp, err := newNetworkLogExporter(ctx, netCfg)
		if err != nil {
			chain.closeOnError()
			return nil, fmt.Errorf("telemetry OTLP log exporter: %w", err)
		}
		spanProcessors = append(spanProcessors, newBatchSpanProcessor(traceExp))
		metricReaders = append(metricReaders, newPeriodicReader(metricExp, resolved.MetricsInterval))
		logProcessors = append(logProcessors, newBatchLogProcessor(logExp))
	}

	chain.tracerProvider = newTracerProvider(res, resolved.SamplingRatio, spanProcessors)
	chain.loggerProvider = newLoggerProvider(res, logProcessors)
	chain.meterProvider = newMeterProvider(res, metricReaders)
	chain.logger = chain.loggerProvider.Logger(instrumentationScopeName)
	return chain, nil
}

// Emit implements telemetry.Telemetry. It never panics, never blocks on I/O, and
// tolerates a nil or closed chain (counting the drop).
//
// Routing: lifecycle events open and close spans (the assembler of slice 3) and
// are deliberately kept off the log signal; every other event becomes an OTel
// log record (slice 2).
func (c *Chain) Emit(ev telemetry.Event) {
	if c == nil {
		return
	}
	if !c.enabled || c.closed.Load() {
		c.dropped.Add(1)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			c.dropped.Add(1)
			log.Printf("telemetry: export leg recovered from panic: %v", r)
		}
	}()
	c.received.Add(1)
	if isLifecycleType(ev.Type) {
		// Slice 3 installs the span assembler here. Routing lifecycle events
		// away from the log signal now keeps them from being mis-typed as point
		// events once the assembler lands.
		return
	}
	emitLog(c.logger, ev)
}

// Close flushes and shuts down every provider, then closes the segment writer.
// It is idempotent. The close sequence is bounded (D9/NFR-5) and errors are
// joined rather than fatal. ctx must be non-nil, as Go convention requires.
func (c *Chain) Close(ctx context.Context) error {
	if c == nil || !c.enabled {
		return nil
	}
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, closeTotalTimeout)
	defer cancel()

	var errs []error
	flush := func(name string, fn func(context.Context) error) {
		fctx, done := context.WithTimeout(ctx, closeFlushTimeout)
		defer done()
		if err := fn(fctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if c.tracerProvider != nil {
		flush("trace flush", c.tracerProvider.ForceFlush)
	}
	if c.loggerProvider != nil {
		flush("log flush", c.loggerProvider.ForceFlush)
	}
	if c.meterProvider != nil {
		flush("metric flush", c.meterProvider.ForceFlush)
	}
	if c.tracerProvider != nil {
		flush("trace shutdown", c.tracerProvider.Shutdown)
	}
	if c.loggerProvider != nil {
		flush("log shutdown", c.loggerProvider.Shutdown)
	}
	if c.meterProvider != nil {
		flush("metric shutdown", c.meterProvider.Shutdown)
	}
	if c.segment != nil {
		if err := c.segment.Close(); err != nil {
			errs = append(errs, fmt.Errorf("segment close: %w", err))
		}
	}
	return errors.Join(errs...)
}

// SelfMetrics returns the chain's own counters.
func (c *Chain) SelfMetrics() SelfSnapshot {
	if c == nil {
		return SelfSnapshot{}
	}
	snap := SelfSnapshot{Received: c.received.Load(), Dropped: c.dropped.Load()}
	if c.segment != nil {
		snap.FileDropped = c.segment.DroppedTotal()
	}
	return snap
}

// closeOnError releases a partially constructed chain.
func (c *Chain) closeOnError() {
	if c.segment != nil {
		_ = c.segment.Close()
		c.segment = nil
	}
}

func newBatchSpanProcessor(exp sdktrace.SpanExporter) sdktrace.SpanProcessor {
	return sdktrace.NewBatchSpanProcessor(exp,
		sdktrace.WithMaxQueueSize(batchQueueSize),
		sdktrace.WithBatchTimeout(batchFlushTimeout),
	)
}

func newBatchLogProcessor(exp sdklog.Exporter) sdklog.Processor {
	return sdklog.NewBatchProcessor(exp,
		sdklog.WithMaxQueueSize(batchQueueSize),
		sdklog.WithExportInterval(batchFlushTimeout),
	)
}

func newPeriodicReader(exp sdkmetric.Exporter, interval time.Duration) sdkmetric.Reader {
	return sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(interval))
}

func newTracerProvider(res *resource.Resource, ratio float64, processors []sdktrace.SpanProcessor) *sdktrace.TracerProvider {
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	}
	for _, p := range processors {
		opts = append(opts, sdktrace.WithSpanProcessor(p))
	}
	return sdktrace.NewTracerProvider(opts...)
}

func newLoggerProvider(res *resource.Resource, processors []sdklog.Processor) *sdklog.LoggerProvider {
	opts := []sdklog.LoggerProviderOption{sdklog.WithResource(res)}
	for _, p := range processors {
		opts = append(opts, sdklog.WithProcessor(p))
	}
	return sdklog.NewLoggerProvider(opts...)
}

func newMeterProvider(res *resource.Resource, readers []sdkmetric.Reader) *sdkmetric.MeterProvider {
	opts := []sdkmetric.Option{sdkmetric.WithResource(res)}
	for _, r := range readers {
		opts = append(opts, sdkmetric.WithReader(r))
	}
	return sdkmetric.NewMeterProvider(opts...)
}

// newResource builds the process-wide resource explicitly with
// resource.NewSchemaless: the SDK's default resource constructor would read
// OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME from the environment, which the
// workspace forbids.
func newResource() *resource.Resource {
	return resource.NewSchemaless(
		attribute.String("service.name", serviceName),
		attribute.String("service.version", buildVersion()),
		attribute.String("service.instance.id", newInstanceID()),
		attribute.Int("process.pid", os.Getpid()),
	)
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil || info.Main.Version == "" {
		return "devel"
	}
	return info.Main.Version
}

// newInstanceID returns a random RFC-4122 v4 UUID. crypto/rand is used directly
// to avoid adding a UUID dependency for one identifier.
func newInstanceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var _ telemetry.Telemetry = (*Chain)(nil)
