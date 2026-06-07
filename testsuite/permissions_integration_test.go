package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestToolRegistryPermissionEnforcement(t *testing.T) {
}

func TestToolRegistryNetworkHITLApproval(t *testing.T) {
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
	perms    *permissions.PermissionSet
	manager  *authorization.PermissionManager
	agent    string
	path     string
	host     string
	ran      bool
}

func (t *permissionedTool) Name() string        { return t.toolName }
func (t *permissionedTool) Description() string { return "integration test tool" }
func (t *permissionedTool) Category() string    { return "integration" }
func (t *permissionedTool) Parameters() []ports.ToolParameter {
	return nil
}
func (t *permissionedTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	if t.manager != nil {
		if t.path != "" {
			if err := t.manager.CheckFileAccess(ctx, t.agent, permissions.FileSystemRead, t.path); err != nil {
				return nil, err
			}
		}
		if t.host != "" {
			if err := t.manager.CheckNetwork(ctx, t.agent, "egress", "tcp", t.host, 443); err != nil {
				return nil, err
			}
		}
	}
	t.ran = true
	return &ports.ToolResult{Success: true}, nil
}
func (t *permissionedTool) IsAvailable(context.Context) bool { return true }
func (t *permissionedTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: t.perms}
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
