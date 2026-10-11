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

// ErrStaleApplyDropped is the typed sentinel ApplyResult returns when the
// epoch guard rejects a result as stale (D-4, guard-first): the result
// mutated NOTHING — no refs, no gaps, no slice, no metadata — and the drop is
// the designed behavior, not a failure. Callers distinguish it with
// errors.As and emit contextstream.stale_apply_dropped on their own sink.
type ErrStaleApplyDropped struct {
	CurrentEpoch uint64
	GotEpoch     uint64
	RequestID    string
}

func (e *ErrStaleApplyDropped) Error() string {
	return fmt.Sprintf("contextstream: stale apply dropped: result epoch %d is older than current slice epoch %d (request %q)",
		e.GotEpoch, e.CurrentEpoch, e.RequestID)
}

// ApplyResult writes streamed refs and the compiled slice into an envelope,
// merging into the existing assembly metadata instead of replacing it. The
// epoch guard is the FIRST statement over compilation content (D-3): a
// result whose epoch is older than the stored slice's is dropped as-is and
// reported through the typed ErrStaleApplyDropped sentinel — the envelope is
// byte-identical after a stale apply. Equal-epoch re-application (node
// retry) is a replace, idempotent by content. Streamed refs dedup by chunk
// ID against the current reference set, so re-applied compilations cannot
// accumulate the reference list. AssemblyMetadata.EpochID never regresses
// (max semantics under update); the epoch is stamped so every applied
// compilation is attributable to a memory state. Emission of
// contextstream.stale_apply_dropped belongs to the caller (the coordinator
// and the stream node own sinks; a dropped result here returns, it does not
// report).
func ApplyResult(env *contextdata.Envelope, result *Result, epoch uint64) error {
	if env == nil || result == nil {
		return nil
	}
	if result.Compilation != nil {
		if current := env.StreamedSliceSnapshot(); current != nil && epoch < current.Epoch {
			return &ErrStaleApplyDropped{
				CurrentEpoch: current.Epoch,
				GotEpoch:     epoch,
				RequestID:    result.Request.ID,
			}
		}
		seen := make(map[contextdata.ChunkID]struct{})
		for _, ref := range env.ReferencesSnapshot().StreamedContext {
			seen[ref.ChunkID] = struct{}{}
		}
		for _, ref := range result.Compilation.StreamedRefs {
			id := contextdata.ChunkID(ref)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			env.AddStreamedContextReference(contextdata.ChunkReference{ChunkID: id})
		}
		ApplyStaleGaps(env, result.Compilation)
		if err := applyStreamedSlice(env, result.Compilation, epoch); err != nil {
			return err
		}
	}
	if result.Record != nil {
		env.UpdateAssemblyMetadata(func(meta contextdata.AssemblyMeta) contextdata.AssemblyMeta {
			meta.CompilationID = result.Record.ID
			if epoch > meta.EpochID {
				meta.EpochID = epoch
			}
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
// compilation result. The epoch guard lives in ApplyResult (guard-first, D-3);
// by the time this runs the result is current or equal-epoch (a replace). A
// zero-chunk compilation stores an empty slice: it is a legitimate state
// (D-3) that the renderer turns into zero prompt bytes.
func applyStreamedSlice(env *contextdata.Envelope, compilation *contextports.CompilationResult, epoch uint64) error {
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
