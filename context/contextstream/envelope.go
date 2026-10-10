package contextstream

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// streamedSliceByteBudget caps the stored slice at 2× the compilation budget
// in bytes, estimating 4 bytes per token (NFR-2). A slice beyond the bound is
// an internal inconsistency of the compiler's own accounting and fails the
// applying node rather than bloating the envelope.
const streamedSliceByteBudgetFactor = 2

// ApplyResult writes streamed refs and the compiled slice into an envelope,
// merging into the existing assembly metadata instead of replacing it. The
// slice is stored under the epoch guard (D-4): a result whose epoch is older
// than the stored slice's is dropped — never rendered — and counted via
// contextstream.stale_apply_dropped. The epoch is stamped so every applied
// compilation is attributable to a memory state.
func ApplyResult(ctx context.Context, env *contextdata.Envelope, result *Result, epoch uint64) error {
	if env == nil || result == nil {
		return nil
	}
	if result.Compilation != nil {
		for _, ref := range result.Compilation.StreamedRefs {
			env.AddStreamedContextReference(contextdata.ChunkReference{ChunkID: contextdata.ChunkID(ref)})
		}
		ApplyStaleGaps(env, result.Compilation)
		if err := applyStreamedSlice(ctx, env, result.Compilation, epoch); err != nil {
			return err
		}
	}
	if result.Record != nil {
		env.UpdateAssemblyMetadata(func(meta contextdata.AssemblyMeta) contextdata.AssemblyMeta {
			meta.CompilationID = result.Record.ID
			meta.EpochID = epoch
			return meta
		})
	}
	if result.Trim.ShortfallTokens > 0 || len(result.Trim.Substitutions) > 0 {
		env.SetWorkingValueWithClass("contextstream.trimmed", true, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass("contextstream.shortfall_tokens", result.Trim.ShortfallTokens, contextdata.MemoryClassTask)
	}
	if result.Request.ID != "" {
		env.SetWorkingValueWithClass("contextstream.request_id", result.Request.ID, contextdata.MemoryClassTask)
	}
	if result.Err != nil {
		env.SetWorkingValueWithClass("contextstream.error", result.Err.Error(), contextdata.MemoryClassTask)
	}
	return nil
}

// applyStreamedSlice builds the envelope's StreamedSlice from the typed
// compilation result and stores it under the epoch guard. A zero-chunk
// compilation stores an empty slice: it is a legitimate state (D-3) that the
// renderer turns into zero prompt bytes.
func applyStreamedSlice(ctx context.Context, env *contextdata.Envelope, compilation *contextports.CompilationResult, epoch uint64) error {
	if current := env.StreamedSliceSnapshot(); current != nil && epoch < current.Epoch {
		EmitStaleApplyDropped(ctx, current.Epoch, epoch, current.RequestID)
		return nil
	}

	slice := &contextdata.StreamedSlice{
		RequestID:  compilation.Record.ID,
		Epoch:      epoch,
		CompiledAt: time.Now().UTC(),
		CacheHit:   compilation.Record.CacheHit,
	}
	for _, chunk := range compilation.StreamedChunks {
		slice.Chunks = append(slice.Chunks, contextdata.StreamedChunk{
			ChunkID:       contextdata.ChunkID(chunk.ChunkID),
			ContentHash:   chunk.ContentHash,
			Body:          chunk.Body,
			TokenEstimate: chunk.TokenEstimate,
			TrustClass:    chunk.TrustClass,
			IsSummary:     chunk.IsSummary,
		})
	}
	slice.FinalTokens = compilation.Record.FinalTokens
	slice.BudgetTokens = compilation.Record.OriginalBudget
	slice.GapMessages = append([]string(nil), compilation.SkippedStaleChunks...)

	if slice.BudgetTokens > 0 {
		byteEstimate := 0
		for _, chunk := range slice.Chunks {
			byteEstimate += chunk.TokenEstimate * 4
		}
		if limit := streamedSliceByteBudgetFactor * slice.BudgetTokens * 4; byteEstimate > limit {
			return fmt.Errorf("contextstream: streamed slice exceeds 2x budget: %d bytes > %d (budget %d tokens)", byteEstimate, limit, slice.BudgetTokens)
		}
	}

	env.SetStreamedSlice(slice)
	return nil
}

// ApplyStaleGaps surfaces stale chunks skipped during compilation into the envelope.
func ApplyStaleGaps(env *contextdata.Envelope, compilation *contextports.CompilationResult) {
	if env == nil || compilation == nil || len(compilation.SkippedStaleChunks) == 0 {
		return
	}
	ids := make([]string, 0, len(compilation.SkippedStaleChunks))
	for _, id := range compilation.SkippedStaleChunks {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	env.SetWorkingValueWithClass("contextstream.skipped_stale_chunks", ids, contextdata.MemoryClassTask)
}

// ApplyRequestMetadata annotates an envelope before a streaming request starts.
func ApplyRequestMetadata(env *contextdata.Envelope, req Request) error {
	if env == nil {
		return nil
	}
	if req.ID != "" {
		env.SetWorkingValueWithClass("contextstream.request_id", req.ID, contextdata.MemoryClassTask)
	}
	if req.Mode != "" {
		env.SetWorkingValueWithClass("contextstream.mode", string(req.Mode), contextdata.MemoryClassTask)
	}
	if req.MaxTokens > 0 {
		env.SetWorkingValueWithClass("contextstream.max_tokens", req.MaxTokens, contextdata.MemoryClassTask)
	}
	if req.EventLogSeq > 0 {
		env.SetWorkingValueWithClass("contextstream.event_log_seq", req.EventLogSeq, contextdata.MemoryClassTask)
	}
	if len(req.Metadata) > 0 {
		env.SetWorkingValueWithClass("contextstream.request_metadata", req.Metadata, contextdata.MemoryClassTask)
	}
	if req.RequestedAt.IsZero() {
		env.SetWorkingValueWithClass("contextstream.requested_at", "", contextdata.MemoryClassTask)
	} else {
		env.SetWorkingValueWithClass("contextstream.requested_at", req.RequestedAt.UTC().Format(time.RFC3339Nano), contextdata.MemoryClassTask)
	}
	return nil
}

// EmitStaleApplyDropped reports an epoch-guard rejection (D-4). Emission is a
// no-op without a sink in the context.
func EmitStaleApplyDropped(ctx context.Context, currentEpoch, resultEpoch uint64, requestID string) {
	tel := telemetry.TelemetryFromContext(ctx)
	if tel == nil {
		return
	}
	tel.Emit(telemetry.Event{
		Type:      telemetry.EventContextStreamStaleApplyDropped,
		Message:   "stale stream result dropped by epoch guard",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"current_epoch": currentEpoch,
			"result_epoch":  resultEpoch,
			"request_id":    requestID,
		},
	})
}
