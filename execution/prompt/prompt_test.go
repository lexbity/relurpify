package prompt

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	AgentGenericDefault_prompt_test = "agent.generic.default"
	Agentone_prompt_test            = "agent.one"
	Agentthree_prompt_test          = "agent.three"
	Debug_prompt_test               = "debug"
	FrameworkPromptV2_prompt_test   = "framework.prompt/v2"
	Name_prompt_test                = "name"
	System_prompt_test              = "system"
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
	if result.Config.Schema != FrameworkPromptV2_prompt_test {
		t.Fatalf("Schema = %q, want framework.prompt/v2", result.Config.Schema)
	}
	if result.Config.ID != AgentGenericDefault_prompt_test {
		t.Fatalf("ID = %q, want agent.generic.default", result.Config.ID)
	}
	if got, want := result.Config.Tags, []string{System_prompt_test, "agent", Debug_prompt_test}; !reflect.DeepEqual(got, want) {
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
		Schema: FrameworkPromptV2_prompt_test,
		ID:     AgentGenericDefault_prompt_test,
		Body:   "Hello {name}.",
		Variables: map[string]VariableDecl{
			Name_prompt_test: {Default: "world"},
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
		"one.prompt":   {Data: []byte(promptFile(Agentone_prompt_test, []string{System_prompt_test, "agent"}, nil, "One"))},
		"two.prompt":   {Data: []byte(promptFile("agent.two", []string{Debug_prompt_test}, nil, "Two"))},
		"three.prompt": {Data: []byte(promptFile(Agentthree_prompt_test, []string{System_prompt_test, Debug_prompt_test}, nil, "Three"))},
	}, "fixtures"); err != nil {
		t.Fatalf("LoadFS: %v", err)
	}

	if got, want := idsOf(reg.Filter(FilterOptions{Tags: []string{System_prompt_test}})), []string{Agentone_prompt_test, Agentthree_prompt_test}; !reflect.DeepEqual(got, want) {
		t.Fatalf("single-tag filter = %#v, want %#v", got, want)
	}
	if got, want := idsOf(reg.Filter(FilterOptions{Tags: []string{System_prompt_test, Debug_prompt_test}})), []string{Agentone_prompt_test, Agentthree_prompt_test, "agent.two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("multi-tag filter = %#v, want %#v", got, want)
	}
}

func TestResolvePrompt_SafeNodeExclusions(t *testing.T) {
	cfg := &PromptConfig{
		Schema: FrameworkPromptV2_prompt_test,
		ID:     AgentGenericDefault_prompt_test,
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
			Name_prompt_test: {Default: "world"},
		},
	}

	out, _, err := resolvePrompt(cfg, RuntimeContext{Variables: map[string]string{Name_prompt_test: "Alice"}})
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
