package config

import (
	"os"
	"path/filepath"
	"testing"

	permissions "codeburg.org/lexbit/relurpify/platform/configpermissions"
)

func TestContractFingerprint_StableOnUnchangedInput(t *testing.T) {
	workspace := t.TempDir()
	writeSecurityFiles(t, workspace, "allow")

	c := BuildEffectiveAgentContract("euclo", &AgentSpec{
		Implementation: "coding",
		Version:        "2",
		Model:          AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, ResourceSpec{}, SecuritySpec{}, SourceSummary{})

	first := ContractFingerprint(c, workspace)
	second := ContractFingerprint(c, workspace)
	third := ContractFingerprint(c, workspace)
	fourth := ContractFingerprint(c, workspace)

	if first != second {
		t.Fatal("fingerprint changed between first and second call")
	}
	if second != third {
		t.Fatal("fingerprint changed between second and third call")
	}
	if third != fourth {
		t.Fatal("fingerprint changed between third and fourth call")
	}
}

func TestContractFingerprint_Stable100x(t *testing.T) {
	workspace := t.TempDir()
	writeSecurityFiles(t, workspace, "allow")

	c := BuildEffectiveAgentContract("euclo", &AgentSpec{
		Model: AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
			"bash":    {Execute: AgentPermissionAsk},
		},
		Bash: AgentBashPermissions{DenyPatterns: []string{"pattern1", "pattern2"}},
	}, permissions.PermissionSet{}, ResourceSpec{}, SecuritySpec{}, SourceSummary{})

	fp := ContractFingerprint(c, workspace)
	for i := 0; i < 100; i++ {
		got := ContractFingerprint(c, workspace)
		if got != fp {
			t.Fatalf("iteration %d: fingerprint changed", i)
		}
	}
}

func TestContractFingerprint_ChangesWhenLocaltoolChanges(t *testing.T) {
	workspace := t.TempDir()
	writeSecurityFiles(t, workspace, "allow")

	c := BuildEffectiveAgentContract("euclo", &AgentSpec{
		Model: AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, ResourceSpec{}, SecuritySpec{}, SourceSummary{})

	allowFP := ContractFingerprint(c, workspace)

	// Change the localtool policy to "deny".
	writeSecurityFiles(t, workspace, "deny")
	denyFP := ContractFingerprint(c, workspace)

	if denyFP == allowFP {
		t.Fatal("fingerprint should change when localtool policy changes")
	}
}

func TestContractFingerprint_ChangesWhenContractChanges(t *testing.T) {
	workspace := t.TempDir()
	writeSecurityFiles(t, workspace, "allow")

	base := BuildEffectiveAgentContract("euclo", &AgentSpec{
		Model: AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, ResourceSpec{}, SecuritySpec{}, SourceSummary{})

	baseFP := ContractFingerprint(base, workspace)

	modified := BuildEffectiveAgentContract("euclo", &AgentSpec{
		Model: AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionDeny},
		},
	}, permissions.PermissionSet{}, ResourceSpec{}, SecuritySpec{}, SourceSummary{})

	modifiedFP := ContractFingerprint(modified, workspace)

	if modifiedFP == baseFP {
		t.Fatal("fingerprint should change when ToolExecutionPolicy changes")
	}
}

func TestContractFingerprint_NilContract(t *testing.T) {
	workspace := t.TempDir()
	writeSecurityFiles(t, workspace, "allow")

	fp := ContractFingerprint(nil, workspace)
	var zero [32]byte
	if fp == zero {
		t.Fatal("nil contract should produce non-zero fingerprint")
	}
}

// writeSecurityFiles creates the minimal security policy files for fingerprint
// testing, with the given cli_git execute value.
func writeSecurityFiles(t *testing.T, workspace, cliGitExec string) {
	t.Helper()

	dir := filepath.Join(workspace, "relurpify_cfg", "security")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	localtool := filepath.Join(dir, "localtool.policy.yaml")
	data := []byte("schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: " + cliGitExec + "\n")
	if err := os.WriteFile(localtool, data, 0o600); err != nil {
		t.Fatalf("write localtool: %v", err)
	}

	shell := filepath.Join(dir, "shell.policy.yaml")
	if err := os.WriteFile(shell, []byte("schema: relurpify/policy/shell/v1\nrules:\n  - id: test\n    pattern: test\n    action: block\n"), 0o600); err != nil {
		t.Fatalf("write shell: %v", err)
	}

	sandbox := filepath.Join(dir, "sandbox.policy.yaml")
	if err := os.WriteFile(sandbox, []byte("schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n"), 0o600); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}
}
