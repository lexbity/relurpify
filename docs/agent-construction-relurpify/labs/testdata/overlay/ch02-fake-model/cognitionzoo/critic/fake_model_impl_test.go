// Package critic hosts the critic-loop paradigm built in the book
// "Agent Construction with Relurpify": draft, independent critique,
// conditional revise. Chapter 2 seeds the package with the fake model
// every later chapter drives — a table-driven implementation of the
// model.LanguageModel contract, deterministic enough to test a loop.
//
// The whole package is _test.go files until Chapter 6, when the
// critic-runner command becomes its first importer: the repository's
// consumer gate refuses production packages nobody imports, and the
// book's code obeys that gate from its first line.
package critic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/model"
)

// FakeTurn is one scripted model response. A FakeModel replays its turns
// in order and cycles back to the first when it runs out, so a bounded
// loop can be driven turn by turn without any randomness.
type FakeTurn struct {
	// Chunks are emitted in order by GenerateStream. When Text is empty,
	// the non-streaming methods join the chunks into the response text.
	Chunks []string
	// Text is returned by Generate and Chat. Empty means "join Chunks".
	Text     string
	Thinking string
	// FinishReason defaults to "stop" when left empty.
	FinishReason string
	// ToolCalls are returned by ChatWithTools (and carried on the
	// non-streaming response).
	ToolCalls []model.ToolCall
}

// FakeModel is a deterministic, table-driven implementation of the
// model.LanguageModel contract. It is the book's test engine: every
// chapter after this one drives the critic through FakeModel instead of
// a network, so a test that passes is a test about the agent, not about
// a model's mood.
//
// The streaming half implements the normative contract stated on
// model.LanguageModel: the call-time context is the only stop signal
// (R1), cancellation closes the channel (R2), the producer closes the
// channel exactly once (R3), producer-side failures degrade instead of
// panicking (R5), and send contention resolves on ctx.Done() with the
// token dropped and counted (R6). DroppedTokens reports that count.
type FakeModel struct {
	mu sync.Mutex

	turns     []FakeTurn
	next      int
	err       error
	pace      time.Duration
	calls     int
	lastInput string
	dropped   int
}

// Compile-time proof that FakeModel satisfies the contract it claims.
var _ model.LanguageModel = (*FakeModel)(nil)

// NewFakeModel returns a model that replays turns in order, cycling back
// to the first turn after the last.
func NewFakeModel(turns ...FakeTurn) *FakeModel {
	return &FakeModel{turns: append([]FakeTurn(nil), turns...)}
}

// SetError makes every subsequent call fail with err — the one-line way
// to test how an agent degrades.
func (f *FakeModel) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// SetPace inserts a pause of d before each streamed chunk, so tests can
// cancel a stream mid-flight deterministically.
func (f *FakeModel) SetPace(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pace = d
}

// Calls reports how many model invocations were made, across all four
// methods.
func (f *FakeModel) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// LastInput reports the prompt (or, for Chat and ChatWithTools, the last
// message content) of the most recent call.
func (f *FakeModel) LastInput() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastInput
}

// DroppedTokens reports how many streamed tokens were dropped because the
// consumer's context ended mid-send — the R6 counter.
func (f *FakeModel) DroppedTokens() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dropped
}

func (f *FakeModel) record(input string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastInput = input
	return f.err
}

func (f *FakeModel) nextTurn() (FakeTurn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.turns) == 0 {
		return FakeTurn{}, errors.New("fake model has no scripted turns")
	}
	turn := f.turns[f.next]
	f.next = (f.next + 1) % len(f.turns)
	return turn, nil
}

func (f *FakeModel) response(turn FakeTurn) *model.LLMResponse {
	text := turn.Text
	if text == "" {
		text = strings.Join(turn.Chunks, "")
	}
	reason := turn.FinishReason
	if reason == "" {
		reason = "stop"
	}
	return &model.LLMResponse{
		Text:         text,
		Thinking:     turn.Thinking,
		FinishReason: reason,
		Usage:        model.TokenUsage{Estimated: true},
		ToolCalls:    turn.ToolCalls,
	}
}

// Generate implements model.LanguageModel.
func (f *FakeModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	if err := f.record(prompt); err != nil {
		return nil, err
	}
	turn, err := f.nextTurn()
	if err != nil {
		return nil, err
	}
	return f.response(turn), nil
}

// GenerateStream implements the streaming half of model.LanguageModel.
func (f *FakeModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	if err := f.record(prompt); err != nil {
		return nil, err
	}
	turn, err := f.nextTurn()
	if err != nil {
		return nil, err
	}
	out := make(chan string)
	go f.pump(ctx, turn, out)
	return out, nil
}

// pump is the stream's producer: it owns the channel and is the only code
// that closes it (R3).
func (f *FakeModel) pump(ctx context.Context, turn FakeTurn, out chan string) {
	defer close(out)
	for _, chunk := range turn.Chunks {
		if f.paceOf() > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(f.paceOf()):
			}
		}
		// R6: send contention resolves on ctx.Done(); the token is
		// dropped and counted rather than blocking the producer.
		select {
		case out <- chunk:
		case <-ctx.Done():
			f.noteDrop()
			return
		}
	}
}

func (f *FakeModel) paceOf() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pace
}

func (f *FakeModel) noteDrop() {
	f.mu.Lock()
	f.dropped++
	f.mu.Unlock()
}

// Chat implements model.LanguageModel.
func (f *FakeModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	if err := f.record(lastMessageContent(messages)); err != nil {
		return nil, err
	}
	turn, err := f.nextTurn()
	if err != nil {
		return nil, err
	}
	return f.response(turn), nil
}

// ChatWithTools implements model.LanguageModel. Scripted ToolCalls ride
// on the response untouched — the fake never invents tool decisions.
func (f *FakeModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	if err := f.record(lastMessageContent(messages)); err != nil {
		return nil, err
	}
	turn, err := f.nextTurn()
	if err != nil {
		return nil, err
	}
	return f.response(turn), nil
}

func lastMessageContent(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}
