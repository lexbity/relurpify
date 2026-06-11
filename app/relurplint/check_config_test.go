package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

func TestConfigCheckCleanRepo(t *testing.T) {
	c := configCheck{}
	diags := c.Run(testRepoRoot)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for clean repo, got %d: %+v", len(diags), diags)
	}
}

func TestConfigCheckBadPolicy(t *testing.T) {
	workspace := writeValidWorkspace(t)
	policyPath := filepath.Join(workspace, "relurpify_cfg", "security", "sandbox.policy.yaml")
	os.WriteFile(policyPath, []byte("schema: relurpify/policy/sandbox/v1\n\nprotected_paths: [invalid\n"), fs.PublicFileMode) // public: test fixture

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
	os.WriteFile(filepath.Join(toolsDir, "broken.tool.yaml"), []byte("schema: relurpify/tool/v1\ninvalid: true\n"), fs.PublicFileMode) // public: test fixture

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
	ws := t.TempDir()
	testhelper.WriteValidWorkspace(t, ws)
	return ws
}
