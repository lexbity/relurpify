package contextstream

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// ApplyResult writes streamed refs and metadata into an envelope.
func ApplyResult(env *contextdata.Envelope, result *Result) error {
	if env == nil || result == nil {
		return nil
	}
	if result.Compilation != nil {
		for _, ref := range result.Compilation.StreamedRefs {
			env.AddStreamedContextReference(ref)
		}
	}
	if result.Record != nil {
		env.AssemblyMetadata = result.Record.AssemblyMetadata
	}
	if result.Trim.Truncated {
		env.SetWorkingValue("contextstream.trimmed", true, contextdata.MemoryClassTask)
		env.SetWorkingValue("contextstream.shortfall_tokens", result.Trim.ShortfallTokens, contextdata.MemoryClassTask)
	}
	if result.Request.ID != "" {
		env.SetWorkingValue("contextstream.request_id", result.Request.ID, contextdata.MemoryClassTask)
	}
	if result.Err != nil {
		env.SetWorkingValue("contextstream.error", result.Err.Error(), contextdata.MemoryClassTask)
	}
	return nil
}

// ApplyRequestMetadata annotates an envelope before a streaming request starts.
func ApplyRequestMetadata(env *contextdata.Envelope, req Request) error {
	if env == nil {
		return nil
	}
	if req.ID != "" {
		env.SetWorkingValue("contextstream.request_id", req.ID, contextdata.MemoryClassTask)
	}
	if req.Mode != "" {
		env.SetWorkingValue("contextstream.mode", string(req.Mode), contextdata.MemoryClassTask)
	}
	if req.MaxTokens > 0 {
		env.SetWorkingValue("contextstream.max_tokens", req.MaxTokens, contextdata.MemoryClassTask)
	}
	if req.EventLogSeq > 0 {
		env.SetWorkingValue("contextstream.event_log_seq", req.EventLogSeq, contextdata.MemoryClassTask)
	}
	if len(req.Metadata) > 0 {
		env.SetWorkingValue("contextstream.request_metadata", req.Metadata, contextdata.MemoryClassTask)
	}
	if req.RequestedAt.IsZero() {
		env.SetWorkingValue("contextstream.requested_at", "", contextdata.MemoryClassTask)
	} else {
		env.SetWorkingValue("contextstream.requested_at", req.RequestedAt.UTC().Format(time.RFC3339Nano), contextdata.MemoryClassTask)
	}
	return nil
}
