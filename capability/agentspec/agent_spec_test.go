package agentspec

import (
	"strings"
	"testing"
)

func TestAgentBashPermissionsValidate(t *testing.T) {
	ok := []AgentBashPermissions{
		{},
		{Default: AgentPermissionAllow},
		{Default: AgentPermissionAsk},
		{Default: AgentPermissionDeny},
		{AllowPatterns: []string{"git *"}, DenyPatterns: []string{"rm *"}},
	}
	for i, bash := range ok {
		if err := bash.Validate(); err != nil {
			t.Errorf("case %d: unexpected error: %v", i, err)
		}
	}

	bad := []AgentBashPermissions{
		{Default: AgentPermissionLevel("alow")},
		{Default: AgentPermissionLevel("ALLOWED")},
		{AllowPatterns: []string{" "}},
		{DenyPatterns: []string{"git *", "\t"}},
	}
	for i, bash := range bad {
		err := bash.Validate()
		if err == nil {
			t.Errorf("case %d: expected error", i)
			continue
		}
		if !strings.Contains(err.Error(), "bash_permissions") {
			t.Errorf("case %d: error should name the field, got %q", i, err.Error())
		}
	}
}

func TestAgentRuntimeSpecValidateRejectsBashTypo(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode:  AgentModePrimary,
		Model: AgentModelConfig{Provider: "test-provider", Name: "test-model"},
		Bash:  AgentBashPermissions{Default: AgentPermissionLevel("alow")},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected a typo in bash_permissions.default to be rejected")
	}
	if !strings.Contains(err.Error(), "bash_permissions") {
		t.Fatalf("error should name bash_permissions, got %q", err.Error())
	}
}

func TestAgentFilePermissionSetValidateRejectsTypo(t *testing.T) {
	matrix := AgentFileMatrix{
		Write: AgentFilePermissionSet{Default: AgentPermissionLevel("alow")},
	}
	if err := matrix.Validate(); err == nil {
		t.Fatal("expected a typo in file permission default to be rejected")
	}
}
