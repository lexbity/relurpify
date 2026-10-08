package contextstream

import (
	"context"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// emitCompilerStarted reports the start of one compilation job (FR-12).
func (t *Trigger) emitCompilerStarted(ctx context.Context, req Request) {
	metadata := map[string]any{
		"request_id":    req.ID,
		"budget_tokens": req.MaxTokens,
		"mode":          string(req.Mode),
	}
	if text := trimQueryString(req.Query.Text); text != "" {
		metadata["query"] = text
	}
	t.emitTelemetry(ctx, telemetry.EventCompilerStarted, "compiler started", metadata)
}

// emitCompilerOutcome reports the outcome of one compilation job: completion,
// cache disposition, budget pressure, and substitution decisions (FR-12).
func (t *Trigger) emitCompilerOutcome(ctx context.Context, req Request, res *Result, startedAt time.Time) {
	if res == nil {
		return
	}
	durationMs := time.Since(startedAt).Milliseconds()
	base := map[string]any{
		"request_id":    req.ID,
		"budget_tokens": req.MaxTokens,
		"duration_ms":   durationMs,
	}
	if res.Compilation != nil {
		record := res.Compilation.Record
		if len(res.Compilation.StreamedRefs) > 0 {
			base["streamed_refs"] = len(res.Compilation.StreamedRefs)
		}
		if record.FinalTokens > 0 {
			base["final_tokens"] = record.FinalTokens
		}
		if record.CacheHit {
			t.emitTelemetry(ctx, telemetry.EventCompilerCacheHit, "compiler cache hit", cloneMetadata(base))
		} else {
			t.emitTelemetry(ctx, telemetry.EventCompilerCacheMiss, "compiler cache miss", cloneMetadata(base))
		}
		if record.Error != "" {
			base["error"] = record.Error
		}
	}
	if res.Err != nil {
		base["error"] = res.Err.Error()
	}
	t.emitTelemetry(ctx, telemetry.EventCompilerCompleted, "compiler completed", cloneMetadata(base))

	if res.Trim.ShortfallTokens > 0 {
		metadata := cloneMetadata(base)
		metadata["shortfall_tokens"] = res.Trim.ShortfallTokens
		t.emitTelemetry(ctx, telemetry.EventCompilerBudgetExceeded, "compiler budget exceeded", metadata)
	}
	if len(res.Trim.Substitutions) > 0 {
		metadata := cloneMetadata(base)
		metadata["substitutions"] = len(res.Trim.Substitutions)
		t.emitTelemetry(ctx, telemetry.EventCompilerSummarySubstituted, "compiler summary substituted", metadata)
	}
}

// emitTelemetry stamps correlation from ctx and dispatches one event. Emission
// is nil-guarded so a trigger without a sink remains a valid, silent trigger.
func (t *Trigger) emitTelemetry(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
	if t == nil || t.telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	t.telemetry.Emit(ev)
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func trimQueryString(text string) string {
	const max = 256
	if len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}
