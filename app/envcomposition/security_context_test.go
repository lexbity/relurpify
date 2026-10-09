package envcomposition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	gpermissions "codeburg.org/lexbit/relurpify/governance/permissions"
)

// ctxMarkerKey is a private context key used to prove ctx propagation (AC-10).
type ctxMarkerKey struct{}

// ctxCapturingHITL records the context on which an approval was requested.
type ctxCapturingHITL struct {
	ctx context.Context
}

func (h *ctxCapturingHITL) RequestPermission(ctx context.Context, _ fauthorization.PermissionRequest) (*fauthorization.PermissionGrant, error) {
	h.ctx = ctx
	return &fauthorization.PermissionGrant{ID: "grant-1"}, nil
}

// TestCommandPolicyContext proves the envcomposition command policy runs on the
// caller's context (D9): a value set by the command's caller must be visible to
// the authorization path and any HITL ask it raises.
func TestCommandPolicyContext(t *testing.T) {
	hitl := &ctxCapturingHITL{}
	manager, err := fauthorization.NewPermissionManager(t.TempDir(), &gpermissions.PermissionSet{}, nil, hitl)
	require.NoError(t, err)

	sec, err := BuildSecurityRuntime(context.Background(), SecurityRuntimeInput{
		ExistingRunner:    fakeRunner{},
		PermissionManager: manager,
		AgentID:           "test-agent",
	})
	require.NoError(t, err)
	require.NotNil(t, sec.CommandPolicy)

	marker := "caller-context-marker"
	ctx := context.WithValue(context.Background(), ctxMarkerKey{}, marker)

	// An undeclared executable escalates to HITL under the default ask policy,
	// which is where the propagated context is observable.
	require.NoError(t, sec.CommandPolicy.AllowCommand(ctx, sandbox.CommandRequest{Args: []string{"echo", "hi"}}))

	require.NotNil(t, hitl.ctx, "the command policy must reach HITL for an undeclared executable")
	require.Equal(t, marker, hitl.ctx.Value(ctxMarkerKey{}),
		"the request context must reach the authorization path, not the boot context")
}
