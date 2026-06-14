package testhelper

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWriteCleanWorkspace_NoAgentsDirCreated(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspace(t, workspace, WorkspaceOpts{
		Provider:   "offline",
		CliGitExec: "allow",
	})

	agentsDir := filepath.Join(workspace, "relurpify_cfg", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("read agents dir: %v", err)
	}
	if len(entries) > 0 {
		t.Fatalf("agents/ dir has %d entries; expected empty", len(entries))
	}
}

func TestWriteCleanWorkspace_LocalToolPolicyRoundTrips(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspace(t, workspace, WorkspaceOpts{
		Provider:   "offline",
		CliGitExec: "allow",
	})

	policyPath := filepath.Join(workspace, "relurpify_cfg", "security", "localtool.policy.yaml")
	data, err := os.ReadFile(filepath.Clean(policyPath))
	if err != nil {
		t.Fatalf("read localtool policy: %v", err)
	}

	var decoded struct {
		Tools map[string]struct {
			Execute string `yaml:"execute"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml decode: %v", err)
	}
	entry, ok := decoded.Tools["cli_git"]
	if !ok {
		t.Fatal("cli_git not in policy")
	}
	if entry.Execute != "allow" {
		t.Fatalf("cli_git execute = %q, want %q", entry.Execute, "allow")
	}
}

func TestWriteCleanWorkspaceAsk_SetsAskPolicy(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspaceAsk(t, workspace, WorkspaceOpts{
		Provider: "offline",
	})

	policyPath := filepath.Join(workspace, "relurpify_cfg", "security", "localtool.policy.yaml")
	data, err := os.ReadFile(filepath.Clean(policyPath))
	if err != nil {
		t.Fatalf("read localtool policy: %v", err)
	}

	var decoded struct {
		Tools map[string]struct {
			Execute string `yaml:"execute"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml decode: %v", err)
	}
	entry, ok := decoded.Tools["cli_git"]
	if !ok {
		t.Fatal("cli_git not in policy")
	}
	if entry.Execute != "ask" {
		t.Fatalf("cli_git execute = %q, want %q", entry.Execute, "ask")
	}
}

func TestWriteCleanWorkspace_ProviderOverridden(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspace(t, workspace, WorkspaceOpts{
		Provider: "offline",
	})

	wsPath := filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
	data, err := os.ReadFile(filepath.Clean(wsPath))
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}

	var cfg struct {
		Model struct {
			Provider string `yaml:"provider"`
		} `yaml:"model"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml decode: %v", err)
	}
	if cfg.Model.Provider != "offline" {
		t.Fatalf("provider = %q, want %q", cfg.Model.Provider, "offline")
	}
}
