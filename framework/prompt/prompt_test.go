package prompt

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseBytes_V2ContractHappyPath(t *testing.T) {
	src := `---
schema framework.prompt/v2
id agent.generic.default
tag "system"
tag ["agent", "debug"]
var tone = "direct"
---

# Greeting

Use {tone} language.
`
	result, err := ParseBytes([]byte(src), "sample.prompt")
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if result.Config.Schema != "framework.prompt/v2" {
		t.Fatalf("Schema = %q, want framework.prompt/v2", result.Config.Schema)
	}
	if result.Config.ID != "agent.generic.default" {
		t.Fatalf("ID = %q, want agent.generic.default", result.Config.ID)
	}
	if got, want := result.Config.Tags, []string{"system", "agent", "debug"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tags = %#v, want %#v", got, want)
	}
	out, _, err := resolvePrompt(result.Config, RuntimeContext{})
	if err != nil {
		t.Fatalf("resolvePrompt: %v", err)
	}
	if out != "# Greeting\n\nUse direct language." {
		t.Fatalf("rendered output = %q", out)
	}
}

func TestResolvePrompt_Deterministic(t *testing.T) {
	cfg := &PromptConfig{
		Schema: "framework.prompt/v2",
		ID:     "agent.generic.default",
		Body:   "Hello {name}.",
		Variables: map[string]VariableDecl{
			"name": {Default: "world"},
		},
	}

	a, _, err := resolvePrompt(cfg, RuntimeContext{})
	if err != nil {
		t.Fatalf("resolvePrompt first: %v", err)
	}
	b, _, err := resolvePrompt(cfg, RuntimeContext{})
	if err != nil {
		t.Fatalf("resolvePrompt second: %v", err)
	}
	if a != b {
		t.Fatalf("outputs differ: %q != %q", a, b)
	}
}

func TestRegistry_TagSearchContracts(t *testing.T) {
	reg := NewRegistry()
	if err := reg.LoadFS(fstest.MapFS{
		"one.prompt":   {Data: []byte(promptFile("agent.one", []string{"system", "agent"}, nil, "One"))},
		"two.prompt":   {Data: []byte(promptFile("agent.two", []string{"debug"}, nil, "Two"))},
		"three.prompt": {Data: []byte(promptFile("agent.three", []string{"system", "debug"}, nil, "Three"))},
	}, "fixtures"); err != nil {
		t.Fatalf("LoadFS: %v", err)
	}

	if got, want := idsOf(reg.Filter(FilterOptions{Tags: []string{"system"}})), []string{"agent.one", "agent.three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("single-tag filter = %#v, want %#v", got, want)
	}
	if got, want := idsOf(reg.Filter(FilterOptions{Tags: []string{"system", "debug"}})), []string{"agent.one", "agent.three", "agent.two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("multi-tag filter = %#v, want %#v", got, want)
	}
}

func TestResolvePrompt_SafeNodeExclusions(t *testing.T) {
	cfg := &PromptConfig{
		Schema: "framework.prompt/v2",
		ID:     "agent.generic.default",
		Body: strings.Join([]string{
			"Hello {name}",
			"",
			"Use `inline {name}` literally.",
			"",
			"```text",
			"{name}",
			"```",
			"",
			`[link {name}](https://example.com/{name})`,
		}, "\n"),
		Variables: map[string]VariableDecl{
			"name": {Default: "world"},
		},
	}

	out, _, err := resolvePrompt(cfg, RuntimeContext{Variables: map[string]string{"name": "Alice"}})
	if err != nil {
		t.Fatalf("resolvePrompt: %v", err)
	}
	if !strings.Contains(out, "Hello Alice") {
		t.Fatalf("output = %q, want substituted paragraph text", out)
	}
	if !strings.Contains(out, "`inline {name}`") {
		t.Fatalf("output = %q, want inline code preserved", out)
	}
	if !strings.Contains(out, "```text\n{name}\n```") {
		t.Fatalf("output = %q, want fenced code preserved", out)
	}
	if !strings.Contains(out, "[link Alice](https://example.com/{name})") {
		t.Fatalf("output = %q, want link destination preserved", out)
	}
}
