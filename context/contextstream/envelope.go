package contextstream

import (
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

// ApplyResult writes streamed refs and metadata into an envelope.
func ApplyResult(env *contextdata.Envelope, result *Result) error {
	if env == nil || result == nil {
		return nil
	}
	if result.Compilation != nil {
		for _, ref := range result.Compilation.StreamedRefs {
			env.AddStreamedContextReference(contextdata.ChunkReference{ChunkID: contextdata.ChunkID(ref)})
		}
		ApplyStaleGaps(env, result.Compilation)
	}
	if result.Record != nil {
		env.SetAssemblyMetadata(contextdata.AssemblyMeta{CompilationID: result.Record.ID})
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
