package authorization

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// literalTarget classifies a host token through the canonical resolver. Every
// host used here is an IP literal, so no DNS is performed.
func literalTarget(t *testing.T, host string) netpolicy.Target {
	t.Helper()
	target, err := netpolicy.ResolveTarget(context.Background(), host, netpolicy.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("ResolveTarget(%q): %v", host, err)
	}
	return target
}

func TestCheckNetworkBlocksIPv4Loopback(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "127.0.0.1"), 8080)
	if err == nil {
		t.Fatal("expected error for loopback address, got nil")
	}
}

func TestCheckNetworkBlocksIPv4LoopbackRange(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "127.255.255.255"), 80)
	if err == nil {
		t.Fatal("expected error for loopback range address, got nil")
	}
}

func TestCheckNetworkBlocksMetadataService(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "169.254.169.254"), 80)
	if err == nil {
		t.Fatal("expected error for metadata service address, got nil")
	}
}

func TestCheckNetworkBlocksRFC1918ClassA(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "10.0.0.1"), 443)
	if err == nil {
		t.Fatal("expected error for RFC-1918 class A address, got nil")
	}
}

func TestCheckNetworkBlocksRFC1918ClassB(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "172.31.255.255"), 443)
	if err == nil {
		t.Fatal("expected error for RFC-1918 class B address, got nil")
	}
}

func TestCheckNetworkBlocksRFC1918ClassC(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "192.168.1.1"), 443)
	if err == nil {
		t.Fatal("expected error for RFC-1918 class C address, got nil")
	}
}

func TestCheckNetworkBlocksIPv6Loopback(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "::1"), 8080)
	if err == nil {
		t.Fatal("expected error for IPv6 loopback, got nil")
	}
}

func TestCheckNetworkBlocksIPv6UniqueLocal(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "fc00::1"), 443)
	if err == nil {
		t.Fatal("expected error for IPv6 unique-local, got nil")
	}
}

func TestCheckNetworkBlocksIPv6LinkLocal(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "fe80::1"), 443)
	if err == nil {
		t.Fatal("expected error for IPv6 link-local, got nil")
	}
}

// TestCheckNetworkBlocksNonDottedLoopback proves the inet_aton spellings that
// the child process can dereference are classified, not skipped as names.
func TestCheckNetworkBlocksNonDottedLoopback(t *testing.T) {
	for _, host := range []string{"2130706433", "0x7f000001", "0177.0.0.1", "127.1"} {
		m := testPermissionManager(t)
		err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, host), 80)
		if err == nil {
			t.Errorf("expected error for inet_aton loopback spelling %q, got nil", host)
		}
	}
}

func TestCheckNetworkBlocksUnspecified(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::"} {
		m := testPermissionManager(t)
		err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, host), 80)
		if err == nil {
			t.Errorf("expected error for unspecified address %q, got nil", host)
		}
	}
}

func TestCheckNetworkBlocksResolvedPrivateTarget(t *testing.T) {
	m := testPermissionManager(t)
	target := netpolicy.Target{Token: "internal.example", Class: netpolicy.ClassPrivate}
	if err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", target, 443); err == nil {
		t.Fatal("expected deny for a resolved-private target")
	}
}

func TestCheckNetworkBlocksUnspecifiedLiteral(t *testing.T) {
	m := testPermissionManager(t)
	err := m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "0.0.0.0"), 80)
	if err == nil {
		t.Fatal("expected deny for the unspecified literal 0.0.0.0")
	}
}

func TestCheckNetworkBlocksPrivateEvenIfDeclared(t *testing.T) {
	// Even if a permission is explicitly declared for a private IP, the
	// hard-coded denylist must still block it.
	declared := &ucperms.PermissionSet{
		Network: []ucperms.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "10.0.0.1", Port: 443},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	err = m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "10.0.0.1"), 443)
	if err == nil {
		t.Fatal("expected error even with declared permission for private IP")
	}
}

func TestCheckNetworkAllowsPublicIP(t *testing.T) {
	declared := &ucperms.PermissionSet{
		Network: []ucperms.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "8.8.8.8", Port: 443},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	err = m.CheckNetwork(context.Background(), "agent-1", "egress", "tcp", literalTarget(t, "8.8.8.8"), 443)
	if err != nil {
		t.Fatalf("expected no error for public IP, got: %v", err)
	}
}

func TestDefaultDecisionAllowRejectedAtRegistration(t *testing.T) {
	perm := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	if err := m.SetDefaultDecision(permissions.DecisionAllow); err == nil {
		t.Fatal("expected SetDefaultDecision(allow) to be rejected")
	}
	if got := m.DefaultPolicy(); got != string(permissions.DecisionAsk) {
		t.Fatalf("default after rejected allow = %q, want ask", got)
	}
	if err := m.SetDefaultDecision(permissions.Decision("alow")); err == nil {
		t.Fatal("expected an invalid decision to be rejected")
	}
}

func TestDefaultPolicyAskIsValid(t *testing.T) {
	perm := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	_ = m.SetDefaultDecision(permissions.DecisionAsk)
}

func TestDefaultPolicyDenyIsValid(t *testing.T) {
	perm := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	_ = m.SetDefaultDecision(permissions.DecisionDeny)
}

func TestUndeclaredToolPermissionDeniedNotSilent(t *testing.T) {
	perm := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", perm, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	// With default=Ask and no HITL provider, undeclared permissions must
	// return an error (not silent allow).
	tool := &testAuthTool{name: "test_tool"}
	err = m.AuthorizeTool(context.Background(), "agent-1", tool, nil)
	if err == nil {
		t.Fatal("expected error for undeclared tool permission with no HITL provider, got nil")
	}
}

// testPermissionManager creates a permission manager with minimal permissions
// and no HITL provider for testing network blocking.
func testPermissionManager(t *testing.T) *PermissionManager {
	t.Helper()
	declared := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	audit := newTestAuditLogger(t)
	m, err := NewPermissionManager("/workspace", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	return m
}

// testAuthTool implements authorization.Tool for testing.
type testAuthTool struct {
	name string
}

func (t *testAuthTool) Name() string { return t.name }
func (t *testAuthTool) Permissions() ToolPermissions {
	return ToolPermissions{
		Permissions: &ucperms.PermissionSet{
			Executables: []ucperms.ExecutablePermission{
				{Binary: "some-binary"},
			},
		},
	}
}
func (t *testAuthTool) Tags() []string { return nil }
