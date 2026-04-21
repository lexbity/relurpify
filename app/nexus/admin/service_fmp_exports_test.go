package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"github.com/stretchr/testify/require"
)

func TestSetAndListTenantFMPExports(t *testing.T) {
	t.Parallel()

	exportStore, err := db.NewSQLiteFMPExportStore(filepath.Join(t.TempDir(), "fmp_exports.db"))
	require.NoError(t, err)
	defer exportStore.Close()

	svc := NewService(ServiceConfig{FMPExports: exportStore}).(*service)
	_, err = svc.SetTenantFMPExport(context.Background(), SetTenantFMPExportRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "admin"},
			},
			TenantID: "tenant-1",
		},
		ExportName: "exp.run",
		Enabled:    false,
	})
	require.NoError(t, err)

	result, err := svc.ListTenantFMPExports(context.Background(), ListTenantFMPExportsRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "admin"},
			},
			TenantID: "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Exports, 1)
	require.Equal(t, "exp.run", result.Exports[0].ExportName)
	require.False(t, result.Exports[0].Enabled)
}

func TestTenantExportStoreFeedsFMPRouteAndAcceptPolicy(t *testing.T) {
	t.Parallel()

	exportStore, err := db.NewSQLiteFMPExportStore(filepath.Join(t.TempDir(), "fmp_exports.db"))
	require.NoError(t, err)
	defer exportStore.Close()
	require.NoError(t, exportStore.SetTenantExportEnabled(context.Background(), "tenant-1", "exp.run", false))

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	discovery := &fwfmp.InMemoryDiscoveryStore{}
	ownership := &fwfmp.InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	require.NoError(t, ownership.CreateLineage(context.Background(), lineage))
	require.NoError(t, ownership.UpsertAttempt(context.Background(), core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: now,
	}))

	mesh := &fwfmp.Service{
		Ownership: ownership,
		Discovery: discovery,
		Nexus:     fwfmp.NexusAdapter{Exports: exportStore},
		Now:       func() time.Time { return now },
	}
	require.NoError(t, mesh.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "local.mesh",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-local",
			NodeID:                  "node-local",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          1024,
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-local"},
			Signature:               "sig-rt-local",
		},
		Signature: "sig-rt-local",
	}))
	require.NoError(t, mesh.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "local.mesh",
		RuntimeID:   "rt-local",
		NodeID:      "node-local",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}))

	routes, err := mesh.ResolveRoutes(context.Background(), fwfmp.RouteSelectionRequest{
		TenantID:         "tenant-1",
		ExportName:       "exp.run",
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 64,
	})
	require.NoError(t, err)
	require.Empty(t, routes)
}
