package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestBuildDoctorReport_ReportsBuiltinContractSource(t *testing.T) {
	workspace := t.TempDir()
	// Create minimal relurpify_cfg with security policies.
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	securityDir := filepath.Join(cfgRoot, "security")
	if err := os.MkdirAll(securityDir, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "localtool.policy.yaml"), []byte("schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: allow\n"), 0o600); err != nil {
		t.Fatalf("write localtool: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "shell.policy.yaml"), []byte("schema: relurpify/policy/shell/v1\nrules: []\n"), 0o600); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "sandbox.policy.yaml"), []byte("schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n"), 0o600); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}

	// Set ManifestPath to non-existent file.
	cfg := Config{
		Workspace:    workspace,
		ManifestPath: filepath.Join(workspace, "relurpify_cfg", "agents", "euclo.yaml"),
	}

	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	if report.ContractSource != "builtin+split" {
		t.Fatalf("ContractSource = %q, want %q", report.ContractSource, "builtin+split")
	}
	if report.ManifestFingerprint == "" {
		t.Fatal("ManifestFingerprint should not be empty for built-in contract")
	}
	if report.ManifestExists {
		t.Fatal("ManifestExists should be false when agents/ has no file")
	}
}

func TestFormatSandboxDetail_EmptyShowsFailClosedMessage(t *testing.T) {
	got := formatSandboxDetail("")
	if !strings.Contains(got, "FAIL TO START") {
		t.Errorf("expected FAIL TO START message for empty detail, got: %q", got)
	}
	if strings.Contains(got, "tool sandboxing disabled") {
		t.Errorf("should not contain old 'tool sandboxing disabled', got: %q", got)
	}
}

func TestFormatSandboxDetail_ErrorMessageShowsFailClosed(t *testing.T) {
	got := formatSandboxDetail("runsc not found")
	if !strings.Contains(got, "FAIL TO START") {
		t.Errorf("expected FAIL TO START message for error detail, got: %q", got)
	}
	if !strings.Contains(got, "runsc not found") {
		t.Errorf("expected original error preserved, got: %q", got)
	}
}

func TestFormatSandboxDetail_VersionStringPassesThrough(t *testing.T) {
	got := formatSandboxDetail("runsc version 1.2.3")
	if got != "runsc version 1.2.3" {
		t.Errorf("version string should pass through unchanged, got: %q", got)
	}
}

func TestFormatSandboxDetail_DockerErrorShowsFailClosed(t *testing.T) {
	got := formatSandboxDetail("error: docker daemon not running")
	if !strings.Contains(got, "FAIL TO START") {
		t.Errorf("expected FAIL TO START for docker error, got: %q", got)
	}
	if !strings.Contains(got, "docker daemon not running") {
		t.Errorf("expected original error preserved, got: %q", got)
	}
}
