package authorization

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestCheckNetworkBlocksIPv4Loopback(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "127.0.0.1", 8080)
	if err == nil {
		t.Fatal("expected error for loopback address, got nil")
	}
}

func TestCheckNetworkBlocksIPv4LoopbackRange(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "127.255.255.255", 80)
	if err == nil {
		t.Fatal("expected error for loopback range address, got nil")
	}
}

func TestCheckNetworkBlocksMetadataService(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "169.254.169.254", 80)
	if err == nil {
		t.Fatal("expected error for metadata service address, got nil")
	}
}

func TestCheckNetworkBlocksRFC1918ClassA(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "10.0.0.1", 443)
	if err == nil {
		t.Fatal("expected error for RFC-1918 class A address, got nil")
	}
}

func TestCheckNetworkBlocksRFC1918ClassB(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "172.31.255.255", 443)
	if err == nil {
		t.Fatal("expected error for RFC-1918 class B address, got nil")
	}
}

func TestCheckNetworkBlocksRFC1918ClassC(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "192.168.1.1", 443)
	if err == nil {
		t.Fatal("expected error for RFC-1918 class C address, got nil")
	}
}

func TestCheckNetworkBlocksIPv6Loopback(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "::1", 8080)
	if err == nil {
		t.Fatal("expected error for IPv6 loopback, got nil")
	}
}

func TestCheckNetworkBlocksIPv6UniqueLocal(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "fc00::1", 443)
	if err == nil {
		t.Fatal("expected error for IPv6 unique-local, got nil")
	}
}

func TestCheckNetworkBlocksIPv6LinkLocal(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "fe80::1", 443)
	if err == nil {
		t.Fatal("expected error for IPv6 link-local, got nil")
	}
}

func TestCheckNetworkBlocksPrivateEvenIfDeclared(t *testing.T) {
	// Even if a permission is explicitly declared for a private IP, the
	// hard-coded denylist must still block it.
	declared := &contracts.PermissionSet{
		Network: []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "10.0.0.1", Port: 443},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	err = m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "10.0.0.1", 443)
	if err == nil {
		t.Fatal("expected error even with declared permission for private IP")
	}
}

func TestCheckNetworkAllowsPublicIP(t *testing.T) {
	declared := &contracts.PermissionSet{
		Network: []contracts.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "8.8.8.8", Port: 443},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	err = m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", "8.8.8.8", 443)
	if err != nil {
		t.Fatalf("expected no error for public IP, got: %v", err)
	}
}

func TestDefaultPolicyAllowRejectedAtRegistration(t *testing.T) {
	perm := &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	m.SetDefaultPolicy(agentspec.AgentPermissionAllow)
}

func TestDefaultPolicyAskIsValid(t *testing.T) {
	perm := &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	m.SetDefaultPolicy(agentspec.AgentPermissionAsk)
}

func TestDefaultPolicyDenyIsValid(t *testing.T) {
	perm := &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	m.SetDefaultPolicy(agentspec.AgentPermissionDeny)
}

func TestUndeclaredToolPermissionDeniedNotSilent(t *testing.T) {
	perm := &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	// With default=Ask and no HITL provider, undeclared permissions must
	// return an error (not silent allow).
	tool := &testTool{name: "test_tool"}
	err = m.AuthorizeTool(context.Background(), "agent-1", tool, nil)
	if err == nil {
		t.Fatal("expected error for undeclared tool permission with no HITL provider, got nil")
	}
}

// testPermissionManager creates a permission manager with minimal permissions
// and no HITL provider for testing network blocking.
func testPermissionManager(t *testing.T) *PermissionManager {
	t.Helper()
	declared := &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	m, err := NewPermissionManager("/workspace", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	return m
}

// testTool implements contracts.Tool for testing.
type testTool struct {
	name string
}

func (t *testTool) Name() string                          { return t.name }
func (t *testTool) Description() string                   { return "test tool" }
func (t *testTool) Category() string                      { return "test" }
func (t *testTool) Parameters() []contracts.ToolParameter { return nil }
func (t *testTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	return &contracts.ToolResult{Success: true}, nil
}
func (t *testTool) IsAvailable(ctx context.Context) bool { return true }
func (t *testTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{
		Permissions: &contracts.PermissionSet{
			Executables: []contracts.ExecutablePermission{
				{Binary: "some-binary"},
			},
		},
	}
}
func (t *testTool) Tags() []string { return nil }
