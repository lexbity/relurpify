package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/core"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	"github.com/stretchr/testify/require"
)

func TestListFMPContinuationsFiltersToAuthorizedTenant(t *testing.T) {
	t.Parallel()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership}
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	require.NoError(t, ownership.CreateLineage(context.Background(), core.LineageRecord{
		LineageID:      "lineage-a",
		TenantID:       "tenant-a",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		UpdatedAt:      now,
		LineageVersion: 1,
	}))
	require.NoError(t, ownership.CreateLineage(context.Background(), core.LineageRecord{
		LineageID:      "lineage-b",
		TenantID:       "tenant-b",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          core.SubjectRef{TenantID: "tenant-b", Kind: core.SubjectKindServiceAccount, ID: "svc-b"},
		UpdatedAt:      now.Add(time.Minute),
		LineageVersion: 2,
	}))

	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	result, err := svc.ListFMPContinuations(context.Background(), ListFMPContinuationsRequest{
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
	lineage := core.LineageRecord{
		LineageID:      "lineage-1",
		TenantID:       "tenant-a",
		TaskClass:      "agent.run",
		ContextClass:   "workflow-runtime",
		Owner:          core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		SessionID:      "sess-1",
		UpdatedAt:      now,
		LineageVersion: 1,
	}
	require.NoError(t, ownership.CreateLineage(context.Background(), lineage))
	_, err = eventLog.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Timestamp: now,
			Type:      core.FrameworkEventFMPResumeCommitted,
			Actor:     core.EventActor{Kind: "service_account", ID: "svc-a", TenantID: "tenant-a", SubjectKind: core.SubjectKindServiceAccount},
			Partition: "local",
			Payload:   []byte(`{"lineage_id":"lineage-1","new_attempt":"attempt-b"}`),
		},
		{
			Timestamp: now,
			Type:      core.FrameworkEventFMPResumeCommitted,
			Actor:     core.EventActor{Kind: "service_account", ID: "svc-a", TenantID: "tenant-a", SubjectKind: core.SubjectKindServiceAccount},
			Partition: "local",
			Payload:   []byte(`{"lineage_id":"lineage-2","new_attempt":"attempt-c"}`),
		},
		{
			Timestamp: now,
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: "local",
			Payload:   []byte(`{"channel":"webchat"}`),
		},
	})
	require.NoError(t, err)

	svc := NewService(ServiceConfig{FMP: mesh, Events: eventLog, Partition: "local"}).(*service)
	result, err := svc.ReadFMPContinuationAudit(context.Background(), ReadFMPContinuationAuditRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
		LineageID: "lineage-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Lineage)
	require.Equal(t, "lineage-1", result.Lineage.LineageID)
	require.Len(t, result.Events, 1)
	require.Equal(t, core.FrameworkEventFMPResumeCommitted, result.Events[0].Type)
}

func TestReadFMPContinuationAuditDeniesCrossTenantAccess(t *testing.T) {
	t.Parallel()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{Ownership: ownership}
	require.NoError(t, ownership.CreateLineage(context.Background(), core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-a",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
	}))

	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	_, err := svc.ReadFMPContinuationAudit(context.Background(), ReadFMPContinuationAuditRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-b",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       core.SubjectRef{TenantID: "tenant-b", Kind: core.SubjectKindServiceAccount, ID: "admin-b"},
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
