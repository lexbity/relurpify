package admin

import (
	"context"
	"testing"

	rexcontrolplane "codeburg.org/lexbit/relurpify/named/rex/controlplane"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestReadRexSLOSignalsUsesProvider(t *testing.T) {
	t.Parallel()

	provider := &fakeRexSLOProvider{
		signals: rexcontrolplane.SLOSignals{
			TotalWorkflows:      8,
			RunningWorkflows:    3,
			CompletedWorkflows:  4,
			FailedWorkflows:     1,
			RecoverySensitive:   2,
			DegradedWorkflowIDs: []string{"wf-failed"},
		},
		cachedAt: 12345,
	}
	svc := NewService(ServiceConfig{
		RexRuntime:  fakeAdminRexRuntime{},
		RexProvider: provider,
	}).(*service)

	result, err := svc.ReadRexSLOSignals(context.Background(), ReadRexSLOSignalsRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:admin:global"},
				Subject:       identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Equal(t, 8, result.TotalWorkflows)
	require.Equal(t, 3, result.RunningWorkflows)
	require.Equal(t, 4, result.CompletedWorkflows)
	require.Equal(t, 1, result.FailedWorkflows)
	require.Equal(t, 2, result.RecoverySensitive)
	require.Equal(t, []string{"wf-failed"}, result.DegradedWorkflows)
	require.Equal(t, int64(12345), result.CachedAt)
}

type fakeRexSLOProvider struct {
	signals  rexcontrolplane.SLOSignals
	cachedAt int64
	calls    int
}

func (f *fakeRexSLOProvider) ReadSLOSignals(context.Context) (rexcontrolplane.SLOSignals, int64, error) {
	f.calls++
	return f.signals, f.cachedAt, nil
}
