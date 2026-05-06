package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedRunArtifactsDerivesSetupAndExecutionRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	runRoot := filepath.Join(root, "relurpify_cfg", "test_run", "run-1")
	artifacts := NewPreparedRunArtifacts(root, runRoot, "euclo", "run-1")

	if want := filepath.Join(runRoot, "setup"); artifacts.SetupDir != want {
		t.Fatalf("SetupDir = %q, want %q", artifacts.SetupDir, want)
	}
	if want := filepath.Join(runRoot, "execution"); artifacts.ExecutionDir != want {
		t.Fatalf("ExecutionDir = %q, want %q", artifacts.ExecutionDir, want)
	}
	if want := filepath.Join(runRoot, "setup", "prepared_run.json"); artifacts.DescriptorPath() != want {
		t.Fatalf("DescriptorPath = %q, want %q", artifacts.DescriptorPath(), want)
	}
	if want := filepath.Join(runRoot, "execution", "report.json"); artifacts.RunReportPath != want {
		t.Fatalf("RunReportPath = %q, want %q", artifacts.RunReportPath, want)
	}
	if want := filepath.Join(runRoot, "execution", "telemetry"); artifacts.ExecutionTelemetryDir != want {
		t.Fatalf("ExecutionTelemetryDir = %q, want %q", artifacts.ExecutionTelemetryDir, want)
	}
}

func TestPreparedRunArtifactsEnsureCreatesRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	runRoot := filepath.Join(root, "relurpify_cfg", "test_run", "run-1")
	artifacts := NewPreparedRunArtifacts(root, runRoot, "euclo", "run-1")
	if err := artifacts.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for _, dir := range []string{
		artifacts.SetupDir,
		artifacts.SetupWorkspaceDir,
		artifacts.SetupLogsDir,
		artifacts.SetupTelemetryDir,
		artifacts.ExecutionDir,
		artifacts.ExecutionLogsDir,
		artifacts.ExecutionTelemetryDir,
		artifacts.VerificationDir,
	} {
		if info, err := os.Stat(dir); err != nil {
			t.Fatalf("Stat %q: %v", dir, err)
		} else if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
	}
}
