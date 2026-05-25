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
  - relurpify_cfg/agent.manifest.yaml
  - relurpify_cfg/config.yaml
  - relurpify_cfg/nexus.yaml
  - relurpify_cfg/policy_rules.yaml
  - relurpify_cfg/model_profiles
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
  git:
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
}
