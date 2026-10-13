package otelleg

import (
	"context"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/observing"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Metric instrument names and attributes (spec §6.6).
const (
	instrumentLLMUsage   = "relurpify.llm.tokens"
	instrumentLLMCalls   = "relurpify.llm.calls"
	instrumentPerfPrefix = "relurpify.perf."
	instrumentToolPrefix = "relurpify.tool."
	instrumentDropped    = "relurpify.telemetry.dropped"
	instrumentOpenSpans  = "relurpify.telemetry.open_spans"
	instrumentExportLag  = "relurpify.export.lag"

	attrGenAIModel     = "gen_ai.request.model"
	attrUsageKind      = "relurpify.tokens.kind"
	attrUsageEstimated = "relurpify.tokens.estimated"
	attrLLMEstimated   = "relurpify.llm.estimated"
	attrDroppedKind    = "relurpify.dropped.kind"
	attrSignal         = "relurpify.signal"

	usageKindPrompt     = "prompt"
	usageKindCompletion = "completion"

	unitTokens = "{token}"
	unitCalls  = "{call}"
	unitEvents = "{event}"
	unitSpans  = "{span}"
	unitRatio  = "1"
	unitMillis = "ms"
	unitNanos  = "ns"
	unitCount  = "{count}"
)

// perfGauge maps one perfstats snapshot field to a gauge name and unit.
type perfGauge struct {
	name string
	unit string
	get  func(perfstats.Snapshot) int64
}

// perfGauges is the 13-instrument perfstats bridge (spec §6.6). It is immutable.
var perfGauges = []perfGauge{ //nolint:gochecknoglobals // immutable instrument table
	{"branch_clones", unitCount, func(s perfstats.Snapshot) int64 { return s.BranchClones }},
	{"branch_merge_count", unitCount, func(s perfstats.Snapshot) int64 { return s.BranchMergeCount }},
	{"branch_merge_duration_nanos", unitNanos, func(s perfstats.Snapshot) int64 { return s.BranchMergeDurationNanos }},
	{"artifact_budget_rescan_count", unitCount, func(s perfstats.Snapshot) int64 { return s.ArtifactBudgetRescanCount }},
	{"artifact_budget_rescan_items", unitCount, func(s perfstats.Snapshot) int64 { return s.ArtifactBudgetRescanItems }},
	{"progressive_file_read_count", unitCount, func(s perfstats.Snapshot) int64 { return s.ProgressiveFileReadCount }},
	{"progressive_file_reread_count", unitCount, func(s perfstats.Snapshot) int64 { return s.ProgressiveFileRereadCount }},
	{"progressive_demotion_read_count", unitCount, func(s perfstats.Snapshot) int64 { return s.ProgressiveDemotionReadCount }},
	{"retrieval_schema_check_count", unitCount, func(s perfstats.Snapshot) int64 { return s.RetrievalSchemaCheckCount }},
	{"retrieval_corpus_stamp_count", unitCount, func(s perfstats.Snapshot) int64 { return s.RetrievalCorpusStampCount }},
	{"runtime_memory_search_count", unitCount, func(s perfstats.Snapshot) int64 { return s.RuntimeMemorySearchCount }},
	{"runtime_memory_search_duration_nanos", unitNanos, func(s perfstats.Snapshot) int64 { return s.RuntimeMemorySearchDurationNanos }},
	{"capability_registry_rebuild_count", unitCount, func(s perfstats.Snapshot) int64 { return s.CapabilityRegistryRebuildCount }},
}

// toolGauge maps one ToolCallMetrics.Snapshot key to a gauge name/unit. The
// success_rate key is the only float64 field.
type toolGauge struct {
	key   string
	name  string
	unit  string
	isF64 bool
}

// toolGauges is the 7-instrument ToolCallMetrics bridge (spec §6.6).
var toolGauges = []toolGauge{ //nolint:gochecknoglobals // immutable instrument table
	{key: "calls_total", name: "calls_total", unit: unitCalls},
	{key: "calls_failed", name: "calls_failed", unit: unitCalls},
	{key: "success_rate", name: "success_rate", unit: unitRatio, isF64: true},
	{key: "truncations_total", name: "truncations_total", unit: unitCalls},
	{key: "denials_total", name: "denials_total", unit: unitCalls},
	{key: "doom_loops_total", name: "doom_loops_total", unit: unitCalls},
	{key: "avg_duration_ns", name: "avg_duration_ns", unit: unitNanos},
}

// droppedSource is one series of the relurpify.telemetry.dropped gauge.
type droppedSource struct {
	kind  string
	value func() int64
}

// usageRecorder decorates an observing.UsageObserver: it forwards every record
// to the delegate (budget tracking is never replaced) and records the token and
// call instruments.
type usageRecorder struct {
	delegate observing.UsageObserver
	model    string
	tokens   metric.Float64Histogram
	calls    metric.Int64Counter
}

// RecordTokenUsage implements observing.UsageObserver.
func (r *usageRecorder) RecordTokenUsage(usage observing.TokenUsage) {
	if r == nil {
		return
	}
	if r.delegate != nil {
		r.delegate.RecordTokenUsage(usage)
	}
	if r.tokens == nil && r.calls == nil {
		return
	}
	ctx := context.Background()
	modelAttrs := make([]attribute.KeyValue, 0, 1)
	if r.model != "" {
		modelAttrs = append(modelAttrs, attribute.String(attrGenAIModel, r.model))
	}
	with := func(extra ...attribute.KeyValue) []attribute.KeyValue {
		out := make([]attribute.KeyValue, 0, len(modelAttrs)+len(extra))
		out = append(out, modelAttrs...)
		return append(out, extra...)
	}
	if r.tokens != nil {
		r.tokens.Record(ctx, float64(usage.PromptTokens), metric.WithAttributes(
			with(attribute.String(attrUsageKind, usageKindPrompt), attribute.Bool(attrUsageEstimated, usage.Estimated))...))
		r.tokens.Record(ctx, float64(usage.CompletionTokens), metric.WithAttributes(
			with(attribute.String(attrUsageKind, usageKindCompletion), attribute.Bool(attrUsageEstimated, usage.Estimated))...))
	}
	if r.calls != nil {
		r.calls.Add(ctx, 1, metric.WithAttributes(
			with(attribute.Bool(attrLLMEstimated, usage.Estimated))...))
	}
}

// ConsumeResetNotice implements observing.UsageObserver.
func (r *usageRecorder) ConsumeResetNotice() (any, bool) {
	if r == nil || r.delegate == nil {
		return nil, false
	}
	return r.delegate.ConsumeResetNotice()
}

// exportLag holds the self-meter export-lag histogram. The exporters are built
// before the meter provider, so the histogram is installed after construction;
// the lock makes the handoff race-free.
type exportLag struct {
	mu   sync.RWMutex
	hist metric.Float64Histogram
}

func (l *exportLag) setHistogram(hist metric.Float64Histogram) {
	l.mu.Lock()
	l.hist = hist
	l.mu.Unlock()
}

func (l *exportLag) record(ctx context.Context, signal string, lag time.Duration) {
	l.mu.RLock()
	hist := l.hist
	l.mu.RUnlock()
	if hist == nil {
		return
	}
	millis := float64(lag) / float64(time.Millisecond)
	if millis < 0 {
		millis = 0
	}
	hist.Record(ctx, millis, metric.WithAttributes(attribute.String(attrSignal, signal)))
}

// lagSpanExporter records the emit→export delay for a batch before delegating.
type lagSpanExporter struct {
	inner  sdktrace.SpanExporter
	lag    *exportLag
	signal string
}

func (e *lagSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if e.lag != nil {
		var newest time.Time
		for _, span := range spans {
			if end := span.EndTime(); end.After(newest) {
				newest = end
			}
		}
		if !newest.IsZero() {
			e.lag.record(ctx, e.signal, time.Since(newest))
		}
	}
	return e.inner.ExportSpans(ctx, spans)
}

func (e *lagSpanExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

// lagLogExporter records the emit→export delay for a batch before delegating.
type lagLogExporter struct {
	inner  sdklog.Exporter
	lag    *exportLag
	signal string
}

func (e *lagLogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	if e.lag != nil {
		var newest time.Time
		for i := range records {
			if ts := records[i].Timestamp(); ts.After(newest) {
				newest = ts
			}
		}
		if !newest.IsZero() {
			e.lag.record(ctx, e.signal, time.Since(newest))
		}
	}
	return e.inner.Export(ctx, records)
}

func (e *lagLogExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

func (e *lagLogExporter) ForceFlush(ctx context.Context) error {
	return e.inner.ForceFlush(ctx)
}

// bridgeConfig carries the sources the meter bridge observes.
type bridgeConfig struct {
	meter          metric.Meter
	selfMeter      metric.Meter
	usage          *usageRecorder
	toolMetrics    *telemetry.ToolCallMetrics
	droppedSources []droppedSource
	openSpans      func() int64
	lag            *exportLag
}

// meterBridge owns the metric instruments and their callback registrations.
type meterBridge struct {
	registrations []metric.Registration
}

// newMeterBridge builds the instruments. Regular instruments come from the
// OTLP+file meter; self instruments (dropped, open_spans, export lag) come from
// the file-only self meter, which is what keeps self-telemetry off the network
// (D8 loop protection).
func newMeterBridge(cfg bridgeConfig) (*meterBridge, error) {
	bridge := &meterBridge{}

	tokens, err := cfg.meter.Float64Histogram(instrumentLLMUsage, metric.WithUnit(unitTokens))
	if err != nil {
		return nil, err
	}
	calls, err := cfg.meter.Int64Counter(instrumentLLMCalls, metric.WithUnit(unitCalls))
	if err != nil {
		return nil, err
	}
	if cfg.usage != nil {
		cfg.usage.tokens = tokens
		cfg.usage.calls = calls
	}

	registrations, err := registerPerfGauges(cfg.meter)
	if err != nil {
		return nil, err
	}
	bridge.registrations = append(bridge.registrations, registrations...)

	if cfg.toolMetrics != nil {
		registrations, err = registerToolGauges(cfg.meter, cfg.toolMetrics)
		if err != nil {
			return nil, err
		}
		bridge.registrations = append(bridge.registrations, registrations...)
	}

	if cfg.lag != nil {
		histogram, err := cfg.selfMeter.Float64Histogram(instrumentExportLag, metric.WithUnit(unitMillis))
		if err != nil {
			return nil, err
		}
		cfg.lag.setHistogram(histogram)
	}

	registrations, err = registerDroppedGauge(cfg.selfMeter, cfg.droppedSources)
	if err != nil {
		return nil, err
	}
	bridge.registrations = append(bridge.registrations, registrations...)

	registrations, err = registerOpenSpansGauge(cfg.selfMeter, cfg.openSpans)
	if err != nil {
		return nil, err
	}
	bridge.registrations = append(bridge.registrations, registrations...)

	return bridge, nil
}

func (b *meterBridge) shutdown() {
	if b == nil {
		return
	}
	for _, reg := range b.registrations {
		_ = reg.Unregister()
	}
	b.registrations = nil
}

func registerPerfGauges(meter metric.Meter) ([]metric.Registration, error) {
	instruments := make([]metric.Int64ObservableGauge, len(perfGauges))
	for i, gauge := range perfGauges {
		inst, err := meter.Int64ObservableGauge(instrumentPerfPrefix+gauge.name, metric.WithUnit(gauge.unit))
		if err != nil {
			return nil, err
		}
		instruments[i] = inst
	}
	observables := make([]metric.Observable, len(instruments))
	for i := range instruments {
		observables[i] = instruments[i]
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		snapshot := perfstats.Get()
		for i, gauge := range perfGauges {
			observer.ObserveInt64(instruments[i], gauge.get(snapshot))
		}
		return nil
	}, observables...)
	if err != nil {
		return nil, err
	}
	return []metric.Registration{registration}, nil
}

func registerToolGauges(meter metric.Meter, metrics *telemetry.ToolCallMetrics) ([]metric.Registration, error) {
	intInstruments := make([]metric.Int64ObservableGauge, len(toolGauges))
	floatInstruments := make([]metric.Float64ObservableGauge, len(toolGauges))
	observables := make([]metric.Observable, 0, len(toolGauges))
	for i, gauge := range toolGauges {
		name := instrumentToolPrefix + gauge.name
		if gauge.isF64 {
			inst, err := meter.Float64ObservableGauge(name, metric.WithUnit(gauge.unit))
			if err != nil {
				return nil, err
			}
			floatInstruments[i] = inst
			observables = append(observables, inst)
			continue
		}
		inst, err := meter.Int64ObservableGauge(name, metric.WithUnit(gauge.unit))
		if err != nil {
			return nil, err
		}
		intInstruments[i] = inst
		observables = append(observables, inst)
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		snapshot := metrics.Snapshot()
		if snapshot == nil {
			return nil
		}
		for i, gauge := range toolGauges {
			raw, ok := snapshot[gauge.key]
			if !ok {
				continue
			}
			if gauge.isF64 {
				if value, ok := raw.(float64); ok {
					observer.ObserveFloat64(floatInstruments[i], value)
				}
				continue
			}
			if value, ok := raw.(int64); ok {
				observer.ObserveInt64(intInstruments[i], value)
			}
		}
		return nil
	}, observables...)
	if err != nil {
		return nil, err
	}
	return []metric.Registration{registration}, nil
}

func registerDroppedGauge(meter metric.Meter, sources []droppedSource) ([]metric.Registration, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	gauge, err := meter.Int64ObservableGauge(instrumentDropped, metric.WithUnit(unitEvents))
	if err != nil {
		return nil, err
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		for _, source := range sources {
			if source.value == nil {
				continue
			}
			observer.ObserveInt64(gauge, source.value(), metric.WithAttributes(attribute.String(attrDroppedKind, source.kind)))
		}
		return nil
	}, gauge)
	if err != nil {
		return nil, err
	}
	return []metric.Registration{registration}, nil
}

func registerOpenSpansGauge(meter metric.Meter, openSpans func() int64) ([]metric.Registration, error) {
	if openSpans == nil {
		return nil, nil
	}
	gauge, err := meter.Int64ObservableGauge(instrumentOpenSpans, metric.WithUnit(unitSpans))
	if err != nil {
		return nil, err
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(gauge, openSpans())
		return nil
	}, gauge)
	if err != nil {
		return nil, err
	}
	return []metric.Registration{registration}, nil
}
