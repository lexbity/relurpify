package authorization

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"github.com/stretchr/testify/require"
)

// unavailableAuditLogger mimics a chain logger whose writer is stalled past
// the enqueue timeout: every strict grant's durable enqueue fails.
type unavailableAuditLogger struct{}

func (unavailableAuditLogger) Log(context.Context, policy.AuditRecord) error {
	return policy.ErrAuditUnavailable
}
func (unavailableAuditLogger) Query(context.Context, policy.AuditQuery) ([]policy.AuditRecord, error) {
	return nil, nil
}

// TestGrantFailsClosedWhenAuditUnavailable is the grant-fails-closed
// integration (SBH-1 INV-5): when the strict audit chain cannot durably
// accept a governed grant, the grant MUST fail with the audit reason instead
// of silently proceeding unrecorded.
func TestGrantFailsClosedWhenAuditUnavailable(t *testing.T) {
	declared := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{{Binary: "echo"}},
	}
	pm, err := NewPermissionManager("/tmp", declared, unavailableAuditLogger{}, nil)
	require.NoError(t, err)

	err = pm.CheckExecutable(context.Background(), "agent-1", "echo", nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, policy.ErrAuditUnavailable)
	require.Contains(t, err.Error(), "audit unavailable — action not performed")
}

// TestDenyStillDeniesWhenAuditUnavailable: a denial must remain a denial even
// when the audit enqueue fails — audit failure never converts a denial into a
// grant.
func TestDenyStillDeniesWhenAuditUnavailable(t *testing.T) {
	declared := &ucperms.PermissionSet{
		Executables: []ucperms.ExecutablePermission{{Binary: "echo"}},
	}
	pm, err := NewPermissionManager("/tmp", declared, unavailableAuditLogger{}, nil)
	require.NoError(t, err)
	_ = pm.SetDefaultDecision(permissions.DecisionDeny)

	err = pm.CheckExecutable(context.Background(), "agent-1", "bash", nil, nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, policy.ErrAuditUnavailable)
}
