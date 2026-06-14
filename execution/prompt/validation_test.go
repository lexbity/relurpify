package prompt

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	Agentdup_validation_test            = "agent.dup"
	AgentGenericDefault_validation_test = "agent.generic.default"
	FrameworkPromptV2_validation_test   = "framework.prompt/v2"
)

func TestValidateConfig_MissingSchema(t *testing.T) {
	issues := validateConfig(&PromptConfig{
		ID:   AgentGenericDefault_validation_test,
		Body: "Hello",
	})
	if !hasIssueContaining(issues, "unknown schema") {
		t.Fatalf("issues = %#v, want missing schema error", issues)
	}
}

func TestValidateConfig_MissingID(t *testing.T) {
	issues := validateConfig(&PromptConfig{
		Schema: FrameworkPromptV2_validation_test,
		Body:   "Hello",
	})
	if !hasIssueContaining(issues, "missing required field: id") {
		t.Fatalf("issues = %#v, want missing id error", issues)
	}
}

func TestRegistry_DuplicateIDError(t *testing.T) {
	reg := NewRegistry()
	err := reg.LoadFS(fstest.MapFS{
		"a.prompt": {Data: []byte(promptFile(Agentdup_validation_test, nil, nil, "one"))},
		"b.prompt": {Data: []byte(promptFile(Agentdup_validation_test, nil, nil, "two"))},
	}, "fixtures")
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
	var dupErr *DuplicateIDError
	if !errors.As(err, &dupErr) {
		t.Fatalf("error = %v, want DuplicateIDError", err)
	}
	if dupErr.ID != Agentdup_validation_test {
		t.Fatalf("DuplicateIDError.ID = %q, want agent.dup", dupErr.ID)
	}
}

func TestParseBytes_InvalidTagSyntax(t *testing.T) {
	_, err := ParseBytes([]byte(`---
schema framework.prompt/v2
id agent.generic.default
tag system
---

body
`), "bad-tag.prompt")
	if err == nil {
		t.Fatal("expected invalid tag syntax error")
	}
	if !strings.Contains(err.Error(), "expected quoted string") {
		t.Fatalf("error = %v, want quoted-string failure", err)
	}
}

func TestParseBytes_InvalidTagListElement(t *testing.T) {
	_, err := ParseBytes([]byte(`---
schema framework.prompt/v2
id agent.generic.default
tag ["system", debug]
---

body
`), "bad-tag-list.prompt")
	if err == nil {
		t.Fatal("expected invalid tag list element error")
	}
	if !strings.Contains(err.Error(), "expected quoted string") {
		t.Fatalf("error = %v, want quoted-string failure", err)
	}
}

func TestParseBytes_DuplicateVariableName(t *testing.T) {
	_, err := ParseBytes([]byte(`---
schema framework.prompt/v2
id agent.generic.default
var tone = "direct"
var tone = "gentle"
---

body
`), "dup-var.prompt")
	if err == nil {
		t.Fatal("expected duplicate variable name error")
	}
	if !strings.Contains(err.Error(), "duplicate variable") {
		t.Fatalf("error = %v, want duplicate variable", err)
	}
}

func TestValidateConfig_UnresolvedVariableReference(t *testing.T) {
	issues := validateConfig(&PromptConfig{
		Schema: FrameworkPromptV2_validation_test,
		ID:     AgentGenericDefault_validation_test,
		Body:   "Use {tone}.",
	})
	if !hasIssueContaining(issues, "unknown variable") {
		t.Fatalf("issues = %#v, want unresolved variable error", issues)
	}
}

func TestValidateConfig_InvalidMarkdownBody(t *testing.T) {
	issues := validateConfig(&PromptConfig{
		Schema: FrameworkPromptV2_validation_test,
		ID:     AgentGenericDefault_validation_test,
		Body:   "Use {1bad}.",
	})
	if !hasIssueContaining(issues, "invalid variable reference") {
		t.Fatalf("issues = %#v, want invalid markdown/body error", issues)
	}
}

func TestValidateConfig_EmptyBodyPolicy(t *testing.T) {
	issues := validateConfig(&PromptConfig{
		Schema: FrameworkPromptV2_validation_test,
		ID:     AgentGenericDefault_validation_test,
		Body:   "  ",
	})
	if !hasIssueContaining(issues, "prompt body is required") {
		t.Fatalf("issues = %#v, want empty body error", issues)
	}
}

func hasIssueContaining(issues []ValidationIssue, substr string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, substr) {
			return true
		}
	}
	return false
}
