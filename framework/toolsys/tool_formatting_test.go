package toolsys

import "testing"

func TestParseToolCallsFromTextAcceptsMultilineJSONStringLiterals(t *testing.T) {
	raw := `{
  "name": "file_write",
  "arguments": {
    "content": "pub fn add(a: i32, b: i32) -> String {
    format!(\"{}\", a + b)
}",
    "path": "testsuite/agenttest_fixtures/rustsuite/src/lib.rs"
  }
}`

	calls := ParseToolCallsFromText(raw)
	if len(calls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(calls))
	}
	if calls[0].Name != "file_write" {
		t.Fatalf("expected file_write, got %q", calls[0].Name)
	}
	if got := calls[0].Args["path"]; got != "testsuite/agenttest_fixtures/rustsuite/src/lib.rs" {
		t.Fatalf("unexpected path %v", got)
	}
	content, _ := calls[0].Args["content"].(string)
	if content == "" {
		t.Fatal("expected content argument")
	}
	if content == raw {
		t.Fatal("expected normalized content, got raw payload back")
	}
}
