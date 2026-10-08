package authorization

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

func newToolAuthManager(t *testing.T) (*PermissionManager, *policy.InMemoryAuditLogger) {
	t.Helper()
	audit := policy.NewInMemoryAuditLogger(32)
	pm, err := NewPermissionManager("/tmp", &permissions.PermissionSet{}, audit, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	return pm, audit
}

func TestAuthorizeToolByNameRequiresName(t *testing.T) {
	pm, _ := newToolAuthManager(t)
	if err := pm.AuthorizeToolByName(context.Background(), "agent-1", "  "); err == nil {
		t.Fatal("expected an error for an empty tool name")
	}
}

func TestAuthorizeToolByNameFailsClosedWithoutHITL(t *testing.T) {
	pm, _ := newToolAuthManager(t) // default policy: ask, no HITL provider
	if err := pm.AuthorizeToolByName(context.Background(), "agent-1", "file_read"); err == nil {
		t.Fatal("expected fail-closed error for an undeclared tool with no HITL provider")
	}
}

func TestAuthorizeToolByNameDenyPolicy(t *testing.T) {
	pm, _ := newToolAuthManager(t)
	pm.SetDefaultPolicy("deny")
	if err := pm.AuthorizeToolByName(context.Background(), "agent-1", "file_read"); err == nil {
		t.Fatal("expected a deny error")
	}
}

func TestAuthorizeToolByNameAskPolicyWithHITL(t *testing.T) {
	broker := NewHITLBroker(time.Minute, nil)
	defer broker.Stop()
	broker.AutoApprove = true
	pm, err := NewPermissionManager("/tmp", &permissions.PermissionSet{}, policy.NewInMemoryAuditLogger(32), broker)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	if err := pm.AuthorizeToolByName(context.Background(), "agent-1", "file_read"); err != nil {
		t.Fatalf("expected approval, got %v", err)
	}
}

// TestEnforcerCheckToolInvokeCarriesToolName proves the Enforcer passes the
// requested tool name through to authorization (observable in the audit record)
// instead of dropping it.
func TestEnforcerCheckToolInvokeCarriesToolName(t *testing.T) {
	pm, audit := newToolAuthManager(t)
	pm.SetDefaultPolicy("deny")
	e := NewEnforcer(pm)

	decision := e.Check(context.Background(), governanceports.AccessRequest{
		Principal: governanceports.Principal{AgentID: "agent-1"},
		Action:    governanceports.ActionToolInvoke,
		Resource:  governanceports.Resource{Kind: "tool", ID: "file_read"},
	})
	if decision.Allow {
		t.Fatal("expected deny under the deny policy")
	}

	records, err := audit.Query(context.Background(), policy.AuditQuery{})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	var found bool
	for _, r := range records {
		if r.Action == "tool:file_read" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit did not carry the tool name: %+v", records)
	}
}
