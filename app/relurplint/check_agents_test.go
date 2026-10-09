package main

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

func writeAgentDocument(t *testing.T, workspace, body string) {
	t.Helper()
	dir := filepath.Join(workspace, "relurpify_cfg", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	content := `apiVersion: relurpify/v1
kind: AgentManifest
metadata:
  name: test-agent
spec:
` + body
	testhelper.MustWrite(t, filepath.Join(dir, "agent.yaml"), content)
}

func TestAgentsCheckWarnsOnBashRulesWithoutDefault(t *testing.T) {
	workspace := t.TempDir()
	writeAgentDocument(t, workspace, `  agent:
    implementation: coding
    model:
      provider: ollama
      name: gemma
    bash_permissions:
      allow_patterns:
        - "git *"
`)

	c := agentsCheck{}
	diags := c.Run(workspace)
	if len(diags) != 1 {
		t.Fatalf("expected exactly one warning, got %d: %+v", len(diags), diags)
	}
	if diags[0].Code != codeAgentBashDefault {
		t.Fatalf("expected code %s, got %s", codeAgentBashDefault, diags[0].Code)
	}
	if diags[0].Severity != SeverityWarning {
		t.Fatalf("expected warning severity, got %v", diags[0].Severity)
	}
}

func TestAgentsCheckSilentWithExplicitDefault(t *testing.T) {
	workspace := t.TempDir()
	writeAgentDocument(t, workspace, `  agent:
    implementation: coding
    model:
      provider: ollama
      name: gemma
    bash_permissions:
      allow_patterns:
        - "git *"
      default: allow
`)

	if diags := (agentsCheck{}).Run(workspace); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}

func TestAgentsCheckSilentWithoutRules(t *testing.T) {
	workspace := t.TempDir()
	writeAgentDocument(t, workspace, `  agent:
    implementation: coding
    model:
      provider: ollama
      name: gemma
    bash_permissions:
      default: ask
`)

	if diags := (agentsCheck{}).Run(workspace); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}
