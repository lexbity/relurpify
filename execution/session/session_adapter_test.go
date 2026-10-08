package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/authorization"
)

// recordingHITLApprover is a test double for the HITL capability required by the
// security controller. It records every request and returns a configured grant
// or error.
type recordingHITLApprover struct {
	mu       sync.Mutex
	requests []authorization.PermissionRequest
	grant    *authorization.PermissionGrant
	err      error
}

func (m *recordingHITLApprover) RequestPermission(_ context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	return m.grant, m.err
}

func (m *recordingHITLApprover) lastRequest(t *testing.T) authorization.PermissionRequest {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		t.Fatal("no permission request recorded")
	}
	return m.requests[len(m.requests)-1]
}

func TestSecurityControllerRequestApprovalDelegatesToBroker(t *testing.T) {
	approver := &recordingHITLApprover{grant: &authorization.PermissionGrant{ApprovedBy: "operator"}}
	c := newSecurityController(&Workspace{Registration: &Registration{ID: "agent-x", HITL: approver}})

	decision, err := c.RequestApproval(context.Background(), ApprovalRequest{
		Action:  "fs:write:/tmp/x",
		Reason:  "need write access",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !decision.Approved {
		t.Fatalf("decision not approved: %+v", decision)
	}
	if decision.Reason != "operator" {
		t.Fatalf("decision reason = %q, want %q", decision.Reason, "operator")
	}

	got := approver.lastRequest(t)
	if got.Permission.Action != "fs:write:/tmp/x" {
		t.Errorf("permission action = %q, want %q", got.Permission.Action, "fs:write:/tmp/x")
	}
	if !got.Permission.RequiresHITL {
		t.Error("permission RequiresHITL = false, want true")
	}
	if got.Justification != "need write access" {
		t.Errorf("justification = %q, want %q", got.Justification, "need write access")
	}
	if got.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", got.Timeout)
	}
}

func TestSecurityControllerRequestApprovalWithoutBroker(t *testing.T) {
	c := newSecurityController(&Workspace{Registration: &Registration{ID: "agent-x"}})
	_, err := c.RequestApproval(context.Background(), ApprovalRequest{Action: "x"})
	if !errors.Is(err, ErrSecurityUnavailable) {
		t.Fatalf("err = %v, want ErrSecurityUnavailable", err)
	}
}

func TestSecurityControllerRequestApprovalWithoutRegistration(t *testing.T) {
	c := newSecurityController(&Workspace{})
	_, err := c.RequestApproval(context.Background(), ApprovalRequest{Action: "x"})
	if !errors.Is(err, ErrSecurityUnavailable) {
		t.Fatalf("err = %v, want ErrSecurityUnavailable", err)
	}
}

func TestSecurityControllerRequestApprovalUnsupportedBroker(t *testing.T) {
	c := newSecurityController(&Workspace{Registration: &Registration{HITL: struct{}{}}})
	_, err := c.RequestApproval(context.Background(), ApprovalRequest{Action: "x"})
	if !errors.Is(err, ErrSecurityUnavailable) {
		t.Fatalf("err = %v, want ErrSecurityUnavailable", err)
	}
}

func TestSecurityControllerRequestApprovalDeniedIsFailClosed(t *testing.T) {
	approver := &recordingHITLApprover{err: errors.New("permission denied: nope")}
	c := newSecurityController(&Workspace{Registration: &Registration{HITL: approver}})

	decision, err := c.RequestApproval(context.Background(), ApprovalRequest{Action: "x"})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision.Approved {
		t.Fatal("denied request reported as approved")
	}
	if decision.Reason == "" {
		t.Fatal("denied request missing reason")
	}
}
