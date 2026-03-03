package framework_test

import (
	"context"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestPermissionSetValidate ensures Validate catches missing paths/binaries and
// accepts well-formed permission sets.
func TestPermissionSetValidate(t *testing.T) {
	valid := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/workspace/**"},
		},
		Executables: []core.ExecutablePermission{
			{Binary: "go", Args: []string{"test"}},
		},
	}
	require.NoError(t, valid.Validate())

	invalid := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead}},
	}
	require.Error(t, invalid.Validate(), "missing path should fail validation")

	badExec := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/**"}},
		Executables: []core.ExecutablePermission{
			{Binary: ""},
		},
	}
	require.Error(t, badExec.Validate(), "missing binary should fail validation")
}

// TestPermissionManagerAuthorizeToolEnforcesSubset verifies that tool-specific
// manifests cannot request filesystem scopes beyond the agent manifest.
func TestPermissionManagerAuthorizeToolEnforcesSubset(t *testing.T) {
	ctx := context.Background()
	manager := newTestManager(t, "/workspace", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/workspace/**"},
			{Action: core.FileSystemList, Path: "/workspace/**"},
		},
	})
	// Use explicit Deny so undeclared permissions are hard-blocked without HITL.
	manager.SetDefaultPolicy(runtime.AgentPermissionDeny)

	okTool := stubTool{
		name: "list",
		perms: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemRead, Path: "/workspace/**"},
			},
		},
	}
	require.NoError(t, manager.AuthorizeTool(ctx, "agent-1", okTool, nil))

	badTool := stubTool{
		name: "escape",
		perms: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemRead, Path: "/etc/**"},
			},
		},
	}
	err := manager.AuthorizeTool(ctx, "agent-1", badTool, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds agent permissions")
}

// TestPermissionManagerCheckFileAccess checks file authorization rejects
// traversal attempts and unauthorized actions.
func TestPermissionManagerCheckFileAccess(t *testing.T) {
	ctx := context.Background()
	manager := newTestManager(t, "/workspace", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/workspace/src/**"},
		},
	})

	require.NoError(t, manager.CheckFileAccess(ctx, "agent-1", core.FileSystemRead, "src/main.go"))

	err := manager.CheckFileAccess(ctx, "agent-1", core.FileSystemRead, "../etc/passwd")
	require.Error(t, err, "path traversal should be denied")

	err = manager.CheckFileAccess(ctx, "agent-1", core.FileSystemWrite, "src/main.go")
	require.Error(t, err, "write action not declared should be denied")
}

// TestPermissionHelpers confirms helper constructors produce intuitive globs
// and executable permissions.
func TestPermissionHelpers(t *testing.T) {
	fs := core.NewFileSystemPermissionSet("/workspace", core.FileSystemRead, core.FileSystemList)
	require.Len(t, fs.FileSystem, 2)
	require.Equal(t, "/workspace/**", fs.FileSystem[0].Path)

	exec := core.NewExecutionPermissionSet("/workspace", "python3", []string{"script.py"})
	require.Len(t, exec.Executables, 1)
	require.Equal(t, "python3", exec.Executables[0].Binary)
	require.Contains(t, exec.FileSystem, core.FileSystemPermission{Action: core.FileSystemExecute, Path: "/workspace/**"})
}

type stubTool struct {
	name  string
	perms *core.PermissionSet
}

// Name identifies the stub tool in registry lookups.
func (t stubTool) Name() string { return t.name }

// Description satisfies the Tool interface.
func (t stubTool) Description() string { return "stub" }

// Category returns the testing category for clarity.
func (t stubTool) Category() string { return "test" }

// Parameters indicates the stub tool takes no arguments.
func (t stubTool) Parameters() []core.ToolParameter { return nil }

// Execute returns a successful result so authorization paths can be tested in
// isolation.
func (t stubTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}

// IsAvailable indicates the stub is always ready to run.
func (t stubTool) IsAvailable(context.Context, *core.Context) bool { return true }

// Permissions returns the configured permission set for the stub tool.
func (t stubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: t.perms}
}

// Tags returns nil as the stub tool has no tags.
func (t stubTool) Tags() []string { return nil }

// newTestManager is a helper that fails tests immediately when the permission
// manager cannot be constructed.
func newTestManager(t *testing.T, base string, perms *core.PermissionSet) *runtime.PermissionManager {
	t.Helper()
	manager, err := runtime.NewPermissionManager(base, perms, nil, nil)
	require.NoError(t, err)
	return manager
}

func TestPermissionManagerHITLFlow(t *testing.T) {
	ctx := context.Background()
	hitl := &stubHITLProvider{
		grants: []*runtime.PermissionGrant{{
			ID: "grant-1",
			Permission: core.PermissionDescriptor{
				Type:     core.PermissionTypeFilesystem,
				Action:   string(core.FileSystemRead),
				Resource: "/workspace/file.txt",
			},
			Scope: runtime.GrantScopeSession,
		}},
	}
	perms := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{
			Action:       core.FileSystemRead,
			Path:         "/workspace/**",
			HITLRequired: true,
		}},
	}
	manager, err := runtime.NewPermissionManager("/workspace", perms, nil, hitl)
	require.NoError(t, err)

	require.NoError(t, manager.CheckFileAccess(ctx, "agent-hitl", core.FileSystemRead, "file.txt"))
	require.Len(t, hitl.requests, 1, "expected HITL approval request")

	require.NoError(t, manager.CheckFileAccess(ctx, "agent-hitl", core.FileSystemRead, "file.txt"))
	require.Len(t, hitl.requests, 1, "cached grant should avoid duplicate HITL calls")
}

func TestPermissionManagerCapabilityCheck(t *testing.T) {
	ctx := context.Background()
	manager := newTestManager(t, "/workspace", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/workspace/**"},
		},
		Capabilities: []core.CapabilityPermission{
			{Capability: "NET_ADMIN"},
		},
	})

	require.NoError(t, manager.CheckCapability(ctx, "agent", "NET_ADMIN"))
	require.Error(t, manager.CheckCapability(ctx, "agent", "SYS_PTRACE"))
}

type stubHITLProvider struct {
	grants   []*runtime.PermissionGrant
	requests []runtime.PermissionRequest
}

func (s *stubHITLProvider) RequestPermission(ctx context.Context, req runtime.PermissionRequest) (*runtime.PermissionGrant, error) {
	s.requests = append(s.requests, req)
	var grant *runtime.PermissionGrant
	if len(s.grants) > 0 {
		grant = s.grants[0]
		s.grants = s.grants[1:]
	} else {
		grant = &runtime.PermissionGrant{}
	}
	if grant.Permission.Action == "" {
		grant.Permission = req.Permission
	}
	return grant, nil
}
