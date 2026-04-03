package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lexcodex/relurpify/app/nexus/db"
	nexusserver "github.com/lexcodex/relurpify/app/nexus/server"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/lexcodex/relurpify/framework/middleware/session"
	rexruntime "github.com/lexcodex/relurpify/named/rex/runtime"
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
	boundaries  map[string]core.SessionBoundary
	delegations map[string][]core.SessionDelegationRecord
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
func (s runtimeSessionStore) ListDelegationsBySessionID(_ context.Context, sessionID string) ([]core.SessionDelegationRecord, error) {
	records := s.delegations[sessionID]
	out := make([]core.SessionDelegationRecord, len(records))
	copy(out, records)
	return out, nil
}
func (s runtimeSessionStore) ListDelegationsByTenantID(context.Context, string) ([]core.SessionDelegationRecord, error) {
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

type nexusRuntimeEndpoint struct {
	descriptor core.RuntimeDescriptor
	pkg        *fwfmp.PortableContextPackage
	attempt    *core.AttemptRecord
	receipt    *core.ResumeReceipt
}

func (n nexusRuntimeEndpoint) Descriptor(context.Context) (core.RuntimeDescriptor, error) {
	return n.descriptor, nil
}

func (n nexusRuntimeEndpoint) ExportContext(context.Context, core.LineageRecord, core.AttemptRecord) (*fwfmp.PortableContextPackage, error) {
	return n.pkg, nil
}

func (n nexusRuntimeEndpoint) ValidateContext(context.Context, core.ContextManifest, core.SealedContext) error {
	return nil
}

func (n nexusRuntimeEndpoint) ImportContext(context.Context, core.LineageRecord, core.ContextManifest, core.SealedContext) (*fwfmp.PortableContextPackage, error) {
	return n.pkg, nil
}

func (n nexusRuntimeEndpoint) CreateAttempt(context.Context, core.LineageRecord, core.HandoffAccept, *fwfmp.PortableContextPackage) (*core.AttemptRecord, error) {
	return n.attempt, nil
}

func (n nexusRuntimeEndpoint) FenceAttempt(context.Context, core.FenceNotice) error {
	return nil
}

func (n nexusRuntimeEndpoint) IssueReceipt(context.Context, core.LineageRecord, core.AttemptRecord, *fwfmp.PortableContextPackage) (*core.ResumeReceipt, error) {
	return n.receipt, nil
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

func TestAdvertiseConnectedNodeToFMPSetsAuthoritativeRuntimeAttestation(t *testing.T) {
	store := &fwfmp.InMemoryDiscoveryStore{}
	mesh := &fwfmp.Service{Discovery: store}
	nodeDesc := core.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-1",
		Name:       "Node One",
		Platform:   core.NodePlatformHeadless,
		TrustClass: core.TrustClassRemoteApproved,
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		PairedAt:   time.Now().UTC(),
	}
	frame := fwgateway.NodeConnectInfo{
		NodeID:                  "node-1",
		TrustDomain:             "mesh.local",
		RuntimeID:               "runtime-1",
		RuntimeVersion:          "2.1.0",
		CompatibilityClass:      "compat-a",
		SupportedContextClasses: []string{"workflow-runtime"},
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	}
	require.NoError(t, nexusserver.AdvertiseConnectedNodeToFMP(context.Background(), mesh, nodeDesc, frame))
	runtimes, err := store.ListRuntimeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	require.Equal(t, "nexus.node_enrollment.v1", runtimes[0].Runtime.AttestationProfile)
	require.Equal(t, "key-1", runtimes[0].Runtime.AttestationClaims["peer_key_id"])
	require.Equal(t, fwgateway.TransportProfileWebSocketTLS, runtimes[0].Runtime.AttestationClaims["transport"])
	require.NotEmpty(t, runtimes[0].Runtime.Signature)
}

func TestAdvertiseConnectedNodeToFMPPrefersAttachedRexRuntimeMetadataOverSpoofedFrame(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery := &fwfmp.InMemoryDiscoveryStore{}
	mesh := &fwfmp.Service{Discovery: discovery}
	rexProvider, err := nexusserver.NewRexRuntimeProvider(ctx, t.TempDir())
	require.NoError(t, err)
	defer rexProvider.Close()
	rexProvider.AttachFMPService(mesh)

	rexDescriptor, err := rexProvider.RuntimeEndpoint.Descriptor(context.Background())
	require.NoError(t, err)

	nodeDesc := core.NodeDescriptor{
		ID:         rexDescriptor.NodeID,
		TenantID:   "tenant-1",
		Name:       "Stored Node",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassRemoteApproved,
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: rexDescriptor.NodeID},
		PairedAt:   time.Now().UTC(),
	}
	frame := fwgateway.NodeConnectInfo{
		NodeID:                  rexDescriptor.NodeID,
		TrustDomain:             "mesh.spoofed",
		RuntimeID:               "spoofed-runtime",
		RuntimeVersion:          "9.9.9-spoofed",
		CompatibilityClass:      "spoofed-compat",
		SupportedContextClasses: []string{"spoofed-context"},
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	}

	require.NoError(t, nexusserver.AdvertiseConnectedNodeToFMPWithRuntimeForTest(context.Background(), mesh, nodeDesc, frame, rexProvider))

	runtimes, err := discovery.ListRuntimeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	require.Equal(t, rexDescriptor.RuntimeID, runtimes[0].Runtime.RuntimeID)
	require.Equal(t, rexDescriptor.RuntimeVersion, runtimes[0].Runtime.RuntimeVersion)
	require.Equal(t, rexDescriptor.CompatibilityClass, runtimes[0].Runtime.CompatibilityClass)
	require.Equal(t, rexDescriptor.SupportedContextClasses, runtimes[0].Runtime.SupportedContextClasses)
	require.Equal(t, rexDescriptor.TrustDomain, runtimes[0].Runtime.TrustDomain)
	require.Equal(t, rexDescriptor.RuntimeVersion, runtimes[0].Runtime.AttestationClaims["runtime_version"])
	require.Equal(t, nodeDesc.ID, runtimes[0].Runtime.AttestationClaims["node_id"])
	require.Equal(t, nodeDesc.TenantID, runtimes[0].Runtime.AttestationClaims["tenant_id"])
	require.Equal(t, string(nodeDesc.TrustClass), runtimes[0].Runtime.AttestationClaims["trust_class"])
	require.Equal(t, frame.PeerKeyID, runtimes[0].Runtime.AttestationClaims["peer_key_id"])
}

func TestHandleGatewayNodeConnectionAdvertisesAuthoritativeRexRuntimeMetadata(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer nodeStore.Close()

	now := time.Now().UTC()
	require.NoError(t, identityStore.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:       "tenant-1",
		NodeID:         "node-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		PublicKey:      []byte("pk"),
		KeyID:          "key-1",
		PairedAt:       now,
		LastVerifiedAt: now,
		AuthMethod:     core.AuthMethodNodeChallenge,
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
	discovery := &fwfmp.InMemoryDiscoveryStore{}
	mesh := &fwfmp.Service{Discovery: discovery}
	rexProvider, err := nexusserver.NewRexRuntimeProvider(ctx, t.TempDir())
	require.NoError(t, err)
	defer rexProvider.Close()
	rexProvider.AttachFMPService(mesh)
	rexDescriptor, err := rexProvider.RuntimeEndpoint.Descriptor(context.Background())
	require.NoError(t, err)

	var (
		done   = make(chan error, 1)
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				done <- err
				return
			}
			done <- nexusserver.HandleGatewayNodeConnection(ctx, manager, identityStore, mesh, fwgateway.ConnectionPrincipal{
				Authenticated: true,
				Actor:         core.EventActor{Kind: "node", ID: "node-1", TenantID: "tenant-1", SubjectKind: core.SubjectKindNode},
			}, fwgateway.NodeConnectInfo{
				NodeID:           "node-1",
				TrustDomain:      "mesh.spoofed",
				RuntimeID:        "spoofed-runtime",
				RuntimeVersion:   "9.9.9-spoofed",
				CompatibilityClass: "spoofed-compat",
				SupportedContextClasses: []string{"spoofed-context"},
				TransportProfile: fwgateway.TransportProfileWebSocketTLS,
				PeerKeyID:        "key-1",
			}, conn, rexProvider)
		}))
	)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		runtimes, err := discovery.ListRuntimeAdvertisements(context.Background())
		require.NoError(t, err)
		if len(runtimes) != 1 {
			return false
		}
		nodes, err := discovery.ListNodeAdvertisements(context.Background())
		require.NoError(t, err)
		if len(nodes) != 1 {
			return false
		}
		return runtimes[0].Runtime.RuntimeID == rexDescriptor.RuntimeID && nodes[0].Node.Name == "Stored Node"
	}, 2*time.Second, 20*time.Millisecond)

	runtimes, err := discovery.ListRuntimeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	require.Equal(t, rexDescriptor.RuntimeID, runtimes[0].Runtime.RuntimeID)
	require.Equal(t, rexDescriptor.RuntimeVersion, runtimes[0].Runtime.RuntimeVersion)
	require.Equal(t, rexDescriptor.CompatibilityClass, runtimes[0].Runtime.CompatibilityClass)
	require.Equal(t, rexDescriptor.SupportedContextClasses, runtimes[0].Runtime.SupportedContextClasses)
	require.Equal(t, rexDescriptor.TrustDomain, runtimes[0].Runtime.TrustDomain)

	nodes, err := discovery.ListNodeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "Stored Node", nodes[0].Node.Name)
	require.Equal(t, core.NodePlatformLinux, nodes[0].Node.Platform)
	require.Equal(t, core.TrustClassRemoteApproved, nodes[0].Node.TrustClass)
	require.Equal(t, "a1", nodes[0].Node.Tags["rack"])

	require.NoError(t, client.Close())
	cancel()

	err = <-done
	if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && !strings.Contains(err.Error(), "use of closed network connection") && !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("HandleGatewayNodeConnection() error = %v", err)
	}
}

func TestNodeRPCConnForTransportUsesFramedConnWhenProfileNegotiated(t *testing.T) {
	base := &nexusserverTestRPCConn{}
	conn := nexusserver.NodeRPCConnForTransportForTest(fwgateway.ConnectionPrincipal{
		Principal: &core.AuthenticatedPrincipal{SessionID: "sess-1"},
	}, fwgateway.NodeConnectInfo{
		NodeID:           "node-1",
		RuntimeID:        "runtime-1",
		TransportProfile: fwgateway.TransportProfileWebSocketTLS,
	}, base)

	_, ok := conn.(*fwnode.FramedRPCConn)
	require.True(t, ok)
}

func TestNodeRPCConnForTransportKeepsLegacyConnWithoutProfile(t *testing.T) {
	base := &nexusserverTestRPCConn{}
	conn := nexusserver.NodeRPCConnForTransportForTest(fwgateway.ConnectionPrincipal{}, fwgateway.NodeConnectInfo{}, base)
	require.Same(t, base, conn)
}

func TestChunkTransportFrameHandlerOpenReadAckCancel(t *testing.T) {
	store := &fwfmp.InMemoryOwnershipStore{}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	require.NoError(t, store.CreateLineage(context.Background(), lineage))
	require.NoError(t, store.UpsertAttempt(context.Background(), attempt))
	packager := fwfmp.JSONPackager{
		RuntimeStore:    fakeWorkflowRuntimeStore{},
		KeyResolver:     testRecipientKeysForNexus(),
		LocalRecipient:  "runtime://mesh-a/node-1/rt-1",
		InlineThreshold: 8,
		ChunkSize:       16,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, fwfmp.RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	require.NoError(t, err)
	require.Equal(t, core.TransferModeChunked, pkg.Manifest.TransferMode)
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	require.NoError(t, err)

	mesh := &fwfmp.Service{
		Ownership: store,
		Transfers: &fwfmp.InMemoryChunkTransferManager{DefaultWindow: 1},
	}
	rpc := &nexusserverTestRPCConn{}
	ws := &fwnode.WSConnection{Conn: rpc}
	handler := nexusserver.ChunkTransportFrameHandlerForTest(mesh)

	openFrame := mustRawFrame(t, map[string]any{
		"type":       "fmp.chunk.open",
		"lineage_id": lineage.LineageID,
		"manifest":   pkg.Manifest,
		"sealed":     sealed,
	})
	require.NoError(t, handler(context.Background(), ws, openFrame))
	require.Len(t, rpc.writes, 1)
	openResp := rpc.writes[0].(map[string]any)
	require.Equal(t, "fmp.chunk.opened", openResp["type"])
	sessionMap := openResp["session"].(map[string]any)
	transferID := sessionMap["transfer_id"].(string)

	readFrame := mustRawFrame(t, map[string]any{
		"type":        "fmp.chunk.read",
		"transfer_id": transferID,
		"max_chunks":  1,
	})
	require.NoError(t, handler(context.Background(), ws, readFrame))
	require.Len(t, rpc.writes, 2)
	readResp := rpc.writes[1].(map[string]any)
	require.Equal(t, "fmp.chunk.data", readResp["type"])

	ackFrame := mustRawFrame(t, map[string]any{
		"type":        "fmp.chunk.ack",
		"lineage_id":  lineage.LineageID,
		"transfer_id": transferID,
		"acked_index": 0,
		"window_size": 2,
	})
	require.NoError(t, handler(context.Background(), ws, ackFrame))
	require.Len(t, rpc.writes, 3)
	ackResp := rpc.writes[2].(map[string]any)
	require.Equal(t, "fmp.chunk.acked", ackResp["type"])

	cancelFrame := mustRawFrame(t, map[string]any{
		"type":        "fmp.chunk.cancel",
		"lineage_id":  lineage.LineageID,
		"transfer_id": transferID,
		"reason":      "done",
	})
	require.NoError(t, handler(context.Background(), ws, cancelFrame))
	require.Len(t, rpc.writes, 4)
	cancelResp := rpc.writes[3].(map[string]any)
	require.Equal(t, "fmp.chunk.cancelled", cancelResp["type"])
}

func TestMeshTransportFrameHandlerRegistersRuntimeAndAdvertisesExport(t *testing.T) {
	store := &fwfmp.InMemoryDiscoveryStore{}
	mesh := &fwfmp.Service{Discovery: store}
	rpc := &nexusserverTestRPCConn{}
	ws := &fwnode.WSConnection{
		Conn: rpc,
		Descriptor: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   time.Now().UTC(),
		},
	}
	handler := nexusserver.MeshTransportFrameHandlerForTest(mesh, fwgateway.NodeConnectInfo{
		NodeID:                  "node-1",
		TrustDomain:             "mesh.local",
		RuntimeID:               "runtime-1",
		RuntimeVersion:          "2.1.0",
		CompatibilityClass:      "compat-a",
		SupportedContextClasses: []string{"workflow-runtime"},
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	})

	registerFrame := mustRawFrame(t, map[string]any{
		"type":         "fmp.runtime.register",
		"trust_domain": "mesh.local",
		"runtime": map[string]any{
			"runtime_id":                "runtime-1",
			"runtime_version":           "2.1.0",
			"supported_context_classes": []string{"workflow-runtime"},
			"compatibility_class":       "compat-a",
			"attestation_profile":       "node.runtime.v1",
			"attestation_claims": map[string]string{
				"node_id":     "node-1",
				"peer_key_id": "key-1",
			},
			"signature": "sig-runtime-1",
		},
		"signature": "sig-runtime-1",
	})
	require.NoError(t, handler(context.Background(), ws, registerFrame))
	require.Len(t, rpc.writes, 1)
	registerResp := rpc.writes[0].(map[string]any)
	require.Equal(t, "fmp.runtime.registered", registerResp["type"])

	exportFrame := mustRawFrame(t, map[string]any{
		"type":         "fmp.export.advertise",
		"trust_domain": "mesh.local",
		"runtime_id":   "runtime-1",
		"export": map[string]any{
			"export_name":                    "agent.resume",
			"accepted_context_classes":       []string{"workflow-runtime"},
			"required_compatibility_classes": []string{"compat-a"},
			"route_mode":                     string(core.RouteModeGateway),
		},
	})
	require.NoError(t, handler(context.Background(), ws, exportFrame))
	require.Len(t, rpc.writes, 2)
	exportResp := rpc.writes[1].(map[string]any)
	require.Equal(t, "fmp.export.advertised", exportResp["type"])

	runtimes, err := store.ListRuntimeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	require.Equal(t, "runtime-1", runtimes[0].Runtime.RuntimeID)
	exports, err := store.ListExportAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, exports, 1)
	require.Equal(t, "agent.resume", exports[0].Export.ExportName)
	require.Equal(t, "runtime-1", exports[0].RuntimeID)
	require.Equal(t, "node-1", exports[0].NodeID)
}

func TestMeshTransportFrameHandlerAdvertisesExportWithLiveRexDRMetadata(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery := &fwfmp.InMemoryDiscoveryStore{}
	mesh := &fwfmp.Service{
		Discovery:         discovery,
		PartitionDetector: &fwfmp.AtomicPartitionState{},
	}
	rexProvider, err := nexusserver.NewRexRuntimeProvider(ctx, t.TempDir())
	require.NoError(t, err)
	defer rexProvider.Close()
	rexProvider.AttachFMPService(mesh)

	startedAt := time.Date(2026, 4, 2, 23, 0, 0, 0, time.UTC)
	require.NoError(t, rexProvider.WorkflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-dr-export",
		TaskID:      "task-dr-export",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "exercise export dr metadata",
		Status:      memory.WorkflowRunStatusNeedsReplan,
		CreatedAt:   startedAt,
		UpdatedAt:   startedAt,
	}))
	require.NoError(t, rexProvider.WorkflowStore.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:          "run-dr-export",
		WorkflowID:     "wf-dr-export",
		Status:         memory.WorkflowRunStatusRunning,
		RuntimeVersion: "rex.v9",
		StartedAt:      startedAt,
	}))
	rexProvider.Agent.Runtime.BeginExecution("wf-dr-export", "run-dr-export")(nil)

	rpc := &nexusserverTestRPCConn{}
	ws := &fwnode.WSConnection{
		Conn: rpc,
		Descriptor: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   time.Now().UTC(),
		},
	}
	connectInfo := fwgateway.NodeConnectInfo{
		NodeID:                  "node-1",
		TrustDomain:             "mesh.local",
		RuntimeID:               "runtime-1",
		RuntimeVersion:          "2.1.0",
		CompatibilityClass:      "compat-a",
		SupportedContextClasses: []string{"workflow-runtime"},
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	}
	handler := nexusserver.MeshTransportFrameHandlerWithRuntimeForTest(mesh, connectInfo, rexProvider)

	require.NoError(t, handler(context.Background(), ws, mustRawFrame(t, map[string]any{
		"type":         "fmp.runtime.register",
		"trust_domain": "mesh.local",
		"runtime": map[string]any{
			"runtime_id":                "runtime-1",
			"runtime_version":           "2.1.0",
			"supported_context_classes": []string{"workflow-runtime"},
			"compatibility_class":       "compat-a",
			"attestation_profile":       "node.runtime.v1",
			"attestation_claims": map[string]string{
				"node_id":     "node-1",
				"peer_key_id": "key-1",
			},
			"signature": "sig-runtime-1",
		},
		"signature": "sig-runtime-1",
	})))
	require.NoError(t, handler(context.Background(), ws, mustRawFrame(t, map[string]any{
		"type":         "fmp.export.advertise",
		"trust_domain": "mesh.local",
		"runtime_id":   "runtime-1",
		"export": map[string]any{
			"export_name":                    "agent.resume",
			"accepted_context_classes":       []string{"workflow-runtime"},
			"required_compatibility_classes": []string{"compat-a"},
			"route_mode":                     string(core.RouteModeGateway),
		},
	})))

	exports, err := discovery.ListExportAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, exports, 1)
	require.True(t, exports[0].FailoverReady)
	require.Equal(t, string(memory.WorkflowRunStatusNeedsReplan), exports[0].RecoveryState)
	require.Equal(t, "rex.v9", exports[0].RuntimeVersion)
	require.True(t, exports[0].LastCheckpoint.Equal(startedAt))
}

func TestMeshTransportFrameHandlerAdvertisesDegradedRexDRMetadataWhenPartitioned(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery := &fwfmp.InMemoryDiscoveryStore{}
	partition := &fwfmp.AtomicPartitionState{}
	mesh := &fwfmp.Service{
		Discovery:         discovery,
		PartitionDetector: partition,
	}
	rexProvider, err := nexusserver.NewRexRuntimeProvider(ctx, t.TempDir())
	require.NoError(t, err)
	defer rexProvider.Close()
	rexProvider.AttachFMPService(mesh)

	startedAt := time.Date(2026, 4, 2, 23, 30, 0, 0, time.UTC)
	require.NoError(t, rexProvider.WorkflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-dr-degraded",
		TaskID:      "task-dr-degraded",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "exercise degraded export dr metadata",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   startedAt,
		UpdatedAt:   startedAt,
	}))
	require.NoError(t, rexProvider.WorkflowStore.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:          "run-dr-degraded",
		WorkflowID:     "wf-dr-degraded",
		Status:         memory.WorkflowRunStatusRunning,
		RuntimeVersion: "rex.v10",
		StartedAt:      startedAt,
	}))
	rexProvider.Agent.Runtime.BeginExecution("wf-dr-degraded", "run-dr-degraded")(nil)
	partition.SetPartitioned(true)

	rpc := &nexusserverTestRPCConn{}
	ws := &fwnode.WSConnection{
		Conn: rpc,
		Descriptor: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   time.Now().UTC(),
		},
	}
	connectInfo := fwgateway.NodeConnectInfo{
		NodeID:                  "node-1",
		TrustDomain:             "mesh.local",
		RuntimeID:               "runtime-1",
		RuntimeVersion:          "2.1.0",
		CompatibilityClass:      "compat-a",
		SupportedContextClasses: []string{"workflow-runtime"},
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	}
	handler := nexusserver.MeshTransportFrameHandlerWithRuntimeForTest(mesh, connectInfo, rexProvider)

	require.NoError(t, handler(context.Background(), ws, mustRawFrame(t, map[string]any{
		"type":         "fmp.runtime.register",
		"trust_domain": "mesh.local",
		"runtime": map[string]any{
			"runtime_id":                "runtime-1",
			"runtime_version":           "2.1.0",
			"supported_context_classes": []string{"workflow-runtime"},
			"compatibility_class":       "compat-a",
			"attestation_profile":       "node.runtime.v1",
			"attestation_claims": map[string]string{
				"node_id":     "node-1",
				"peer_key_id": "key-1",
			},
			"signature": "sig-runtime-1",
		},
		"signature": "sig-runtime-1",
	})))
	require.NoError(t, handler(context.Background(), ws, mustRawFrame(t, map[string]any{
		"type":         "fmp.export.advertise",
		"trust_domain": "mesh.local",
		"runtime_id":   "runtime-1",
		"export": map[string]any{
			"export_name":                    "agent.resume",
			"accepted_context_classes":       []string{"workflow-runtime"},
			"required_compatibility_classes": []string{"compat-a"},
			"route_mode":                     string(core.RouteModeGateway),
		},
	})))

	exports, err := discovery.ListExportAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, exports, 1)
	require.Equal(t, string(memory.WorkflowRunStatusRunning), exports[0].RecoveryState)
	require.Equal(t, "rex.v10", exports[0].RuntimeVersion)
	require.True(t, exports[0].LastCheckpoint.Equal(startedAt))
	require.True(t, partition.IsPartitioned())
	require.Equal(t, rexruntime.HealthDegraded, rexProvider.RuntimeProjection().Health)
}

func TestMeshTransportFrameHandlerExecutesTenantBoundResume(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()

	require.NoError(t, identityStore.UpsertTenant(context.Background(), core.TenantRecord{ID: "tenant-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:       "tenant-1",
		NodeID:         "node-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		PublicKey:      []byte("pk"),
		PairedAt:       now,
		LastVerifiedAt: now,
		AuthMethod:     core.AuthMethodNodeChallenge,
	}))
	require.NoError(t, sessionStore.UpsertBoundary(context.Background(), "tenant-1:webchat:conv-1", &core.SessionBoundary{
		SessionID:      "sess-1",
		RoutingKey:     "tenant-1:webchat:conv-1",
		TenantID:       "tenant-1",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ChannelID:      "webchat",
		PeerID:         "conv-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
	}))
	require.NoError(t, sessionStore.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		SessionID:  "sess-1",
		TenantID:   "tenant-1",
		Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
		Operations: []core.SessionOperation{core.SessionOperationResume},
		CreatedAt:  now,
	}))

	ownership := &fwfmp.InMemoryOwnershipStore{}
	sourceSvc := &fwfmp.Service{
		Ownership: ownership,
		Packager: fwfmp.JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeysForNexus(),
			DefaultRecipients: []string{"runtime://mesh-a/node-1/rt-1"},
			LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		},
		Now: func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
		TrustClass:   core.TrustClassRemoteApproved,
		Delegations: []core.SessionDelegationRecord{{
			SessionID:  "sess-1",
			TenantID:   "tenant-1",
			Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
			Operations: []core.SessionOperation{core.SessionOperationResume},
			CreatedAt:  now,
		}},
	}
	require.NoError(t, sourceSvc.CreateLineage(context.Background(), lineage))
	sourceAttempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: now,
	}
	require.NoError(t, ownership.UpsertAttempt(context.Background(), sourceAttempt))
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, sourceAttempt.AttemptID, "exp.run", "issuer", fwfmp.RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	require.NoError(t, err)

	mesh := &fwfmp.Service{
		Ownership: ownership,
		Runtime: nexusRuntimeEndpoint{
			descriptor: core.RuntimeDescriptor{
				RuntimeID:               "rt-1",
				NodeID:                  "node-1",
				RuntimeVersion:          "2.1.0",
				CompatibilityClass:      "compat-a",
				SupportedContextClasses: []string{"workflow-runtime"},
				MaxContextSize:          1024,
			},
			pkg: pkg,
			attempt: &core.AttemptRecord{
				AttemptID: "lineage-1:rt-1:resume",
				LineageID: "lineage-1",
				RuntimeID: "rt-1",
				State:     core.AttemptStateResumePending,
				StartTime: now,
			},
			receipt: &core.ResumeReceipt{
				ReceiptID:         "receipt-1",
				LineageID:         "lineage-1",
				AttemptID:         "lineage-1:rt-1:resume",
				RuntimeID:         "rt-1",
				Status:            core.ReceiptStatusRunning,
				ImportedContextID: pkg.Manifest.ContextID,
				StartTime:         now,
			},
		},
		Nexus: fwfmp.NexusAdapter{
			Tenants:  identityStore,
			Subjects: identityStore,
			Nodes:    identityStore,
			Sessions: sessionStore,
		},
		Now: func() time.Time { return now },
	}

	rpc := &nexusserverTestRPCConn{}
	ws := &fwnode.WSConnection{
		Conn: rpc,
		Descriptor: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   now,
		},
	}
	handler := nexusserver.MeshTransportFrameHandlerForTest(mesh, fwgateway.NodeConnectInfo{
		NodeID:                  "node-1",
		TrustDomain:             "mesh.local",
		RuntimeID:               "rt-1",
		RuntimeVersion:          "2.1.0",
		CompatibilityClass:      "compat-a",
		SupportedContextClasses: []string{"workflow-runtime"},
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	})

	resumeFrame := mustRawFrame(t, map[string]any{
		"type": "fmp.resume.execute",
		"actor": map[string]any{
			"tenant_id": "tenant-1",
			"kind":      string(core.SubjectKindServiceAccount),
			"id":        "delegate-1",
		},
		"offer":       offer,
		"destination": map[string]any{"export_name": "exp.run", "accepted_context_classes": []string{"workflow-runtime"}, "route_mode": string(core.RouteModeGateway)},
		"manifest":    pkg.Manifest,
		"sealed":      sealed,
	})
	require.NoError(t, handler(context.Background(), ws, resumeFrame))
	require.Len(t, rpc.writes, 1)
	resp := rpc.writes[0].(map[string]any)
	if resp["type"] != "fmp.resume.executed" {
		t.Fatalf("unexpected resume response: %+v", resp)
	}
	authz := resp["authorized"].(map[string]any)
	require.Equal(t, true, authz["Delegated"])
	commit := resp["commit"].(map[string]any)
	require.Equal(t, "lineage-1", commit["lineage_id"])
}

type transportResumeFixture struct {
	now            time.Time
	ownership      *fwfmp.InMemoryOwnershipStore
	rexProvider    *nexusserver.RexRuntimeProvider
	handler        func(context.Context, *fwnode.WSConnection, map[string]json.RawMessage) error
	ws             *fwnode.WSConnection
	rpc            *nexusserverTestRPCConn
	resumeFrame    map[string]json.RawMessage
	lineageID      string
	sourceAttempt  string
	destAttempt    string
	importWorkflow string
}

func newTransportResumeFixture(t *testing.T) *transportResumeFixture {
	t.Helper()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	liveNow := time.Now().UTC()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, identityStore.Close()) })

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sessionStore.Close()) })

	require.NoError(t, identityStore.UpsertTenant(context.Background(), core.TenantRecord{ID: "tenant-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:       "tenant-1",
		NodeID:         "node-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		PublicKey:      []byte("pk"),
		PairedAt:       now,
		LastVerifiedAt: now,
		AuthMethod:     core.AuthMethodNodeChallenge,
	}))
	require.NoError(t, sessionStore.UpsertBoundary(context.Background(), "tenant-1:webchat:conv-1", &core.SessionBoundary{
		SessionID:      "sess-1",
		RoutingKey:     "tenant-1:webchat:conv-1",
		TenantID:       "tenant-1",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ChannelID:      "webchat",
		PeerID:         "conv-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      liveNow,
		LastActivityAt: liveNow,
	}))
	require.NoError(t, sessionStore.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		SessionID:  "sess-1",
		TenantID:   "tenant-1",
		Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
		Operations: []core.SessionOperation{core.SessionOperationResume},
		CreatedAt:  liveNow,
	}))

	ownership := &fwfmp.InMemoryOwnershipStore{}
	signer := fwfmp.NewEd25519SignerFromSeed([]byte("node-runtime-transport-real-rex"))
	rexProvider, err := nexusserver.NewRexRuntimeProvider(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(rexProvider.Close)

	mesh := &fwfmp.Service{
		Ownership: ownership,
		Trust:     &fwfmp.InMemoryTrustBundleStore{},
		Signer:    signer,
		Nexus: fwfmp.NexusAdapter{
			Tenants:  identityStore,
			Subjects: identityStore,
			Nodes:    identityStore,
			Sessions: sessionStore,
		},
		Now: func() time.Time { return now },
	}
	rexProvider.AttachFMPService(mesh)
	rexDescriptor, err := rexProvider.RuntimeEndpoint.Descriptor(context.Background())
	require.NoError(t, err)
	require.NoError(t, identityStore.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:       "tenant-1",
		NodeID:         rexDescriptor.NodeID,
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: rexDescriptor.NodeID},
		TrustClass:     core.TrustClassRemoteApproved,
		PublicKey:      []byte("pk"),
		PairedAt:       now,
		LastVerifiedAt: now,
		AuthMethod:     core.AuthMethodNodeChallenge,
	}))

	runtimeRecipient := "runtime://local/rex"
	runtimeKey := sha256.Sum256([]byte(runtimeRecipient))
	sourceSvc := &fwfmp.Service{
		Ownership: ownership,
		Packager: fwfmp.JSONPackager{
			RuntimeStore: richerWorkflowRuntimeStore{},
			KeyResolver: &fwfmp.TrustBundleRecipientKeyResolver{
				Static: map[string][][]byte{
					runtimeRecipient: {runtimeKey[:]},
				},
			},
			DefaultRecipients: []string{runtimeRecipient},
			LocalRecipient:    "runtime://mesh-a/source/rt-a",
		},
		Signer: signer,
		Now:    func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-transport",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
		TrustClass:   core.TrustClassRemoteApproved,
		Delegations: []core.SessionDelegationRecord{{
			SessionID:  "sess-1",
			TenantID:   "tenant-1",
			Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
			Operations: []core.SessionOperation{core.SessionOperationResume},
			CreatedAt:  liveNow,
		}},
	}
	require.NoError(t, sourceSvc.CreateLineage(context.Background(), lineage))
	sourceAttempt := core.AttemptRecord{
		AttemptID: "attempt-transport",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-source",
		State:     core.AttemptStateRunning,
		StartTime: now,
	}
	require.NoError(t, ownership.UpsertAttempt(context.Background(), sourceAttempt))
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, sourceAttempt.AttemptID, "exp.run", "issuer", fwfmp.RuntimeQuery{WorkflowID: "wf-import", RunID: "run-import"})
	require.NoError(t, err)

	rpc := &nexusserverTestRPCConn{}
	ws := &fwnode.WSConnection{
		Conn: rpc,
		Descriptor: core.NodeDescriptor{
			ID:         rexDescriptor.NodeID,
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: rexDescriptor.NodeID},
			PairedAt:   now,
		},
	}
	connectInfo := fwgateway.NodeConnectInfo{
		NodeID:                  rexDescriptor.NodeID,
		TrustDomain:             "mesh.local",
		RuntimeID:               rexDescriptor.RuntimeID,
		RuntimeVersion:          "spoofed-frame-version",
		CompatibilityClass:      rexDescriptor.CompatibilityClass,
		SupportedContextClasses: append([]string(nil), rexDescriptor.SupportedContextClasses...),
		TransportProfile:        fwgateway.TransportProfileWebSocketTLS,
		PeerKeyID:               "key-1",
	}

	return &transportResumeFixture{
		now:            now,
		ownership:      ownership,
		rexProvider:    rexProvider,
		handler:        nexusserver.MeshTransportFrameHandlerWithRuntimeForTest(mesh, connectInfo, rexProvider),
		ws:             ws,
		rpc:            rpc,
		resumeFrame: mustRawFrame(t, map[string]any{
			"type": "fmp.resume.execute",
			"actor": map[string]any{
				"tenant_id": "tenant-1",
				"kind":      string(core.SubjectKindServiceAccount),
				"id":        "delegate-1",
			},
			"offer":       offer,
			"destination": map[string]any{"export_name": "exp.run", "accepted_context_classes": []string{"workflow-runtime"}, "route_mode": string(core.RouteModeGateway)},
			"manifest":    pkg.Manifest,
			"sealed":      sealed,
		}),
		lineageID:      lineage.LineageID,
		sourceAttempt:  sourceAttempt.AttemptID,
		destAttempt:    lineage.LineageID + ":rex:resume",
		importWorkflow: "wf-import",
	}
}

func TestMeshTransportFrameHandlerResumePersistsImportedRexWorkflowAndFencesSource(t *testing.T) {
	t.Parallel()

	fixture := newTransportResumeFixture(t)

	require.NoError(t, fixture.handler(context.Background(), fixture.ws, fixture.resumeFrame))
	require.Len(t, fixture.rpc.writes, 1)

	resp := fixture.rpc.writes[0].(map[string]any)
	require.Equal(t, "fmp.resume.executed", resp["type"])

	authz := resp["authorized"].(map[string]any)
	require.Equal(t, true, authz["Delegated"])

	commit := resp["commit"].(map[string]any)
	require.Equal(t, fixture.lineageID, commit["lineage_id"])
	require.Equal(t, fixture.destAttempt, commit["new_attempt_id"])

	require.Eventually(t, func() bool {
		workflow, ok, err := fixture.rexProvider.WorkflowStore.GetWorkflow(context.Background(), fixture.importWorkflow)
		require.NoError(t, err)
		if !ok || workflow == nil {
			return false
		}
		run, ok, err := fixture.rexProvider.WorkflowStore.GetRun(context.Background(), fixture.destAttempt)
		require.NoError(t, err)
		return ok && run != nil && run.WorkflowID == fixture.importWorkflow
	}, 5*time.Second, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		artifacts, err := fixture.rexProvider.WorkflowStore.ListWorkflowArtifacts(context.Background(), fixture.importWorkflow, fixture.destAttempt)
		require.NoError(t, err)
		kinds := map[string]bool{}
		for _, artifact := range artifacts {
			kinds[artifact.Kind] = true
		}
		return kinds["rex.fmp_import"] && kinds["rex.fmp_lineage"] && kinds["rex.task_request"]
	}, 5*time.Second, 20*time.Millisecond)

	source, ok, err := fixture.ownership.GetAttempt(context.Background(), fixture.sourceAttempt)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, source)
	require.Equal(t, core.AttemptStateCommittedRemote, source.State)
	require.True(t, source.Fenced)

	dest, ok, err := fixture.ownership.GetAttempt(context.Background(), fixture.destAttempt)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, dest)
	require.Contains(t, []core.AttemptState{core.AttemptStateRunning, core.AttemptStateCompleted, core.AttemptStateFailed}, dest.State)

	lineage, ok, err := fixture.ownership.GetLineage(context.Background(), fixture.lineageID)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, lineage)
	require.Equal(t, fixture.destAttempt, lineage.CurrentOwnerAttempt)
}

func TestMeshTransportFrameHandlerResumeReplayAfterFenceDoesNotCreateDuplicateRexAttempt(t *testing.T) {
	t.Parallel()

	fixture := newTransportResumeFixture(t)

	require.NoError(t, fixture.handler(context.Background(), fixture.ws, fixture.resumeFrame))
	require.Len(t, fixture.rpc.writes, 1)
	fixture.rpc.writes = nil

	require.NoError(t, fixture.handler(context.Background(), fixture.ws, fixture.resumeFrame))
	require.Len(t, fixture.rpc.writes, 1)

	resp := fixture.rpc.writes[0].(map[string]any)
	require.Contains(t, []any{"fmp.resume.error", "fmp.resume.refused"}, resp["type"])

	require.Eventually(t, func() bool {
		run, ok, err := fixture.rexProvider.WorkflowStore.GetRun(context.Background(), fixture.destAttempt)
		require.NoError(t, err)
		return ok && run != nil && run.WorkflowID == fixture.importWorkflow
	}, 5*time.Second, 20*time.Millisecond)

	var artifactsBefore []memory.WorkflowArtifactRecord
	require.Eventually(t, func() bool {
		var err error
		artifactsBefore, err = fixture.rexProvider.WorkflowStore.ListWorkflowArtifacts(context.Background(), fixture.importWorkflow, fixture.destAttempt)
		require.NoError(t, err)
		kinds := map[string]bool{}
		for _, artifact := range artifactsBefore {
			kinds[artifact.Kind] = true
		}
		return kinds["rex.fmp_import"] && kinds["rex.fmp_lineage"] && kinds["rex.task_request"]
	}, 5*time.Second, 20*time.Millisecond)
	artifactIDsBefore := map[string]bool{}
	requiredArtifactIDs := map[string]string{
		"rex.fmp_import":  fixture.destAttempt + ":fmp-import",
		"rex.fmp_lineage": fixture.destAttempt + ":fmp-lineage",
		"rex.task_request": fixture.destAttempt + ":task-request",
	}
	for _, artifact := range artifactsBefore {
		artifactIDsBefore[artifact.ArtifactID] = true
	}
	for kind, artifactID := range requiredArtifactIDs {
		require.True(t, artifactIDsBefore[artifactID], "missing %s artifact before replay", kind)
	}

	require.NoError(t, fixture.handler(context.Background(), fixture.ws, fixture.resumeFrame))
	require.Len(t, fixture.rpc.writes, 2)

	resp = fixture.rpc.writes[1].(map[string]any)
	require.Contains(t, []any{"fmp.resume.error", "fmp.resume.refused"}, resp["type"])

	artifactsAfter, err := fixture.rexProvider.WorkflowStore.ListWorkflowArtifacts(context.Background(), fixture.importWorkflow, fixture.destAttempt)
	require.NoError(t, err)
	artifactIDsAfter := map[string]bool{}
	for _, artifact := range artifactsAfter {
		artifactIDsAfter[artifact.ArtifactID] = true
	}
	for kind, artifactID := range requiredArtifactIDs {
		require.True(t, artifactIDsAfter[artifactID], "missing %s artifact after replay", kind)
	}
	for artifactID := range artifactIDsBefore {
		require.True(t, artifactIDsAfter[artifactID], "artifact disappeared after replay: %s", artifactID)
	}

	source, ok, err := fixture.ownership.GetAttempt(context.Background(), fixture.sourceAttempt)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, source)
	require.Equal(t, core.AttemptStateCommittedRemote, source.State)
	require.True(t, source.Fenced)

	dest, ok, err := fixture.ownership.GetAttempt(context.Background(), fixture.destAttempt)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, dest)
	require.Equal(t, fixture.lineageID, dest.LineageID)

	lineage, ok, err := fixture.ownership.GetLineage(context.Background(), fixture.lineageID)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, lineage)
	require.Equal(t, fixture.destAttempt, lineage.CurrentOwnerAttempt)
}

func TestMeshTransportFrameHandlerResumePersistsImportedRexWorkflow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	liveNow := time.Now().UTC()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()

	require.NoError(t, identityStore.UpsertTenant(context.Background(), core.TenantRecord{ID: "tenant-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:       "tenant-1",
		NodeID:         "node-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		PublicKey:      []byte("pk"),
		PairedAt:       now,
		LastVerifiedAt: now,
		AuthMethod:     core.AuthMethodNodeChallenge,
	}))
	require.NoError(t, sessionStore.UpsertBoundary(context.Background(), "tenant-1:webchat:conv-1", &core.SessionBoundary{
		SessionID:      "sess-1",
		RoutingKey:     "tenant-1:webchat:conv-1",
		TenantID:       "tenant-1",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ChannelID:      "webchat",
		PeerID:         "conv-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      liveNow,
		LastActivityAt: liveNow,
	}))
	require.NoError(t, sessionStore.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		SessionID:  "sess-1",
		TenantID:   "tenant-1",
		Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
		Operations: []core.SessionOperation{core.SessionOperationResume},
		CreatedAt:  liveNow,
	}))

	ownership := &fwfmp.InMemoryOwnershipStore{}
	signer := fwfmp.NewEd25519SignerFromSeed([]byte("node-runtime-real-rex-resume"))
	rexProvider, err := nexusserver.NewRexRuntimeProvider(ctx, t.TempDir())
	require.NoError(t, err)
	defer rexProvider.Close()

	mesh := &fwfmp.Service{
		Ownership: ownership,
		Trust:     &fwfmp.InMemoryTrustBundleStore{},
		Signer:    signer,
		Nexus: fwfmp.NexusAdapter{
			Tenants:  identityStore,
			Subjects: identityStore,
			Nodes:    identityStore,
			Sessions: sessionStore,
		},
		Now: func() time.Time { return now },
	}
	rexProvider.AttachFMPService(mesh)

	runtimeRecipient := "runtime://local/rex"
	runtimeKey := sha256.Sum256([]byte(runtimeRecipient))
	sourceSvc := &fwfmp.Service{
		Ownership: ownership,
		Packager: fwfmp.JSONPackager{
			RuntimeStore: richerWorkflowRuntimeStore{},
			KeyResolver: &fwfmp.TrustBundleRecipientKeyResolver{
				Static: map[string][][]byte{
					runtimeRecipient: {runtimeKey[:]},
				},
			},
			DefaultRecipients: []string{runtimeRecipient},
			LocalRecipient:    "runtime://mesh-a/source/rt-a",
		},
		Signer: signer,
		Now:    func() time.Time { return now },
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-real",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-1",
		TrustClass:   core.TrustClassRemoteApproved,
		Delegations: []core.SessionDelegationRecord{{
			SessionID:  "sess-1",
			TenantID:   "tenant-1",
			Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
			Operations: []core.SessionOperation{core.SessionOperationResume},
			CreatedAt:  liveNow,
		}},
	}
	require.NoError(t, sourceSvc.CreateLineage(context.Background(), lineage))
	sourceAttempt := core.AttemptRecord{
		AttemptID: "attempt-real",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-source",
		State:     core.AttemptStateRunning,
		StartTime: now,
	}
	require.NoError(t, ownership.UpsertAttempt(context.Background(), sourceAttempt))
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, sourceAttempt.AttemptID, "exp.run", "issuer", fwfmp.RuntimeQuery{WorkflowID: "wf-import", RunID: "run-import"})
	require.NoError(t, err)

	executed, commit, authorized, refusal, err := mesh.ResumeHandoffForNode(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rex", "", core.SubjectRef{
		TenantID: "tenant-1",
		Kind:     core.SubjectKindServiceAccount,
		ID:       "delegate-1",
	}, pkg.Manifest, *sealed)
	require.NoError(t, err)
	require.Nil(t, refusal)
	require.NotNil(t, authorized)
	require.True(t, authorized.Delegated)
	require.NotNil(t, executed)
	require.NotNil(t, commit)
	require.Equal(t, "lineage-real", commit.LineageID)
	require.Equal(t, "lineage-real:rex:resume", commit.NewAttemptID)

	require.Eventually(t, func() bool {
		workflow, ok, err := rexProvider.WorkflowStore.GetWorkflow(context.Background(), "wf-import")
		require.NoError(t, err)
		if !ok || workflow == nil {
			return false
		}
		run, ok, err := rexProvider.WorkflowStore.GetRun(context.Background(), "lineage-real:rex:resume")
		require.NoError(t, err)
		if !ok || run == nil {
			return false
		}
		return run.WorkflowID == "wf-import"
	}, 5*time.Second, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		artifacts, err := rexProvider.WorkflowStore.ListWorkflowArtifacts(context.Background(), "wf-import", "lineage-real:rex:resume")
		require.NoError(t, err)
		kinds := map[string]bool{}
		for _, artifact := range artifacts {
			kinds[artifact.Kind] = true
		}
		return kinds["rex.fmp_import"] && kinds["rex.fmp_lineage"] && kinds["rex.task_request"]
	}, 5*time.Second, 20*time.Millisecond)

	attempt, ok, err := ownership.GetAttempt(context.Background(), "lineage-real:rex:resume")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, attempt)
	require.Contains(t, []core.AttemptState{core.AttemptStateRunning, core.AttemptStateCompleted, core.AttemptStateFailed}, attempt.State)

	source, ok, err := ownership.GetAttempt(context.Background(), "attempt-real")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, source)
	require.Equal(t, core.AttemptStateCommittedRemote, source.State)
	require.True(t, source.Fenced)

	lineageRecord, ok, err := ownership.GetLineage(context.Background(), "lineage-real")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "lineage-real:rex:resume", lineageRecord.CurrentOwnerAttempt)
}

type nexusserverTestRPCConn struct {
	writes []any
}

func (n *nexusserverTestRPCConn) WriteJSON(v any) error {
	n.writes = append(n.writes, normalizeWrittenJSON(v))
	return nil
}
func (n *nexusserverTestRPCConn) ReadJSON(any) error { return nil }
func (n *nexusserverTestRPCConn) Close() error       { return nil }

type fakeWorkflowRuntimeStore struct{}

func (fakeWorkflowRuntimeStore) QueryWorkflowRuntime(context.Context, string, string) (map[string]any, error) {
	return map[string]any{
		"workflow_id": "wf-1",
		"run_id":      "run-1",
		"events":      []string{"checkpointed"},
	}, nil
}

type richerWorkflowRuntimeStore struct{}

func (richerWorkflowRuntimeStore) QueryWorkflowRuntime(context.Context, string, string) (map[string]any, error) {
	return map[string]any{
		"workflow_id": "wf-import",
		"run_id":      "run-import",
		"task": map[string]any{
			"id":          "task-import",
			"type":        string(core.TaskTypeAnalysis),
			"instruction": "inspect imported workflow state",
			"context": map[string]any{
				"workflow_id":        "wf-import",
				"gateway.session_id": "sess-1",
			},
			"metadata": map[string]any{
				"origin": "fmp-test",
			},
		},
		"state": map[string]any{
			"workflow_id":        "wf-import",
			"run_id":             "run-import",
			"gateway.session_id": "sess-1",
		},
		"events": []string{"checkpointed"},
	}, nil
}

func testRecipientKeysForNexus() fwfmp.StaticRecipientKeyResolver {
	return fwfmp.StaticRecipientKeyResolver{
		"runtime://mesh-a/node-1/rt-1": []byte("0123456789abcdef0123456789abcdef"),
	}
}

func mustRawFrame(t *testing.T, value map[string]any) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var frame map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &frame))
	return frame
}

func normalizeWrittenJSON(v any) any {
	data, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(data, &out)
	return out
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

func TestAdvertiseConnectedNodeToFMPPublishesNodeAndRuntime(t *testing.T) {
	t.Parallel()

	discovery := &fwfmp.InMemoryDiscoveryStore{}
	mesh := &fwfmp.Service{Discovery: discovery}
	nodeDesc := core.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-1",
		Name:       "Node One",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassRemoteApproved,
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
	}
	err := nexusserver.AdvertiseConnectedNodeToFMP(context.Background(), mesh, nodeDesc, fwgateway.NodeConnectInfo{
		TrustDomain:             "mesh.local",
		RuntimeID:               "runtime-1",
		RuntimeVersion:          "2.1.0",
		CompatibilityClass:      "v2",
		SupportedContextClasses: []string{"workflow-runtime"},
	})
	require.NoError(t, err)

	nodes, err := discovery.ListNodeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "mesh.local", nodes[0].TrustDomain)
	require.Equal(t, "node-1", nodes[0].Node.ID)

	runtimes, err := discovery.ListRuntimeAdvertisements(context.Background())
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	require.Equal(t, "runtime-1", runtimes[0].Runtime.RuntimeID)
	require.Equal(t, "2.1.0", runtimes[0].Runtime.RuntimeVersion)
	require.Equal(t, "v2", runtimes[0].Runtime.CompatibilityClass)
	require.Equal(t, []string{"workflow-runtime"}, runtimes[0].Runtime.SupportedContextClasses)
}
