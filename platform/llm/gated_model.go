package llm

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/model"
)

// NewGatedModel wraps a language model with the process-wide physical-request
// gate. A nil gate returns the model unchanged for diagnostic and non-generation
// paths; generation paths receive the gate through composition (D-04).
func NewGatedModel(inner model.LanguageModel, gate *model.CallGate) model.LanguageModel {
	if gate == nil || inner == nil {
		return inner
	}
	return &gatedModel{LanguageModel: inner, gate: gate}
}

// gatedModel holds the lane lease for the complete client-owned transport
// request/response lifetime, including stream draining.
type gatedModel struct {
	model.LanguageModel
	gate *model.CallGate
}

func (m *gatedModel) acquire(ctx context.Context) (*model.Ticket, error) {
	ticket, err := m.gate.Acquire(ctx, model.GenerateCallID(), model.GenerateAttemptID())
	if err != nil {
		return nil, fmt.Errorf("model lane: %w", err)
	}
	return ticket, nil
}

// callError distinguishes an active-lane deadline from caller cancellation.
func callError(ctx context.Context, ticket *model.Ticket, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() == nil && ticket.Context.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%w: %w", model.ErrActiveTimeout, err)
	}
	return err
}

// Generate implements LanguageModel with gate enforcement.
func (m *gatedModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	ticket, err := m.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer ticket.Released()
	resp, err := m.LanguageModel.Generate(ticket.Context, prompt, options) //nolint:contextcheck // ticket.Context derives from ctx and carries the lane marker
	return resp, callError(ctx, ticket, err)
}

// GenerateStream implements LanguageModel with gate enforcement. The lease is
// released only after the producer closes the underlying stream.
func (m *gatedModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	ticket, err := m.acquire(ctx)
	if err != nil {
		return nil, err
	}

	underlying, err := m.LanguageModel.GenerateStream(ticket.Context, prompt, options) //nolint:contextcheck // ticket.Context derives from ctx and carries the lane marker
	if err != nil {
		ticket.Released()
		return nil, callError(ctx, ticket, err)
	}

	out := make(chan string, 100)
	go func() {
		defer close(out)
		for {
			select {
			case token, ok := <-underlying:
				if !ok {
					ticket.Released()
					return
				}
				select {
				case out <- token:
				case <-ticket.Context.Done():
					reclaimStream(underlying, ticket, m.gate.CancellationGracePeriod())
					return
				}
			case <-ticket.Context.Done():
				reclaimStream(underlying, ticket, m.gate.CancellationGracePeriod())
				return
			}
		}
	}()
	return out, nil
}

// Chat implements LanguageModel with gate enforcement.
func (m *gatedModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	ticket, err := m.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer ticket.Released()
	resp, err := m.LanguageModel.Chat(ticket.Context, messages, options) //nolint:contextcheck // ticket.Context derives from ctx and carries the lane marker
	return resp, callError(ctx, ticket, err)
}

// ChatWithTools implements LanguageModel with gate enforcement.
func (m *gatedModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	ticket, err := m.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer ticket.Released()
	resp, err := m.LanguageModel.ChatWithTools(ticket.Context, messages, tools, options) //nolint:contextcheck // ticket.Context derives from ctx and carries the lane marker
	return resp, callError(ctx, ticket, err)
}

// reclaimStream waits for producer closure before acknowledging physical
// reclamation. If closure is unaccounted for, poison keeps later transports out.
func reclaimStream(source <-chan string, ticket *model.Ticket, grace time.Duration) {
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-source:
			if !ok {
				ticket.Released()
				return
			}
		case <-timer.C:
			ticket.Poison("stream producer did not close after cancellation")
			return
		}
	}
}

// SetProfile preserves the inner model's profile contract through the lane.
func (m *gatedModel) SetProfile(profile *model.ModelProfile) {
	if setter, ok := m.LanguageModel.(interface{ SetProfile(*model.ModelProfile) }); ok {
		setter.SetProfile(profile)
	}
}

// ToolRepairStrategy reports the inner model's effective repair policy.
func (m *gatedModel) ToolRepairStrategy() string {
	if profiled, ok := m.LanguageModel.(model.ProfiledModel); ok {
		return profiled.ToolRepairStrategy()
	}
	return model.RepairStrategyHeuristicOnly
}

// MaxToolsPerCall reports the inner profile's tool limit.
func (m *gatedModel) MaxToolsPerCall() int {
	if profiled, ok := m.LanguageModel.(model.ProfiledModel); ok {
		return profiled.MaxToolsPerCall()
	}
	return 0
}

// UsesNativeToolCalling preserves native calling mode through the lane.
func (m *gatedModel) UsesNativeToolCalling() bool {
	if profiled, ok := m.LanguageModel.(model.ProfiledModel); ok {
		return profiled.UsesNativeToolCalling()
	}
	return false
}

var _ model.ProfiledModel = (*gatedModel)(nil)
