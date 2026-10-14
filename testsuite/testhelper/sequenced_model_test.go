package testhelper

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
)

func drainStream(t *testing.T, ch <-chan string) string {
	t.Helper()
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk)
	}
	return sb.String()
}

func TestSequencedModelConsumesTurnsInOrder(t *testing.T) {
	m := NewSequencedModel(
		ModelTurn{Text: "first"},
		ModelTurn{Text: "second", ToolCalls: []model.ToolCall{{Name: "probe", Args: map[string]any{}}}},
	)
	ctx := context.Background()

	resp, err := m.Chat(ctx, []model.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "first", resp.Text)
	assert.Empty(t, resp.ToolCalls)

	resp, err = m.ChatWithTools(ctx, []model.Message{{Role: "user", Content: "run"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "probe", resp.ToolCalls[0].Name)
	assert.Equal(t, "second", resp.Text)

	// The last turn repeats once the queue is drained.
	for i := 0; i < 2; i++ {
		resp, err = m.Generate(ctx, "again", nil)
		require.NoError(t, err)
		assert.Equal(t, "second", resp.Text)
		require.Len(t, resp.ToolCalls, 1)
	}

	assert.Equal(t, 4, m.InvocationCount())
}

func TestSequencedModelRecordsEveryPromptVerbatim(t *testing.T) {
	m := NewSequencedModel(ModelTurn{Text: "ok"})
	ctx := context.Background()

	_, err := m.Generate(ctx, "prompt one", nil)
	require.NoError(t, err)
	_, err = m.Chat(ctx, []model.Message{
		{Role: "user", Content: "turn"},
		{Role: "assistant", Content: "reply"},
	}, nil)
	require.NoError(t, err)

	messages := m.Messages()
	require.Len(t, messages, 2)
	require.Len(t, messages[0], 1)
	assert.Equal(t, "user", messages[0][0].Role)
	assert.Equal(t, "prompt one", messages[0][0].Content)
	require.Len(t, messages[1], 2)
	assert.Equal(t, "assistant", messages[1][1].Role)

	// The recorded slices are copies: mutating them must not affect the model.
	messages[1][0].Content = "mutated"
	assert.Equal(t, "turn", m.Messages()[1][0].Content)
}

func TestSequencedModelExhaustionFailsLoudly(t *testing.T) {
	m := NewSequencedModel()
	_, err := m.Chat(context.Background(), nil, nil)
	require.ErrorIs(t, err, ErrTurnsExhausted)
	assert.Equal(t, 1, m.InvocationCount(), "exhausted calls must still count as invocations")
}

func TestSequencedModelCancelledContextFails(t *testing.T) {
	m := NewSequencedModel(ModelTurn{Text: "never delivered"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Chat(ctx, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSequencedModelGenerateStreamChunksText(t *testing.T) {
	text := strings.Repeat("a", 100) // > 3 chunks of 32 bytes
	m := NewSequencedModel(ModelTurn{Text: text})

	ch, err := m.GenerateStream(context.Background(), "prompt", nil)
	require.NoError(t, err)
	assert.Equal(t, text, drainStream(t, ch))

	// A final partial chunk below the chunk size is delivered as-is.
	m2 := NewSequencedModel(ModelTurn{Text: "short"})
	ch, err = m2.GenerateStream(context.Background(), "prompt", nil)
	require.NoError(t, err)
	assert.Equal(t, "short", drainStream(t, ch))
}

func TestSequencedModelGenerateStreamExhausted(t *testing.T) {
	m := NewSequencedModel()
	_, err := m.GenerateStream(context.Background(), "prompt", nil)
	require.ErrorIs(t, err, ErrTurnsExhausted)
}

func TestSequencedModelGenerateStreamCancelledMidStream(t *testing.T) {
	m := NewSequencedModel(ModelTurn{Text: strings.Repeat("a", 300)})
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := m.GenerateStream(ctx, "prompt", nil)
	require.NoError(t, err)
	cancel()
	// The stream goroutine must observe the cancellation and close instead of
	// blocking forever on an unread channel.
	select {
	case _, open := <-ch:
		if open {
			// Chunks delivered before cancellation are fine; keep draining.
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close after context cancellation")
	}
}

func TestSequencedModelZeroTurnsSliceIsCopied(t *testing.T) {
	turns := []ModelTurn{{Text: "only"}}
	m := NewSequencedModel(turns...)
	turns[0].Text = "mutated"

	resp, err := m.Chat(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "only", resp.Text, "constructor must copy the turn slice")
}
