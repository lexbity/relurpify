package agentgraph

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
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

func (n *StreamTriggerNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
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

	switch req.Mode {
	case contextstream.ModeBackground:
		job, err := trigger.RequestBackground(ctx, req)
		if err != nil {
			return nil, err
		}
		env.SetWorkingValue("contextstream.job_id", job.ID, contextdata.MemoryClassTask)
		env.SetWorkingValue("contextstream.job_mode", string(req.Mode), contextdata.MemoryClassTask)
		go func() {
			result, err := job.Wait(context.Background())
			if result != nil {
				_ = contextstream.ApplyResult(env, result)
			}
			if err != nil {
				env.SetWorkingValue("contextstream.background_error", err.Error(), contextdata.MemoryClassTask)
			}
		}()
		return &core.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]any{
				"contextstream_job_id": job.ID,
				"mode":                 string(req.Mode),
				"requested_query":      n.Query.Text,
			},
		}, nil
	default:
		result, err := trigger.RequestBlocking(ctx, req)
		if result != nil {
			_ = contextstream.ApplyResult(env, result)
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
			data["trimmed"] = result.Trim.Truncated
			data["streamed_ref_count"] = len(result.Compilation.StreamedRefs)
		}
		return &core.Result{
			NodeID:  n.id,
			Success: true,
			Data:    data,
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
	if env != nil && env.TaskID != "" {
		return env.TaskID + ".stream"
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
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "contextstream.*"},
			WriteKeys:                []string{"contextstream.*"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState},
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
