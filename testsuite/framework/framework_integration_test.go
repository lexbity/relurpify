package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/fs"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
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
	if err := fs.WriteFileSecure(testFile, []byte("smoke test")); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx := context.Background()
	if err := env.PermissionManager.CheckFileAccess(ctx, "smoke-agent", permissions.FileSystemRead, testFile); err != nil {
		t.Fatalf("permission check failed: %v", err)
	}

	if err := env.AuditSink.Log(ctx, policy.AuditRecord{AgentID: "smoke-agent", Action: "smoke-action", Type: "filesystem", Permission: testFile, Result: "granted"}); err != nil {
		t.Fatalf("audit logging failed: %v", err)
	}
	env.TelemetrySink.Emit(telemetry.Event{Type: telemetry.EventNodeFinish, NodeID: "smoke-node", TaskID: "smoke-task", Message: "smoke test completed"})

	if got := env.AuditSink.Records(); len(got) == 0 {
		t.Fatal("expected audit records to be captured")
	}
	if got := env.TelemetrySink.Events(); len(got) == 0 {
		t.Fatal("expected telemetry events to be captured")
	}

	registryTool := &smokeTool{name: "smoke-tool"}
	if err := env.Registry.Register(context.Background(), registryTool); err != nil {
		t.Fatalf("tool registration failed: %v", err)
	}
	if _, ok := env.Registry.Get("smoke-tool"); !ok {
		t.Fatal("registered tool not found in registry")
	}

	envelope := contextdata.NewEnvelope("smoke-task", "smoke-session")
	envelope.SetWorkingValueWithClass("smoke-key", "smoke-value", contextdata.MemoryClassTask)
	if value, ok := contextdata.GetTyped[string](envelope, "smoke-key"); !ok || value != "smoke-value" {
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
	if err := fs.WriteFileSecure(probe, []byte("probe")); err != nil {
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
func (s *smokeTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "input", Type: "string", Description: "input parameter"},
	}
}

func (s *smokeTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"result": "smoke",
		},
	}, nil
}

func (s *smokeTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}

func (s *smokeTool) Tags() []string {
	return []string{toolcapabilities.TagReadOnly}
}

func (s *smokeTool) IsAvailable(context.Context) bool { return true }
