package agentgraph

import (
	"context"
	"fmt"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// StreamTriggerNode requests a compiler-triggered streamed context update and
// applies the compiler result back to the envelope. The Trigger is resolved
// from the execution context via contextstream.TriggerFromContext.
type StreamTriggerNode struct {
	id                    string
	Query                 retrieval.RetrievalQuery
	MaxTokens             int
	Mode                  contextstream.Mode
	BudgetShortfallPolicy string
	Metadata              map[string]any
}

// NewContextStreamNode creates a streaming trigger node.
func NewContextStreamNode(id string, query retrieval.RetrievalQuery, maxTokens int) *StreamTriggerNode {
	return &StreamTriggerNode{
		id:                    id,
		Query:                 query,
		MaxTokens:             maxTokens,
		Mode:                  contextstream.ModeBlocking,
		BudgetShortfallPolicy: "emit_partial",
	}
}

func (n *StreamTriggerNode) ID() string { return n.id }

func (n *StreamTriggerNode) Type() NodeType { return NodeTypeStream }

func (n *StreamTriggerNode) Contract() NodeContract {
	return streamTriggerNodeContract(n)
}

func (n *StreamTriggerNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("stream trigger node is nil")
	}
	if env == nil {
		return nil, fmt.Errorf("stream trigger node %q missing envelope", n.id)
	}
	trigger := contextstream.TriggerFromContext(ctx)
	if trigger == nil {
		return nil, fmt.Errorf("stream trigger node %q: no compiler trigger in context", n.id)
	}

	req := n.buildRequest(env)
	if err := contextstream.ApplyRequestMetadata(env, req); err != nil {
		return nil, err
	}
	coord := EpochCoordinatorFromContext(ctx)
	if coord != nil && req.Metadata == nil {
		req.Metadata = make(map[string]any)
	}
	if coord != nil {
		req.Metadata["epoch_id"] = coord.EpochID()
	}

	switch req.Mode {
	case contextstream.ModeBackground:
		if coord == nil {
			return nil, fmt.Errorf("stream trigger node %q: background stream requires an epoch coordinator", n.id)
		}
		job, err := trigger.RequestBackground(coord.RunContext(), req)
		if err != nil {
			return nil, err
		}
		coord.TrackStreamJob(job)
		env.SetWorkingValueWithClass("contextstream.job_id", job.ID, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass("contextstream.job_mode", string(req.Mode), contextdata.MemoryClassTask)
		return &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data: execution.NewToolResultPayload(map[string]any{
				"contextstream_job_id":               job.ID,
				"mode":                               string(req.Mode),
				"requested_query":                    n.Query.Text,
				"contextstream_background_requested": true,
			}),
		}, nil
	default:
		var epoch uint64
		if coord != nil {
			epoch = coord.EpochID()
		}
		result, err := trigger.RequestBlocking(ctx, req)
		if result != nil {
			if applyErr := contextstream.ApplyResult(ctx, env, result, epoch); applyErr != nil {
				return nil, applyErr
			}
			// Bounded summary only: the node contract caps working-state
			// entries at 4096 bytes, and the full result's bodies live on the
			// envelope slice where the renderer reads them.
			summary := map[string]any{
				"request_id": result.Request.ID,
				"epoch":      epoch,
			}
			if result.Compilation != nil {
				summary["chunks"] = len(result.Compilation.StreamedChunks)
				summary["tokens"] = result.Compilation.Record.FinalTokens
			}
			env.SetWorkingValueWithClass("contextstream.result", summary, contextdata.MemoryClassTask)
		}
		if err != nil {
			return nil, err
		}
		data := map[string]any{
			"mode":             string(req.Mode),
			"requested_query":  n.Query.Text,
			"shortfall_tokens": 0,
		}
		if result != nil {
			data["shortfall_tokens"] = result.Trim.ShortfallTokens
			data["trimmed"] = result.Trim.ShortfallTokens > 0 || len(result.Trim.Substitutions) > 0
			data["streamed_ref_count"] = len(result.Compilation.StreamedRefs)
		}
		return &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data:    execution.NewToolResultPayload(data),
		}, nil
	}
}

func (n *StreamTriggerNode) mode() contextstream.Mode {
	if n == nil || n.Mode == "" {
		return contextstream.ModeBlocking
	}
	return n.Mode
}

func (n *StreamTriggerNode) requestID(env *contextdata.Envelope) string {
	if n == nil {
		return ""
	}
	if n.id != "" {
		return n.id + ".stream"
	}
	if env != nil && env.TaskIDSnapshot() != "" {
		return env.TaskIDSnapshot() + ".stream"
	}
	return "stream.request"
}

func (n *StreamTriggerNode) buildRequest(env *contextdata.Envelope) contextstream.Request {
	return contextstream.Request{
		ID:                    n.requestID(env),
		Query:                 n.Query,
		MaxTokens:             n.MaxTokens,
		EventLogSeq:           env.AssemblyMetadataSnapshot().EventLogSeq,
		BudgetShortfallPolicy: n.BudgetShortfallPolicy,
		Mode:                  n.mode(),
		RequestedAt:           time.Now().UTC(),
		Metadata:              cloneAnyMap(n.Metadata),
	}
}

func streamTriggerNodeContract(n *StreamTriggerNode) NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectContext,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "contextstream.*"},
			WriteKeys:                []string{"contextstream.*"},
			AllowedMemoryClasses:     []relurpctx.MemoryClass{relurpctx.MemoryClassWorking},
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassTaskMetadata, relurpctx.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
		},
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
