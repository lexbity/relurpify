package config

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

func TestValidateWorkspaceTreeCleanRepo(t *testing.T) {
	report := ValidateWorkspaceTree(testhelper.RepoRoot(t))
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

func writeMinimalValidWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	testhelper.WriteValidWorkspace(t, ws)
	return ws
}
