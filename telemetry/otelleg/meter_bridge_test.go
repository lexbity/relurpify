package otelleg

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/observing"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

type recordingUsageObserver struct {
	calls int
	last  observing.TokenUsage
}

func (o *recordingUsageObserver) RecordTokenUsage(usage observing.TokenUsage) {
	o.calls++
	o.last = usage
}

func (o *recordingUsageObserver) ConsumeResetNotice() (any, bool) { return nil, false }

type meterHarness struct {
	reader *sdkmetric.ManualReader
	bridge *meterBridge
}

func newMeterHarness(t *testing.T, cfg bridgeConfig) *meterHarness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	cfg.meter = provider.Meter(instrumentationScopeName)
	cfg.selfMeter = cfg.meter
	bridge, err := newMeterBridge(cfg)
	if err != nil {
		t.Fatalf("newMeterBridge: %v", err)
	}
	t.Cleanup(func() {
		bridge.shutdown()
		_ = provider.Shutdown(context.Background())
	})
	return &meterHarness{reader: reader, bridge: bridge}
}

func (h *meterHarness) collect(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	return rm
}

func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Metrics{}
}

func attrValue(set attribute.Set, key string) (attribute.Value, bool) {
	return set.Value(attribute.Key(key))
}

func TestTokenUsageRecordsHistogram(t *testing.T) {
	delegate := &recordingUsageObserver{}
	usage := &usageRecorder{delegate: delegate, model: testModelName}
	h := newMeterHarness(t, bridgeConfig{usage: usage})

	usage.RecordTokenUsage(observing.TokenUsage{PromptTokens: 10, CompletionTokens: 5})
	usage.RecordTokenUsage(observing.TokenUsage{PromptTokens: 3, Estimated: true})

	rm := h.collect(t)
	histogram, ok := findMetric(t, rm, instrumentLLMUsage).Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("llm tokens data is %T, want Histogram[float64]", findMetric(t, rm, instrumentLLMUsage).Data)
	}
	var total uint64
	var promptEstimatedCount uint64
	var promptEstimatedSum float64
	for _, dp := range histogram.DataPoints {
		total += dp.Count
		kind, _ := attrValue(dp.Attributes, attrUsageKind)
		estimated, _ := attrValue(dp.Attributes, attrUsageEstimated)
		model, _ := attrValue(dp.Attributes, attrGenAIModel)
		if model.AsString() != testModelName {
			t.Errorf("datapoint missing model attribute: %v", dp.Attributes)
		}
		if kind.AsString() == usageKindPrompt && estimated.AsBool() {
			promptEstimatedCount += dp.Count
			promptEstimatedSum += dp.Sum
		}
	}
	if total != 4 {
		t.Fatalf("token histogram total count = %d, want 4 (2 calls x 2 kinds)", total)
	}
	if promptEstimatedCount != 1 || promptEstimatedSum != 3 {
		t.Fatalf("estimated prompt datapoint = (count %d, sum %v), want (1, 3)", promptEstimatedCount, promptEstimatedSum)
	}

	sum, ok := findMetric(t, rm, instrumentLLMCalls).Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("llm calls data is %T, want Sum[int64]", findMetric(t, rm, instrumentLLMCalls).Data)
	}
	var callTotal int64
	for _, dp := range sum.DataPoints {
		callTotal += dp.Value
	}
	if callTotal != 2 {
		t.Fatalf("llm calls total = %d, want 2", callTotal)
	}

	if delegate.calls != 2 {
		t.Fatalf("delegate received %d records, want 2 (budget tracking must not be replaced)", delegate.calls)
	}
}

func TestPerfGaugesReadable(t *testing.T) {
	perfstats.Reset()
	perfstats.IncBranchClone()
	perfstats.IncRetrievalSchemaCheck()
	perfstats.ObserveRuntimeMemorySearch(2 * time.Second)
	h := newMeterHarness(t, bridgeConfig{})

	rm := h.collect(t)
	for name, want := range map[string]int64{
		instrumentPerfPrefix + "branch_clones":                        1,
		instrumentPerfPrefix + "retrieval_schema_check_count":         1,
		instrumentPerfPrefix + "runtime_memory_search_count":          1,
		instrumentPerfPrefix + "runtime_memory_search_duration_nanos": int64(2 * time.Second),
	} {
		gauge, ok := findMetric(t, rm, name).Data.(metricdata.Gauge[int64])
		if !ok {
			t.Fatalf("%s data is not Gauge[int64]", name)
		}
		if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != want {
			t.Fatalf("%s = %v, want %d", name, gauge.DataPoints, want)
		}
	}
}

func TestToolCallGauges(t *testing.T) {
	metrics := telemetry.NewToolCallMetrics()
	metrics.RecordCall(true, 10*time.Millisecond)
	metrics.RecordCall(false, 20*time.Millisecond)
	metrics.RecordDenial()
	metrics.RecordDoomLoop()
	h := newMeterHarness(t, bridgeConfig{toolMetrics: metrics})

	rm := h.collect(t)
	for name, want := range map[string]int64{
		instrumentToolPrefix + "calls_total":       2,
		instrumentToolPrefix + "calls_failed":      1,
		instrumentToolPrefix + "truncations_total": 0,
		instrumentToolPrefix + "denials_total":     1,
		instrumentToolPrefix + "doom_loops_total":  1,
		instrumentToolPrefix + "avg_duration_ns":   int64(15 * time.Millisecond),
	} {
		gauge, ok := findMetric(t, rm, name).Data.(metricdata.Gauge[int64])
		if !ok {
			t.Fatalf("%s data is not Gauge[int64]", name)
		}
		if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != want {
			t.Fatalf("%s = %v, want %d", name, gauge.DataPoints, want)
		}
	}
	rate, ok := findMetric(t, rm, instrumentToolPrefix+"success_rate").Data.(metricdata.Gauge[float64])
	if !ok || len(rate.DataPoints) != 1 || rate.DataPoints[0].Value != 0.5 {
		t.Fatalf("success_rate = %v, want 0.5", rate.DataPoints)
	}
}

func TestDroppedAndOpenSpansGauges(t *testing.T) {
	dropped := int64(4)
	h := newMeterHarness(t, bridgeConfig{
		droppedSources: []droppedSource{
			{kind: "chain", value: func() int64 { return dropped }},
			{kind: "file", value: func() int64 { return 2 }},
		},
		openSpans: func() int64 { return 3 },
	})

	rm := h.collect(t)
	gauge, ok := findMetric(t, rm, instrumentDropped).Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatal("dropped data is not Gauge[int64]")
	}
	kinds := map[string]int64{}
	for _, dp := range gauge.DataPoints {
		kind, _ := attrValue(dp.Attributes, attrDroppedKind)
		kinds[kind.AsString()] = dp.Value
	}
	if kinds["chain"] != 4 || kinds["file"] != 2 {
		t.Fatalf("dropped kinds = %v, want chain=4 file=2", kinds)
	}
	open, ok := findMetric(t, rm, instrumentOpenSpans).Data.(metricdata.Gauge[int64])
	if !ok || len(open.DataPoints) != 1 || open.DataPoints[0].Value != 3 {
		t.Fatalf("open_spans = %v, want 3", open.DataPoints)
	}
}

func TestExportLagHistogram(t *testing.T) {
	lag := &exportLag{}
	h := newMeterHarness(t, bridgeConfig{lag: lag})
	lag.record(context.Background(), signalTraces, 5*time.Millisecond)

	rm := h.collect(t)
	histogram, ok := findMetric(t, rm, instrumentExportLag).Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatal("export lag data is not Histogram[float64]")
	}
	if len(histogram.DataPoints) != 1 {
		t.Fatalf("export lag datapoints = %d, want 1", len(histogram.DataPoints))
	}
	dp := histogram.DataPoints[0]
	signal, _ := attrValue(dp.Attributes, attrSignal)
	if signal.AsString() != signalTraces {
		t.Fatalf("export lag signal = %q, want %q", signal.AsString(), signalTraces)
	}
	if dp.Count != 1 || dp.Sum != 5 {
		t.Fatalf("export lag = (count %d, sum %v), want (1, 5)", dp.Count, dp.Sum)
	}
}

// TestSelfMetricsExcludeOTLP is the loop-protection proof (D8): with only a
// collector endpoint configured (no file leg), regular metrics reach OTLP but
// self-metrics never do.
func TestSelfMetricsExcludeOTLP(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mu.Lock()
		bodies[r.URL.Path] = append(bodies[r.URL.Path], body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	chain, err := New(ctx, Settings{Enabled: true, Endpoint: server.URL, Timeout: time.Second, FileEnabled: false},
		WithUsageObserver(&recordingUsageObserver{}, testModelName))
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}
	chain.UsageRecorder().RecordTokenUsage(observing.TokenUsage{PromptTokens: 7, CompletionTokens: 2})
	if err := chain.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	received := bodies["/v1/metrics"]
	if len(received) == 0 {
		t.Fatal("no OTLP metric export received")
	}
	names := map[string]bool{}
	for _, body := range received {
		for name := range metricRequestNames(t, body) {
			names[name] = true
		}
	}
	if !names[instrumentLLMUsage] {
		t.Fatalf("regular metric missing from OTLP export: %v", names)
	}
	for _, self := range []string{instrumentDropped, instrumentOpenSpans, instrumentExportLag} {
		if names[self] {
			t.Fatalf("self-metric %q leaked to OTLP (loop protection broken)", self)
		}
	}
}

func metricRequestNames(t *testing.T, body []byte) map[string]bool {
	t.Helper()
	var data metricspb.MetricsData
	if err := proto.Unmarshal(body, &data); err != nil {
		t.Fatalf("decode OTLP metric body: %v", err)
	}
	names := map[string]bool{}
	for _, resourceMetrics := range data.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				names[metric.GetName()] = true
			}
		}
	}
	return names
}
