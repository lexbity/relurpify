package otelleg

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/observing"
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
	OpenSpans   int64
}

// Chain is the OTel export leg. It implements telemetry.Telemetry and io.Closer.
// It is safe for concurrent use and its Emit never panics and never blocks.
type Chain struct {
	settings Settings
	enabled  bool

	tracerProvider    *sdktrace.TracerProvider
	loggerProvider    *sdklog.LoggerProvider
	meterProvider     *sdkmetric.MeterProvider
	selfMeterProvider *sdkmetric.MeterProvider
	segment           *segmentWriter
	logger            otellog.Logger
	assembler         *spanAssembler
	meter             *meterBridge
	usage             *usageRecorder

	closed   atomic.Bool
	received atomic.Uint64
	dropped  atomic.Uint64
}

// Option configures a Chain at construction.
type Option func(*options)

type options struct {
	clock          func() time.Time
	toolMetrics    *telemetry.ToolCallMetrics
	usageDelegate  observing.UsageObserver
	usageModel     string
	droppedSources []droppedSource
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

// WithToolCallMetrics injects the process tool-call metrics instance the meter
// bridge polls. Without it, the relurpify.tool.* gauges are not registered.
func WithToolCallMetrics(metrics *telemetry.ToolCallMetrics) Option {
	return func(o *options) {
		if metrics != nil {
			o.toolMetrics = metrics
		}
	}
}

// WithUsageObserver injects the UsageObserver the UsageRecorder decorates.
// model, when non-empty, is exported as gen_ai.request.model.
func WithUsageObserver(delegate observing.UsageObserver, model string) Option {
	return func(o *options) {
		o.usageDelegate = delegate
		o.usageModel = model
	}
}

// WithDroppedSource registers an extra relurpify.telemetry.dropped series. The
// broadcast sink is the intended user (its DroppedTotal); wiring supplies it.
func WithDroppedSource(kind string, source func() int64) Option {
	return func(o *options) {
		if source != nil {
			o.droppedSources = append(o.droppedSources, droppedSource{kind: kind, value: source})
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
	chain := &Chain{settings: resolved, enabled: resolved.Enabled}
	chain.usage = &usageRecorder{delegate: cfg.usageDelegate, model: cfg.usageModel}
	if !resolved.Enabled {
		return chain, nil
	}
	res := newResource()
	lag := &exportLag{}

	var spanProcessors []sdktrace.SpanProcessor
	var logProcessors []sdklog.Processor
	var metricReaders []sdkmetric.Reader
	var selfMetricReaders []sdkmetric.Reader

	if resolved.FileEnabled {
		chain.segment = newSegmentWriter(resolved.FileDir, segmentPrefix, resolved.FileNameOverride, resolved.FileRetention, resolved.FileMaxSizeMB, cfg.clock)
		traceExp, err := newFileTraceExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry file trace exporter: %w", err)
		}
		metricExp, err := newFileMetricExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry file metric exporter: %w", err)
		}
		selfMetricExp, err := newFileMetricExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry file self-metric exporter: %w", err)
		}
		logExp, err := newFileLogExporter(ctx, chain.segment)
		if err != nil {
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry file log exporter: %w", err)
		}
		spanProcessors = append(spanProcessors, newBatchSpanProcessor(&lagSpanExporter{inner: traceExp, lag: lag, signal: signalTraces}))
		metricReaders = append(metricReaders, newPeriodicReader(metricExp, resolved.MetricsInterval))
		selfMetricReaders = append(selfMetricReaders, newPeriodicReader(selfMetricExp, resolved.MetricsInterval))
		logProcessors = append(logProcessors, newBatchLogProcessor(&lagLogExporter{inner: logExp, lag: lag, signal: signalLogs}))
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
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry OTLP trace exporter: %w", err)
		}
		metricExp, err := newNetworkMetricExporter(ctx, netCfg)
		if err != nil {
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry OTLP metric exporter: %w", err)
		}
		logExp, err := newNetworkLogExporter(ctx, netCfg)
		if err != nil {
			chain.closeOnError(ctx)
			return nil, fmt.Errorf("telemetry OTLP log exporter: %w", err)
		}
		spanProcessors = append(spanProcessors, newBatchSpanProcessor(&lagSpanExporter{inner: traceExp, lag: lag, signal: signalTraces}))
		metricReaders = append(metricReaders, newPeriodicReader(metricExp, resolved.MetricsInterval))
		logProcessors = append(logProcessors, newBatchLogProcessor(&lagLogExporter{inner: logExp, lag: lag, signal: signalLogs}))
	}

	chain.tracerProvider = newTracerProvider(res, resolved.SamplingRatio, spanProcessors)
	chain.loggerProvider = newLoggerProvider(res, logProcessors)
	chain.meterProvider = newMeterProvider(res, metricReaders)
	chain.selfMeterProvider = newMeterProvider(res, selfMetricReaders)
	chain.logger = chain.loggerProvider.Logger(instrumentationScopeName)
	chain.assembler = newSpanAssembler( //nolint:contextcheck // the assembler's background sweep owns no caller context
		chain.tracerProvider.Tracer(instrumentationScopeName),
		chain.logger,
		resolved.OrphanTimeout,
		maxOpenSpans,
		cfg.clock,
		func() { chain.dropped.Add(1) },
	)

	bridge, err := newMeterBridge(bridgeConfig{
		meter:          chain.meterProvider.Meter(instrumentationScopeName),
		selfMeter:      chain.selfMeterProvider.Meter(instrumentationScopeName),
		usage:          chain.usage,
		toolMetrics:    cfg.toolMetrics,
		droppedSources: chain.droppedSources(cfg.droppedSources),
		openSpans:      func() int64 { return chain.assembler.openSpans.Load() },
		lag:            lag,
	})
	if err != nil {
		chain.closeOnError(ctx)
		return nil, fmt.Errorf("telemetry meter bridge: %w", err)
	}
	chain.meter = bridge
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
		c.assembler.Emit(ev)
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
	// Final assembler sweep (and drain) before the providers flush (D9).
	c.assembler.shutdown() //nolint:contextcheck // the assembler owns its own background context
	// Unregister observable callbacks before shutting the providers down.
	c.meter.shutdown()
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
	if c.selfMeterProvider != nil {
		flush("self metric flush", c.selfMeterProvider.ForceFlush)
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
	if c.selfMeterProvider != nil {
		flush("self metric shutdown", c.selfMeterProvider.Shutdown)
	}
	if c.segment != nil {
		if err := c.segment.Close(); err != nil {
			errs = append(errs, fmt.Errorf("segment close: %w", err))
		}
	}
	return errors.Join(errs...)
}

// UsageRecorder returns the decorator the composition root installs on the
// usage observer. It forwards every record to the configured delegate and
// records the token/call instruments (spec §6.6).
func (c *Chain) UsageRecorder() observing.UsageObserver {
	if c == nil {
		return nil
	}
	return c.usage
}

// SegmentDir returns the directory the local OTLP-JSON segments are written to,
// or "" when the file leg is disabled. Readers (the TUI export, tooling) walk
// the segments from here.
func (c *Chain) SegmentDir() string {
	if c == nil {
		return ""
	}
	return c.settings.FileDir
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
	if c.assembler != nil {
		snap.OpenSpans = c.assembler.openSpans.Load()
	}
	return snap
}

// droppedSources is the built-in relurpify.telemetry.dropped series set.
func (c *Chain) droppedSources(extra []droppedSource) []droppedSource {
	sources := []droppedSource{
		{kind: "chain", value: func() int64 { return saturatingInt64(c.dropped.Load()) }},
		{kind: "file", value: func() int64 {
			if c.segment == nil {
				return 0
			}
			return saturatingInt64(c.segment.DroppedTotal())
		}},
	}
	return append(sources, extra...)
}

// closeOnError releases a partially constructed chain.
func (c *Chain) closeOnError(ctx context.Context) {
	if c.assembler != nil {
		c.assembler.stop()
	}
	if c.tracerProvider != nil {
		_ = c.tracerProvider.Shutdown(ctx)
	}
	if c.loggerProvider != nil {
		_ = c.loggerProvider.Shutdown(ctx)
	}
	if c.meterProvider != nil {
		_ = c.meterProvider.Shutdown(ctx)
	}
	if c.selfMeterProvider != nil {
		_ = c.selfMeterProvider.Shutdown(ctx)
	}
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

// saturatingInt64 converts an unsigned counter to int64, clamping instead of
// wrapping (a counter near MaxInt64 would otherwise report a negative gauge).
func saturatingInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

var _ telemetry.Telemetry = (*Chain)(nil)
