package prompt

import (
	"strings"
	"testing"
)

const (
	AgentGenericDefault_markdown_test = "agent.generic.default"
	Name_markdown_test = "name"
)


func TestResolvePrompt_SubstitutesOnlyEligibleMarkdownText(t *testing.T) {
	cfg := &PromptConfig{
		ID: AgentGenericDefault_markdown_test,
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
			"",
			`<span data-name="{name}"></span>`,
		}, "\n"),
		Variables: map[string]VariableDecl{
			Name_markdown_test: {Default: "world"},
		},
	}

	out, vars, err := resolvePrompt(cfg, RuntimeContext{Variables: map[string]string{Name_markdown_test: "Alice"}})
	if err != nil {
		t.Fatalf("resolvePrompt: %v", err)
	}
	if vars[Name_markdown_test] != "Alice" {
		t.Fatalf("resolved variable = %q, want Alice", vars[Name_markdown_test])
	}
	if !strings.Contains(out, "Hello Alice") {
		t.Fatalf("output missing substituted paragraph text: %q", out)
	}
	if !strings.Contains(out, "`inline {name}`") {
		t.Fatalf("output missing preserved inline code: %q", out)
	}
	if !strings.Contains(out, "```text\n{name}\n```") {
		t.Fatalf("output missing preserved fenced code block: %q", out)
	}
	if !strings.Contains(out, "[link Alice](https://example.com/{name})") {
		t.Fatalf("output missing link label substitution or destination preservation: %q", out)
	}
	if !strings.Contains(out, `<span data-name="{name}"></span>`) {
		t.Fatalf("output missing preserved raw HTML: %q", out)
	}
}

func TestResolvePrompt_EscapesBraces(t *testing.T) {
	cfg := &PromptConfig{
		ID:   AgentGenericDefault_markdown_test,
		Body: `Use \{literal\} and {name}.`,
		Variables: map[string]VariableDecl{
			Name_markdown_test: {Default: "world"},
		},
	}

	out, _, err := resolvePrompt(cfg, RuntimeContext{})
	if err != nil {
		t.Fatalf("resolvePrompt: %v", err)
	}
	if out != "Use {literal} and world." {
		t.Fatalf("output = %q, want Use {literal} and world.", out)
	}
}

func TestResolvePrompt_UnknownVariableErrors(t *testing.T) {
	cfg := &PromptConfig{
		ID:   AgentGenericDefault_markdown_test,
		Body: `Hello {missing}.`,
	}

	_, _, err := resolvePrompt(cfg, RuntimeContext{})
	if err == nil {
		t.Fatal("expected unknown variable error")
	}
	if !strings.Contains(err.Error(), "unknown variable") {
		t.Fatalf("error = %v, want unknown variable", err)
	}
}

func TestValidateConfig_IgnoresCodeBlockVariables(t *testing.T) {
	cfg := &PromptConfig{
		ID: AgentGenericDefault_markdown_test,
		Body: strings.Join([]string{
			"```text",
			"{tone}",
			"```",
		}, "\n"),
		Variables: map[string]VariableDecl{
			"tone": {Default: "direct"},
		},
	}

	issues := validateConfig(cfg)
	found := false
	for _, issue := range issues {
		if issue.Severity == SeverityWarning && strings.Contains(issue.Message, "variable declared but not used: tone") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unused-variable warning for code block-only reference, got %#v", issues)
	}
}
