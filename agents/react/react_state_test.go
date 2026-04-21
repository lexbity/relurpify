package react

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestReactDecisionAndMessageHelpers(t *testing.T) {
	if got := normalizeArguments(map[string]interface{}{"a": 1}); got["a"] != 1 {
		t.Fatalf("unexpected normalized arguments map: %#v", got)
	}
	if got := normalizeArguments(`{"a":1}`); got["a"] != float64(1) {
		t.Fatalf("unexpected normalized JSON arguments: %#v", got)
	}
	if got := normalizeArguments("raw"); got["value"] != "raw" {
		t.Fatalf("unexpected normalized raw arguments: %#v", got)
	}
	if got := normalizeArguments(123); len(got) != 0 {
		t.Fatalf("unexpected default normalized arguments: %#v", got)
	}

	if err := parseError(""); err != nil {
		t.Fatalf("expected nil parseError, got %v", err)
	}
	if err := parseError("boom"); err == nil {
		t.Fatal("expected parseError for non-empty string")
	}

	state := core.NewContext()
	saveReactMessages(state, []core.Message{{Role: "user", Content: "hello"}})
	messages := getReactMessages(state)
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("unexpected saved messages: %#v", messages)
	}
	messages[0].Content = "changed"
	if fresh := getReactMessages(state); fresh[0].Content != "hello" {
		t.Fatalf("expected defensive copy from getReactMessages, got %#v", fresh)
	}

	appendAssistantMessage(state, &core.LLMResponse{Text: "response"})
	messages = getReactMessages(state)
	if len(messages) != 2 || messages[1].Role != "assistant" || messages[1].Content != "response" {
		t.Fatalf("unexpected assistant message append: %#v", messages)
	}

	state.Set("react.tool_observations", []any{
		map[string]any{"tool": "file_read", "phase": "explore", "summary": "read ok", "success": true},
	})
	observations := getToolObservations(state)
	if len(observations) != 1 || observations[0].Tool != "file_read" || observations[0].Summary != "read ok" {
		t.Fatalf("unexpected tool observations: %#v", observations)
	}

	if got := firstMeaningfulLine("\n  first line  \nsecond"); got != "first line" {
		t.Fatalf("unexpected firstMeaningfulLine result: %q", got)
	}
	if got := finalOutputSummary(map[string]interface{}{"summary": "done"}); got != "done" {
		t.Fatalf("unexpected finalOutputSummary result: %q", got)
	}
	if got := finalOutputSummary("plain"); got != "plain" {
		t.Fatalf("unexpected finalOutputSummary fallback: %q", got)
	}
	if got := reactTaskScope(state); got != "" {
		t.Fatalf("expected empty task scope without task id, got %q", got)
	}
	state.Set("task.id", "task-1")
	if got := reactTaskScope(state); got != "task-1" {
		t.Fatalf("unexpected task scope: %q", got)
	}
}

func TestReactThinkNodeRepairDecision(t *testing.T) {
	model := &stubLLM{
		responses: []*core.LLMResponse{{Text: `{"thought":"repair","action":"tool","tool":"file_read","arguments":{},"complete":false}`}},
	}
	agent := &ReActAgent{
		Model:  model,
		Config: &core.Config{Model: "test-model"},
	}
	node := &reactThinkNode{agent: agent}
	resp, err := node.repairDecision(context.Background(), []core.Tool{stubTool{name: "file_read"}}, "broken response", false)
	if err != nil {
		t.Fatalf("repairDecision: %v", err)
	}
	if !strings.Contains(resp, "file_read") {
		t.Fatalf("unexpected repaired response: %q", resp)
	}
	if model.generateCalls != 1 {
		t.Fatalf("expected repairDecision to call the model once, got %d", model.generateCalls)
	}
}
