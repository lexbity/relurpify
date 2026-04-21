package react

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestReactHelperParsingAndFormatting(t *testing.T) {
	if got := ExtractJSON(`prefix {"answer": 42} suffix`); got != `{"answer": 42}` {
		t.Fatalf("unexpected ExtractJSON result: %q", got)
	}
	if got := ExtractJSONSnippet(`prefix {"answer": 42} suffix`); got != `{"answer": 42}` {
		t.Fatalf("unexpected ExtractJSONSnippet result: %q", got)
	}
	if got := ExtractJSON("no-json-here"); got != "{}" {
		t.Fatalf("expected fallback JSON object, got %q", got)
	}
	if got := ExtractJSONSnippet("no-json-here"); got != "" {
		t.Fatalf("expected empty snippet fallback, got %q", got)
	}

	if got := stringify(nil); got != "" {
		t.Fatalf("expected empty stringify for nil, got %q", got)
	}
	if got := stringify(17); got != "17" {
		t.Fatalf("expected numeric stringify, got %q", got)
	}
	if got := firstNonEmpty("  ", "\t", " keep "); got != "keep" {
		t.Fatalf("unexpected firstNonEmpty result: %q", got)
	}
	if got := summarizeText("abcdef", 3); got != "abc..." {
		t.Fatalf("unexpected summarizeText result: %q", got)
	}
	if got := summarizeText(" ok ", 0); got != "ok" {
		t.Fatalf("unexpected summarizeText passthrough: %q", got)
	}

	if got := defaultIterationsForMode("debug"); got != 20 {
		t.Fatalf("unexpected debug iteration budget: %d", got)
	}
	if got := defaultIterationsForMode("review"); got != 12 {
		t.Fatalf("unexpected review iteration budget: %d", got)
	}
	if got := defaultIterationsForMode("custom"); got != 8 {
		t.Fatalf("unexpected default iteration budget: %d", got)
	}

	task := &core.Task{Context: map[string]any{"mode": " verify "}}
	if got := taskMode(task); got != "verify" {
		t.Fatalf("unexpected task mode: %q", got)
	}
	if got := taskMode(nil); got != "" {
		t.Fatalf("expected empty task mode for nil task, got %q", got)
	}

	if got := firstSearchResultPath([]any{
		"ignore",
		map[string]any{"file": "  /tmp/a.go  "},
		map[string]any{"file": "/tmp/b.go"},
	}); got != "/tmp/a.go" {
		t.Fatalf("unexpected firstSearchResultPath result: %q", got)
	}
	if got := firstSearchResultPath([]map[string]any{{"file": " /tmp/c.go "}}); got != "/tmp/c.go" {
		t.Fatalf("unexpected typed firstSearchResultPath result: %q", got)
	}
	if got := firstSearchResultPath(nil); got != "" {
		t.Fatalf("expected empty firstSearchResultPath fallback, got %q", got)
	}
}

func TestReactHelperWarningsAndConversions(t *testing.T) {
	notice := buildAnchorNotices([]any{
		map[string]any{"term": "alpha", "definition": "first", "status": "fresh"},
		map[string]any{"term": "beta", "definition": "second", "status": "drifted"},
		map[string]any{"term": "gamma", "definition": "third", "status": "superseded"},
	})
	if notice == "" || !strings.Contains(notice, "beta") || !strings.Contains(notice, "gamma") {
		t.Fatalf("unexpected anchor notices: %q", notice)
	}
	if got := buildAnchorNotices(nil); got != "" {
		t.Fatalf("expected empty anchor notices for nil input, got %q", got)
	}

	warning := buildDerivationDepthWarning(map[string]any{
		"depth":         5,
		"total_loss":    0.75,
		"origin_system": "graph",
	})
	if warning == "" || !strings.Contains(warning, "5 transformations") || !strings.Contains(warning, "graph") {
		t.Fatalf("unexpected derivation warning: %q", warning)
	}
	if got := buildDerivationDepthWarning(map[string]any{"depth": 1, "total_loss": 0.1}); got != "" {
		t.Fatalf("expected empty derivation warning for low risk input, got %q", got)
	}

	if got := toInt("12"); got != 12 {
		t.Fatalf("unexpected toInt string conversion: %d", got)
	}
	if got := toInt(3.8); got != 3 {
		t.Fatalf("unexpected toInt float conversion: %d", got)
	}
	if got := toInt(nil); got != 0 {
		t.Fatalf("unexpected toInt nil conversion: %d", got)
	}

	if got := toFloat("1.5"); got != 1.5 {
		t.Fatalf("unexpected toFloat string conversion: %f", got)
	}
	if got := toFloat(4); got != 4 {
		t.Fatalf("unexpected toFloat int conversion: %f", got)
	}
	if got := toFloat(nil); got != 0 {
		t.Fatalf("unexpected toFloat nil conversion: %f", got)
	}
}

func TestReactThinkNodePromptAndContextExtraction(t *testing.T) {
	agent := &ReActAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				Prompt: "Prefer small, well-scoped steps.",
			},
		},
	}
	node := &reactThinkNode{
		id:    "think",
		agent: agent,
		task: &core.Task{
			Context: map[string]any{
				"stream_callback": func(string) {},
				"context_file_contents": []core.ContextFileContent{
					{Path: "README.md", Summary: "project overview"},
				},
			},
		},
	}

	if cb := node.streamCallback(); cb == nil {
		t.Fatal("expected stream callback to be extracted")
	}
	node.streamCallback()("noop")
	if got := renderContextFiles(node.task, 256); got == "" || !strings.Contains(got, "README.md") {
		t.Fatalf("unexpected rendered context files: %q", got)
	}
	extracted := extractContextFiles(node.task)
	if len(extracted) != 1 || extracted[0].Path != "README.md" {
		t.Fatalf("unexpected extracted context files: %#v", extracted)
	}

	prompt := node.buildSystemPrompt([]core.Tool{
		stubTool{name: "lsp_hover", params: []core.ToolParameter{{Name: "value"}}},
		stubTool{name: "ast_query", params: []core.ToolParameter{{Name: "value"}}},
	})
	if !strings.Contains(prompt, "Code Analysis Capabilities") || !strings.Contains(prompt, "Skill Guidance") || !strings.Contains(prompt, "Prefer small, well-scoped steps.") {
		t.Fatalf("unexpected system prompt: %q", prompt)
	}
}
