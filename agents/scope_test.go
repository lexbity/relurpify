package agents

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type scopeTestWriteTool struct{ name string }

func (t scopeTestWriteTool) Name() string        { return t.name }
func (t scopeTestWriteTool) Description() string { return "write test tool" }
func (t scopeTestWriteTool) Category() string    { return "testing" }
func (t scopeTestWriteTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t scopeTestWriteTool) Execute(_ context.Context, _ *core.Context, _ map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t scopeTestWriteTool) IsAvailable(_ context.Context, _ *core.Context) bool { return true }
func (t scopeTestWriteTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "."}},
	}}
}
func (t scopeTestWriteTool) Tags() []string { return nil }

var docPathGlobs = []string{"**/*.md", "**/*.rst", "**/*.txt", "docs/**", "README*", "CHANGELOG*", "CONTRIBUTING*"}

func TestScopeRegistryDocPathsRejectsNonDocWrites(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(scopeTestWriteTool{name: "writer"}))

	scoped := ScopeRegistry(registry, ToolScope{AllowRead: true, AllowWrite: true, WritePathGlobs: docPathGlobs})
	_, err := scoped.InvokeCapability(context.Background(), core.NewContext(), "tool:writer", map[string]interface{}{"path": "main.go"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `write to "main.go" blocked`)
}

func TestScopeRegistryDocPathsAllowsDocWrites(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(scopeTestWriteTool{name: "writer"}))

	scoped := ScopeRegistry(registry, ToolScope{AllowRead: true, AllowWrite: true, WritePathGlobs: docPathGlobs})
	result, err := scoped.InvokeCapability(context.Background(), core.NewContext(), "tool:writer", map[string]interface{}{"path": "docs/api.md"})
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestScopeRegistryNoGlobsAllowsAllWrites(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(scopeTestWriteTool{name: "writer"}))

	scoped := ScopeRegistry(registry, ToolScope{AllowRead: true, AllowWrite: true})
	result, err := scoped.InvokeCapability(context.Background(), core.NewContext(), "tool:writer", map[string]interface{}{"path": "main.go"})
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestScopeRegistryWriteToolDroppedWhenWriteDisallowed(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(scopeTestWriteTool{name: "writer"}))

	scoped := ScopeRegistry(registry, ToolScope{AllowRead: true, AllowWrite: false})
	_, err := scoped.InvokeCapability(context.Background(), core.NewContext(), "tool:writer", map[string]interface{}{"path": "main.go"})
	require.Error(t, err)
}
