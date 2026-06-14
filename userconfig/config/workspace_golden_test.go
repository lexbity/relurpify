package config

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCheckedInWorkspaceConfigDecodesCleanly(t *testing.T) {
	repoRoot := repoRootForTest(t)
	cfgPath := filepath.Join(repoRoot, "relurpify_cfg", "workspace.yaml")

	cfg, err := LoadWorkspaceConfig(cfgPath, repoRoot, WorkspaceLoadOptions{Strict: true})
	if err != nil {
		t.Fatalf("checked-in workspace.yaml strict decode failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil WorkspaceConfig")
	}
	if cfg.Model.Name != "gemma4:e4b" {
		t.Fatalf("model.name = %q, want gemma4:e4b", cfg.Model.Name)
	}
	if cfg.Model.Provider != "ollama" {
		t.Fatalf("model.provider = %q, want ollama", cfg.Model.Provider)
	}
}

func TestTemplateWorkspaceConfigDecodesCleanly(t *testing.T) {
	repoRoot := repoRootForTest(t)
	tmplPath := filepath.Join(repoRoot, "templates", "workspace", "workspace.yaml")

	cfg, err := LoadWorkspaceConfig(tmplPath, repoRoot, WorkspaceLoadOptions{Strict: true})
	if err != nil {
		t.Fatalf("template workspace.yaml strict decode failed (remove agents: block?): %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil WorkspaceConfig")
	}
	if cfg.Model.Name != "gemma4:e4b" {
		t.Fatalf("model.name = %q, want gemma4:e4b", cfg.Model.Name)
	}
	if cfg.Model.Provider != "ollama" {
		t.Fatalf("model.provider = %q, want ollama", cfg.Model.Provider)
	}
}

func TestTemplateRoundTripIsStable(t *testing.T) {
	repoRoot := repoRootForTest(t)
	tmplPath := filepath.Join(repoRoot, "templates", "workspace", "workspace.yaml")

	cfg, err := LoadWorkspaceConfig(tmplPath, repoRoot, WorkspaceLoadOptions{Strict: true})
	if err != nil {
		t.Fatalf("template decode: %v", err)
	}
	reEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}

	var decodedAgain WorkspaceConfig
	if err := yaml.Unmarshal(reEncoded, &decodedAgain); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if decodedAgain.Model.Name != "gemma4:e4b" {
		t.Fatalf("after round-trip model.name = %q, want gemma4:e4b", decodedAgain.Model.Name)
	}
}

func TestCheckedInConfigRoundTripIsStable(t *testing.T) {
	repoRoot := repoRootForTest(t)
	cfgPath := filepath.Join(repoRoot, "relurpify_cfg", "workspace.yaml")

	cfg, err := LoadWorkspaceConfig(cfgPath, repoRoot, WorkspaceLoadOptions{Strict: true})
	if err != nil {
		t.Fatalf("checked-in decode: %v", err)
	}
	reEncoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}

	var decodedAgain WorkspaceConfig
	if err := yaml.Unmarshal(reEncoded, &decodedAgain); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if decodedAgain.Model.Name != "gemma4:e4b" {
		t.Fatalf("after round-trip model.name = %q, want gemma4:e4b", decodedAgain.Model.Name)
	}
}

func TestTemplateHasNoInlineAgents(t *testing.T) {
	repoRoot := repoRootForTest(t)
	tmplPath := filepath.Join(repoRoot, "templates", "workspace", "workspace.yaml")

	data, err := ReadFileRaw(tmplPath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "agents:") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Fatalf("template still has inline agents: block: %q", line)
		}
	}
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	// Walk up from test file to find go.mod
	dir := "."
	for i := 0; i < 10; i++ {
		if _, err := ReadFileRaw(filepath.Join(dir, "go.mod")); err == nil {
			abs, _ := filepath.Abs(dir)
			return abs
		}
		dir = filepath.Join("..", dir)
	}
	t.Fatal("could not determine repo root")
	return ""
}
