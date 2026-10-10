package testhelper

import (
	"context"
	"errors"
	"sync"

	"codeburg.org/lexbit/relurpify/model"
)

// ErrTurnsExhausted is returned by SequencedModel when every scripted turn has
// been consumed and the agent still calls the model. The harness surfaces it
// as a report error: silent model starvation would otherwise produce confusing
// agent-side failures (or hangs) instead of a loud test diagnosis.
var ErrTurnsExhausted = errors.New("testhelper: sequenced model turns exhausted")

// ModelTurn is one scripted model response. When ToolCalls is set the
// ChatWithTools response carries them; Text is always carried.
type ModelTurn struct {
	Text      string
	ToolCalls []model.ToolCall
}

// SequencedModel is a queue-based, message-capturing model.LanguageModel for
// dry runs (§5.8): turns are consumed in order and the last turn repeats once
// the queue is empty. Every Chat/ChatWithTools call records the messages it
// received, verbatim — the dry-run report's prompt-content assertions read
// that record. Thread-safe.
type SequencedModel struct {
	mu          sync.Mutex
	turns       []ModelTurn
	messages    [][]model.Message
	invocations int
}

// NewSequencedModel queues the given turns; the last one repeats.
func NewSequencedModel(turns ...ModelTurn) *SequencedModel {
	return &SequencedModel{turns: append([]ModelTurn(nil), turns...)}
}

// Messages returns a copy of every recorded message slice, in call order.
func (m *SequencedModel) Messages() [][]model.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]model.Message, len(m.messages))
	for i, msgs := range m.messages {
		out[i] = append([]model.Message(nil), msgs...)
	}
	return out
}

// InvocationCount returns how many Generate/Chat/ChatWithTools calls happened.
func (m *SequencedModel) InvocationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.invocations
}

func (m *SequencedModel) next(ctx context.Context, messages []model.Message) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.invocations++
	if len(m.turns) == 0 {
		m.mu.Unlock()
		return nil, ErrTurnsExhausted
	}
	turn := m.turns[0]
	if len(m.turns) > 1 {
		m.turns = m.turns[1:]
	}
	m.messages = append(m.messages, append([]model.Message(nil), messages...))
	m.mu.Unlock()
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return &model.LLMResponse{Text: turn.Text, ToolCalls: append([]model.ToolCall(nil), turn.ToolCalls...)}, nil
}

// Generate consumes the next turn and records the prompt as a user message —
// prompt-based (non-native tool-calling) paradigms pass their whole prompt
// this way, so the record stays the complete "what did the model see" surface.
func (m *SequencedModel) Generate(ctx context.Context, prompt string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.next(ctx, []model.Message{{Role: "user", Content: prompt}})
}

// GenerateStream consumes the next turn and streams its text in fixed chunks.
func (m *SequencedModel) GenerateStream(ctx context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	resp, err := m.next(ctx, nil)
	if err != nil {
		return nil, err
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		text := resp.Text
		const chunkSize = 32
		for len(text) > chunkSize {
			select {
			case ch <- text[:chunkSize]:
				text = text[chunkSize:]
			case <-ctx.Done():
				return
			}
		}
		if text != "" {
			select {
			case ch <- text:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

// Chat consumes the next turn and records the messages.
func (m *SequencedModel) Chat(ctx context.Context, messages []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.next(ctx, messages)
}

// ChatWithTools consumes the next turn and records the messages; the response
// carries the turn's tool calls.
func (m *SequencedModel) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.next(ctx, messages)
}

var _ model.LanguageModel = (*SequencedModel)(nil)
