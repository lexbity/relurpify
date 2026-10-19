package react

import (
	"encoding/json"
	"strings"
	"testing"
)

// A model writing Python regex inside a JSON string emits `\w`/`\s`, which JSON
// forbids. ExtractJSON must repair the escapes so the decision parses instead of
// failing the step (and letting the repair call replace the intended action).
func TestExtractJSONRepairsInvalidEscapes(t *testing.T) {
	raw := "```json\n" + `{"thought": "fix regex", "action": "file_write", "tool": "file_write", "arguments": {"path": "stringkit.py", "content": "import re\n\ndef slugify(text):\n    return re.sub(r'[^\w\s-]', '', text)\n"}}` + "\n```"

	snippet := ExtractJSON(raw)
	var generic map[string]any
	if err := json.Unmarshal([]byte(snippet), &generic); err != nil {
		t.Fatalf("regex escapes were not repaired into valid JSON: %v\nsnippet: %q", err, snippet)
	}
	args, ok := generic["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("arguments lost in repair: %v", generic["arguments"])
	}
	content, _ := args["content"].(string)
	if !strings.Contains(content, `\w`) || !strings.Contains(content, `\s`) {
		t.Fatalf("literal regex escapes lost in repair: %q", content)
	}
	if !strings.Contains(content, "import re\n\ndef slugify") {
		t.Fatalf("valid \\n escapes must survive as real newlines: %q", content)
	}
}

// parseDecision must accept a file_write decision carrying regex escapes: the
// regression is that it errored ("invalid escape sequence \w in string") and the
// react loop's repair step then replaced the write with an unrelated action.
func TestParseDecisionAcceptsRegexEscapes(t *testing.T) {
	raw := "```json\n" + `{"thought": "fix", "action": "file_write", "tool": "file_write", "arguments": {"path": "s.py", "content": "re.sub(r'[^\w\s-]', '', t)"}, "complete": false}` + "\n```"
	parsed, err := parseDecision(raw)
	if err != nil {
		t.Fatalf("parseDecision failed on regex escapes: %v", err)
	}
	if parsed.Tool != "file_write" {
		t.Fatalf("tool = %q, want file_write", parsed.Tool)
	}
	if got, _ := parsed.Arguments["content"].(string); !strings.Contains(got, `\w`) {
		t.Fatalf("content lost its literal escapes: %q", got)
	}
}

// The escape repair must not touch a snippet that is already valid JSON.
func TestExtractJSONValidEscapesUnchanged(t *testing.T) {
	object := `{"thought": "a\nb", "tool": "file_read", "arguments": {"path": "x.py"}}`
	raw := "```json\n" + object + "\n```"
	if got := ExtractJSON(raw); got != object {
		t.Fatalf("valid JSON was rewritten:\n got %q\nwant %q", got, object)
	}
}
