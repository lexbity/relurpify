package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/db"
	nexusserver "github.com/lexcodex/relurpify/app/nexus/server"
	"github.com/lexcodex/relurpify/framework/core"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
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
	require.Equal(t, "fmp.resume.executed", resp["type"])
	authz := resp["authorized"].(map[string]any)
	require.Equal(t, true, authz["Delegated"])
	commit := resp["commit"].(map[string]any)
	require.Equal(t, "lineage-1", commit["lineage_id"])
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
