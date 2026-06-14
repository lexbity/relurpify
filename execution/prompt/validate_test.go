package prompt

import (
	"strings"
	"testing"
)

const (
	Hello_validate_test               = "Hello"
	AgentGenericDefault_validate_test = "agent.generic.default"
	FrameworkPromptV2_validate_test   = "framework.prompt/v2"
)

func TestValidateConfig_RejectsMalformedV2Files(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *PromptConfig
		wantSub string
	}{
		{
			name: "missing schema",
			cfg: &PromptConfig{
				ID:   AgentGenericDefault_validate_test,
				Body: Hello_validate_test,
			},
			wantSub: "unknown schema",
		},
		{
			name: "missing id",
			cfg: &PromptConfig{
				Schema: FrameworkPromptV2_validate_test,
				Body:   Hello_validate_test,
			},
			wantSub: "missing required field: id",
		},
		{
			name: "unused variable",
			cfg: &PromptConfig{
				Schema: FrameworkPromptV2_validate_test,
				ID:     AgentGenericDefault_validate_test,
				Body:   Hello_validate_test,
				Variables: map[string]VariableDecl{
					"tone": {Default: "direct"},
				},
			},
			wantSub: "variable declared but not used: tone",
		},
		{
			name: "unresolved variable",
			cfg: &PromptConfig{
				Schema: FrameworkPromptV2_validate_test,
				ID:     AgentGenericDefault_validate_test,
				Body:   "Hello {tone}",
			},
			wantSub: "unknown variable",
		},
		{
			name: "invalid body reference",
			cfg: &PromptConfig{
				Schema: FrameworkPromptV2_validate_test,
				ID:     AgentGenericDefault_validate_test,
				Body:   "Hello {1bad}",
			},
			wantSub: "invalid variable reference",
		},
		{
			name: "empty body",
			cfg: &PromptConfig{
				Schema: FrameworkPromptV2_validate_test,
				ID:     AgentGenericDefault_validate_test,
				Body:   "  ",
			},
			wantSub: "prompt body is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := validateConfig(tc.cfg)
			if !hasIssueContaining(issues, tc.wantSub) {
				t.Fatalf("issues = %#v, want message containing %q", issues, tc.wantSub)
			}
		})
	}
}

func TestValidateConfig_VariableDefaultsAndUsage(t *testing.T) {
	issues := validateConfig(&PromptConfig{
		Schema: FrameworkPromptV2_validate_test,
		ID:     AgentGenericDefault_validate_test,
		Body:   "Hello {name}",
		Variables: map[string]VariableDecl{
			"name": {Default: "world"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateConfig_InvalidTagSyntaxIsRejectedByParser(t *testing.T) {
	_, err := ParseBytes([]byte(`---
schema framework.prompt/v2
id agent.generic.default
tag system
---

body
`), "bad-tag.prompt")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "expected quoted string") {
		t.Fatalf("error = %v, want quoted string failure", err)
	}
}
