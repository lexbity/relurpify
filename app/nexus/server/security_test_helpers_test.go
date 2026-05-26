package server

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSecurityPolicyFixtures(t *testing.T, workspace string) {
	t.Helper()
	securityDir := filepath.Join(workspace, "relurpify_cfg", "security")
	if err := os.MkdirAll(securityDir, 0o755); err != nil {
		t.Fatalf("mkdir security dir: %v", err)
	}
	mustWrite := func(name, body string) {
		path := filepath.Join(securityDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("sandbox.policy.yaml", `schema: relurpify/policy/sandbox/v1
read_only_root: false
protected_paths:
  - relurpify_cfg/workspace.yaml
  - relurpify_cfg/security
  - relurpify_cfg/model/profiles
  - relurpify_cfg/agents
  - relurpify_cfg/tools
no_new_privileges: true
allowed_env_keys: []
denied_env_keys: []
network_rules: []
`)
	mustWrite("shell.policy.yaml", `schema: relurpify/policy/shell/v1
rules:
  - id: deny-git-reset-hard
    pattern: '(^|\s)git\s+reset\s+--hard(\s|$)'
    reason: "Destructive git reset is blocked"
    action: block
`)
	mustWrite("localtool.policy.yaml", `schema: relurpify/policy/localtool/v1
tools:
  cli_git:
    execute: ask
`)
	mustWrite("workspaceingestion.policy.yaml", `schema: relurpify/policy/ingestion/v1
rules:
  - id: allow-workspace-ingestion
    name: "Workspace ingestion"
    priority: 100
    enabled: true
    effect:
      action: allow
      reason: "Allow workspace ingestion for configured sources"
`)

	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools", "shell", "fileops")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools dir: %v", err)
	}
	mustWriteTool := func(name, body string) {
		path := filepath.Join(toolsDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWriteTool("cli_git.tool.yaml", `schema: relurpify/tool/v1
name: cli_git
family: fileops
intent: [inspect, repository]
description: Runs git with the provided arguments.
execution:
  backend: subprocess
  command:
    base: ["git"]
  sandbox:
    allowed_root: ${workspace}
    timeout_seconds: 30
  allow_stdin: true
  supports_workdir: true
capability:
  trust_class: builtin_trusted
  risk_class: [execute]
  effect_class: [filesystem_read]
`)
}
