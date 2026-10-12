package react

import (
	"encoding/json"
	"testing"
)

// TestExtractJSONSalvagesTruncatedDecision pins the live scratch-turn failure:
// gemma4:e4b emitted the decision object's opening constructs and then EOS
// before the closing brace. The salvage must close the open constructs so the
// decision parses instead of failing the step as unparseable.
func TestExtractJSONSalvagesTruncatedDecision(t *testing.T) {
	truncated := "{\n  \"thought\": \"I need to find the file hello.py first.\",\n  \"action\": \"file_list\",\n  \"tool\": \"file_list\",\n  \"arguments\": {\"directory\": \".\"}\n"
	snippet := ExtractJSON(truncated)
	var generic map[string]any
	if err := json.Unmarshal([]byte(snippet), &generic); err != nil {
		t.Fatalf("truncated decision did not balance into valid JSON: %v\nsnippet: %q", err, snippet)
	}
	if generic["tool"] != "file_list" {
		t.Fatalf("tool field lost in salvage: %v", generic["tool"])
	}
	args, ok := generic["arguments"].(map[string]any)
	if !ok || args["directory"] != "." {
		t.Fatalf("arguments lost in salvage: %v", generic["arguments"])
	}
}

func TestExtractJSONSalvageCases(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"cut mid-string", `{"thought": "fixing he`},
		{"dangling comma at cut", `{"thought": "t", "arguments": {"directory": ".",`},
		{"unclosed nested object", `{"thought": "t", "arguments": {"directory": "."`},
		{"unclosed nested array", `{"thought": "t", "candidates": ["a", "b"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snippet := ExtractJSON(tc.raw)
			var generic map[string]any
			if err := json.Unmarshal([]byte(snippet), &generic); err != nil {
				t.Fatalf("did not balance into valid JSON: %v\nsnippet: %q", err, snippet)
			}
		})
	}
}

func TestExtractJSONWellFormedUnchanged(t *testing.T) {
	object := `{"thought": "t, } trailing comma text", "complete": true}`
	raw := "prefix ```json\n" + object + "\n``` suffix"
	if got := ExtractJSON(raw); got != object {
		t.Fatalf("well-formed JSON was rewritten:\n got %q\nwant %q", got, object)
	}
	var generic map[string]any
	if err := json.Unmarshal([]byte(ExtractJSON(raw)), &generic); err != nil {
		t.Fatalf("well-formed JSON broken by salvage: %v", err)
	}
	if generic["complete"] != true {
		t.Fatalf("complete flag lost: %v", generic)
	}
}

func TestExtractJSONNoBracesSentinel(t *testing.T) {
	if got := ExtractJSON("no json here at all"); got != "{}" {
		t.Fatalf("expected {} sentinel, got %q", got)
	}
}
