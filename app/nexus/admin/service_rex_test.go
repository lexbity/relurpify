package admin

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	rexnexus "github.com/lexcodex/relurpify/named/rex/nexus"
	rexproof "github.com/lexcodex/relurpify/named/rex/proof"
	rexruntime "github.com/lexcodex/relurpify/named/rex/runtime"
	"github.com/stretchr/testify/require"
)

func TestDescribeRexRuntime(t *testing.T) {
	t.Parallel()

	svc := NewService(ServiceConfig{
		RexRuntime: fakeAdminRexRuntime{},
	}).(*service)

	result, err := svc.DescribeRexRuntime(context.Background(), DescribeRexRuntimeRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "rex", result.Registration.Name)
	require.Equal(t, rexruntime.HealthHealthy, result.Runtime.Health)
}

func TestReadRexAdminSnapshot(t *testing.T) {
	t.Parallel()

	svc := NewService(ServiceConfig{
		RexRuntime: fakeAdminRexRuntime{},
	}).(*service)

	result, err := svc.ReadRexAdminSnapshot(context.Background(), ReadRexAdminSnapshotRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
	})
	require.NoError(t, err)
	require.Equal(t, rexruntime.HealthHealthy, result.Snapshot.Runtime.Health)
	require.Equal(t, "pass", result.Snapshot.Runtime.LastProof.VerificationStatus)
}

type fakeAdminRexRuntime struct{}

func (fakeAdminRexRuntime) Registration() rexnexus.Registration {
	return rexnexus.Registration{Name: "rex", RuntimeType: "nexus-managed", Managed: true}
}

func (fakeAdminRexRuntime) RuntimeProjection() rexnexus.Projection {
	return rexnexus.Projection{Health: rexruntime.HealthHealthy, LastProof: rexproof.ProofSurface{VerificationStatus: "pass"}}
}

func (fakeAdminRexRuntime) AdminSnapshot(context.Context) (rexnexus.AdminSnapshot, error) {
	return rexnexus.AdminSnapshot{Runtime: fakeAdminRexRuntime{}.RuntimeProjection()}, nil
}
