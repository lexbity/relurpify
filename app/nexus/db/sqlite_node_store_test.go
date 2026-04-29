package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet"
	"codeburg.org/lexbit/relurpify/relurpnet/node"
	"github.com/stretchr/testify/require"
)

func TestSQLiteNodeStoreListPendingPairingsFiltersExpiredRows(t *testing.T) {
	store, err := NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Now().UTC()
	require.NoError(t, store.SavePendingPairing(context.Background(), node.PendingPairing{
		Code: "expired",
		Cred: node.NodeCredential{
			DeviceID:  "node-expired",
			IssuedAt:  now.Add(-2 * time.Minute),
			PublicKey: []byte("expired-key"),
		},
		ExpiresAt: now.Add(-time.Minute),
	}))
	require.NoError(t, store.SavePendingPairing(context.Background(), node.PendingPairing{
		Code: "active",
		Cred: node.NodeCredential{
			DeviceID:  "node-active",
			IssuedAt:  now.Add(-time.Minute),
			PublicKey: []byte("active-key"),
		},
		ExpiresAt: now.Add(time.Minute),
	}))

	pairings, err := store.ListPendingPairings(context.Background())
	require.NoError(t, err)
	require.Len(t, pairings, 1)
	require.Equal(t, "active", pairings[0].Code)

	expired, err := store.GetPendingPairing(context.Background(), "expired")
	require.NoError(t, err)
	require.Nil(t, expired)
}

func TestSQLiteNodeStoreDeleteExpiredPendingPairingsRemovesExpiredRows(t *testing.T) {
	store, err := NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Now().UTC()
	require.NoError(t, store.SavePendingPairing(context.Background(), node.PendingPairing{
		Code: "expired-1",
		Cred: node.NodeCredential{
			DeviceID:  "node-1",
			IssuedAt:  now.Add(-3 * time.Minute),
			PublicKey: []byte("expired-key-1"),
		},
		ExpiresAt: now.Add(-2 * time.Minute),
	}))
	require.NoError(t, store.SavePendingPairing(context.Background(), node.PendingPairing{
		Code: "expired-2",
		Cred: node.NodeCredential{
			DeviceID:  "node-2",
			IssuedAt:  now.Add(-2 * time.Minute),
			PublicKey: []byte("expired-key-2"),
		},
		ExpiresAt: now.Add(-time.Second),
	}))
	require.NoError(t, store.SavePendingPairing(context.Background(), node.PendingPairing{
		Code: "active",
		Cred: node.NodeCredential{
			DeviceID:  "node-3",
			IssuedAt:  now.Add(-time.Minute),
			PublicKey: []byte("active-key"),
		},
		ExpiresAt: now.Add(time.Minute),
	}))

	deleted, err := store.DeleteExpiredPendingPairings(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, 2, deleted)

	pairings, err := store.ListPendingPairings(context.Background())
	require.NoError(t, err)
	require.Len(t, pairings, 1)
	require.Equal(t, "active", pairings[0].Code)
}

func TestSQLiteNodeStorePersistsTenantOwnerAndCredentialMetadata(t *testing.T) {
	store, err := NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.UpsertNode(context.Background(), node.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-1",
		Name:       "Node One",
		Platform:   relurpnet.NodePlatformLinux,
		TrustClass: core.TrustClassWorkspaceTrusted,
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		PairedAt:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		ApprovedCapabilities: []core.CapabilityDescriptor{{
			ID:   "camera.capture",
			Name: "camera.capture",
			Kind: core.CapabilityKindTool,
		}},
	}))
	require.NoError(t, store.SaveCredential(context.Background(), node.NodeCredential{
		DeviceID:  "node-1",
		TenantID:  "tenant-1",
		PublicKey: []byte("public-key"),
		KeyID:     "key-1",
		IssuedAt:  time.Date(2026, 3, 10, 12, 1, 0, 0, time.UTC),
	}))

	nodeDesc, err := store.GetNode(context.Background(), "node-1")
	require.NoError(t, err)
	require.NotNil(t, nodeDesc)
	require.Equal(t, "tenant-1", nodeDesc.TenantID)
	require.Equal(t, core.SubjectKindNode, nodeDesc.Owner.Kind)
	require.Len(t, nodeDesc.ApprovedCapabilities, 1)
	require.Equal(t, "camera.capture", nodeDesc.ApprovedCapabilities[0].ID)

	cred, err := store.GetCredential(context.Background(), "node-1")
	require.NoError(t, err)
	require.NotNil(t, cred)
	require.Equal(t, "tenant-1", cred.TenantID)
	require.Equal(t, "key-1", cred.KeyID)
}
