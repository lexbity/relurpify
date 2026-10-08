package testsupport

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestMockHITLBrokerFailsClosedByDefault(t *testing.T) {
	broker := NewMockHITLBroker()
	_, err := broker.RequestPermission(context.Background(), authorization.PermissionRequest{
		Permission: permissions.PermissionDescriptor{Action: "euclo.policy.gate"},
	})
	if err == nil {
		t.Fatal("expected a fail-closed error with no configured response")
	}
	if got := len(broker.Requests()); got != 1 {
		t.Fatalf("recorded requests = %d, want 1", got)
	}
}

func TestMockHITLBrokerAutoApprove(t *testing.T) {
	broker := NewAutoApproveHITLBroker()
	grant, err := broker.RequestPermission(context.Background(), authorization.PermissionRequest{
		Permission: permissions.PermissionDescriptor{Action: "euclo.policy.gate"},
	})
	if err != nil {
		t.Fatalf("auto-approve returned error: %v", err)
	}
	if grant == nil || grant.ApprovedBy != "testsupport" {
		t.Fatalf("unexpected grant: %#v", grant)
	}
}
