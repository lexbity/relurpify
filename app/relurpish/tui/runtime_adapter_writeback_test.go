package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// TestSaveLocalToolPolicy_RoundTrip verifies SaveLocalToolPolicy writes a valid
// file that LoadLocalToolPolicy can read back, with the correct tool policy value.
func TestSaveLocalToolPolicy_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "localtool.policy.yaml")

	tools := map[string]security.ToolPolicy{
		"bash":    {Execute: "allow"},
		"cli_git": {Execute: "deny"},
	}
	if err := config.SaveLocalToolPolicy(policyPath, tools); err != nil {
		t.Fatalf("SaveLocalToolPolicy: %v", err)
	}

	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	loaded, err := security.LoadLocalToolPolicy(policyPath, dir, config.StrictDecode)
	if err != nil {
		t.Fatalf("LoadLocalToolPolicy: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded policy is nil")
	}
	if got := loaded["bash"].Execute; got != "allow" {
		t.Errorf("bash execute = %q, want %q", got, "allow")
	}
	if got := loaded["cli_git"].Execute; got != "deny" {
		t.Errorf("cli_git execute = %q, want %q", got, "deny")
	}
}

// TestSaveLocalToolPolicy_BackupCreated verifies that a backup file is created
// when saving over an existing policy file.
func TestSaveLocalToolPolicy_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "localtool.policy.yaml")

	// Write initial policy.
	initial := map[string]security.ToolPolicy{
		"bash": {Execute: "ask"},
	}
	if err := config.SaveLocalToolPolicy(policyPath, initial); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Write updated policy.
	updated := map[string]security.ToolPolicy{
		"bash": {Execute: "allow"},
	}
	if err := config.SaveLocalToolPolicy(policyPath, updated); err != nil {
		t.Fatalf("updated save: %v", err)
	}

	// Verify backup directory exists and contains a backup.
	backupDir := filepath.Join(dir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	var found bool
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "localtool.policy.yaml") && strings.HasSuffix(entry.Name(), ".bak") {
			found = true
			break
		}
	}
	if !found {
		t.Error("no backup file found in backups/")
	}
}

// TestSaveLocalToolPolicy_RejectsInvalid validates that SaveLocalToolPolicy
// rejects invalid execute values.
func TestSaveLocalToolPolicy_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "localtool.policy.yaml")

	invalid := map[string]security.ToolPolicy{
		"bash": {Execute: "invalid_value"},
	}
	if err := config.SaveLocalToolPolicy(policyPath, invalid); err == nil {
		t.Error("expected error for invalid execute value, got nil")
	}
}
