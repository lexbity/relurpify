package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestUpdateNodeCapabilitiesPersistsApprovedSet(t *testing.T) {
	t.Parallel()

	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer nodeStore.Close()

	require.NoError(t, nodeStore.UpsertNode(context.Background(), core.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-1",
		Name:       "Node One",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassRemoteApproved,
		PairedAt:   time.Now().UTC(),
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
	}))

	svc := NewService(ServiceConfig{Nodes: nodeStore}).(*service)
	result, err := svc.UpdateNodeCapabilities(context.Background(), UpdateNodeCapabilitiesRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "admin-1"},
			},
			TenantID: "tenant-1",
		},
		NodeID: "node-1",
		Capabilities: []core.CapabilityDescriptor{
			{ID: "camera.capture", Name: "camera.capture", Kind: core.CapabilityKindTool},
			{ID: "camera.capture", Name: "camera.capture", Kind: core.CapabilityKindTool},
			{Name: "microphone.listen", Kind: core.CapabilityKindTool},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result.Node)
	require.Len(t, result.Node.ApprovedCapabilities, 2)

	stored, err := nodeStore.GetNode(context.Background(), "node-1")
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Len(t, stored.ApprovedCapabilities, 2)
	require.Equal(t, "camera.capture", stored.ApprovedCapabilities[0].ID)
	require.Equal(t, "microphone.listen", stored.ApprovedCapabilities[1].Name)
}

func TestUpdateNodeCapabilitiesDeniesCrossTenantAccess(t *testing.T) {
	t.Parallel()

	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer nodeStore.Close()

	require.NoError(t, nodeStore.UpsertNode(context.Background(), core.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-b",
		Name:       "Node One",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassRemoteApproved,
		PairedAt:   time.Now().UTC(),
		Owner:      core.SubjectRef{TenantID: "tenant-b", Kind: core.SubjectKindNode, ID: "node-1"},
	}))

	svc := NewService(ServiceConfig{Nodes: nodeStore}).(*service)
	_, err = svc.UpdateNodeCapabilities(context.Background(), UpdateNodeCapabilitiesRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-1"},
			},
			TenantID: "tenant-b",
		},
		NodeID:       "node-1",
		Capabilities: []core.CapabilityDescriptor{{ID: "camera.capture", Name: "camera.capture", Kind: core.CapabilityKindTool}},
	})
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorPolicyDenied, adminErr.Code)
}
