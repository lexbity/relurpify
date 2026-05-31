package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolsCheckCleanRepo(t *testing.T) {
	c := toolsCheck{}
	diags := c.Run(repoRoot())
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for clean repo, got %d: %+v", len(diags), diags)
	}
}

func TestToolsCheckUnderdeclaredManifest(t *testing.T) {
	workspace := writeValidWorkspace(t)
	addUnderdeclaredManifest(t, workspace)

	c := toolsCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected SEC-2 diagnostics for underdeclared manifest, got none")
	}
	for _, d := range diags {
		if d.Check != "tools" {
			t.Fatalf("expected Check=tools, got %q", d.Check)
		}
		if d.Code != "tool.underdeclared" {
			t.Fatalf("expected Code=tool.underdeclared, got %q", d.Code)
		}
		if d.Severity != SeverityError {
			t.Fatalf("expected Severity=error, got %v", d.Severity)
		}
		if d.Message == "" {
			t.Fatal("expected non-empty message")
		}
	}
	hasNetwork := false
	hasEgress := false
	hasExternal := false
	for _, d := range diags {
		if strings.Contains(d.Message, "network") {
			hasNetwork = true
		}
		if strings.Contains(d.Message, "network_egress") {
			hasEgress = true
		}
		if strings.Contains(d.Message, "external_state") {
			hasExternal = true
		}
	}
	if !hasNetwork {
		t.Fatalf("expected 'network' risk to be flagged, got: %+v", diags)
	}
	if !hasEgress {
		t.Fatalf("expected 'network_egress' effect to be flagged, got: %+v", diags)
	}
	if !hasExternal {
		t.Fatalf("expected 'external_state' effect to be flagged, got: %+v", diags)
	}
}

func TestToolsCheckSchemaError(t *testing.T) {
	workspace := writeValidWorkspace(t)
	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools")
	os.WriteFile(filepath.Join(toolsDir, "broken.tool.yaml"), []byte("schema: relurpify/tool/v1\nname: broken\n"), 0o644)

	c := toolsCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for broken tool manifest, got none")
	}
	hasSchemaError := false
	for _, d := range diags {
		if d.Code == "tool.schema" {
			hasSchemaError = true
			break
		}
	}
	if !hasSchemaError {
		t.Fatalf("expected tool.schema diagnostic, got: %+v", diags)
	}
}

func TestToolsCheckExcludesConfigIssues(t *testing.T) {
	workspace := writeValidWorkspace(t)
	// Corrupt a config file (not a tool)
	policyPath := filepath.Join(workspace, "relurpify_cfg", "security", "sandbox.policy.yaml")
	os.WriteFile(policyPath, []byte("schema: relurpify/policy/sandbox/v1\n\nprotected_paths: [invalid\n"), 0o644)

	c := toolsCheck{}
	diags := c.Run(workspace)
	for _, d := range diags {
		if d.Check == "config" {
			t.Fatalf("tools check should not include config issues, got: %+v", d)
		}
	}
}

func addUnderdeclaredManifest(t *testing.T, workspace string) {
	t.Helper()
	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools")
	content := `schema: relurpify/tool/v1
name: cli_curl
version: "1"
family: network
description: "Test curl with underdeclared capabilities"
execution:
  backend: subprocess
  command:
    base: [curl]
  sandbox:
    network_access: true
    timeout_seconds: 30
  allow_stdin: true
  supports_workdir: true
capability:
  trust_class: builtin_trusted
  risk_class:
    - execute
  effect_class:
    - process_spawn
`
	mustWrite(t, filepath.Join(toolsDir, "curl.tool.yaml"), content)
}
