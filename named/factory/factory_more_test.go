package factory

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type factoryTool struct {
	name  string
	perms core.ToolPermissions
}

func (t factoryTool) Name() string                     { return t.name }
func (t factoryTool) Description() string              { return "stub" }
func (t factoryTool) Category() string                 { return "test" }
func (t factoryTool) Parameters() []core.ToolParameter { return nil }
func (t factoryTool) Execute(context.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t factoryTool) IsAvailable(context.Context) bool  { return true }
func (t factoryTool) Permissions() core.ToolPermissions { return t.perms }
func (t factoryTool) Tags() []string                    { return nil }

func registryPrecheckCount(t *testing.T, reg *capability.Registry) int {
	t.Helper()
	value := reflect.ValueOf(reg).Elem().FieldByName("prechecks")
	require.True(t, value.IsValid())
	return value.Len()
}

func TestHelpersAndScopeRegistry(t *testing.T) {
	var env ayenitd.WorkspaceEnvironment
	converted := envToWorkspace(env)
	require.Equal(t, ayenitd.WorkspaceEnvironment{}, converted)

	spec := ApplyManifestDefaults(nil)
	require.NotNil(t, spec)
	require.Empty(t, spec.Implementation)

	original := &core.AgentRuntimeSpec{Implementation: "react"}
	require.Same(t, original, ApplyManifestDefaults(original))

	base := capability.NewRegistry()
	require.NoError(t, base.Register(factoryTool{name: "plain"}))
	require.NoError(t, base.Register(factoryTool{
		name: "write",
		perms: core.ToolPermissions{Permissions: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "/tmp/**"}},
		}},
	}))
	require.NoError(t, base.Register(factoryTool{
		name: "exec",
		perms: core.ToolPermissions{Permissions: &core.PermissionSet{
			Executables: []core.ExecutablePermission{{Binary: "git"}},
		}},
	}))
	require.NoError(t, base.Register(factoryTool{
		name: "network",
		perms: core.ToolPermissions{Permissions: &core.PermissionSet{
			Network: []core.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443}},
		}},
	}))

	scoped := ScopeRegistry(base, ToolScope{})
	_, ok := scoped.Get("plain")
	require.True(t, ok)
	_, ok = scoped.Get("write")
	require.False(t, ok)
	_, ok = scoped.Get("exec")
	require.False(t, ok)
	_, ok = scoped.Get("network")
	require.False(t, ok)

	allowAll := ScopeRegistry(base, ToolScope{
		AllowWrite:     true,
		AllowExecute:   true,
		AllowNetwork:   true,
		WritePathGlobs: []string{"**/*.md"},
	})
	_, ok = allowAll.Get("write")
	require.True(t, ok)
	_, ok = allowAll.Get("exec")
	require.True(t, ok)
	_, ok = allowAll.Get("network")
	require.True(t, ok)
	require.Equal(t, 1, registryPrecheckCount(t, allowAll))
}
