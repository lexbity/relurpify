package critic_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	critic "codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/model"
)

func TestFakeModelIsALanguageModel(t *testing.T) {
	var _ model.LanguageModel = critic.NewFakeModel()
}

func TestGenerateStreamEmitsScriptedChunksInOrder(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"draft ", "one"}})

	ctx := context.Background()
	ch, err := fm.GenerateStream(ctx, "write a draft", nil)
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	var got []string
	for chunk := range ch {
		got = append(got, chunk)
	}
	if strings.Join(got, "") != "draft one" {
		t.Fatalf("streamed %q, want %q", strings.Join(got, ""), "draft one")
	}
	if fm.LastInput() != "write a draft" {
		t.Fatalf("LastInput() = %q", fm.LastInput())
	}
}

func TestGenerateReturnsJoinedText(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a", "b"}, FinishReason: "length"})

	resp, err := fm.Generate(context.Background(), "prompt", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "ab" {
		t.Fatalf("Text = %q, want %q", resp.Text, "ab")
	}
	if resp.FinishReason != "length" {
		t.Fatalf("FinishReason = %q, want %q", resp.FinishReason, "length")
	}
}

func TestChatWithToolsReturnsScriptedToolCalls(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{
		ToolCalls: []model.ToolCall{{ID: "t1", Name: "file_read", Args: map[string]any{"path": "stringkit.py"}}},
	})

	resp, err := fm.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "read it"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "file_read" {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
	if fm.LastInput() != "read it" {
		t.Fatalf("LastInput() = %q", fm.LastInput())
	}
}

func TestTurnsCycleInOrder(t *testing.T) {
	fm := critic.NewFakeModel(
		critic.FakeTurn{Chunks: []string{"one"}},
		critic.FakeTurn{Chunks: []string{"two"}},
	)

	ctx := context.Background()
	for i, want := range []string{"one", "two", "one"} {
		resp, err := fm.Generate(ctx, "p", nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if resp.Text != want {
			t.Fatalf("call %d: Text = %q, want %q", i, resp.Text, want)
		}
	}
	if fm.Calls() != 3 {
		t.Fatalf("Calls() = %d, want 3", fm.Calls())
	}
}

func TestGenerateStreamClosesOnCancellation(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a", "b", "c", "d", "e"}})
	fm.SetPace(25 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := fm.GenerateStream(ctx, "p", nil)
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}

	<-ch // first chunk arrived; abandon the stream
	cancel()

	// R2: the channel closes within 250 ms of cancellation.
	closed := make(chan struct{})
	go func() {
		for range ch {
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("channel did not close within 250 ms of cancellation")
	}
}

func TestGenerateStreamDropsAndCountsOnSendContention(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a", "b", "c"}})
	fm.SetPace(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := fm.GenerateStream(ctx, "p", nil)
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}

	// Nobody reads: the producer blocks on its first send until the
	// context ends, then drops and counts the token (R6).
	time.Sleep(30 * time.Millisecond)
	cancel()
	for range ch {
	}

	if got := fm.DroppedTokens(); got != 1 {
		t.Fatalf("DroppedTokens() = %d, want 1", got)
	}
}

func TestErrorInjectionFailsEveryMethod(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"x"}})
	boom := errors.New("boom")
	fm.SetError(boom)

	ctx := context.Background()
	if _, err := fm.Generate(ctx, "p", nil); !errors.Is(err, boom) {
		t.Fatalf("Generate err = %v, want boom", err)
	}
	if _, err := fm.GenerateStream(ctx, "p", nil); !errors.Is(err, boom) {
		t.Fatalf("GenerateStream err = %v, want boom", err)
	}
	if _, err := fm.Chat(ctx, nil, nil); !errors.Is(err, boom) {
		t.Fatalf("Chat err = %v, want boom", err)
	}
	if _, err := fm.ChatWithTools(ctx, nil, nil, nil); !errors.Is(err, boom) {
		t.Fatalf("ChatWithTools err = %v, want boom", err)
	}
}

func TestEmptyScriptFailsCleanly(t *testing.T) {
	fm := critic.NewFakeModel()
	if _, err := fm.Generate(context.Background(), "p", nil); err == nil {
		t.Fatal("Generate on an empty script should fail")
	}
}
