package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// TestAuditChainEndToEnd drives the authorization layer through denied,
// granted, and HITL-approved actions against a REAL file-backed hash chain and
// asserts each governed action produced the correct chain entry (SBH-1 D-10:
// exec, capability/ipc, and permission_request families are strict).
func TestAuditChainEndToEnd(t *testing.T) {
	audit := testhelper.NewTestAuditLogger(t)
	declared := &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "echo"}},
	}
	broker := testhelper.NewAutoApprovingBroker()
	pm, err := fauthorization.NewPermissionManager(t.TempDir(), declared, audit, broker)
	require.NoError(t, err)

	// 1. Denied exec (undeclared binary under a deny default).
	require.NoError(t, pm.SetDefaultDecision(permissions.DecisionDeny))
	err = pm.CheckExecutable(context.Background(), "agent-1", "bash", nil, nil)
	require.Error(t, err)

	// 2. Granted exec (declared binary).
	err = pm.CheckExecutable(context.Background(), "agent-1", "echo", nil, nil)
	require.NoError(t, err)

	// 3. HITL-approved tool (ask default + auto-approving broker). Tool
	// authorization records carry Type=hitl (permission_request family).
	require.NoError(t, pm.SetDefaultDecision(permissions.DecisionAsk))
	err = pm.AuthorizeToolByName(context.Background(), "agent-1", "file_read")
	require.NoError(t, err)

	records, err := audit.Query(context.Background(), policy.AuditQuery{AgentID: "agent-1"})
	require.NoError(t, err)

	var deniedExec, grantedExec, hitlApproval *policy.AuditRecord
	for i := range records {
		r := &records[i]
		switch {
		case r.Result == "denied" && r.Type == "executable":
			deniedExec = r
		case r.Result == "granted" && r.Type == "executable":
			grantedExec = r
		case r.Type == "hitl":
			hitlApproval = r
		}
	}

	require.NotNil(t, deniedExec, "denied exec must be chained")
	require.Equal(t, "exec:binary:bash", deniedExec.Action)
	require.NotEmpty(t, deniedExec.Permission)

	require.NotNil(t, grantedExec, "granted exec must be chained")
	require.Equal(t, "exec:echo", grantedExec.Action)

	require.NotNil(t, hitlApproval, "HITL approval must be chained")
	require.Equal(t, "tool_allowed", hitlApproval.Result)

	// The chain is tamper-evident: verify integrity end-to-end.
	v, err := audit.VerifyChain(context.Background(), policy.AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified, "chain must verify: %s", v.Failure)
	require.GreaterOrEqual(t, v.LastSequence, int64(3))
	require.NoError(t, audit.Close())
}
