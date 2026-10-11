package authorization

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/policy"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

func permissiveTestRequest() PermissionRequest {
	return PermissionRequest{
		Permission: ucperms.PermissionDescriptor{
			Type:         ucperms.PermissionTypeExecutable,
			Action:       "execute",
			Resource:     "bash",
			RequiresHITL: true,
		},
		Justification: "run the build",
		Scope:         policy.GrantScopeSession,
	}
}

// TestAutoApprovingBrokerApprovesImmediately proves the permissive posture:
// a request returns an approved grant synchronously — no pending entry, no
// waiter, no timeout dependency — and every decision reaches the audit hook.
func TestAutoApprovingBrokerApprovesImmediately(t *testing.T) {
	var audited []PermissionDecision
	broker := NewAutoApprovingHITLBroker(time.Hour, nil, func(req PermissionRequest, decision PermissionDecision) {
		audited = append(audited, decision)
	})
	t.Cleanup(broker.Stop)

	done := make(chan *PermissionGrant, 1)
	errCh := make(chan error, 1)
	go func() {
		grant, err := broker.RequestPermission(context.Background(), permissiveTestRequest())
		if err != nil {
			errCh <- err
			return
		}
		done <- grant
	}()

	select {
	case grant := <-done:
		require.NotNil(t, grant)
		require.Equal(t, "hitl:permissive-mode", grant.ApprovedBy)
		require.Equal(t, policy.GrantScopeSession, grant.Scope)
		require.Equal(t, "permissive", grant.Conditions["posture"])
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("permissive broker blocked — the posture must never pend")
	}

	require.Empty(t, broker.PendingRequests(), "no request may pend under the permissive posture")
	require.Len(t, audited, 1, "every auto-approval reaches the audit hook")
	require.True(t, audited[0].Approved)
	require.Equal(t, "hitl:permissive-mode", audited[0].ApprovedBy)
}

// TestAutoApprovingSubmitAsyncNeverPends proves the async path resolves
// immediately under the posture: the returned ID is the audited decision's ID
// and nothing appears in the pending set.
func TestAutoApprovingSubmitAsyncNeverPends(t *testing.T) {
	var audited []PermissionDecision
	broker := NewAutoApprovingHITLBroker(time.Hour, nil, func(req PermissionRequest, decision PermissionDecision) {
		audited = append(audited, decision)
	})
	t.Cleanup(broker.Stop)

	id, err := broker.SubmitAsync(context.Background(), permissiveTestRequest())
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.Equal(t, id, audited[0].RequestID)
	require.Empty(t, broker.PendingRequests())
}

// TestGovernedBrokerStillPends pins the default: without the posture, a
// request blocks until resolved (or cancelled) — the permissive constructor
// must be the only way in.
func TestGovernedBrokerStillPends(t *testing.T) {
	broker := NewHITLBroker(time.Hour, nil)
	t.Cleanup(broker.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := broker.RequestPermission(ctx, permissiveTestRequest())
	require.Error(t, err, "the governed broker must block on a decision (ctx expiry here)")
	require.Empty(t, broker.PendingRequests(), "the cancelled request is reaped")
}
