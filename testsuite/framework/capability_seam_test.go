package framework

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TestCapabilityDiscovery validates that capability descriptors can be
// discovered and enumerated correctly.
func TestCapabilityDiscovery(t *testing.T) {
	t.Run("registry can be created", func(t *testing.T) {
		registry := capability.NewCapabilityRegistry()
		if registry == nil {
			t.Fatal("registry should not be nil")
		}
	})

	t.Run("registry starts empty", func(t *testing.T) {
		registry := capability.NewCapabilityRegistry()

		// Try to get a non-existent capability
		_, ok := registry.Get("non-existent")
		if ok {
			t.Error("getting non-existent capability should return false")
		}
	})

	t.Run("capability descriptors are stable", func(t *testing.T) {
		// Register a test capability
		registry := capability.NewCapabilityRegistry()
		tool := &testTool{
			name:        "test-tool",
			description: "test tool for discovery",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet("/test", contracts.FileSystemRead),
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Get the capability descriptor
		cap, ok := registry.Get("test-tool")
		if !ok {
			t.Fatal("failed to get capability")
		}

		// Verify descriptor is stable
		if cap.Name() != "test-tool" {
			t.Errorf("expected name 'test-tool', got %s", cap.Name())
		}
		if cap.Description() != "test tool for discovery" {
			t.Errorf("expected description 'test tool for discovery', got %s", cap.Description())
		}
		if cap.Category() != "test" {
			t.Errorf("expected category 'test', got %s", cap.Category())
		}
	})
}

// TestCapabilityRegistration validates that capabilities can be registered
// and their permission sets are enforced.
func TestCapabilityRegistration(t *testing.T) {
	t.Run("tool can be registered", func(t *testing.T) {
		registry := capability.NewCapabilityRegistry()
		tool := &testTool{
			name:        "registered-tool",
			description: "tool for registration test",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet("/test", contracts.FileSystemRead),
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Verify the tool can be retrieved
		_, ok := registry.Get("registered-tool")
		if !ok {
			t.Error("failed to get registered tool")
		}
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		registry := capability.NewCapabilityRegistry()
		tool := &testTool{
			name:        "duplicate-tool",
			description: "tool for duplicate test",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet("/test", contracts.FileSystemRead),
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Try to register the same tool again
		err := registry.Register(tool)
		if err == nil {
			t.Error("duplicate registration should fail")
		}
	})

	t.Run("permission set is preserved", func(t *testing.T) {
		registry := capability.NewCapabilityRegistry()
		perms := core.NewFileSystemPermissionSet("/test", contracts.FileSystemRead, contracts.FileSystemWrite)
		tool := &testTool{
			name:        "permission-tool",
			description: "tool for permission test",
			category:    "test",
			permissions: perms,
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Get the tool and verify permissions
		cap, ok := registry.Get("permission-tool")
		if !ok {
			t.Fatal("failed to get capability")
		}

		toolPerms := cap.Permissions()
		// ToolPermissions is a struct with a Permissions field
		// Normalize and compare permissions
		normalized := NormalizeFileSystemPermissions(toolPerms.Permissions.FileSystem)
		expected := NormalizeFileSystemPermissions(perms.FileSystem)

		if len(normalized) != len(expected) {
			t.Errorf("permission count mismatch: %d vs %d", len(normalized), len(expected))
		}
		for i := range normalized {
			if normalized[i].Action != expected[i].Action {
				t.Errorf("permission action mismatch at index %d", i)
			}
		}
	})
}

// TestInvocationGating validates that tool execution is gated by the
// permission manager.
func TestInvocationGating(t *testing.T) {
	t.Run("tool execution without permission manager", func(t *testing.T) {
		registry := capability.NewCapabilityRegistry()
		tool := &testTool{
			name:        "ungated-tool",
			description: "tool without permission manager",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet("/test", contracts.FileSystemRead),
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Execute the tool without permission manager
		cap, _ := registry.Get("ungated-tool")
		_, err := cap.Execute(context.Background(), nil)
		if err != nil {
			t.Errorf("tool should execute without permission manager, got error: %v", err)
		}
	})

	t.Run("tool execution with permission manager", func(t *testing.T) {
		env := NewTestEnvironment(t)

		registry := capability.NewCapabilityRegistry()

		// Create a permission manager
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		// Register permission manager with registry
		registry.UsePermissionManager("test-agent", manager)

		// Create a tool that checks permissions
		tool := &permissionedTestTool{
			name:        "permissioned-tool",
			description: "tool with permission checks",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead),
			manager:     manager,
			agent:       "test-agent",
			basePath:    env.WorkspacePath,
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Execute the tool with permission manager
		cap, _ := registry.Get("permissioned-tool")
		_, err = cap.Execute(context.Background(), nil)
		if err != nil {
			t.Errorf("tool should execute with valid permissions, got error: %v", err)
		}
	})
}

// TestToolPermissionEnforcement validates that tool execution is properly
// gated by permission checks.
func TestToolPermissionEnforcement(t *testing.T) {
	t.Run("tool execution denied without permission", func(t *testing.T) {
		env := NewTestEnvironment(t)

		registry := capability.NewCapabilityRegistry()

		// Create a permission manager with no permissions
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemWrite)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		registry.UsePermissionManager("test-agent", manager)

		// Create a tool that requires read permission (not granted)
		tool := &permissionedTestTool{
			name:        "denied-tool",
			description: "tool that should be denied",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead),
			manager:     manager,
			agent:       "test-agent",
			basePath:    env.WorkspacePath,
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Execute the tool - should be denied
		cap, _ := registry.Get("denied-tool")
		_, err = cap.Execute(context.Background(), nil)
		if err == nil {
			t.Error("tool execution should be denied without permission")
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for denied tool execution")
		}
		if len(records) > 0 && records[0].Result != "denied" {
			t.Errorf("expected audit result 'denied', got %s", records[0].Result)
		}
	})

	t.Run("tool execution allowed with permission", func(t *testing.T) {
		env := NewTestEnvironment(t)

		registry := capability.NewCapabilityRegistry()

		// Create a permission manager with read permission
		perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead)
		manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to create permission manager: %v", err)
		}

		registry.UsePermissionManager("test-agent", manager)

		// Create a tool that requires read permission (granted)
		tool := &permissionedTestTool{
			name:        "allowed-tool",
			description: "tool that should be allowed",
			category:    "test",
			permissions: core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead),
			manager:     manager,
			agent:       "test-agent",
			basePath:    env.WorkspacePath,
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("failed to register tool: %v", err)
		}

		// Execute the tool - should be allowed
		cap, _ := registry.Get("allowed-tool")
		_, err = cap.Execute(context.Background(), nil)
		if err != nil {
			t.Errorf("tool execution should be allowed with permission, got error: %v", err)
		}

		// Verify audit record was created
		records := env.AuditSink.Records()
		if len(records) == 0 {
			t.Error("expected audit record for allowed tool execution")
		}
		if len(records) > 0 && records[0].Result != "tool_allowed" {
			t.Errorf("expected audit result 'tool_allowed', got %s", records[0].Result)
		}
	})
}

// testTool is a simple tool implementation for testing.
type testTool struct {
	name        string
	description string
	category    string
	permissions *contracts.PermissionSet
	executed    bool
}

func (t *testTool) Name() string                          { return t.name }
func (t *testTool) Description() string                   { return t.description }
func (t *testTool) Category() string                      { return t.category }
func (t *testTool) Parameters() []contracts.ToolParameter { return nil }
func (t *testTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	t.executed = true
	return &contracts.ToolResult{Success: true}, nil
}
func (t *testTool) IsAvailable(context.Context) bool { return true }
func (t *testTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: t.permissions}
}
func (t *testTool) Tags() []string { return nil }

// permissionedTestTool is a tool that checks permissions before execution.
type permissionedTestTool struct {
	name        string
	description string
	category    string
	permissions *contracts.PermissionSet
	manager     *authorization.PermissionManager
	agent       string
	basePath    string
	executed    bool
}

func (t *permissionedTestTool) Name() string                          { return t.name }
func (t *permissionedTestTool) Description() string                   { return t.description }
func (t *permissionedTestTool) Category() string                      { return t.category }
func (t *permissionedTestTool) Parameters() []contracts.ToolParameter { return nil }
func (t *permissionedTestTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	// Check permissions before execution
	if t.manager != nil {
		testPath := t.basePath + "/test.txt"
		if err := t.manager.CheckFileAccess(ctx, t.agent, contracts.FileSystemRead, testPath); err != nil {
			return nil, err
		}
	}
	t.executed = true
	return &contracts.ToolResult{Success: true}, nil
}
func (t *permissionedTestTool) IsAvailable(context.Context) bool { return true }
func (t *permissionedTestTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: t.permissions}
}
func (t *permissionedTestTool) Tags() []string { return nil }
