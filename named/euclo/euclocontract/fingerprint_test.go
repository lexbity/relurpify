package euclocontract

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestContractFingerprint_StableOnUnchangedInput(t *testing.T) {
	workspace := t.TempDir()
	writeTestSecurityFiles(t, workspace, "allow")

	c := config.BuildEffectiveAgentContract("euclo", &config.AgentSpec{
		Implementation: "coding",
		Model:          config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	first := config.ContractFingerprint(c, workspace)
	second := config.ContractFingerprint(c, workspace)
	if first != second {
		t.Fatal("fingerprint changed between calls on unchanged input")
	}
}

func TestContractFingerprint_Stable100x(t *testing.T) {
	workspace := t.TempDir()
	writeTestSecurityFiles(t, workspace, "allow")

	c := config.BuildEffectiveAgentContract("euclo", &config.AgentSpec{
		Model: config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	fp := config.ContractFingerprint(c, workspace)
	for i := 0; i < 100; i++ {
		got := config.ContractFingerprint(c, workspace)
		if got != fp {
			t.Fatalf("iteration %d: fingerprint changed", i)
		}
	}
}

func TestContractFingerprint_ChangesWhenLocaltoolChanges(t *testing.T) {
	workspace := t.TempDir()
	writeTestSecurityFiles(t, workspace, "allow")

	c := config.BuildEffectiveAgentContract("euclo", &config.AgentSpec{
		Model: config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	allowFP := config.ContractFingerprint(c, workspace)

	writeTestSecurityFiles(t, workspace, "deny")
	denyFP := config.ContractFingerprint(c, workspace)

	if denyFP == allowFP {
		t.Fatal("fingerprint should change when localtool policy changes")
	}
}

func TestContractFingerprint_ChangesWhenContractChanges(t *testing.T) {
	workspace := t.TempDir()
	writeTestSecurityFiles(t, workspace, "allow")

	base := config.BuildEffectiveAgentContract("euclo", &config.AgentSpec{
		Model: config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	baseFP := config.ContractFingerprint(base, workspace)

	modified := config.BuildEffectiveAgentContract("euclo", &config.AgentSpec{
		Model: config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionDeny},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	modifiedFP := config.ContractFingerprint(modified, workspace)

	if modifiedFP == baseFP {
		t.Fatal("fingerprint should change when ToolExecutionPolicy changes")
	}
}

func TestContractFingerprint_NilContract(t *testing.T) {
	workspace := t.TempDir()
	writeTestSecurityFiles(t, workspace, "allow")

	fp := config.ContractFingerprint(nil, workspace)
	var zero [32]byte
	if fp == zero {
		t.Fatal("nil contract should produce non-zero fingerprint")
	}
}

func writeTestSecurityFiles(t *testing.T, workspace, cliGitExec string) {
	t.Helper()
	dir := filepath.Join(workspace, "relurpify_cfg", "security")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "localtool.policy.yaml"), []byte("schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: "+cliGitExec+"\n"), 0o600); err != nil {
		t.Fatalf("write localtool: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shell.policy.yaml"), []byte("schema: relurpify/policy/shell/v1\nrules:\n  - id: test\n    pattern: test\n    action: block\n"), 0o600); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sandbox.policy.yaml"), []byte("schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n"), 0o600); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}
}
