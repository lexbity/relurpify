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

const speculativeWaitTimeout = 200 * time.Millisecond

// StreamTriggerNode requests a compiler-triggered streamed context update and
// applies the compiler result back to the envelope.
type StreamTriggerNode struct {
	id                    string
	Trigger               *contextstream.Trigger
	Query                 retrieval.RetrievalQuery
	MaxTokens             int
	Mode                  contextstream.Mode
	BudgetShortfallPolicy string
	Metadata              map[string]any
}

// NewContextStreamNode creates a streaming trigger node.
func NewContextStreamNode(id string, trigger *contextstream.Trigger, query retrieval.RetrievalQuery, maxTokens int) *StreamTriggerNode {
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
	return streamTriggerNodeContract(n)
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

	req := n.request(env, false)
	if err := contextstream.ApplyRequestMetadata(env, req); err != nil {
		return nil, err
	}

	switch req.Mode {
	case contextstream.ModeBackground:
		job, ok := n.speculativeJob(env)
		if !ok || job == nil {
			var err error
			job, err = n.requestBackground(ctx, req)
			if err != nil {
				return nil, err
			}
		}
		n.storeJob(env, job)
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
				"requested_query":      n.Query.Text,
			},
		}, nil
	default:
		result, err := n.requestBlocking(ctx, env, req)
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

func (n *StreamTriggerNode) request(env *contextdata.Envelope, speculative bool) contextstream.Request {
	req := contextstream.Request{
		ID:                    n.requestID(env),
		Query:                 n.Query,
		MaxTokens:             n.MaxTokens,
		EventLogSeq:           env.AssemblyMetadataSnapshot().EventLogSeq,
		BudgetShortfallPolicy: n.BudgetShortfallPolicy,
		Mode:                  n.mode(),
		RequestedAt:           time.Now().UTC(),
		Metadata:              cloneAnyMap(n.Metadata),
	}
	if speculative {
		if req.Metadata == nil {
			req.Metadata = make(map[string]any, 1)
		}
		req.Metadata["speculative"] = true
	}
	return req
}

func (n *StreamTriggerNode) requestBlocking(ctx context.Context, env *contextdata.Envelope, req contextstream.Request) (*contextstream.Result, error) {
	if job, ok := n.speculativeJob(env); ok {
		waitCtx, cancel := context.WithTimeout(ctx, speculativeWaitTimeout)
		defer cancel()
		result, err := job.Wait(waitCtx)
		if err == nil && result != nil {
			n.clearSpeculativeJob(env)
			return result, nil
		}
	}
	return n.Trigger.RequestBlocking(ctx, req)
}

func (n *StreamTriggerNode) requestBackground(ctx context.Context, req contextstream.Request) (*contextstream.Job, error) {
	return n.Trigger.RequestBackground(ctx, req)
}

func (n *StreamTriggerNode) speculativeJob(env *contextdata.Envelope) (*contextstream.Job, bool) {
	if env == nil {
		return nil, false
	}
	if job, ok := env.GetWorkingValue(n.speculativeJobKey()); ok {
		if typed, ok := job.(*contextstream.Job); ok && typed != nil {
			return typed, true
		}
	}
	return nil, false
}

func (n *StreamTriggerNode) storeJob(env *contextdata.Envelope, job *contextstream.Job) {
	if env == nil || job == nil {
		return
	}
	env.SetWorkingValue(n.speculativeJobKey(), job, contextdata.MemoryClassTask)
	env.SetWorkingValue(n.speculativeIDKey(), job.ID, contextdata.MemoryClassTask)
	env.SetWorkingValue("contextstream.job_id", job.ID, contextdata.MemoryClassTask)
}

func (n *StreamTriggerNode) clearSpeculativeJob(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	env.DeleteWorkingValue(n.speculativeJobKey())
	env.DeleteWorkingValue(n.speculativeIDKey())
}

func (n *StreamTriggerNode) speculativeJobKey() string {
	return "contextstream.speculative_job." + n.id
}

func (n *StreamTriggerNode) speculativeIDKey() string {
	return "contextstream.speculative." + n.id
}

func streamTriggerNodeContract(n *StreamTriggerNode) NodeContract {
	query := retrieval.RetrievalQuery{}
	if n != nil {
		query = n.Query
	}
	return NodeContract{
		SideEffectClass:             SideEffectContext,
		Idempotency:                 IdempotencyReplaySafe,
		SpeculativeCompilationQuery: &query,
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
