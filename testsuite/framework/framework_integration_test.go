package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TestFrameworkSuiteSmoke validates that the framework suite can construct real runtime
// objects, exercise cross-seam behavior, and clean up deterministically.
func TestFrameworkSuiteSmoke(t *testing.T) {
	env := NewTestEnvironment(t)
	if env.WorkspacePath == "" {
		t.Error("workspace path not initialized")
	}
	if env.ManifestRoot == "" {
		t.Error("manifest root not initialized")
	}
	if env.PermissionManager == nil {
		t.Error("permission manager not initialized")
	}
	if env.Registry == nil {
		t.Error("registry not initialized")
	}
	if env.TelemetrySink == nil {
		t.Error("telemetry sink not initialized")
	}
	if env.AuditSink == nil {
		t.Error("audit sink not initialized")
	}

	testFile := filepath.Join(env.WorkspacePath, "smoke-test.txt")
	if err := os.WriteFile(testFile, []byte("smoke test"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx := context.Background()
	if err := env.PermissionManager.CheckFileAccess(ctx, "smoke-agent", contracts.FileSystemRead, testFile); err != nil {
		t.Fatalf("permission check failed: %v", err)
	}

	if err := env.AuditSink.Log(ctx, core.AuditRecord{AgentID: "smoke-agent", Action: "smoke-action", Type: "filesystem", Permission: testFile, Result: "granted"}); err != nil {
		t.Fatalf("audit logging failed: %v", err)
	}
	env.TelemetrySink.Emit(core.Event{Type: core.EventNodeFinish, NodeID: "smoke-node", TaskID: "smoke-task", Message: "smoke test completed"})

	if got := env.AuditSink.Records(); len(got) == 0 {
		t.Fatal("expected audit records to be captured")
	}
	if got := env.TelemetrySink.Events(); len(got) == 0 {
		t.Fatal("expected telemetry events to be captured")
	}

	registryTool := &smokeTool{name: "smoke-tool"}
	if err := env.Registry.Register(registryTool); err != nil {
		t.Fatalf("tool registration failed: %v", err)
	}
	if _, ok := env.Registry.Get("smoke-tool"); !ok {
		t.Fatal("registered tool not found in registry")
	}

	envelope := contextdata.NewEnvelope("smoke-task", "smoke-session")
	envelope.SetWorkingValue("smoke-key", "smoke-value", contextdata.MemoryClassTask)
	if value, ok := envelope.GetWorkingValue("smoke-key"); !ok || value != "smoke-value" {
		t.Fatal("envelope state manipulation failed")
	}

	env.Cleanup()
	env.Cleanup()
	if _, err := os.Stat(env.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace directory still exists after cleanup: %v", err)
	}
}

// TestFrameworkPackageDiscovery validates that the framework suite is discoverable
// from the repository root and that the harness owns a usable runtime boundary.
func TestFrameworkPackageDiscovery(t *testing.T) {
	env := NewTestEnvironment(t)
	if env.WorkspacePath == "" {
		t.Error("workspace path should be set")
	}
	if env.ManifestRoot == "" {
		t.Error("manifest root should be set")
	}
	probe := filepath.Join(env.ManifestRoot, "discovery-probe.txt")
	if err := os.WriteFile(probe, []byte("probe"), 0o644); err != nil {
		t.Fatalf("failed to write manifest probe: %v", err)
	}
	if _, err := os.Stat(probe); err != nil {
		t.Fatalf("manifest probe should exist during test: %v", err)
	}
	env.Cleanup()
	if _, err := os.Stat(env.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace directory still exists after cleanup: %v", err)
	}
}

// smokeTool is a minimal tool implementation for smoke testing.
type smokeTool struct {
	name string
}

func (s *smokeTool) Name() string        { return s.name }
func (s *smokeTool) Description() string { return "smoke test tool" }
func (s *smokeTool) Category() string    { return "test" }
func (s *smokeTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "input", Type: "string", Description: "input parameter"},
	}
}

func (s *smokeTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"result": "smoke",
		},
	}, nil
}

func (s *smokeTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}

func (s *smokeTool) Tags() []string {
	return []string{contracts.TagReadOnly}
}

func (s *smokeTool) IsAvailable(context.Context) bool { return true }
