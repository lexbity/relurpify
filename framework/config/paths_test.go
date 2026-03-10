package config

import (
	"path/filepath"
	"testing"
)

func TestPathsLayout(t *testing.T) {
	root := filepath.Join("/tmp", "proj")
	paths := New(root)
	if got := paths.ConfigRoot(); got != filepath.Join(root, DirName) {
		t.Fatalf("ConfigRoot() = %q", got)
	}
	if got := paths.ManifestFile(); got != filepath.Join(root, DirName, "agent.manifest.yaml") {
		t.Fatalf("ManifestFile() = %q", got)
	}
	if got := paths.NexusConfigFile(); got != filepath.Join(root, DirName, "nexus.yaml") {
		t.Fatalf("NexusConfigFile() = %q", got)
	}
	if got := paths.TelemetryFile(""); got != filepath.Join(root, DirName, "telemetry", "telemetry.jsonl") {
		t.Fatalf("TelemetryFile() = %q", got)
	}
	if got := paths.WorkflowStateFile(); got != filepath.Join(root, DirName, "sessions", "workflow_state.db") {
		t.Fatalf("WorkflowStateFile() = %q", got)
	}
	if got := paths.IdentityStoreFile(); got != filepath.Join(root, DirName, "identities.db") {
		t.Fatalf("IdentityStoreFile() = %q", got)
	}
	if got := paths.AdminTokenStoreFile(); got != filepath.Join(root, DirName, "admin_tokens.db") {
		t.Fatalf("AdminTokenStoreFile() = %q", got)
	}
	if got := paths.PolicyRulesFile(); got != filepath.Join(root, DirName, "policy_rules.yaml") {
		t.Fatalf("PolicyRulesFile() = %q", got)
	}
	if got := paths.TestRunDir("coding", "run-1"); got != filepath.Join(root, DirName, "test_runs", "coding", "run-1") {
		t.Fatalf("TestRunDir() = %q", got)
	}
	if got := paths.TestRunLogsDir("coding", "run-1"); got != filepath.Join(root, DirName, "test_runs", "coding", "run-1", "logs") {
		t.Fatalf("TestRunLogsDir() = %q", got)
	}
	if got := paths.TestRunTelemetryDir("coding", "run-1"); got != filepath.Join(root, DirName, "test_runs", "coding", "run-1", "telemetry") {
		t.Fatalf("TestRunTelemetryDir() = %q", got)
	}
	if got := paths.TestRunArtifactsDir("coding", "run-1"); got != filepath.Join(root, DirName, "test_runs", "coding", "run-1", "artifacts") {
		t.Fatalf("TestRunArtifactsDir() = %q", got)
	}
	if got := paths.TestRunTmpDir("coding", "run-1"); got != filepath.Join(root, DirName, "test_runs", "coding", "run-1", "tmp") {
		t.Fatalf("TestRunTmpDir() = %q", got)
	}
}
