package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// TestEnvironmentConstruction validates that a test environment can be created and torn down.
func TestEnvironmentConstruction(t *testing.T) {
	env := NewTestEnvironment(t)

	if env.WorkspacePath == "" {
		t.Error("workspace path not set")
	}
	if env.ManifestRoot == "" {
		t.Error("manifest root not set")
	}
	if env.PermissionManager == nil {
		t.Error("permission manager not set")
	}
	if env.Registry == nil {
		t.Error("registry not set")
	}
	if env.TelemetrySink == nil {
		t.Error("telemetry sink not set")
	}
	if env.AuditSink == nil {
		t.Error("audit sink not set")
	}

	if _, err := os.Stat(env.WorkspacePath); err != nil {
		t.Errorf("workspace directory does not exist: %v", err)
	}

	if _, err := os.Stat(env.ManifestRoot); err != nil {
		t.Errorf("manifest root directory does not exist: %v", err)
	}

	probe := filepath.Join(env.WorkspacePath, "construction-probe.txt")
	if err := os.WriteFile(probe, []byte("probe"), 0o644); err != nil {
		t.Fatalf("failed to create cleanup probe: %v", err)
	}

	env.Cleanup()
	env.Cleanup()

	if _, err := os.Stat(env.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace directory still exists after cleanup: %v", err)
	}
}

// TestEnvironmentTeardownOnFailure validates that cleanup runs even when a test fails.
func TestEnvironmentTeardownOnFailure(t *testing.T) {
	env := NewTestEnvironment(t)

	probe := filepath.Join(env.WorkspacePath, "teardown-probe.txt")
	if err := os.WriteFile(probe, []byte("teardown"), 0o644); err != nil {
		t.Fatalf("failed to create teardown probe: %v", err)
	}

	if len(env.TelemetrySink.Events()) != 0 {
		t.Fatal("expected empty telemetry sink before cleanup")
	}
	if len(env.AuditSink.Records()) != 0 {
		t.Fatal("expected empty audit sink before cleanup")
	}

	env.TelemetrySink.Emit(core.Event{Type: core.EventNodeFinish, NodeID: "teardown-node", TaskID: "teardown-task"})
	if err := env.AuditSink.Log(context.Background(), core.AuditRecord{AgentID: "teardown-agent", Action: "teardown-action"}); err != nil {
		t.Fatalf("failed to write audit record: %v", err)
	}

	env.Cleanup()

	if _, err := os.Stat(env.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace directory still exists after cleanup: %v", err)
	}
	if got := env.TelemetrySink.Events(); len(got) != 0 {
		t.Fatalf("expected telemetry sink to be cleared, got %d events", len(got))
	}
	if got := env.AuditSink.Records(); len(got) != 0 {
		t.Fatalf("expected audit sink to be cleared, got %d records", len(got))
	}

	// The t.Cleanup registration should be a no-op now that Cleanup already ran.
}

// TestEnvironmentIsolation validates that each test gets its own isolated environment.
func TestEnvironmentIsolation(t *testing.T) {
	env1 := NewTestEnvironment(t)
	env2 := NewTestEnvironment(t)

	// Verify each environment has its own workspace
	if env1.WorkspacePath == env2.WorkspacePath {
		t.Error("environments have the same workspace path")
	}
	if env1.ManifestRoot == env2.ManifestRoot {
		t.Error("environments have the same manifest root")
	}

	// Verify each environment has its own permission manager
	if env1.PermissionManager == env2.PermissionManager {
		t.Error("environments share the same permission manager")
	}

	// Verify each environment has its own registry
	if env1.Registry == env2.Registry {
		t.Error("environments share the same registry")
	}

	// Verify each environment has its own telemetry sink
	if env1.TelemetrySink == env2.TelemetrySink {
		t.Error("environments share the same telemetry sink")
	}

	// Verify each environment has its own audit sink
	if env1.AuditSink == env2.AuditSink {
		t.Error("environments share the same audit sink")
	}
}
