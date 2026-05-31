package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigCheckCleanRepo(t *testing.T) {
	c := configCheck{}
	diags := c.Run(repoRoot())
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for clean repo, got %d: %+v", len(diags), diags)
	}
}

func TestConfigCheckBadPolicy(t *testing.T) {
	workspace := writeValidWorkspace(t)
	policyPath := filepath.Join(workspace, "relurpify_cfg", "security", "sandbox.policy.yaml")
	os.WriteFile(policyPath, []byte("schema: relurpify/policy/sandbox/v1\n\nprotected_paths: [invalid\n"), 0o644)

	c := configCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for bad policy, got none")
	}
	found := false
	for _, d := range diags {
		if d.Check != "config" {
			t.Fatalf("expected Check=config, got %q", d.Check)
		}
		if strings.Contains(d.Loc.File, "sandbox.policy.yaml") {
			found = true
		}
		if d.Severity != SeverityError {
			t.Fatalf("expected Severity=error, got %v", d.Severity)
		}
	}
	if !found {
		t.Fatalf("expected issue in sandbox.policy.yaml, got: %+v", diags)
	}
}

func TestConfigCheckExcludesToolIssues(t *testing.T) {
	workspace := writeValidWorkspace(t)
	// Add a broken tool manifest
	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools")
	os.WriteFile(filepath.Join(toolsDir, "broken.tool.yaml"), []byte("schema: relurpify/tool/v1\ninvalid: true\n"), 0o644)

	c := configCheck{}
	diags := c.Run(workspace)
	for _, d := range diags {
		if d.Check == "tools" || strings.HasSuffix(d.Loc.File, ".tool.yaml") {
			t.Fatalf("config check should not include tool issues, got: %+v", d)
		}
	}
}

func TestConfigCheckEmptyWorkspace(t *testing.T) {
	diags := runCheck(t, configCheck{}, "")
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for empty workspace")
	}
}

func runCheck(t *testing.T, c Check, workspace string) []Diagnostic {
	t.Helper()
	if workspace == "" {
		return c.Run("")
	}
	return c.Run(workspace)
}

func writeValidWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	mustMkdirAll(t, filepath.Join(cfgRoot, "security"))
	mustMkdirAll(t, filepath.Join(cfgRoot, "model", "provider"))
	mustMkdirAll(t, filepath.Join(cfgRoot, "model", "profiles"))
	mustMkdirAll(t, filepath.Join(cfgRoot, "tools"))

	mustWrite(t, filepath.Join(cfgRoot, "workspace.yaml"), `schema: relurpify/workspace/v1
model:
  provider: ollama
  name: qwen2.5-coder:14b
`)
	mustWrite(t, filepath.Join(cfgRoot, "security", "sandbox.policy.yaml"), `schema: relurpify/policy/sandbox/v1

read_only_root: false
protected_paths:
  - relurpify_cfg
no_new_privileges: true
seccomp_profile: ""
network_rules: []
`)
	mustWrite(t, filepath.Join(cfgRoot, "security", "shell.policy.yaml"), `schema: relurpify/policy/shell/v1

rules: []
`)
	mustWrite(t, filepath.Join(cfgRoot, "security", "localtool.policy.yaml"), `schema: relurpify/policy/localtool/v1

tools: {}
`)
	mustWrite(t, filepath.Join(cfgRoot, "security", "workspaceingestion.policy.yaml"), `schema: relurpify/policy/ingestion/v1

rules: []
`)
	mustWrite(t, filepath.Join(cfgRoot, "model", "provider", "ollama.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
available_models:
  - qwen2.5-coder:14b
native_tool_calling: true
max_concurrent: 2
`)
	mustWrite(t, filepath.Join(cfgRoot, "model", "profiles", "default.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: auto
  max_concurrent_tools: 4
  double_encode_args: false
context:
  max_tokens: 8192
  response_reserve_tokens: 512
generation:
  temperature: 0.2
  top_p: 0.95
`)
	return workspace
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
