package testhelper

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func MustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func MustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func WriteValidWorkspace(t *testing.T, workspace string) {
	t.Helper()
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	MustMkdirAll(t, filepath.Join(cfgRoot, "security"))
	MustMkdirAll(t, filepath.Join(cfgRoot, "model", "provider"))
	MustMkdirAll(t, filepath.Join(cfgRoot, "model", "profiles"))
	MustMkdirAll(t, filepath.Join(cfgRoot, "tools"))

	MustWrite(t, filepath.Join(cfgRoot, "workspace.yaml"), `schema: relurpify/workspace/v1
model:
  provider: ollama
  name: qwen2.5-coder:14b
`)
	MustWrite(t, filepath.Join(cfgRoot, "security", "sandbox.policy.yaml"), `schema: relurpify/policy/sandbox/v1

read_only_root: false
protected_paths:
  - relurpify_cfg
no_new_privileges: true
seccomp_profile: ""
network_rules: []
`)
	MustWrite(t, filepath.Join(cfgRoot, "security", "shell.policy.yaml"), `schema: relurpify/policy/shell/v1

rules: []
`)
	MustWrite(t, filepath.Join(cfgRoot, "security", "localtool.policy.yaml"), `schema: relurpify/policy/localtool/v1

tools: {}
`)
	MustWrite(t, filepath.Join(cfgRoot, "security", "workspaceingestion.policy.yaml"), `schema: relurpify/policy/ingestion/v1

rules: []
`)
	MustWrite(t, filepath.Join(cfgRoot, "model", "provider", "ollama.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
available_models:
  - qwen2.5-coder:14b
native_tool_calling: true
max_concurrent: 2
`)
	MustWrite(t, filepath.Join(cfgRoot, "model", "profiles", "default.llm.yaml"), `schema: relurpify/model/profile/v1
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
}
