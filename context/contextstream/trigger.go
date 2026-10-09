package contextstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/telemetry"
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
	Compiler  CompilerInvoker
	telemetry telemetry.Telemetry
}

// NewTrigger creates a trigger for the given compiler.
func NewTrigger(compiler CompilerInvoker) *Trigger {
	return &Trigger{Compiler: compiler}
}

// SetTelemetry attaches the framework telemetry sink. The trigger emits
// compiler.* events when a sink is present and is otherwise silent.
func (t *Trigger) SetTelemetry(tel telemetry.Telemetry) *Trigger {
	if t == nil {
		return nil
	}
	t.telemetry = tel
	return t
}

// RequestBlocking submits the request and waits for the compiler response.
func (t *Trigger) RequestBlocking(ctx context.Context, req Request) (*Result, error) {
	if t == nil || t.Compiler == nil {
		return nil, errors.New("contextstream: missing compiler")
	}
	started := time.Now().UTC()
	t.emitCompilerStarted(ctx, req)
	compilation, err := t.Compiler.Compile(ctx, toCompilationRequest(req))
	res := &Result{
		Request:     req,
		Compilation: compilation,
		StartedAt:   started,
		CompletedAt: time.Now().UTC(),
		Err:         err,
	}
	if compilation != nil {
		res.Record = &compilation.Record
		res.Trim = trimMetadataFromCompilation(req, compilation)
	}
	t.emitCompilerOutcome(ctx, req, res, started)
	if err != nil {
		return res, fmt.Errorf("contextstream: compile request %q: %w", req.ID, err)
	}
	return res, nil
}

// RequestBackground starts a background streaming job that runs on the
// caller-supplied run-lifetime context. The context MUST outlive the node that
// registers the job; the epoch coordinator owns it and cancels it only at run
// completion or quiesce.
func (t *Trigger) RequestBackground(runCtx context.Context, req Request) (*Job, error) {
	if t == nil || t.Compiler == nil {
		return nil, errors.New("contextstream: missing compiler")
	}
	job := NewJob(req)
	job.StartedAt = time.Now().UTC()
	go func() {
		res, err := t.RequestBlocking(runCtx, req)
		job.complete(res, err)
	}()
	return job, nil
}

func toCompilationRequest(req Request) contextports.CompilationRequest {
	return contextports.CompilationRequest{
		BaseContext:  req.Query.Text,
		BudgetTokens: req.MaxTokens,
		Mode:         string(req.Mode),
		EventLogSeq:  req.EventLogSeq,
		Metadata:     req.Metadata,
	}
}

func trimMetadataFromCompilation(req Request, compilation *contextports.CompilationResult) TrimMetadata {
	if compilation == nil {
		return TrimMetadata{}
	}
	return TrimMetadata{
		BudgetTokens:    req.MaxTokens,
		ShortfallTokens: compilation.ShortfallTokens,
		Substitutions:   compilation.Substitutions,
	}
}
