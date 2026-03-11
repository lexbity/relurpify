package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/db"
	nexusserver "github.com/lexcodex/relurpify/app/nexus/server"
	"github.com/lexcodex/relurpify/framework/core"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/lexcodex/relurpify/framework/middleware/session"
	"github.com/stretchr/testify/require"
)

type runtimeNodeStore struct{}

func (runtimeNodeStore) GetNode(context.Context, string) (*core.NodeDescriptor, error) {
	return nil, nil
}
func (runtimeNodeStore) ListNodes(context.Context) ([]core.NodeDescriptor, error) { return nil, nil }
func (runtimeNodeStore) UpsertNode(context.Context, core.NodeDescriptor) error    { return nil }
func (runtimeNodeStore) RemoveNode(context.Context, string) error                 { return nil }
func (runtimeNodeStore) GetCredential(context.Context, string) (*core.NodeCredential, error) {
	return nil, nil
}
func (runtimeNodeStore) SaveCredential(context.Context, core.NodeCredential) error { return nil }
func (runtimeNodeStore) SavePendingPairing(context.Context, fwnode.PendingPairing) error {
	return nil
}
func (runtimeNodeStore) GetPendingPairing(context.Context, string) (*fwnode.PendingPairing, error) {
	return nil, nil
}
func (runtimeNodeStore) ListPendingPairings(context.Context) ([]fwnode.PendingPairing, error) {
	return nil, nil
}
func (runtimeNodeStore) DeletePendingPairing(context.Context, string) error { return nil }
func (runtimeNodeStore) DeleteExpiredPendingPairings(context.Context, time.Time) (int, error) {
	return 0, nil
}

type runtimeSessionStore struct {
	boundaries map[string]core.SessionBoundary
}

func (s runtimeSessionStore) GetBoundary(context.Context, string) (*core.SessionBoundary, error) {
	return nil, nil
}
func (s runtimeSessionStore) GetBoundaryBySessionID(_ context.Context, sessionID string) (*core.SessionBoundary, error) {
	if boundary, ok := s.boundaries[sessionID]; ok {
		copy := boundary
		return &copy, nil
	}
	return nil, nil
}
func (s runtimeSessionStore) UpsertBoundary(context.Context, string, *core.SessionBoundary) error {
	return nil
}
func (s runtimeSessionStore) UpsertDelegation(context.Context, core.SessionDelegationRecord) error {
	return nil
}
func (s runtimeSessionStore) ListDelegationsBySessionID(context.Context, string) ([]core.SessionDelegationRecord, error) {
	return nil, nil
}
func (s runtimeSessionStore) ListBoundaries(context.Context, string) ([]core.SessionBoundary, error) {
	return nil, nil
}
func (s runtimeSessionStore) DeleteBoundary(context.Context, string) error { return nil }
func (s runtimeSessionStore) DeleteExpiredBoundaries(context.Context, time.Time) (int, error) {
	return 0, nil
}

type runtimeNodeConnection struct {
	node core.NodeDescriptor
	caps []core.CapabilityDescriptor
}

func (c runtimeNodeConnection) Node() core.NodeDescriptor { return c.node }
func (c runtimeNodeConnection) Health() core.NodeHealth {
	return core.NodeHealth{Online: true, Foreground: true}
}
func (c runtimeNodeConnection) Capabilities() []core.CapabilityDescriptor {
	return append([]core.CapabilityDescriptor(nil), c.caps...)
}
func (c runtimeNodeConnection) Invoke(context.Context, string, map[string]any) (*core.CapabilityExecutionResult, error) {
	return &core.CapabilityExecutionResult{Success: true, Data: map[string]any{"ok": true}}, nil
}
func (c runtimeNodeConnection) Close(context.Context) error { return nil }

func TestListNodeCapabilitiesAndInvoke(t *testing.T) {
	manager := &fwnode.Manager{Store: runtimeNodeStore{}}
	conn := runtimeNodeConnection{
		node: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformLinux,
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		caps: []core.CapabilityDescriptor{{
			ID:   "camera.capture",
			Name: "camera.capture",
			Kind: core.CapabilityKindTool,
		}},
	}
	require.NoError(t, manager.HandleConnect(context.Background(), conn))

	caps := nexusserver.ListNodeCapabilities(manager, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"},
	})
	require.Len(t, caps, 1)
	require.Equal(t, "node:node-1", caps[0].Source.ProviderID)

	result, err := nexusserver.InvokeNodeCapability(context.Background(), manager, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"},
	}, "camera.capture", map[string]any{"quality": "high"})
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestListNodeCapabilitiesAndInvokeFiltersByTenant(t *testing.T) {
	manager := &fwnode.Manager{Store: runtimeNodeStore{}}
	require.NoError(t, manager.HandleConnect(context.Background(), runtimeNodeConnection{
		node: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-a",
			Name:       "Node A",
			Platform:   core.NodePlatformLinux,
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		caps: []core.CapabilityDescriptor{{ID: "camera.capture", Name: "camera.capture", Kind: core.CapabilityKindTool}},
	}))

	caps := nexusserver.ListNodeCapabilities(manager, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-b"},
	})
	require.Empty(t, caps)

	_, err := nexusserver.InvokeNodeCapability(context.Background(), manager, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-b"},
	}, "camera.capture", map[string]any{"quality": "high"})
	require.Error(t, err)
}

func TestNewNodeDisconnectContextIgnoresParentCancellationAndSetsDeadline(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	disconnectCtx, cancel := nexusserver.NewNodeDisconnectContext(parent)
	defer cancel()

	require.NoError(t, disconnectCtx.Err())

	deadline, ok := disconnectCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(nexusserver.NodeDisconnectTimeout), deadline, time.Second)
}

func TestInvokeAuthorizedNodeCapabilityRequiresSessionOwnership(t *testing.T) {
	manager := &fwnode.Manager{Store: runtimeNodeStore{}}
	require.NoError(t, manager.HandleConnect(context.Background(), runtimeNodeConnection{
		node: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformLinux,
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		caps: []core.CapabilityDescriptor{{ID: "camera.capture", Name: "camera.capture", Kind: core.CapabilityKindTool}},
	}))

	router := &session.DefaultRouter{}
	store := runtimeSessionStore{
		boundaries: map[string]core.SessionBoundary{
			"sess-1": {
				SessionID: "sess-1",
				Partition: "local",
				TenantID:  "tenant-1",
				Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
			},
		},
	}

	result, err := nexusserver.InvokeAuthorizedNodeCapability(context.Background(), router, store, manager, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "user-1", TenantID: "tenant-1", SubjectKind: core.SubjectKindUser},
	}, "sess-1", "camera.capture", map[string]any{"quality": "high"})
	require.NoError(t, err)
	require.True(t, result.Success)

	_, err = nexusserver.InvokeAuthorizedNodeCapability(context.Background(), router, store, manager, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "user-2", TenantID: "tenant-1", SubjectKind: core.SubjectKindUser},
	}, "sess-1", "camera.capture", map[string]any{"quality": "high"})
	require.ErrorIs(t, err, session.ErrSessionBoundaryViolation)
}

func TestConnectedNodeDescriptorUsesEnrollmentAndStoredNodeState(t *testing.T) {
	store, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer nodeStore.Close()

	now := time.Now().UTC()
	require.NoError(t, store.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-1",
		TrustClass: core.TrustClassRemoteApproved,
		Owner: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindNode,
			ID:       "node-1",
		},
		PublicKey:  []byte("pk"),
		PairedAt:   now,
		AuthMethod: core.AuthMethodNodeChallenge,
	}))
	require.NoError(t, nodeStore.UpsertNode(context.Background(), core.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-1",
		Name:       "Stored Node",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassWorkspaceTrusted,
		PairedAt:   now,
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		Tags:       map[string]string{"rack": "a1"},
	}))

	manager := &fwnode.Manager{Store: nodeStore}
	desc, err := nexusserver.ConnectedNodeDescriptorForTest(context.Background(), manager, store, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "node", ID: "node-1", TenantID: "tenant-1", SubjectKind: core.SubjectKindNode},
	}, fwgateway.NodeConnectInfo{
		NodeID:       "node-1",
		NodeName:     "Spoofed Name",
		NodePlatform: string(core.NodePlatformAndroid),
		TrustClass:   string(core.TrustClassWorkspaceTrusted),
	})
	require.NoError(t, err)
	require.Equal(t, "node-1", desc.ID)
	require.Equal(t, "tenant-1", desc.TenantID)
	require.Equal(t, "Stored Node", desc.Name)
	require.Equal(t, core.NodePlatformLinux, desc.Platform)
	require.Equal(t, core.TrustClassRemoteApproved, desc.TrustClass)
	require.Equal(t, "node-1", desc.Owner.ID)
	require.Equal(t, "a1", desc.Tags["rack"])

	caps := nexusserver.ConnectedNodeCapabilitiesForTest(context.Background(), manager, "node-1")
	require.Empty(t, caps)
}

func TestConnectedNodeCapabilitiesComeFromStoredApproval(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer nodeStore.Close()

	now := time.Now().UTC()
	require.NoError(t, store.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-1",
		TrustClass: core.TrustClassRemoteApproved,
		Owner: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindNode,
			ID:       "node-1",
		},
		PublicKey:  []byte("pk"),
		PairedAt:   now,
		AuthMethod: core.AuthMethodNodeChallenge,
	}))
	require.NoError(t, nodeStore.UpsertNode(context.Background(), core.NodeDescriptor{
		ID:       "node-1",
		TenantID: "tenant-1",
		Name:     "Stored Node",
		Platform: core.NodePlatformLinux,
		ApprovedCapabilities: []core.CapabilityDescriptor{{
			ID:   "camera.capture",
			Name: "camera.capture",
			Kind: core.CapabilityKindTool,
		}},
		TrustClass: core.TrustClassRemoteApproved,
		PairedAt:   now,
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
	}))

	manager := &fwnode.Manager{Store: nodeStore}
	caps := nexusserver.ConnectedNodeCapabilitiesForTest(context.Background(), manager, "node-1")
	require.Len(t, caps, 1)
	require.Equal(t, "camera.capture", caps[0].ID)
}
