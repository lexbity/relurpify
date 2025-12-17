package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework"
)

type stubModel struct {
	streamText string
}

func (s stubModel) Generate(context.Context, string, *framework.LLMOptions) (*framework.LLMResponse, error) {
	return &framework.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}
func (s stubModel) GenerateStream(context.Context, string, *framework.LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- s.streamText
	close(ch)
	return ch, nil
}
func (s stubModel) Chat(context.Context, []framework.Message, *framework.LLMOptions) (*framework.LLMResponse, error) {
	return &framework.LLMResponse{Text: "chat", FinishReason: "stop"}, nil
}
func (s stubModel) ChatWithTools(context.Context, []framework.Message, []framework.Tool, *framework.LLMOptions) (*framework.LLMResponse, error) {
	return &framework.LLMResponse{Text: "tools", FinishReason: "stop", ToolCalls: []framework.ToolCall{{Name: "file_read", Args: map[string]any{"path": "x"}}}}, nil
}

func TestTapeModelRecordThenReplay(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	tape := filepath.Join(dir, "tape.jsonl")

	rec, err := NewTapeModel(stubModel{streamText: "streamed"}, tape, "record")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()

	if _, err := rec.Generate(context.Background(), "p", &framework.LLMOptions{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Chat(context.Background(), []framework.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.ChatWithTools(context.Background(), []framework.Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	stream, err := rec.GenerateStream(context.Background(), "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if !fileExists(tape) {
		t.Fatalf("expected tape file at %s", tape)
	}

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := replay.Generate(context.Background(), "p", &framework.LLMOptions{Model: "m"}); err != nil || resp.Text != "ok" {
		t.Fatalf("replay generate: resp=%+v err=%v", resp, err)
	}
	if resp, err := replay.Chat(context.Background(), []framework.Message{{Role: "user", Content: "hi"}}, nil); err != nil || resp.Text != "chat" {
		t.Fatalf("replay chat: resp=%+v err=%v", resp, err)
	}
	if resp, err := replay.ChatWithTools(context.Background(), []framework.Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil || resp.Text != "tools" {
		t.Fatalf("replay chat_with_tools: resp=%+v err=%v", resp, err)
	}
	ch, err := replay.GenerateStream(context.Background(), "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for token := range ch {
		got += token
	}
	if got != "streamed" {
		t.Fatalf("replay stream got %q", got)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
