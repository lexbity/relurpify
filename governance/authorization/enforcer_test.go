package authorization

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

func TestNewEnforcer_nil(t *testing.T) {
	e := NewEnforcer(nil)
	d := e.Check(context.Background(), governanceports.AccessRequest{Action: governanceports.ActionFileRead})
	if d.Allow {
		t.Error("expected deny for nil enforcer")
	}
}

func TestEnforcer_Check_unknownAction(t *testing.T) {
	pm := newTestPermissionManager(t)
	e := NewEnforcer(pm)
	d := e.Check(context.Background(), governanceports.AccessRequest{
		Principal: governanceports.Principal{AgentID: "test-agent"},
		Action:    governanceports.Action("unknown-action"),
	})
	if d.Allow {
		t.Error("expected deny for unknown action")
	}
}

func TestEnforcer_Check_fileRead_allowed(t *testing.T) {
	pm := newTestPermissionManager(t)
	e := NewEnforcer(pm)
	d := e.Check(context.Background(), governanceports.AccessRequest{
		Principal: governanceports.Principal{AgentID: "test-agent"},
		Action:    governanceports.ActionFileRead,
		Resource:  governanceports.Resource{Kind: "path", ID: "/tmp/test.txt"},
	})
	if !d.Allow {
		t.Errorf("expected allow, got deny: %s", d.Reason)
	}
}

func TestEnforcer_Check_fileRead_denied(t *testing.T) {
	pm := newTestPermissionManager(t)
	pm.SetDefaultPolicy("deny")
	e := NewEnforcer(pm)
	d := e.Check(context.Background(), governanceports.AccessRequest{
		Principal: governanceports.Principal{AgentID: "test-agent"},
		Action:    governanceports.ActionFileRead,
		Resource:  governanceports.Resource{Kind: "path", ID: "/etc/passwd"},
	})
	if d.Allow {
		t.Errorf("expected deny for /etc/passwd")
	}
}

func TestEnforcer_Check_principalFromContext(t *testing.T) {
	pm := newTestPermissionManager(t)
	e := NewEnforcer(pm)
	ctx := governanceports.ContextWithPrincipal(context.Background(), governanceports.Principal{AgentID: "ctx-agent"})
	d := e.Check(ctx, governanceports.AccessRequest{
		Action:   governanceports.ActionFileRead,
		Resource: governanceports.Resource{Kind: "path", ID: "/tmp/test.txt"},
	})
	if !d.Allow {
		t.Errorf("expected allow with principal from context, got deny: %s", d.Reason)
	}
}

func TestEnforcer_Check_actionFileWrite(t *testing.T) {
	pm := newTestPermissionManager(t)
	e := NewEnforcer(pm)
	d := e.Check(context.Background(), governanceports.AccessRequest{
		Principal: governanceports.Principal{AgentID: "test-agent"},
		Action:    governanceports.ActionFileWrite,
		Resource:  governanceports.Resource{Kind: "path", ID: "/tmp/write-test.txt"},
	})
	if !d.Allow {
		t.Errorf("expected allow for file write, got: %s", d.Reason)
	}
}

func TestEnforcer_Check_actionCapability(t *testing.T) {
	pm := newTestPermissionManager(t)
	e := NewEnforcer(pm)
	d := e.Check(context.Background(), governanceports.AccessRequest{
		Principal: governanceports.Principal{AgentID: "test-agent"},
		Action:    governanceports.ActionCapability,
		Resource:  governanceports.Resource{Kind: "capability", ID: "test-cap"},
	})
	if !d.Allow {
		t.Errorf("expected allow for capability, got: %s", d.Reason)
	}
}

func TestPrincipalContext_roundTrip(t *testing.T) {
	p := governanceports.Principal{AgentID: "test-agent"}
	ctx := governanceports.ContextWithPrincipal(context.Background(), p)
	got := governanceports.PrincipalFromContext(ctx)
	if got.AgentID != p.AgentID {
		t.Errorf("PrincipalFromContext = %+v, want %+v", got, p)
	}
}

func TestPrincipalContext_empty(t *testing.T) {
	got := governanceports.PrincipalFromContext(context.Background())
	if got.AgentID != "" {
		t.Errorf("expected empty principal, got %+v", got)
	}
}

func TestFailClosed_nilDecision(t *testing.T) {
	var d governanceports.Decision
	if d.Allow {
		t.Error("zero-value Decision must be deny (fail-closed)")
	}
}

// newTestPermissionManager creates a PermissionManager for testing.
func newTestPermissionManager(t *testing.T) *PermissionManager {
	t.Helper()
	declared := &permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{
			{Path: "/tmp/**", Action: permissions.FileSystemRead},
			{Path: "/tmp/**", Action: permissions.FileSystemWrite},
		},
		Capabilities: []permissions.CapabilityPermission{
			{Capability: "test-cap"},
		},
	}
	audit := policy.NewInMemoryAuditLogger(100)
	pm, err := NewPermissionManager("/tmp", declared, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	return pm
}
