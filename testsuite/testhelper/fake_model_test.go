package testhelper

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
)

// TestScriptedModelScriptedBehavior pins the deterministic response surface of
// the conformance fake model.
func TestScriptedModelScriptedBehavior(t *testing.T) {
	tool := model.ToolCall{Name: "probe", Args: map[string]any{}}
	m := NewScriptedModel("fixed text").WithToolCalls(tool)
	ctx := context.Background()

	resp, err := m.Generate(ctx, "prompt", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "fixed text" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "probe" {
		t.Fatalf("Generate response = %+v", resp)
	}

	resp, err = m.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Text != "fixed text" || len(resp.ToolCalls) != 1 {
		t.Fatalf("Chat response = %+v", resp)
	}

	resp, err = m.ChatWithTools(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "probe" {
		t.Fatalf("ChatWithTools response = %+v", resp)
	}
	if got := m.InvocationCount(); got != 3 {
		t.Fatalf("InvocationCount = %d, want 3", got)
	}

	ch, err := m.GenerateStream(ctx, "prompt", nil)
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	if _, open := <-ch; open {
		t.Fatal("GenerateStream must return a closed channel")
	}
	if got := m.InvocationCount(); got != 4 {
		t.Fatalf("InvocationCount after stream = %d, want 4", got)
	}
}

// TestScriptedModelWithoutToolCallsFallsBackToText pins the no-tool-call case.
func TestScriptedModelWithoutToolCallsFallsBackToText(t *testing.T) {
	m := NewScriptedModel("plain")
	resp, err := m.ChatWithTools(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if resp.Text != "plain" || len(resp.ToolCalls) != 0 {
		t.Fatalf("response = %+v, want plain text with no tool calls", resp)
	}
}
