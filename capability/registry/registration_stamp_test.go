package registry

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// stampProbeTool records every registry stamp it receives. It mimics the
// concrete file tools, which enforce through their own stamped fields.
type stampProbeTool struct {
	name    string
	manager PermissionManagerHandle
	agentID string
	spec    *agentspec.AgentRuntimeSpec
	scope   *permissions.FileScopePolicy
}

func (t *stampProbeTool) Name() string                         { return t.name }
func (t *stampProbeTool) Description() string                  { return "stamp probe fixture" }
func (t *stampProbeTool) Category() string                     { return "test" }
func (t *stampProbeTool) Parameters() []ports.ToolParameter    { return nil }
func (t *stampProbeTool) Tags() []string                       { return nil }
func (t *stampProbeTool) IsAvailable(ctx context.Context) bool { return true }
func (t *stampProbeTool) Permissions() ports.ToolPermissions   { return ports.ToolPermissions{} }
func (t *stampProbeTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}
func (t *stampProbeTool) SetPermissionManager(manager PermissionManagerHandle, agentID string) {
	t.manager, t.agentID = manager, agentID
}
func (t *stampProbeTool) SetAgentSpec(spec *agentspec.AgentRuntimeSpec, agentID string) {
	t.spec, t.agentID = spec, agentID
}
func (t *stampProbeTool) SetSandboxScope(scope *permissions.FileScopePolicy) { t.scope = scope }

// probeDecorator is a tool decorator exercising the unwrap contract, standing
// in for the manifest-capability wrapper the Build chain applies.
type probeDecorator struct {
	ports.Tool
}

func (d *probeDecorator) Unwrap() ports.Tool { return d.Tool }

// TestRegistrationStampsReachDecoratedTools pins the registration-time stamp
// path: Use* wiring that precedes Register must reach the CONCRETE tool
// through decorators (the F1 class — a direct type assertion on the wrapper
// silently missed the permission stamp and left workspace writes asking an
// absent approver).
func TestRegistrationStampsReachDecoratedTools(t *testing.T) {
	inner := &stampProbeTool{name: "stamp_probe"}
	decorated := &probeDecorator{Tool: inner}

	r := NewRegistry()
	manager := &stubPermissionManagerHandle{}
	r.UsePermissionManager("agent-1", manager)
	spec := &agentspec.AgentRuntimeSpec{}
	r.UseAgentSpec("agent-1", spec)
	r.UseSandboxScope(permissions.NewFileScopePolicy(t.TempDir(), nil))

	if err := r.Register(context.Background(), decorated); err != nil {
		t.Fatalf("register: %v", err)
	}

	if inner.manager != manager || inner.agentID != "agent-1" {
		t.Fatalf("permission stamp did not reach the concrete tool: manager=%v agentID=%q", inner.manager != nil, inner.agentID)
	}
	if inner.spec != spec {
		t.Fatal("agent-spec stamp did not reach the concrete tool")
	}
	if inner.scope == nil {
		t.Fatal("sandbox-scope stamp did not reach the concrete tool")
	}
}

type stubPermissionManagerHandle struct{}

func (s *stubPermissionManagerHandle) RequireApproval(ctx context.Context, agentID string, desc ucperms.PermissionDescriptor, justification string, scope policy.GrantScope, risk policy.RiskLevel, duration time.Duration) error {
	return nil
}

func (s *stubPermissionManagerHandle) AuthorizeTool(ctx context.Context, agentID string, tool any, args map[string]any) error {
	return nil
}

var _ PermissionManagerHandle = (*stubPermissionManagerHandle)(nil)
