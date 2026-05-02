package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestListFMPContinuationsFiltersToAuthorizedTenant(t *testing.T) {
	t.Parallel()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership}
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	require.NoError(t, ownership.CreateLineage(context.Background(), fwfmp.LineageRecord{
		LineageID:      "lineage-a",
		TenantID:       "tenant-a",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "svc-a"},
		UpdatedAt:      now,
		LineageVersion: 1,
	}))
	require.NoError(t, ownership.CreateLineage(context.Background(), fwfmp.LineageRecord{
		LineageID:      "lineage-b",
		TenantID:       "tenant-b",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          identity.SubjectRef{TenantID: "tenant-b", Kind: identity.SubjectKindServiceAccount, ID: "svc-b"},
		UpdatedAt:      now.Add(time.Minute),
		LineageVersion: 2,
	}))

	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	result, err := svc.ListFMPContinuations(context.Background(), ListFMPContinuationsRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Continuations, 1)
	require.Equal(t, "lineage-a", result.Continuations[0].LineageID)
	require.Equal(t, "tenant-a", result.Continuations[0].TenantID)
}

func TestReadFMPContinuationAuditFiltersByLineage(t *testing.T) {
	t.Parallel()

	eventLog, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership}
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	lineage := fwfmp.LineageRecord{
		LineageID:      "lineage-1",
		TenantID:       "tenant-a",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "svc-a"},
		SessionID:      "sess-1",
		UpdatedAt:      now,
		LineageVersion: 1,
	}
	require.NoError(t, ownership.CreateLineage(context.Background(), lineage))
	_, err = eventLog.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Timestamp: now,
			Type:      fwfmp.FrameworkEventFMPResumeCommitted,
			Actor:     identity.EventActor{Kind: "service_account", ID: "svc-a", TenantID: "tenant-a", SubjectKind: string(identity.SubjectKindServiceAccount)},
			Partition: "local",
			Payload:   []byte(`{"lineage_id":"lineage-1","new_attempt":"attempt-b"}`),
		},
		{
			Timestamp: now,
			Type:      fwfmp.FrameworkEventFMPResumeCommitted,
			Actor:     identity.EventActor{Kind: "service_account", ID: "svc-a", TenantID: "tenant-a", SubjectKind: string(identity.SubjectKindServiceAccount)},
			Partition: "local",
			Payload:   []byte(`{"lineage_id":"lineage-2","new_attempt":"attempt-c"}`),
		},
		{
			Timestamp: now,
			Type:      core.FrameworkEventMessageInbound,
			Actor:     identity.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: "local",
			Payload:   []byte(`{"channel":"webchat"}`),
		},
	})
	require.NoError(t, err)

	svc := NewService(ServiceConfig{FMP: mesh, Events: eventLog, Partition: "local"}).(*service)
	result, err := svc.ReadFMPContinuationAudit(context.Background(), ReadFMPContinuationAuditRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
		LineageID: "lineage-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Lineage)
	require.Equal(t, "lineage-1", result.Lineage.LineageID)
	require.Len(t, result.Events, 1)
	require.Equal(t, fwfmp.FrameworkEventFMPResumeCommitted, result.Events[0].Type)
}

func TestReadFMPContinuationAuditIncludesChainVerification(t *testing.T) {
	t.Parallel()

	signer := fwfmp.NewEd25519SignerFromSeed([]byte("admin-audit-chain"))
	auditStore, err := db.NewSQLiteAuditChainStore(
		filepath.Join(t.TempDir(), "audit.db"),
		signer,
		&fwfmp.Ed25519Verifier{PublicKey: signer.PublicKey()},
	)
	require.NoError(t, err)
	defer auditStore.Close()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership, Audit: auditStore}
	require.NoError(t, ownership.CreateLineage(context.Background(), fwfmp.LineageRecord{
		LineageID:      "lineage-1",
		TenantID:       "tenant-a",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "svc-a"},
		UpdatedAt:      time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		LineageVersion: 1,
	}))
	require.NoError(t, auditStore.Log(context.Background(), core.AuditRecord{
		AgentID:    "runtime-a",
		Action:     "fmp",
		Type:       fwfmp.FrameworkEventFMPHandoffOffered,
		Permission: "mesh",
		Result:     "ok",
		Metadata: map[string]any{
			"lineage_id": "lineage-1",
			"offer_id":   "offer-1",
		},
	}))

	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	result, err := svc.ReadFMPContinuationAudit(context.Background(), ReadFMPContinuationAuditRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
		LineageID: "lineage-1",
	})
	require.NoError(t, err)
	require.Len(t, result.AuditChain, 1)
	require.NotNil(t, result.Verification)
	require.True(t, result.Verification.Verified)
}

func TestVerifyFMPAuditTrailReportsIntegrity(t *testing.T) {
	t.Parallel()

	signer := fwfmp.NewEd25519SignerFromSeed([]byte("admin-audit-verify"))
	auditStore, err := db.NewSQLiteAuditChainStore(
		filepath.Join(t.TempDir(), "audit.db"),
		signer,
		&fwfmp.Ed25519Verifier{PublicKey: signer.PublicKey()},
	)
	require.NoError(t, err)
	defer auditStore.Close()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership, Audit: auditStore}
	require.NoError(t, ownership.CreateLineage(context.Background(), fwfmp.LineageRecord{
		LineageID:      "lineage-1",
		TenantID:       "tenant-a",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "svc-a"},
		UpdatedAt:      time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		LineageVersion: 1,
	}))
	require.NoError(t, auditStore.Log(context.Background(), core.AuditRecord{
		AgentID:    "runtime-a",
		Action:     "fmp",
		Type:       fwfmp.FrameworkEventFMPHandoffOffered,
		Permission: "mesh",
		Result:     "ok",
		Metadata:   map[string]any{"lineage_id": "lineage-1"},
	}))

	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	result, err := svc.VerifyFMPAuditTrail(context.Background(), VerifyFMPAuditTrailRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
		LineageID: "lineage-1",
	})
	require.NoError(t, err)
	require.True(t, result.Verification.Verified)
	require.Equal(t, 1, result.Verification.EntryCount)
}

func TestReadFMPContinuationAuditDeniesCrossTenantAccess(t *testing.T) {
	t.Parallel()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership}
	require.NoError(t, ownership.CreateLineage(context.Background(), fwfmp.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-a",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        identity.SubjectRef{TenantID: "tenant-a", Kind: identity.SubjectKindServiceAccount, ID: "svc-a"},
	}))

	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	_, err := svc.ReadFMPContinuationAudit(context.Background(), ReadFMPContinuationAuditRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-b",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       identity.SubjectRef{TenantID: "tenant-b", Kind: identity.SubjectKindServiceAccount, ID: "admin-b"},
			},
			TenantID: "tenant-a",
		},
		LineageID: "lineage-1",
	})
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorPolicyDenied, adminErr.Code)
}
