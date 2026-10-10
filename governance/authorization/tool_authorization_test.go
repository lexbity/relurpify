package authorization

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// toolApprovingProvider is an inline HITLProvider test double that approves
// every request. It lives here (not in testsuite) so the enforcement tests can
// exercise HITL-ask paths without the production broker's approval workflow.
type toolApprovingProvider struct {
	grants map[string]*PermissionGrant
}

func (p *toolApprovingProvider) RequestPermission(_ context.Context, req PermissionRequest) (*PermissionGrant, error) {
	now := time.Now().UTC()
	grant := &PermissionGrant{
		ID:          "tool-approving-" + req.Permission.Action,
		Permission:  req.Permission,
		Scope:       req.Scope,
		ApprovedBy:  "test",
		GrantedAt:   now,
		ExpiresAt:   now.Add(time.Minute),
		Description: req.Justification,
	}
	if p.grants == nil {
		p.grants = make(map[string]*PermissionGrant)
	}
	p.grants[req.Permission.Action] = grant
	return grant, nil
}

func newToolAuthManager(t *testing.T) (*PermissionManager, *policy.FileChainAuditLogger) {
	t.Helper()
	audit := newTestAuditLogger(t)
	pm, err := NewPermissionManager("/tmp", &ucperms.PermissionSet{}, audit, nil)
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
	_ = pm.SetDefaultDecision(permissions.DecisionDeny)
	if err := pm.AuthorizeToolByName(context.Background(), "agent-1", "file_read"); err == nil {
		t.Fatal("expected a deny error")
	}
}

func TestAuthorizeToolByNameAskPolicyWithHITL(t *testing.T) {
	broker := &toolApprovingProvider{}
	pm, err := NewPermissionManager("/tmp", &ucperms.PermissionSet{}, newTestAuditLogger(t), broker)
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
	_ = pm.SetDefaultDecision(permissions.DecisionDeny)
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
