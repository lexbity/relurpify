package otelleg

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

// Assembler bounds (spec D2/NFR-7).
const (
	spanShardCount = 64
	maxOpenSpans   = 4096
	sweepInterval  = 60 * time.Second

	keySeparator = "\x1f"
)

// spanAssembler pairs lifecycle point events into OTel spans. It is the only
// component that reconstructs spans; emit sites remain point-event emitters
// (spec D2). It is safe for concurrent use and never blocks on I/O.
type spanAssembler struct {
	tracer        trace.Tracer
	logger        otellog.Logger
	orphanTimeout time.Duration
	maxOpen       int
	clock         func() time.Time
	onPanic       func()

	shards    [spanShardCount]assemblerShard
	openSpans atomic.Int64
	closed    atomic.Bool
	done      chan struct{}
	wg        sync.WaitGroup
}

type assemblerShard struct {
	mu    sync.Mutex
	spans map[string]*openSpan
}

type openSpan struct {
	key      string
	family   spanFamily
	span     trace.Span
	startTS  time.Time
	openedAt time.Time
}

// newSpanAssembler builds the assembler and starts its orphan-sweep goroutine.
// The caller owns the lifecycle and must call shutdown.
func newSpanAssembler(tracer trace.Tracer, logger otellog.Logger, orphanTimeout time.Duration, maxOpen int, clock func() time.Time, onPanic func()) *spanAssembler {
	if clock == nil {
		clock = time.Now
	}
	if maxOpen <= 0 {
		maxOpen = maxOpenSpans
	}
	a := &spanAssembler{
		tracer:        tracer,
		logger:        logger,
		orphanTimeout: orphanTimeout,
		maxOpen:       maxOpen,
		clock:         clock,
		onPanic:       onPanic,
		done:          make(chan struct{}),
	}
	for i := range a.shards {
		a.shards[i].spans = make(map[string]*openSpan)
	}
	a.wg.Add(1)
	go a.sweepLoop()
	return a
}

// Emit routes one lifecycle event: a start opens a span, a finish closes it, and
// an unassemblable event degrades to a log record. A panic during assembly is
// contained and reported through onPanic (spec D9).
func (a *spanAssembler) Emit(ev telemetry.Event) {
	if a == nil || a.closed.Load() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			if a.onPanic != nil {
				a.onPanic()
			}
			emitSelfLog(a.logger, "telemetry.span.panic", otellog.SeverityError1,
				fmt.Sprintf("span assembly panic: %v", r))
		}
	}()

	family, ok := lifecycleFamilyFor(ev.Type)
	if !ok {
		return
	}
	if otelSpanFlag(ev) {
		// Dedup rule (D2): an emit site already created a real span for this
		// scope, so the assembler must not fabricate a second one.
		return
	}
	key, _, ok := keyOf(ev, family)
	if !ok {
		// No trace id and no derivable identity: degrade to a point log record
		// rather than fabricate a wrongly-attached span (D2).
		emitLog(a.logger, ev)
		return
	}
	if isLifecycleStart(ev.Type) {
		a.handleStart(ev, family, key)
		return
	}
	a.handleFinish(ev, key)
}

// shutdown stops the sweep goroutine, ends expired orphans, then drains any
// still-open spans (they were never completed; ending them at the current time
// keeps the registry bounded and lets the flush export them).
func (a *spanAssembler) shutdown() {
	if !a.beginStop() {
		return
	}
	now := a.clock()
	a.sweepOrphans(now)
	a.drain(now)
}

// stop stops the sweep goroutine without sweeping or emitting self-records. It
// is the error-path teardown, where no spans have been assembled.
func (a *spanAssembler) stop() {
	a.beginStop()
}

func (a *spanAssembler) beginStop() bool {
	if a == nil {
		return false
	}
	if !a.closed.CompareAndSwap(false, true) {
		return false
	}
	close(a.done)
	a.wg.Wait()
	return true
}

func (a *spanAssembler) handleStart(ev telemetry.Event, family spanFamily, key string) {
	startTS := eventTimestamp(ev)
	openedAt := a.clock()

	// Evict before taking the target shard lock so two shard locks are never
	// held at once (D16.3: no deadlock possible).
	if a.openSpans.Load() >= int64(a.maxOpen) {
		a.evictOldest()
	}

	shard := a.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if existing, ok := shard.spans[key]; ok {
		// A duplicate start for the same key supersedes the older attempt.
		delete(shard.spans, key)
		a.openSpans.Add(-1)
		existing.end(codes.Error, "orphaned: duplicate start", startTS)
	}

	ctx := parentContext(ev)
	opts := []trace.SpanStartOption{trace.WithTimestamp(startTS)}
	if attrs := spanAttributes(ev, family); len(attrs) > 0 {
		opts = append(opts, trace.WithAttributes(attrs...))
	}
	_, span := a.tracer.Start(ctx, spanName(family, ev), opts...)
	shard.spans[key] = &openSpan{
		key:      key,
		family:   family,
		span:     span,
		startTS:  startTS,
		openedAt: openedAt,
	}
	a.openSpans.Add(1)
}

func (a *spanAssembler) handleFinish(ev telemetry.Event, key string) {
	endTS := eventTimestamp(ev)
	shard := a.shardFor(key)
	shard.mu.Lock()
	open, ok := shard.spans[key]
	if !ok {
		shard.mu.Unlock()
		emitLog(a.logger, withMetadata(ev, map[string]any{"relurpify.span.unpaired": true}))
		return
	}
	if endTS.Before(open.startTS) {
		// Cross-goroutine out-of-order arrival: negative duration. Treat as
		// unpaired and leave the open span for the orphan sweep (D2).
		shard.mu.Unlock()
		emitLog(a.logger, withMetadata(ev, map[string]any{"relurpify.span.out_of_order": true}))
		return
	}
	delete(shard.spans, key)
	a.openSpans.Add(-1)
	shard.mu.Unlock()

	if attrs := finishSpanAttributes(ev, open.family); len(attrs) > 0 {
		open.span.SetAttributes(attrs...)
	}
	code, desc := endStatus(ev)
	open.end(code, desc, endTS)
}

// sweepOrphans ends every span open longer than the orphan timeout (D2).
func (a *spanAssembler) sweepOrphans(now time.Time) {
	if a.orphanTimeout <= 0 {
		return
	}
	cutoff := now.Add(-a.orphanTimeout)
	for i := range a.shards {
		shard := &a.shards[i]
		var expired []*openSpan
		shard.mu.Lock()
		for key, open := range shard.spans {
			if open.openedAt.After(cutoff) {
				continue
			}
			delete(shard.spans, key)
			a.openSpans.Add(-1)
			expired = append(expired, open)
		}
		shard.mu.Unlock()
		for _, open := range expired {
			endTS := open.openedAt.Add(a.orphanTimeout)
			open.end(codes.Error, "orphaned: no matching end event", endTS)
			emitSelfLog(a.logger, "telemetry.span.orphaned", otellog.SeverityWarn1,
				"orphaned span ended",
				attribute.String("relurpify.span.family", open.family.name),
				attribute.String("relurpify.span.key", open.key))
		}
	}
}

// evictOldest ends the globally oldest open span as registry_overflow. It scans
// shards one at a time and never holds two locks at once.
func (a *spanAssembler) evictOldest() {
	var oldest *openSpan
	var oldestKey string
	var oldestShard *assemblerShard
	for i := range a.shards {
		shard := &a.shards[i]
		shard.mu.Lock()
		for key, open := range shard.spans {
			if oldest == nil || open.openedAt.Before(oldest.openedAt) {
				oldest, oldestKey, oldestShard = open, key, shard
			}
		}
		shard.mu.Unlock()
	}
	if oldest == nil {
		return
	}
	oldestShard.mu.Lock()
	if current, ok := oldestShard.spans[oldestKey]; ok && current == oldest {
		delete(oldestShard.spans, oldestKey)
		a.openSpans.Add(-1)
		oldest.end(codes.Error, "registry_overflow", oldest.openedAt)
	}
	oldestShard.mu.Unlock()
}

// drain ends every remaining open span with an unset status (it was never
// completed) so the registry empties and the flush can export what it has.
func (a *spanAssembler) drain(now time.Time) {
	for i := range a.shards {
		shard := &a.shards[i]
		shard.mu.Lock()
		spans := make([]*openSpan, 0, len(shard.spans))
		for key, open := range shard.spans {
			delete(shard.spans, key)
			a.openSpans.Add(-1)
			spans = append(spans, open)
		}
		shard.mu.Unlock()
		for _, open := range spans {
			open.span.End(trace.WithTimestamp(now))
		}
	}
}

func (a *spanAssembler) sweepLoop() {
	defer a.wg.Done()
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
			a.sweepOrphans(a.clock())
		}
	}
}

func (a *spanAssembler) shardFor(key string) *assemblerShard {
	return &a.shards[fnv1a(key)%spanShardCount]
}

func (s *openSpan) end(code codes.Code, desc string, endTS time.Time) {
	if s == nil || s.span == nil {
		return
	}
	s.span.SetStatus(code, desc)
	s.span.End(trace.WithTimestamp(endTS))
}

// keyOf builds the D2 span key (TraceID, family, identity). It returns false
// when neither a trace id nor an identity is available.
func keyOf(ev telemetry.Event, family spanFamily) (key, identity string, ok bool) {
	identity = identityFor(family, ev)
	if ev.TraceID == "" && identity == "" {
		return "", "", false
	}
	return ev.TraceID + keySeparator + family.name + keySeparator + identity, identity, true
}

func identityFor(family spanFamily, ev telemetry.Event) string {
	for _, key := range family.identityKeys {
		if value := eventValue(ev, key); value != "" {
			return value
		}
	}
	return ""
}

// eventValue resolves an identity key from the event's first-class correlation
// fields first, then leniently from metadata (D2/D5). First-class fields are
// what StampCorrelation actually populates on the current emit sites.
func eventValue(ev telemetry.Event, key string) string {
	switch key {
	case keyRunID:
		if ev.RunID != "" {
			return ev.RunID
		}
	case keyAgentID:
		if ev.AgentID != "" {
			return ev.AgentID
		}
	case keyNodeID:
		if ev.NodeID != "" {
			return ev.NodeID
		}
	case keyTaskID:
		if ev.TaskID != "" {
			return ev.TaskID
		}
	case keySpanID:
		if ev.SpanID != "" {
			return ev.SpanID
		}
	}
	return metadataString(ev, key)
}

// parentContext attaches the remote parent span context when the event carries
// one. With no explicit parent the span keeps the event's trace id by using the
// emitting scope's span id as the parent; the scope's own id is additionally
// recorded as relurpify.span.event_id (the SDK cannot adopt a caller span id).
func parentContext(ev telemetry.Event) context.Context {
	ctx := context.Background()
	sc := syntheticSpanContext(ev)
	if !sc.TraceID().IsValid() {
		return ctx
	}
	parent := sc
	if raw := eventValue(ev, keyParentSpanID); raw != "" {
		if spanID, err := trace.SpanIDFromHex(raw); err == nil && spanID.IsValid() {
			parent = trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    sc.TraceID(),
				SpanID:     spanID,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
		}
	}
	return trace.ContextWithRemoteSpanContext(ctx, parent)
}

func endStatus(ev telemetry.Event) (codes.Code, string) {
	switch ev.Type {
	case telemetry.EventNodeError:
		return codes.Error, "node error"
	case telemetry.EventDelegationCancel:
		return codes.Error, "cancelled"
	}
	if spanStatus(ev) == statusError {
		return codes.Error, "error"
	}
	return codes.Ok, ""
}

func spanName(family spanFamily, ev telemetry.Event) string {
	switch family.name {
	case familyGraph:
		return "agent graph"
	case familyNode:
		return joinSpanName(familyNode, firstNonEmpty(ev.NodeID, metadataString(ev, keyNodeID)))
	case familyAgent:
		return joinSpanName("agent run", firstNonEmpty(ev.AgentID, metadataString(ev, keyAgentID)))
	case familyDelegation:
		return joinSpanName(familyDelegation, metadataString(ev, keyDelegationID))
	case familyLLM:
		return joinSpanName(familyLLM, metadataString(ev, metaModel))
	case familyTool:
		return joinSpanName(familyTool, metadataString(ev, metaTool))
	case familyCapability:
		return joinSpanName(familyCapability, metadataString(ev, metaCapability))
	default:
		return family.name
	}
}

func joinSpanName(prefix, suffix string) string {
	if suffix == "" {
		return prefix
	}
	return prefix + " " + suffix
}

// spanAttributes is the start-time attribute set: fixed correlation, the
// event's own span id, and the family's key attributes (spec §6.1).
func spanAttributes(ev telemetry.Event, family spanFamily) []attribute.KeyValue {
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
		attrs = append(attrs, attribute.KeyValue{Key: attribute.Key("relurpify.seq"), Value: uintAttrValue(ev.Seq)})
	}
	if id := eventSpanID(ev); id != "" {
		appendString("relurpify.span.event_id", id)
	}

	switch family.name {
	case familyTool:
		attrs = appendSpanAttrs(attrs, ev.Metadata[metaSpanAttrs])
	case familyCapability:
		attrs = appendSpanAttrs(attrs, ev.Metadata[metaSpanAttrs])
		appendString("capability.id", metadataString(ev, metaCapabilityID))
		appendString("capability.name", metadataString(ev, metaCapability))
		appendString("capability.kind", metadataString(ev, metaKind))
		appendString("capability.runtime_family", metadataString(ev, metaRuntimeFamily))
	case familyLLM:
		appendString("gen_ai.request.model", metadataString(ev, metaModel))
		appendString("gen_ai.system", metadataString(ev, metaSystem))
		appendString("finish_reason", metadataString(ev, metaFinishReason))
		appendString("tool_calling_mode", metadataString(ev, metaToolCallingMode))
	}
	return attrs
}

// finishSpanAttributes is the end-time attribute set: tool/capability result
// facts, LLM response facts, and the authoritative duration_ms (spec D2).
func finishSpanAttributes(ev telemetry.Event, family spanFamily) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 6)
	switch family.name {
	case familyTool, familyCapability:
		for _, key := range []string{metaExitCode, metaStdoutBytes, metaArtifactRef} {
			if raw, ok := ev.Metadata[key]; ok {
				if value, ok := projectMetadataValue(raw); ok {
					attrs = append(attrs, attribute.KeyValue{Key: attribute.Key(key), Value: value})
				}
			}
		}
	case familyLLM:
		for _, key := range []string{metaFinishReason, metaToolCallingMode, metaModel} {
			if value := metadataString(ev, key); value != "" {
				attrs = append(attrs, attribute.String(spanAttrKeyForLLM(key), value))
			}
		}
	}
	if raw, ok := ev.Metadata[metaDurationMS]; ok {
		if value, ok := projectMetadataValue(raw); ok {
			attrs = append(attrs, attribute.KeyValue{Key: attribute.Key("relurpify.duration_ms"), Value: value})
		}
	}
	return attrs
}

// spanAttrKeyForLLM maps an LLM metadata key to its span attribute name.
func spanAttrKeyForLLM(key string) string {
	if key == metaModel {
		return "gen_ai.request.model"
	}
	return key
}

func appendSpanAttrs(attrs []attribute.KeyValue, raw any) []attribute.KeyValue {
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return attrs
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(attrs) >= maxAttributes {
			break
		}
		value, ok := projectMetadataValue(m[key])
		if !ok {
			continue
		}
		attrs = append(attrs, attribute.KeyValue{Key: attribute.Key(mapSpanAttrKey(key)), Value: value})
	}
	return attrs
}

// mapSpanAttrKey aligns the registry's span_attrs keys with the spec §6.1
// capability.* namespace.
func mapSpanAttrKey(key string) string {
	switch {
	case key == metaTrustClass:
		return "capability.trust_class"
	case key == metaEffectClass:
		return "capability.effect_class"
	case strings.HasPrefix(key, metaEffectClass+"_"):
		return "capability." + key
	default:
		return normalizeAttrKey(key)
	}
}

func metadataString(ev telemetry.Event, key string) string {
	raw, ok := ev.Metadata[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return fmt.Sprint(raw)
}

func eventSpanID(ev telemetry.Event) string {
	return firstNonEmpty(ev.SpanID, metadataString(ev, "span_id"))
}

func otelSpanFlag(ev telemetry.Event) bool {
	raw, ok := ev.Metadata[metaOTelSpan]
	if !ok {
		return false
	}
	b, ok := raw.(bool)
	return ok && b
}

func withMetadata(ev telemetry.Event, extra map[string]any) telemetry.Event {
	merged := make(map[string]any, len(ev.Metadata)+len(extra))
	for key, value := range ev.Metadata {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	ev.Metadata = merged
	return ev
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// fnv1a is the FNV-1a 64-bit hash used to shard the registry (D2).
func fnv1a(key string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= prime64
	}
	return h
}
