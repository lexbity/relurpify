package cfgload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeWorkspaceConfigLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workspace.yaml")

	// Ensure loading non-existent file returns error
	_, err := LoadRuntimeWorkspaceConfig(configPath)
	if err == nil {
		t.Fatal("expected error loading missing config file")
	}

	cfg := RuntimeWorkspaceConfig{
		Model:          "qwen2.5-coder:14b",
		Provider:       "ollama",
		SandboxBackend: "gvisor",
		Agents:         []string{"euclo", "coding"},
		AllowedCapabilities: []RuntimeCapabilitySelector{
			{
				ID:   "cap-1",
				Name: "read_workspace",
				Kind: "fs",
			},
		},
		LastUpdated: time.Now().Unix(),
	}

	// Save config
	err = SaveRuntimeWorkspaceConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("failed to save runtime workspace config: %v", err)
	}

	// Load and verify
	loaded, err := LoadRuntimeWorkspaceConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load runtime workspace config: %v", err)
	}

	if loaded.Model != cfg.Model {
		t.Errorf("model mismatch: got %q, want %q", loaded.Model, cfg.Model)
	}
	if loaded.Provider != cfg.Provider {
		t.Errorf("provider mismatch: got %q, want %q", loaded.Provider, cfg.Provider)
	}
	if loaded.SandboxBackend != cfg.SandboxBackend {
		t.Errorf("sandbox backend mismatch: got %q, want %q", loaded.SandboxBackend, cfg.SandboxBackend)
	}
	if len(loaded.Agents) != 2 || loaded.Agents[0] != "euclo" {
		t.Errorf("agents mismatch: got %v", loaded.Agents)
	}
	if len(loaded.AllowedCapabilities) != 1 || loaded.AllowedCapabilities[0].ID != "cap-1" {
		t.Errorf("allowed capabilities mismatch: got %v", loaded.AllowedCapabilities)
	}
}

func TestRuntimeProviderConfigLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	providerPath := filepath.Join(tmpDir, "providers.yaml")

	cfg := RuntimeProviderConfig{
		Provider:          "ollama",
		Endpoint:          "http://localhost:11434",
		Model:             "llama3:latest",
		NativeToolCalling: true,
		LastUpdated:       time.Now().Unix(),
	}

	// Save
	err := SaveYAML(providerPath, cfg)
	if err != nil {
		t.Fatalf("failed to save provider config: %v", err)
	}

	// Load
	loaded, err := LoadRuntimeProviderConfig(providerPath)
	if err != nil {
		t.Fatalf("failed to load provider config: %v", err)
	}

	if loaded.Provider != cfg.Provider {
		t.Errorf("provider mismatch: got %q, want %q", loaded.Provider, cfg.Provider)
	}
	if loaded.Endpoint != cfg.Endpoint {
		t.Errorf("endpoint mismatch: got %q, want %q", loaded.Endpoint, cfg.Endpoint)
	}
	if loaded.Model != cfg.Model {
		t.Errorf("model mismatch: got %q, want %q", loaded.Model, cfg.Model)
	}
	if loaded.NativeToolCalling != cfg.NativeToolCalling {
		t.Errorf("native tool calling mismatch: got %v, want %v", loaded.NativeToolCalling, cfg.NativeToolCalling)
	}
}

func TestRuntimeKeybindingConfigLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	keybindingPath := filepath.Join(tmpDir, "keybindings.yaml")

	cfg := RuntimeKeybindingConfig{
		Bindings: []RuntimeKeybindingEntry{
			{
				Action:      "exit",
				Keys:        []string{"ctrl+c", "q"},
				Scope:       "global",
				Description: "Exit the application",
				DefaultKeys: []string{"ctrl+c"},
			},
		},
	}

	// Save
	err := SaveYAML(keybindingPath, cfg)
	if err != nil {
		t.Fatalf("failed to save keybindings: %v", err)
	}

	// Load
	loaded, err := LoadRuntimeKeybindingConfig(keybindingPath)
	if err != nil {
		t.Fatalf("failed to load keybindings: %v", err)
	}

	if len(loaded.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(loaded.Bindings))
	}
	binding := loaded.Bindings[0]
	if binding.Action != "exit" {
		t.Errorf("action mismatch: got %q, want exit", binding.Action)
	}
	if len(binding.Keys) != 2 || binding.Keys[0] != "ctrl+c" {
		t.Errorf("keys mismatch: got %v", binding.Keys)
	}
}

func TestCreateTimestampedBackupAndPruning(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "workspace.yaml")

	// Pre-seed file content
	originalContent := "schema: relurpify/workspace/v1\nmodel: test\n"
	err := os.WriteFile(filePath, []byte(originalContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	// Create backup
	backupPath, err := CreateTimestampedBackup(filePath)
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}

	if backupPath == "" {
		t.Fatal("expected non-empty backup path")
	}
	if !strings.Contains(backupPath, "workspace.yaml") || !strings.HasSuffix(backupPath, ".bak") {
		t.Errorf("unexpected backup path format: %q", backupPath)
	}

	// Verify backup content matches
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}
	if string(backupData) != originalContent {
		t.Errorf("backup content mismatch:\ngot:  %q\nwant: %q", string(backupData), originalContent)
	}

	// Test pruning: create 12 backups, check if only 10 remain (since max is 10)
	for i := 0; i < 11; i++ {
		// Sleep slightly to guarantee unique timestamps or sequence numbers
		time.Sleep(10 * time.Millisecond)
		_, err := CreateTimestampedBackup(filePath)
		if err != nil {
			t.Fatalf("failed to create backup iteration %d: %v", i, err)
		}
	}

	backupDir := filepath.Join(tmpDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("failed to read backup directory: %v", err)
	}

	// Expect exactly 10 backups due to max=10 pruning limit
	if len(entries) != 10 {
		t.Errorf("expected 10 backups after pruning, got %d", len(entries))
	}
}
