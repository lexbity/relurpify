package cfgload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateWorkspaceTreeCleanRepo(t *testing.T) {
	report := ValidateWorkspaceTree(repoRoot(t))
	if report.HasErrors() {
		t.Fatalf("expected no errors in repo root, got: %s", report.Error())
	}
}

func TestValidateWorkspaceTreeEmptyWorkspace(t *testing.T) {
	report := ValidateWorkspaceTree("")
	if !report.HasErrors() {
		t.Fatal("expected error for empty workspace")
	}
}

func TestValidateWorkspaceTreeMinimalValid(t *testing.T) {
	workspace := writeMinimalValidWorkspace(t)
	report := ValidateWorkspaceTree(workspace)
	if report.HasErrors() {
		t.Fatalf("expected no errors for valid workspace, got: %s", report.Error())
	}
}

func TestValidateWorkspaceTreeMissingWorkspaceYaml(t *testing.T) {
	workspace := writeMinimalValidWorkspace(t)
	os.Remove(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"))
	report := ValidateWorkspaceTree(workspace)
	if !report.HasErrors() {
		t.Fatal("expected error for missing workspace.yaml")
	}
}

func TestValidateWorkspaceTreeBadPolicy(t *testing.T) {
	workspace := writeMinimalValidWorkspace(t)
	policyPath := filepath.Join(workspace, "relurpify_cfg", "security", "sandbox.policy.yaml")
	os.WriteFile(policyPath, []byte("schema: relurpify/policy/sandbox/v1\n\nprotected_paths: [invalid\n"), 0o644)
	report := ValidateWorkspaceTree(workspace)
	if !report.HasErrors() {
		t.Fatal("expected error for malformed policy")
	}
	found := false
	for _, issue := range report.Issues {
		if issue.File == "relurpify_cfg/security/sandbox.policy.yaml" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected issue in sandbox.policy.yaml")
	}
}

func TestValidateWorkspaceTreeMissingToolsDir(t *testing.T) {
	workspace := writeMinimalValidWorkspace(t)
	os.RemoveAll(filepath.Join(workspace, "relurpify_cfg", "tools"))
	report := ValidateWorkspaceTree(workspace)
	if report.HasErrors() {
		t.Fatalf("expected no errors for missing tools dir (optional), got: %s", report.Error())
	}
}

func TestValidateWorkspaceTreeReportImplementsError(t *testing.T) {
	report := ValidateWorkspaceTree("")
	if !report.HasErrors() {
		t.Fatal("expected errors for empty workspace")
	}
	err := report.Err()
	if err == nil {
		t.Fatal("expected Err() to return non-nil for report with errors")
	}
	errStr := report.Error()
	if errStr == "" {
		t.Fatal("expected non-empty Error() string")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join("..", ".."))
}

func writeMinimalValidWorkspace(t *testing.T) string {
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

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
