package prompt

import (
	"strings"
	"testing"
)

func TestParseBytes_V2Valid(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.chainer.default
tag "system"
tag ["agent", "debug"]
var tone = "direct"
var audience = "senior engineer"
---

# Prompt Title

Use {tone} language for a {audience}.
`
	result, err := ParseBytes([]byte(src), "sample.prompt")
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	cfg := result.Config
	if cfg.Schema != "framework.prompt/v2" {
		t.Fatalf("Schema = %q, want framework.prompt/v2", cfg.Schema)
	}
	if cfg.ID != "agent.chainer.default" {
		t.Fatalf("ID = %q, want agent.chainer.default", cfg.ID)
	}
	if got, want := cfg.Tags, []string{"system", "agent", "debug"}; !sameStrings(got, want) {
		t.Fatalf("Tags = %#v, want %#v", got, want)
	}
	if cfg.Variables["tone"].Default != "direct" {
		t.Fatalf("tone default = %q, want direct", cfg.Variables["tone"].Default)
	}
	if cfg.Variables["audience"].Default != "senior engineer" {
		t.Fatalf("audience default = %q, want senior engineer", cfg.Variables["audience"].Default)
	}
	if !strings.Contains(cfg.Body, "Use {tone} language") {
		t.Fatalf("Body = %q, expected markdown body text", cfg.Body)
	}
}

func TestParseBytes_RequiresSchema(t *testing.T) {
	src := `---
id agent.chainer.default
tag "system"
---

body
`
	_, err := ParseBytes([]byte(src), "missing-schema.prompt")
	if err == nil {
		t.Fatal("expected error for missing schema")
	}
	if !strings.Contains(err.Error(), "missing required schema statement") {
		t.Fatalf("error = %v, want missing schema message", err)
	}
}

func TestParseBytes_RequiresID(t *testing.T) {
	src := `---
schema framework.prompt/v2
tag "system"
---

body
`
	_, err := ParseBytes([]byte(src), "missing-id.prompt")
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if !strings.Contains(err.Error(), "missing required id statement") {
		t.Fatalf("error = %v, want missing id message", err)
	}
}

func TestParseBytes_TagSingleQuoted(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
tag "system"
---

body
`
	result, err := ParseBytes([]byte(src), "tag-single.prompt")
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if got, want := result.Config.Tags, []string{"system"}; !sameStrings(got, want) {
		t.Fatalf("Tags = %#v, want %#v", got, want)
	}
}

func TestParseBytes_TagListQuoted(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
tag ["agent", "debug"]
---

body
`
	result, err := ParseBytes([]byte(src), "tag-list.prompt")
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if got, want := result.Config.Tags, []string{"agent", "debug"}; !sameStrings(got, want) {
		t.Fatalf("Tags = %#v, want %#v", got, want)
	}
}

func TestParseBytes_VarDeclaration(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
var tone = "direct"
---

body
`
	result, err := ParseBytes([]byte(src), "var.prompt")
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if result.Config.Variables["tone"].Default != "direct" {
		t.Fatalf("tone default = %q, want direct", result.Config.Variables["tone"].Default)
	}
}

func TestParseBytes_MalformedHeaderRejected(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
tag system
---

body
`
	_, err := ParseBytes([]byte(src), "bad-header.prompt")
	if err == nil {
		t.Fatal("expected malformed header error")
	}
	if !strings.Contains(err.Error(), "expected quoted string") {
		t.Fatalf("error = %v, want quoted string failure", err)
	}
}

func TestParseBytes_MalformedListRejected(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
tag ["agent", debug]
---

body
`
	_, err := ParseBytes([]byte(src), "bad-list.prompt")
	if err == nil {
		t.Fatal("expected malformed list error")
	}
	if !strings.Contains(err.Error(), "expected quoted string") {
		t.Fatalf("error = %v, want quoted string failure", err)
	}
}

func TestParseBytes_DuplicateIDRejected(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
id agent.pipeline.default
---

body
`
	_, err := ParseBytes([]byte(src), "dup-id.prompt")
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
	if !strings.Contains(err.Error(), "duplicate id statement") {
		t.Fatalf("error = %v, want duplicate id message", err)
	}
}

func TestParseBytes_RejectsYAMLHeader(t *testing.T) {
	src := `---
apiVersion: framework.prompt/v1
id: agent.generic.default
name: Generic
---

body
`
	_, err := ParseBytes([]byte(src), "legacy.prompt")
	if err == nil {
		t.Fatal("expected legacy header rejection")
	}
	if !strings.Contains(err.Error(), "unknown front matter statement") {
		t.Fatalf("error = %v, want unknown statement rejection", err)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
