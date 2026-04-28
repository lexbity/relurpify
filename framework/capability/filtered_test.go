package capability

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// mockTool is a minimal tool implementation for testing.
type mockTool struct {
	name string
	id   string
}

func (m *mockTool) Name() string                     { return m.name }
func (m *mockTool) Description() string              { return "mock tool" }
func (m *mockTool) Category() string                 { return "test" }
func (m *mockTool) Parameters() []core.ToolParameter { return nil }
func (m *mockTool) Execute(ctx context.Context, state *contracts.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (m *mockTool) IsAvailable(ctx context.Context, state *contracts.Context) bool { return true }
func (m *mockTool) Permissions() core.ToolPermissions                              { return core.ToolPermissions{} }
func (m *mockTool) Tags() []string                                                 { return nil }

// setupTestRegistry creates a registry with some mock tools for testing.
func setupTestRegistry(t *testing.T) *CapabilityRegistry {
	reg := NewCapabilityRegistry()

	// Register some mock tools using the proper registration API
	// Capability IDs will be "tool:" + name (e.g., "tool:file_read")
	toolNames := []string{"file_read", "file_write", "go_test", "shell_exec"}

	for _, name := range toolNames {
		tool := &mockTool{name: name}
		if err := reg.Register(tool); err != nil {
			t.Fatalf("failed to register tool %s: %v", name, err)
		}
	}

	return reg
}

func TestFilteredRegistryPassthrough(t *testing.T) {
	base := setupTestRegistry(t)

	// nil allowed list = passthrough
	f := NewFilteredRegistry(base, nil)
	if !f.IsPassthrough() {
		t.Error("expected passthrough for nil allowed list")
	}

	// empty allowed list = passthrough
	f = NewFilteredRegistry(base, []string{})
	if !f.IsPassthrough() {
		t.Error("expected passthrough for empty allowed list")
	}

	// Should be able to get all tools
	tools := f.ModelCallableTools()
	if len(tools) != 4 {
		t.Errorf("expected 4 tools in passthrough, got %d", len(tools))
	}
}

func TestFilteredRegistryAllowedGet(t *testing.T) {
	base := setupTestRegistry(t)

	f := NewFilteredRegistry(base, []string{"tool:file_read", "tool:go_test"})

	// Allowed tool should be retrievable
	tool, ok := f.Get("file_read")
	if !ok {
		t.Error("expected to get file_read tool")
	}
	if tool == nil {
		t.Error("expected non-nil tool")
	}

	// Another allowed tool
	tool, ok = f.Get("go_test")
	if !ok {
		t.Error("expected to get go_test tool")
	}
	if tool == nil {
		t.Error("expected non-nil tool")
	}
}

func TestFilteredRegistryDeniedGet(t *testing.T) {
	base := setupTestRegistry(t)

	f := NewFilteredRegistry(base, []string{"tool:file_read", "tool:go_test"})

	// Denied tool should not be retrievable
	tool, ok := f.Get("file_write")
	if ok {
		t.Error("expected not to get file_write tool (denied)")
	}
	if tool != nil {
		t.Error("expected nil tool for denied capability")
	}

	// Another denied tool
	tool, ok = f.Get("shell_exec")
	if ok {
		t.Error("expected not to get shell_exec tool (denied)")
	}
	if tool != nil {
		t.Error("expected nil tool for denied capability")
	}
}

func TestFilteredRegistryModelCallableTools(t *testing.T) {
	base := setupTestRegistry(t)

	f := NewFilteredRegistry(base, []string{"tool:file_read", "tool:go_test"})

	tools := f.ModelCallableTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 callable tools, got %d", len(tools))
	}

	// Verify the right tools are present
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	if !names["file_read"] {
		t.Error("expected file_read in callable tools")
	}
	if !names["go_test"] {
		t.Error("expected go_test in callable tools")
	}
	if names["file_write"] {
		t.Error("did not expect file_write in callable tools")
	}
}

func TestFilteredRegistryInvokeAllowed(t *testing.T) {
	// This test verifies the interface - actual invocation would need more setup
	base := setupTestRegistry(t)
	f := NewFilteredRegistry(base, []string{"tool:file_read"})

	// Verify the capability is considered allowed
	if !f.IsAllowed("tool:file_read") {
		t.Error("expected tool:file_read to be allowed")
	}
}

func TestFilteredRegistryInvokeDenied(t *testing.T) {
	base := setupTestRegistry(t)
	f := NewFilteredRegistry(base, []string{"tool:file_read"})

	// Verify the capability is considered denied
	if f.IsAllowed("tool:shell_exec") {
		t.Error("expected tool:shell_exec to be denied")
	}
}

func TestFilteredRegistryIntersect(t *testing.T) {
	base := setupTestRegistry(t)

	// Start with file_read and file_write allowed
	f := NewFilteredRegistry(base, []string{"tool:file_read", "tool:file_write"})

	// Intersect with file_read and go_test - should result in only file_read
	intersected := f.Intersect([]string{"tool:file_read", "tool:go_test"})

	allowed := intersected.AllowedIDs()
	if len(allowed) != 1 {
		t.Errorf("expected 1 allowed ID after intersect, got %d: %v", len(allowed), allowed)
	}
	if len(allowed) > 0 && allowed[0] != "tool:file_read" {
		t.Errorf("expected tool:file_read, got %s", allowed[0])
	}
}

func TestFilteredRegistryIntersectExpand(t *testing.T) {
	// Intersect cannot expand beyond current allowed set
	base := setupTestRegistry(t)

	// Start with only file_read allowed
	f := NewFilteredRegistry(base, []string{"tool:file_read"})

	// Try to intersect with file_read, file_write, go_test
	// Should still only have file_read (intersection, not union)
	intersected := f.Intersect([]string{"tool:file_read", "tool:file_write", "tool:go_test"})

	allowed := intersected.AllowedIDs()
	if len(allowed) != 1 {
		t.Errorf("expected 1 allowed ID, got %d: %v", len(allowed), allowed)
	}
	if len(allowed) > 0 && allowed[0] != "tool:file_read" {
		t.Errorf("expected tool:file_read, got %s", allowed[0])
	}
}

func TestFilteredRegistryIsPassthrough(t *testing.T) {
	base := setupTestRegistry(t)

	// nil allowed = passthrough
	f1 := NewFilteredRegistry(base, nil)
	if !f1.IsPassthrough() {
		t.Error("expected nil allowed to be passthrough")
	}

	// empty allowed = passthrough
	f2 := NewFilteredRegistry(base, []string{})
	if !f2.IsPassthrough() {
		t.Error("expected empty allowed to be passthrough")
	}

	// non-empty allowed = not passthrough
	f3 := NewFilteredRegistry(base, []string{"tool:file_read"})
	if f3.IsPassthrough() {
		t.Error("expected non-empty allowed to not be passthrough")
	}

	// nil receiver = passthrough
	var f4 *FilteredRegistry
	if !f4.IsPassthrough() {
		t.Error("expected nil receiver to be passthrough")
	}
}

func TestFilteredRegistryAllowedIDs(t *testing.T) {
	base := setupTestRegistry(t)

	f := NewFilteredRegistry(base, []string{"tool:file_read", "tool:go_test", "tool:file_write"})

	ids := f.AllowedIDs()
	if len(ids) != 3 {
		t.Errorf("expected 3 allowed IDs, got %d", len(ids))
	}

	// Should be sorted
	expected := []string{"tool:file_read", "tool:file_write", "tool:go_test"}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, id)
		}
	}
}

func TestFilteredRegistryAllowedIDsNilWhenPassthrough(t *testing.T) {
	base := setupTestRegistry(t)

	f := NewFilteredRegistry(base, nil)
	ids := f.AllowedIDs()
	if ids != nil {
		t.Errorf("expected nil for passthrough, got %v", ids)
	}
}

func TestFilteredRegistryNilBase(t *testing.T) {
	// Should handle nil base gracefully
	f := NewFilteredRegistry(nil, []string{"tool:file_read"})

	_, ok := f.Get("file_read")
	if ok {
		t.Error("expected Get to fail with nil base")
	}

	tools := f.ModelCallableTools()
	if tools != nil {
		t.Errorf("expected nil tools with nil base, got %v", tools)
	}
}

func TestFilteredRegistryNilReceiver(t *testing.T) {
	var f *FilteredRegistry

	// All methods should handle nil receiver gracefully
	if !f.IsPassthrough() {
		t.Error("expected nil receiver IsPassthrough to be true")
	}

	if ids := f.AllowedIDs(); ids != nil {
		t.Errorf("expected nil AllowedIDs, got %v", ids)
	}

	if !f.IsAllowed("anything") {
		t.Error("expected nil receiver IsAllowed to be true (passthrough)")
	}

	tool, ok := f.Get("anything")
	if ok || tool != nil {
		t.Error("expected nil receiver Get to return (nil, false)")
	}

	tools := f.ModelCallableTools()
	if tools != nil {
		t.Errorf("expected nil ModelCallableTools, got %v", tools)
	}
}

func TestFilteredRegistryIntersectNilReceiver(t *testing.T) {
	var f *FilteredRegistry

	// Intersect on nil receiver should create new registry with given IDs
	intersected := f.Intersect([]string{"tool:file_read"})
	if intersected == nil {
		t.Fatal("expected non-nil intersected registry")
	}

	ids := intersected.AllowedIDs()
	if len(ids) != 1 || ids[0] != "tool:file_read" {
		t.Errorf("expected [tool:file_read], got %v", ids)
	}
}

func TestFilteredRegistryIntersectPassthrough(t *testing.T) {
	base := setupTestRegistry(t)

	// Passthrough intersected with restriction should apply the restriction
	f := NewFilteredRegistry(base, nil)
	intersected := f.Intersect([]string{"tool:file_read"})

	if !intersected.IsAllowed("tool:file_read") {
		t.Error("expected tool:file_read to be allowed after intersect")
	}
	if intersected.IsAllowed("tool:file_write") {
		t.Error("expected tool:file_write to be denied after intersect")
	}
}

func TestFilteredRegistryEmptyIntersection(t *testing.T) {
	base := setupTestRegistry(t)

	// Non-overlapping restrictions should result in empty allowed set
	f := NewFilteredRegistry(base, []string{"tool:file_read"})
	intersected := f.Intersect([]string{"tool:go_test"})

	ids := intersected.AllowedIDs()
	if len(ids) != 0 {
		t.Errorf("expected empty intersection, got %v", ids)
	}
}

func TestFilteredRegistryWithEmptyStrings(t *testing.T) {
	base := setupTestRegistry(t)

	// Empty strings in allowed list should be ignored
	f := NewFilteredRegistry(base, []string{"tool:file_read", "", "tool:go_test", ""})

	ids := f.AllowedIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 allowed IDs (empty strings ignored), got %d: %v", len(ids), ids)
	}
}

func TestFilteredRegistryAllEmptyStrings(t *testing.T) {
	base := setupTestRegistry(t)

	// All empty strings should result in passthrough
	f := NewFilteredRegistry(base, []string{"", "", ""})

	if !f.IsPassthrough() {
		t.Error("expected passthrough when all allowed IDs are empty")
	}
}

func TestFilteredRegistryDuplicateIDs(t *testing.T) {
	base := setupTestRegistry(t)

	// Duplicate IDs should be deduplicated
	f := NewFilteredRegistry(base, []string{"tool:file_read", "tool:file_read", "tool:go_test"})

	ids := f.AllowedIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 unique allowed IDs, got %d: %v", len(ids), ids)
	}
}
