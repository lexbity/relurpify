package llm

import (
	"context"
	"fmt"

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
		defer func() {
			ticket.Released()
			close(out)
		}()
		for token := range underlying {
			select {
			case out <- token:
			case <-ticket.Context.Done():
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
