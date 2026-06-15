package toolwire

import (
	"reflect"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
)

func TestRenderToolsToPrompt(t *testing.T) {
	if got := RenderToolsToPrompt(nil); got != "No tools available." {
		t.Fatalf("empty render = %q, want %q", got, "No tools available.")
	}

	tools := []model.LLMToolSpec{
		{
			Name:        "file_read",
			Description: "Read a file",
			InputSchema: &model.Schema{
				Type: "object",
				Properties: map[string]*model.Schema{
					"path": {Type: "string", Description: "file path"},
				},
				Required: []string{"path"},
			},
		},
	}

	rendered := RenderToolsToPrompt(tools)
	if !strings.Contains(rendered, "```tool") {
		t.Fatalf("rendered prompt missing tool fence: %q", rendered)
	}
	if !strings.Contains(rendered, `"name":"file_read"`) {
		t.Fatalf("rendered prompt missing tool name: %q", rendered)
	}
	if !strings.Contains(rendered, `"input_schema"`) {
		t.Fatalf("rendered prompt missing schema: %q", rendered)
	}
	if !strings.Contains(rendered, "Read a file") {
		t.Fatalf("rendered prompt missing description: %q", rendered)
	}
}

func TestExtractTopLevelJSONObjects(t *testing.T) {
	text := `prefix {"tool":"one","arguments":{"text":"{not a brace}"}} suffix {"tool":"two","arguments":{}} trailing`
	got := extractTopLevelJSONObjects(text)
	if len(got) != 2 {
		t.Fatalf("object count = %d, want 2 (%#v)", len(got), got)
	}
	if !strings.Contains(got[0], `"tool":"one"`) || !strings.Contains(got[1], `"tool":"two"`) {
		t.Fatalf("unexpected extracted objects: %#v", got)
	}
}

func TestParseToolCallsFromText(t *testing.T) {
	tests := []struct {
		name string
		text string
		max  int
		want []model.ToolCall
	}{
		{
			name: "fenced tool call",
			text: "preface\n```tool\n{\"tool\":\"file_read\",\"arguments\":{\"path\":\"main.go\"}}\n```\npostface",
			max:  1,
			want: []model.ToolCall{{Name: "file_read", Args: map[string]any{"path": "main.go"}}},
		},
		{
			name: "alias fields",
			text: "{\"name\":\"search_grep\",\"args\":{\"pattern\":\"TODO\"}}",
			max:  1,
			want: []model.ToolCall{{Name: "search_grep", Args: map[string]any{"pattern": "TODO"}}},
		},
		{
			name: "multiple top level objects and cap",
			text: "{\"tool\":\"file_read\",\"arguments\":{\"path\":\"a.go\"}} prose {\"tool\":\"file_list\",\"parameters\":{\"path\":\".\"}}",
			max:  1,
			want: []model.ToolCall{{Name: "file_read", Args: map[string]any{"path": "a.go"}}},
		},
		{
			name: "multiline string literal",
			text: `{
  "tool": "file_write",
  "arguments": {
    "path": "note.txt",
    "content": "line1
line2"
  }
}`,
			max:  1,
			want: []model.ToolCall{{Name: "file_write", Args: map[string]any{"path": "note.txt", "content": "line1\nline2"}}},
		},
		{
			name: "prose only",
			text: "no tool call here",
			max:  4,
		},
		{
			name: "malformed json",
			text: "{\"tool\":\"file_read\",\"arguments\":",
			max:  4,
		},
		{
			name: "file path alias",
			text: "{\"tool_name\":\"file_read\",\"parameters\":{\"file_path\":\"main.go\"}}",
			max:  1,
			want: []model.ToolCall{{Name: "file_read", Args: map[string]any{"file_path": "main.go", "path": "main.go"}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseToolCallsFromText(tc.text, tc.max)
			if len(got) != len(tc.want) {
				t.Fatalf("call count = %d, want %d (%#v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i].Name != tc.want[i].Name {
					t.Fatalf("call %d name = %q, want %q", i, got[i].Name, tc.want[i].Name)
				}
				if !reflect.DeepEqual(got[i].Args, tc.want[i].Args) {
					t.Fatalf("call %d args = %#v, want %#v", i, got[i].Args, tc.want[i].Args)
				}
			}
		})
	}
}

func TestTryParseSingleToolCallRejectsMissingName(t *testing.T) {
	if _, ok := tryParseSingleToolCall(`{}`); ok {
		t.Fatal("empty object should not parse as a tool call")
	}
}

func TestNormalizeMultilineJSONStringLiterals(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unchanged",
			in:   `{"tool":"noop"}`,
			want: `{"tool":"noop"}`,
		},
		{
			name: "newline tab return",
			in:   "{\n  \"tool\": \"noop\",\n  \"arguments\": {\"text\": \"line1\nline2\tend\r\"}\n}",
			want: "{\n  \"tool\": \"noop\",\n  \"arguments\": {\"text\": \"line1\\nline2\\tend\\r\"}\n}",
		},
		{
			name: "escaped quotes preserved",
			in:   `{"tool":"noop","arguments":{"text":"say \"hi\""}}`,
			want: `{"tool":"noop","arguments":{"text":"say \"hi\""}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMultilineJSONStringLiterals(tc.in); got != tc.want {
				t.Fatalf("normalized = %q, want %q", got, tc.want)
			}
		})
	}
}

func FuzzTryParseSingleToolCall(f *testing.F) {
	seeds := []string{
		`{"tool":"file_read","arguments":{"path":"main.go"}}`,
		`{"name":"test_tool","args":{"input":"hello"}}`,
		`{"tool_name":"cli_echo","parameters":{"text":"msg"}}`,
		`{"tool":"file_write","arguments":{"path":"/tmp/test","content":"data"}}`,
		`{"tool":"complete","arguments":{}}`,
		`{}`,
		`invalid json`,
		`{"tool":"","arguments":null}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		tryParseSingleToolCall(input)
	})
}
