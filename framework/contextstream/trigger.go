package contextstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
)

type triggerContextKey struct{}

// WithTrigger stores a Trigger in context so StreamTriggerNode and agents
// can retrieve it without holding it as a field.
func WithTrigger(ctx context.Context, t *Trigger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, triggerContextKey{}, t)
}

// TriggerFromContext retrieves the Trigger from context, or nil.
func TriggerFromContext(ctx context.Context) *Trigger {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(triggerContextKey{}).(*Trigger)
	return t
}

// Trigger invokes the compiler on behalf of agent execution.
type Trigger struct {
	Compiler CompilerInvoker
}

// NewTrigger creates a trigger for the given compiler.
func NewTrigger(compiler CompilerInvoker) *Trigger {
	return &Trigger{Compiler: compiler}
}

// RequestBlocking submits the request and waits for the compiler response.
func (t *Trigger) RequestBlocking(ctx context.Context, req Request) (*Result, error) {
	if t == nil || t.Compiler == nil {
		return nil, errors.New("contextstream: missing compiler")
	}
	started := time.Now().UTC()
	compilation, record, err := t.Compiler.Compile(ctx, toCompilationRequest(req))
	res := &Result{
		Request:     req,
		Compilation: compilation,
		Record:      record,
		StartedAt:   started,
		CompletedAt: time.Now().UTC(),
		Err:         err,
	}
	if compilation != nil {
		res.Trim = trimMetadataFromCompilation(req, compilation)
	}
	if err != nil {
		return res, fmt.Errorf("contextstream: compile request %q: %w", req.ID, err)
	}
	return res, nil
}

// RequestBackground starts a background streaming job.
func (t *Trigger) RequestBackground(ctx context.Context, req Request) (*Job, error) {
	if t == nil || t.Compiler == nil {
		return nil, errors.New("contextstream: missing compiler")
	}
	job := NewJob(req)
	job.StartedAt = time.Now().UTC()
	go func() {
		res, err := t.RequestBlocking(ctx, req)
		job.complete(res, err)
	}()
	return job, nil
}

func toCompilationRequest(req Request) compiler.CompilationRequest {
	return compiler.CompilationRequest{
		Query:                 req.Query,
		EventLogSeq:           req.EventLogSeq,
		MaxTokens:             req.MaxTokens,
		BudgetShortfallPolicy: req.BudgetShortfallPolicy,
		Metadata:              cloneRequestMetadata(req.Metadata),
	}
}

func cloneRequestMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func trimMetadataFromCompilation(req Request, compilation *compiler.CompilationResult) TrimMetadata {
	if compilation == nil {
		return TrimMetadata{}
	}
	return TrimMetadata{
		BudgetTokens:    req.MaxTokens,
		ShortfallTokens: compilation.ShortfallTokens,
		Substitutions:   append([]compiler.SummarySubstitution(nil), compilation.Substitutions...),
		Truncated:       compilation.ShortfallTokens > 0 || len(compilation.Substitutions) > 0,
	}
}
