package testsuite

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

func TestToolRegistryPermissionEnforcement(t *testing.T) {
	base := t.TempDir()
	perms := core.NewFileSystemPermissionSet(base, core.FileSystemRead, core.FileSystemList)
	perms.Network = []core.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443}}
	manager, err := authorization.NewPermissionManager(base, perms, nil, nil)
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}
	runtime := &recordingRuntime{}
	manager.AttachRuntime(runtime)

	allowedToolPerms := core.NewFileSystemPermissionSet(base, core.FileSystemRead)
	allowedToolPerms.Network = []core.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443}}
	allowedTool := &permissionedTool{
		toolName: "workspace_reader",
		perms:    allowedToolPerms,
		manager:  manager,
		agent:    "agent-int",
		path:     filepath.Join(base, "file.txt"),
		host:     "example.com",
	}
	escapePerms := core.NewFileSystemPermissionSet("/etc", core.FileSystemRead)
	escapeTool := &permissionedTool{
		toolName: "escape",
		perms:    escapePerms,
		manager:  manager,
		agent:    "agent-int",
		path:     "/etc/passwd",
	}

	registry := capability.NewRegistry()
	if err := registry.Register(allowedTool); err != nil {
		t.Fatalf("register allowed tool: %v", err)
	}
	if err := registry.Register(escapeTool); err != nil {
		t.Fatalf("register escape tool: %v", err)
	}
	registry.UsePermissionManager("agent-int", manager)

	tool, _ := registry.Get("workspace_reader")
	state := core.NewContext()
	if _, err := tool.Execute(context.Background(), state, nil); err != nil {
		t.Fatalf("expected allowed tool to run, got error: %v", err)
	}
	if value, _ := state.Get("tool:workspace_reader"); value != "ok" {
		t.Fatalf("tool state not recorded: %v", value)
	}
	if len(runtime.policies) == 0 || len(runtime.policies[len(runtime.policies)-1].NetworkRules) == 0 {
		t.Fatal("expected network policy to be enforced")
	}

	escape, _ := registry.Get("escape")
	if _, err := escape.Execute(context.Background(), core.NewContext(), nil); err == nil {
		t.Fatal("expected permission error for escape tool")
	}
}

func TestToolRegistryNetworkHITLApproval(t *testing.T) {
	base := t.TempDir()
	hitl := &stubHITL{
		grants: []*authorization.PermissionGrant{{
			ID: "grant-1",
			Permission: core.PermissionDescriptor{
				Type:     core.PermissionTypeNetwork,
				Action:   "net:egress",
				Resource: "api.service.local:443",
			},
			Scope: authorization.GrantScopeSession,
		}},
	}
	perms := core.NewFileSystemPermissionSet(base, core.FileSystemRead)
	perms.Network = []core.NetworkPermission{
		{Direction: "egress", Protocol: "tcp", Host: "api.service.local", Port: 443, HITLRequired: true},
	}
	manager, err := authorization.NewPermissionManager(base, perms, nil, hitl)
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	toolPerms := core.NewFileSystemPermissionSet(base, core.FileSystemRead)
	toolPerms.Network = []core.NetworkPermission{
		{Direction: "egress", Protocol: "tcp", Host: "api.service.local", Port: 443, HITLRequired: true},
	}
	netTool := &permissionedTool{
		toolName: "net_call",
		perms:    toolPerms,
		manager:  manager,
		agent:    "agent-net",
		host:     "api.service.local",
	}

	registry := capability.NewRegistry()
	if err := registry.Register(netTool); err != nil {
		t.Fatalf("register net tool: %v", err)
	}
	registry.UsePermissionManager("agent-net", manager)

	tool, _ := registry.Get("net_call")
	if _, err := tool.Execute(context.Background(), core.NewContext(), nil); err != nil {
		t.Fatalf("expected HITL-enabled tool to run, got error: %v", err)
	}
	if len(hitl.requests) != 1 {
		t.Fatalf("expected exactly one HITL request, got %d", len(hitl.requests))
	}
	if _, err := tool.Execute(context.Background(), core.NewContext(), nil); err != nil {
		t.Fatalf("expected cached grant to allow subsequent run: %v", err)
	}
	if len(hitl.requests) != 1 {
		t.Fatalf("expected cached grant to prevent duplicate HITL calls, got %d", len(hitl.requests))
	}
}

type recordingRuntime struct {
	policies []sandbox.SandboxPolicy
}

func (r *recordingRuntime) Name() string                 { return "recording" }
func (r *recordingRuntime) Verify(context.Context) error { return nil }
func (r *recordingRuntime) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{
		NetworkIsolation:  true,
		ReadOnlyRoot:      true,
		ProtectedPaths:    true,
		NoNewPrivileges:   true,
		Seccomp:           true,
		UserMapping:       true,
		PerCommandWorkdir: true,
	}
}
func (r *recordingRuntime) ValidatePolicy(policy sandbox.SandboxPolicy) error {
	return policy.Validate()
}
func (r *recordingRuntime) ApplyPolicy(_ context.Context, policy sandbox.SandboxPolicy) error {
	r.policies = append(r.policies, policy)
	return nil
}
func (r *recordingRuntime) RunConfig() sandbox.SandboxConfig { return sandbox.SandboxConfig{} }
func (r *recordingRuntime) Policy() sandbox.SandboxPolicy {
	if len(r.policies) == 0 {
		return sandbox.SandboxPolicy{}
	}
	return r.policies[len(r.policies)-1]
}

type permissionedTool struct {
	toolName string
	perms    *core.PermissionSet
	manager  *authorization.PermissionManager
	agent    string
	path     string
	host     string
}

func (t *permissionedTool) Name() string        { return t.toolName }
func (t *permissionedTool) Description() string { return "integration test tool" }
func (t *permissionedTool) Category() string    { return "integration" }
func (t *permissionedTool) Parameters() []core.ToolParameter {
	return nil
}
func (t *permissionedTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if t.manager != nil {
		if t.path != "" {
			if err := t.manager.CheckFileAccess(ctx, t.agent, core.FileSystemRead, t.path); err != nil {
				return nil, err
			}
		}
		if t.host != "" {
			if err := t.manager.CheckNetwork(ctx, t.agent, "egress", "tcp", t.host, 443); err != nil {
				return nil, err
			}
		}
	}
	state.Set("tool:"+t.toolName, "ok")
	return &core.ToolResult{Success: true}, nil
}
func (t *permissionedTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t *permissionedTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: t.perms}
}
func (t *permissionedTool) Tags() []string { return nil }

type stubHITL struct {
	grants   []*authorization.PermissionGrant
	requests []authorization.PermissionRequest
}

func (s *stubHITL) RequestPermission(ctx context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	s.requests = append(s.requests, req)
	if len(s.grants) == 0 {
		return &authorization.PermissionGrant{Permission: req.Permission, Scope: authorization.GrantScopeSession}, nil
	}
	grant := s.grants[0]
	s.grants = s.grants[1:]
	if grant.Permission.Action == "" {
		grant.Permission = req.Permission
	}
	return grant, nil
}
