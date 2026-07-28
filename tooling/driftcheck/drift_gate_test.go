// Tests for the config-tree drift gate (check-config-tree-drift). They verify
// the gate passes when relurpify_cfg/ is in sync with the embedded templates
// and fails on an intentional modification.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/templates"
)

func TestDriftGatePassesWhenInSync(t *testing.T) {
	tmpDir := t.TempDir()
	if err := templates.GenerateConfig(tmpDir); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	cmd := exec.Command("diff", "-qr", tmpDir, "relurpify_cfg")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected no drift, but diff failed:\n%s", string(out))
	}
}

func TestDriftGateFailsOnIntentionalDiff(t *testing.T) {
	tmpDir := t.TempDir()
	if err := templates.GenerateConfig(tmpDir); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	backupPath := filepath.Join(tmpDir, "workspace.yaml.bak")
	targetPath := filepath.Join(tmpDir, "workspace.yaml")
	data, err := os.ReadFile(filepath.Clean(targetPath)) //nolint:gosec // test reads generated output under temp dir
	if err != nil {
		t.Fatalf("read workspace.yaml: %v", err)
	}
	if err := os.WriteFile(backupPath, data, 0644); err != nil { //nolint:gosec // test fixture backup under temp dir
		t.Fatalf("backup workspace.yaml: %v", err)
	}

	if err := os.WriteFile(targetPath, append(data, "\n# drift\n"...), 0644); err != nil { //nolint:gosec // test fixture intentionally drifted under temp dir
		t.Fatalf("modify workspace.yaml: %v", err)
	}
	defer func() {
		_ = os.WriteFile(targetPath, data, 0644) //nolint:gosec // test fixture restore under temp dir
		_ = os.Remove(backupPath)
	}()

	cmd := exec.Command("diff", "-qr", tmpDir, "relurpify_cfg")
	cmd.Dir = repoRoot(t)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected drift gate to detect intentional modification, but diff passed")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}
