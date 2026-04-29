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
// applies the compiler result back to the envelope.
type StreamTriggerNode struct {
	id                    string
	Trigger               *contextstream.Trigger
	Query                 string
	MaxTokens             int
	Mode                  contextstream.Mode
	BudgetShortfallPolicy string
	Metadata              map[string]any
}

// NewContextStreamNode creates a streaming trigger node.
func NewContextStreamNode(id string, trigger *contextstream.Trigger, query string, maxTokens int) *StreamTriggerNode {
	return &StreamTriggerNode{
		id:                    id,
		Trigger:               trigger,
		Query:                 query,
		MaxTokens:             maxTokens,
		Mode:                  contextstream.ModeBlocking,
		BudgetShortfallPolicy: "emit_partial",
	}
}

func (n *StreamTriggerNode) ID() string { return n.id }

func (n *StreamTriggerNode) Type() NodeType { return NodeTypeStream }

func (n *StreamTriggerNode) Contract() NodeContract {
	return streamTriggerNodeContract()
}

func (n *StreamTriggerNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	if n == nil {
		return nil, fmt.Errorf("stream trigger node is nil")
	}
	if n.Trigger == nil {
		return nil, fmt.Errorf("stream trigger node %q missing context stream trigger", n.id)
	}
	if env == nil {
		return nil, fmt.Errorf("stream trigger node %q missing envelope", n.id)
	}

	req := contextstream.Request{
		ID:                    n.requestID(env),
		Query:                 retrieval.RetrievalQuery{Text: n.Query},
		MaxTokens:             n.MaxTokens,
		EventLogSeq:           env.AssemblyMetadata.EventLogSeq,
		BudgetShortfallPolicy: n.BudgetShortfallPolicy,
		Mode:                  n.mode(),
		RequestedAt:           time.Now().UTC(),
		Metadata:              cloneAnyMap(n.Metadata),
	}
	if err := contextstream.ApplyRequestMetadata(env, req); err != nil {
		return nil, err
	}

	switch req.Mode {
	case contextstream.ModeBackground:
		job, err := n.Trigger.RequestBackground(ctx, req)
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
		return &Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]any{
				"contextstream_job_id": job.ID,
				"mode":                 string(req.Mode),
				"requested_query":      n.Query,
			},
		}, nil
	default:
		result, err := n.Trigger.RequestBlocking(ctx, req)
		if result != nil {
			_ = contextstream.ApplyResult(env, result)
		}
		if err != nil {
			return nil, err
		}
		data := map[string]any{
			"mode":             string(req.Mode),
			"requested_query":  n.Query,
			"shortfall_tokens": 0,
		}
		if result != nil {
			data["shortfall_tokens"] = result.Trim.ShortfallTokens
			data["trimmed"] = result.Trim.Truncated
			data["streamed_ref_count"] = len(result.Compilation.StreamedRefs)
		}
		return &Result{
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

func streamTriggerNodeContract() NodeContract {
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
