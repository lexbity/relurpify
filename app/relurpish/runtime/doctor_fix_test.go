package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
	"gopkg.in/yaml.v3"
)

type rawWorkspaceConfig struct {
	Model struct {
		Provider string `yaml:"provider"`
		Name     string `yaml:"name"`
	} `yaml:"model"`
}

func decodeRawWorkspaceConfig(path string) (*rawWorkspaceConfig, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var cfg rawWorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// TestInitializeWorkspaceFromTemplatesWritesValidConfig verifies that
// --fix on an empty workspace produces a valid relurpify_cfg/workspace.yaml
// (AC-1, AC-5b).
func TestInitializeWorkspaceFromTemplatesWritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("normalize config: %v", err)
	}

	if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
		t.Fatalf("initialize workspace from templates: %v", err)
	}

	wsConfigPath := cfg.ConfigPath
	if _, err := os.Stat(wsConfigPath); err != nil {
		t.Fatalf("config file not created at %s: %v", wsConfigPath, err)
	}

	wsCfg, err := decodeRawWorkspaceConfig(wsConfigPath)
	if err != nil {
		t.Fatalf("decode generated config: %v", err)
	}
	if wsCfg.Model.Name != "gemma4:e4b" {
		t.Fatalf("model.name = %q after --fix, want gemma4:e4b", wsCfg.Model.Name)
	}

	// The full default tree must be materialized, not just workspace.yaml:
	// model profiles, the provider catalog, and security policies are all
	// essential for a usable workspace (OOBE: #1).
	configRoot := config.New(dir).ConfigRoot()
	for _, rel := range []string{
		"model/profiles/default.llm.yaml",
		"model/provider/ollama.provider.yaml",
		"model/provider/lmstudio.provider.yaml",
		"model/provider/openai_compatible.provider.yaml",
		"security/sandbox.policy.yaml",
		"security/shell.policy.yaml",
		"security/localtool.policy.yaml",
		"security/workspaceingestion.policy.yaml",
	} {
		if _, err := os.Stat(filepath.Join(configRoot, rel)); err != nil {
			t.Fatalf("essential template %s not materialized: %v", rel, err)
		}
	}

	// The materialized provider catalog must load and resolve through the same
	// path the runtime uses (provider catalog drives behavior end-to-end).
	providers, err := model.LoadProviderDir(filepath.Join(configRoot, "model", "provider"), config.StrictDecode)
	if err != nil {
		t.Fatalf("load materialized provider catalog: %v", err)
	}
	if len(providers) < 3 {
		t.Fatalf("materialized provider catalog has %d providers, want >= 3", len(providers))
	}
}

// TestInitializeWorkspaceIdempotent verifies that a second --fix without
// --overwrite leaves the existing config byte-identical (AC-5b, R-7).
func TestInitializeWorkspaceIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("normalize config: %v", err)
	}

	if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
		t.Fatalf("first initialize: %v", err)
	}

	wsConfigPath := cfg.ConfigPath
	data1, err := os.ReadFile(filepath.Clean(wsConfigPath))
	if err != nil {
		t.Fatalf("read config after first init: %v", err)
	}

	if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
		t.Fatalf("second initialize: %v", err)
	}

	data2, err := os.ReadFile(filepath.Clean(wsConfigPath))
	if err != nil {
		t.Fatalf("read config after second init: %v", err)
	}

	if string(data1) != string(data2) {
		t.Fatal("second --fix without --overwrite mutated the config (byte-identical expected)")
	}
}

// TestInitializeWorkspaceIdempotentKeepModelName verifies that model name
// stays at gemma4:e4b across multiple --fix calls (AC-5b).
func TestInitializeWorkspaceIdempotentKeepModelName(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("normalize config: %v", err)
	}

	if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
		t.Fatalf("first initialize: %v", err)
	}

	wsConfigPath := cfg.ConfigPath
	wsCfg, err := decodeRawWorkspaceConfig(wsConfigPath)
	if err != nil {
		t.Fatalf("decode after first init: %v", err)
	}
	if wsCfg.Model.Name != "gemma4:e4b" {
		t.Fatalf("model.name = %q after first init, want gemma4:e4b", wsCfg.Model.Name)
	}

	if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
		t.Fatalf("second initialize: %v", err)
	}

	wsCfg2, err := decodeRawWorkspaceConfig(wsConfigPath)
	if err != nil {
		t.Fatalf("decode after second init: %v", err)
	}
	if wsCfg2.Model.Name != "gemma4:e4b" {
		t.Fatalf("model.name = %q after second init, want gemma4:e4b", wsCfg2.Model.Name)
	}
}
