package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/adapters/webchat"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusgateway "github.com/lexcodex/relurpify/app/nexus/gateway"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/lexcodex/relurpify/framework/middleware/session"
	rexnexus "github.com/lexcodex/relurpify/named/rex/nexus"
	"github.com/stretchr/testify/require"
)

func TestNexusAppAndAuthHelpers(t *testing.T) {
	app := &NexusApp{}
	require.Equal(t, "local", app.partition())
	require.Equal(t, "/gateway", app.gatewayPath())
	app.Partition = "tenant-1"
	app.Config.Gateway.Path = "/custom"
	require.Equal(t, "tenant-1", app.partition())
	require.Equal(t, "/custom", app.gatewayPath())

	require.Equal(t, "", bearerToken("Basic abc"))
	require.Equal(t, "abc", bearerToken("Bearer abc"))
	require.True(t, isAdminOrOperator(core.AuthenticatedPrincipal{Scopes: []string{"admin"}}))
	require.False(t, isAdminOrOperator(core.AuthenticatedPrincipal{}))

	channels := map[string]map[string]any{"webchat": {"enabled": false}, "telegram": {"enabled": true}}
	require.False(t, enabled(channels, "webchat", true))
	require.True(t, enabled(channels, "telegram", false))
	require.True(t, enabled(channels, "missing", true))

	cfg := nexuscfg.Config{Channels: map[string]map[string]any{"webchat": {"enabled": true}, "bad": {"enabled": func() {}}}}
	raw := channelConfigs(cfg)
	require.Contains(t, raw, "webchat")
	require.NotContains(t, raw, "bad")
}

func TestNexusSnapshotAndTransportHelpers(t *testing.T) {
	materializer := nexusgateway.NewStateMaterializer()
	require.NoError(t, materializer.Apply(context.Background(), []core.FrameworkEvent{{
		Seq:       1,
		Type:      core.FrameworkEventSessionCreated,
		Partition: "local",
		Timestamp: time.Now().UTC(),
		Actor:     core.EventActor{ID: "sess-1", Kind: "session", TenantID: "tenant-a"},
	}}))
	principal := fwgateway.ConnectionPrincipal{Authenticated: true, Principal: &core.AuthenticatedPrincipal{Scopes: []string{"nexus:admin:global"}}}
	snapshot, err := snapshotForPrincipal(context.Background(), materializer, nil, principal)
	require.NoError(t, err)
	require.Contains(t, snapshot, "active_sessions")
	require.NotContains(t, snapshot, "tenant_id")

	tenantPrincipal := fwgateway.ConnectionPrincipal{Authenticated: true, Actor: core.EventActor{TenantID: "tenant-a"}, Principal: &core.AuthenticatedPrincipal{Scopes: []string{"nexus:admin"}}}
	snapshot, err = snapshotForPrincipal(context.Background(), materializer, nil, tenantPrincipal)
	require.NoError(t, err)
	require.Equal(t, "tenant-a", snapshot["tenant_id"])

	profile := &fwgateway.FMPTransportPolicy{
		RequireNodeTransport:  true,
		RequireTLS:            true,
		AllowLoopbackInsecure: false,
		AllowedProfiles:       []string{fwgateway.TransportProfileWebSocketTLS},
		SessionTTL:            2 * time.Minute,
		MaxClockSkew:          time.Minute,
		NonceStore:            &fwgateway.InMemoryTransportNonceStore{Now: func() time.Time { return time.Now().UTC() }},
		Now:                   func() time.Time { return time.Now().UTC() },
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(fwgateway.HeaderFMPTransportProfile, fwgateway.TransportProfileWebSocketTLS)
	req.Header.Set(fwgateway.HeaderFMPSessionNonce, "nonce-1")
	req.Header.Set(fwgateway.HeaderFMPSessionIssuedAt, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano))
	req.Header.Set(fwgateway.HeaderFMPSessionExpiresAt, time.Now().UTC().Add(30*time.Second).Format(time.RFC3339Nano))
	req.Header.Set(fwgateway.HeaderFMPPeerKeyID, "peer-1")
	req.TLS = &tls.ConnectionState{}
	require.NoError(t, validateFederationTransport(req, profile, "local"))

	rr := httptest.NewRecorder()
	writeFederationForwardResponse(rr, http.StatusOK, fwfmp.GatewayForwardTransportResponse{Result: &core.GatewayForwardResult{TrustDomain: "local", DestinationExport: "dst", RouteMode: core.RouteModeGateway}})
	require.Equal(t, http.StatusOK, rr.Code)

	audit := &recordingAuditLogger{}
	mesh := &fwfmp.Service{Audit: audit}
	auditFederationTransport(req, mesh, core.GatewayForwardRequest{TrustDomain: "local", SourceDomain: "src", DestinationExport: "dst"}, "ok", "")
	require.Len(t, audit.records, 1)
}

func TestRexRuntimeAndNodeHelpers(t *testing.T) {
	require.Equal(t, "abc", stringValue("abc"))
	require.Empty(t, stringValue(123))
	require.Equal(t, map[string]any{"x": 1}, mapStringAny(map[string]any{"x": 1}))
	require.Nil(t, mapStringAny(nil))
	require.Equal(t, map[string]string{"x": "1"}, mapStringString(map[string]any{"x": "1", "y": 2}))

	provider := &RexRuntimeProvider{}
	descriptor := provider.runtimeDescriptor()
	require.Equal(t, "rex", descriptor.RuntimeID)
	require.Contains(t, provider.runtimeRecipient(), descriptor.RuntimeID)

	task, state, err := rexTaskFromArgs(map[string]any{
		"instruction": "do work",
		"task_id":     "task-1",
		"task_type":   "code",
		"context":     map[string]any{"a": 1},
		"metadata":    map[string]any{"b": "2"},
		"workflow_id": "wf-1",
		"run_id":      "run-1",
	})
	require.NoError(t, err)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, "wf-1", state.GetString("rex.workflow_id"))
	require.Equal(t, "run-1", state.GetString("rex.run_id"))
}

func TestRexEventAndParsingHelpers(t *testing.T) {
	require.True(t, isRexControlPlaneEvent(core.FrameworkEventFMPHandoffOffered))
	require.False(t, isRexControlPlaneEvent("nope"))
	require.Equal(t, "hello", extractSessionMessageInstruction(json.RawMessage(`"hello"`)))
	require.Equal(t, "hi", extractSessionMessageInstruction(json.RawMessage(`{"text":"hi"}`)))
	require.Equal(t, "", extractSessionMessageInstruction(nil))
}

func TestNexusServerWiringAndAuthHelpers(t *testing.T) {
	mesh := &fwfmp.Service{}
	wireFMPNexusAdapter(mesh, nil, nil)
	require.NotNil(t, mesh)

	manager := channel.NewManager(nil, nil)
	cfg := nexuscfg.Config{
		Channels: map[string]map[string]any{
			"webchat":  {"enabled": true},
			"telegram": {"enabled": true},
			"discord":  {"enabled": false},
		},
	}
	require.NoError(t, registerConfiguredAdapters(manager, cfg, &webchat.Adapter{}))

	_ = listGatewayCapabilities(nil, &RexRuntimeProvider{}, fwgateway.ConnectionPrincipal{})
	require.Equal(t, "session store unavailable", mustErrString(t, func() error {
		_, err := InvokeAuthorizedGatewayCapability(context.Background(), nil, nil, nil, &RexRuntimeProvider{}, fwgateway.ConnectionPrincipal{}, "sess-1", rexCapabilityID, nil)
		return err
	}))

	called := false
	next := adminAuthMiddleware(func(context.Context, string) (fwgateway.ConnectionPrincipal, error) {
		return fwgateway.ConnectionPrincipal{Principal: &core.AuthenticatedPrincipal{Scopes: []string{"admin"}}}, nil
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin/mcp", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	rr := httptest.NewRecorder()
	next.ServeHTTP(rr, req)
	require.True(t, called)
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	adminAuthMiddleware(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/mcp", nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	gateway := &FederatedMeshGateway{Mesh: &fwfmp.Service{}}
	err := gateway.ImportAdvertisements(context.Background(), core.SubjectRef{}, []core.ExportAdvertisement{{
		TrustDomain: "mesh.remote",
		Export: core.ExportDescriptor{
			ExportName:       "agent.resume",
			RouteMode:        core.RouteModeGateway,
			SensitivityLimit: core.SensitivityClassLow,
		},
		RuntimeID: "runtime-1",
		NodeID:    "node-1",
	}}, "mesh.remote")
	require.Error(t, err)
	_, _, err = gateway.ForwardSealedContext(context.Background(), core.GatewayForwardRequest{})
	require.Error(t, err)
}

func TestNodeRuntimeHelpers(t *testing.T) {
	require.Equal(t, "node-1:default", fallbackNodeRuntimeID(core.NodeDescriptor{ID: "node-1"}, fwgateway.NodeConnectInfo{}))
	require.Equal(t, "2.1.0", fallbackRuntimeVersion(fwgateway.NodeConnectInfo{RuntimeVersion: "2.1.0"}))
	require.Equal(t, "default", fallbackCompatibilityClass(fwgateway.NodeConnectInfo{}))
	require.Equal(t, "nexus-register:node-1:runtime-1:2.1.0:peer-1:http.tls.v1", runtimeRegistrationSignatureFromValues("node-1", "runtime-1", "2.1.0", "peer-1", "http.tls.v1"))
	require.Equal(t, "nexus-register:node-1:runtime-1:2.1.0:peer-1:http.tls.v1", runtimeRegistrationSignature(core.NodeDescriptor{ID: "node-1"}, fwgateway.NodeConnectInfo{RuntimeID: "runtime-1", RuntimeVersion: "2.1.0", PeerKeyID: "peer-1", TransportProfile: "http.tls.v1"}))
	require.Equal(t, map[string]string{"x": "1"}, copyNodeTags(map[string]string{"x": "1"}))
	require.Nil(t, copyNodeTags(nil))

	fakeStore := &serverNodeStore{
		nodes: map[string]core.NodeDescriptor{
			"node-1": {ID: "node-1", ApprovedCapabilities: []core.CapabilityDescriptor{{ID: "cap-1", Name: "cap-1"}}},
		},
	}
	manager := &fwnode.Manager{Store: fakeStore}
	require.Len(t, connectedNodeCapabilities(context.Background(), manager, "node-1"), 1)
	require.Nil(t, connectedNodeCapabilities(context.Background(), manager, ""))

	principal := fwgateway.ConnectionPrincipal{Principal: &core.AuthenticatedPrincipal{SessionID: "sess-1"}}
	conn := NodeRPCConnForTransportForTest(principal, fwgateway.NodeConnectInfo{TransportProfile: "http.tls.v1", RuntimeID: "runtime-1"}, &stubRPCConn{})
	_, ok := conn.(*fwnode.FramedRPCConn)
	require.True(t, ok)

	require.Equal(t, "session store unavailable", mustErrString(t, func() error {
		_, err := InvokeAuthorizedNodeCapability(context.Background(), nil, nil, manager, principal, "sess-1", "cap-1", nil)
		return err
	}))

	router := &stubSessionRouter{}
	store := &stubSessionStore{boundary: &core.SessionBoundary{SessionID: "sess-1"}}
	connDesc := core.NodeDescriptor{Name: "node-1", ID: "node-1", Platform: core.NodePlatformLinux, TrustClass: core.TrustClassWorkspaceTrusted, ApprovedCapabilities: []core.CapabilityDescriptor{{ID: "cap-1", Name: "cap-1"}}}
	require.NoError(t, manager.HandleConnect(context.Background(), stubNodeConn{desc: connDesc}))
	result, err := InvokeAuthorizedNodeCapability(context.Background(), router, store, manager, fwgateway.ConnectionPrincipal{Actor: core.EventActor{TenantID: ""}}, "sess-1", "cap-1", map[string]any{"x": 1})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, router.authorized)

	require.Error(t, HandleGatewayNodeConnection(context.Background(), nil, nil, nil, principal, fwgateway.NodeConnectInfo{}, nil, nil))
}

func TestNodeRuntimeDescriptorAndTransportHelpers(t *testing.T) {
	tenantID := "tenant-a"
	var enrollments identity.Store = &identityStore{
		enrollments: map[string]*core.NodeEnrollment{
			"node-1": {
				NodeID:     "node-1",
				TenantID:   tenantID,
				TrustClass: core.TrustClassWorkspaceTrusted,
				Owner:      core.SubjectRef{TenantID: tenantID, Kind: core.SubjectKindUser, ID: "owner-1"},
				PairedAt:   time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC),
			},
		},
	}
	manager := &fwnode.Manager{
		Store: &serverNodeStore{
			nodes: map[string]core.NodeDescriptor{
				"node-1": {
					ID:         "node-1",
					Name:       "Node One",
					Platform:   core.NodePlatformLinux,
					TrustClass: core.TrustClassWorkspaceTrusted,
					Tags:       map[string]string{"role": "edge"},
					ApprovedCapabilities: []core.CapabilityDescriptor{{
						ID:   "cap-1",
						Name: "cap-1",
					}},
				},
			},
		},
	}
	principal := fwgateway.ConnectionPrincipal{
		Actor:         core.EventActor{TenantID: tenantID, ID: "node-1"},
		Authenticated: true,
	}
	frame := fwgateway.NodeConnectInfo{
		NodeID:                  "node-1",
		TrustDomain:             "mesh.local",
		RuntimeID:               "runtime-1",
		RuntimeVersion:          "1.2.3",
		CompatibilityClass:      "compat-a",
		SupportedContextClasses: []string{"planning"},
		TransportProfile:        "http.tls.v1",
		PeerKeyID:               "peer-1",
	}

	nodeDesc, err := ConnectedNodeDescriptorForTest(context.Background(), manager, enrollments, principal, frame)
	require.NoError(t, err)
	require.Equal(t, "Node One", nodeDesc.Name)
	require.Equal(t, map[string]string{"role": "edge"}, nodeDesc.Tags)

	disconnectCtx, cancel := NewNodeDisconnectContext(context.Background())
	require.NotNil(t, disconnectCtx)
	cancel()

	runtimeProvider := &RexRuntimeProvider{
		RuntimeEndpoint: &rexnexus.RuntimeEndpoint{
			DescriptorValue: core.RuntimeDescriptor{
				RuntimeID:               "runtime-override",
				TrustDomain:             "mesh.local",
				RuntimeVersion:          "9.9.9",
				CompatibilityClass:      "compat-b",
				SupportedContextClasses: []string{"override"},
			},
		},
	}
	descriptor := advertisedRuntimeDescriptor(nodeDesc, frame, runtimeProvider)
	require.Equal(t, "runtime-override", descriptor.RuntimeID)
	require.Equal(t, "mesh.local", descriptor.TrustDomain)
	require.Equal(t, "9.9.9", descriptor.RuntimeVersion)
	require.Equal(t, []string{"override"}, descriptor.SupportedContextClasses)
	require.Equal(t, "compat-b", descriptor.CompatibilityClass)

	meshStore := &recordingDiscoveryStore{}
	mesh := &fwfmp.Service{Discovery: meshStore}
	require.NoError(t, AdvertiseConnectedNodeToFMPWithRuntimeForTest(context.Background(), mesh, nodeDesc, frame, runtimeProvider))
	require.Len(t, meshStore.nodeAds, 1)
	require.Len(t, meshStore.runtimeAds, 1)
	require.Equal(t, "runtime-override", meshStore.runtimeAds[0].Runtime.RuntimeID)
	require.Equal(t, "node-1", meshStore.runtimeAds[0].Runtime.NodeID)
	require.Equal(t, "mesh.local", meshStore.runtimeAds[0].TrustDomain)

	handler := MeshTransportFrameHandlerWithRuntimeForTest(mesh, frame, runtimeProvider)
	conn := &fwnode.WSConnection{Conn: &recordingRPCConn{}, Descriptor: nodeDesc}
	registerFrame := map[string]json.RawMessage{
		"type":      json.RawMessage(`"fmp.runtime.register"`),
		"signature": json.RawMessage(`"sig-register"`),
		"runtime":   mustRawJSON(t, descriptor),
	}
	require.NoError(t, handler(context.Background(), conn, registerFrame))
	require.Len(t, conn.Conn.(*recordingRPCConn).writes, 1)

	exportFrame := map[string]json.RawMessage{
		"type":      json.RawMessage(`"fmp.export.advertise"`),
		"signature": json.RawMessage(`"sig-export"`),
		"export": mustRawJSON(t, core.ExportDescriptor{
			ExportName:       "agent.resume",
			RouteMode:        core.RouteModeGateway,
			SensitivityLimit: core.SensitivityClassLow,
		}),
	}
	require.NoError(t, handler(context.Background(), conn, exportFrame))
	require.Len(t, conn.Conn.(*recordingRPCConn).writes, 2)
}

func mustErrString(t *testing.T, fn func() error) string {
	t.Helper()
	err := fn()
	if err == nil {
		t.Fatal("expected error")
	}
	return err.Error()
}

type recordingAuditLogger struct {
	records []core.AuditRecord
}

func (r *recordingAuditLogger) Log(_ context.Context, record core.AuditRecord) error {
	r.records = append(r.records, record)
	return nil
}

func (r *recordingAuditLogger) Query(context.Context, core.AuditQuery) ([]core.AuditRecord, error) {
	return append([]core.AuditRecord(nil), r.records...), nil
}

type stubRPCConn struct{}

func (stubRPCConn) WriteJSON(v any) error { return nil }
func (stubRPCConn) ReadJSON(v any) error  { return nil }
func (stubRPCConn) Close() error          { return nil }

type stubNodeConn struct {
	desc core.NodeDescriptor
}

func (s stubNodeConn) Node() core.NodeDescriptor                 { return s.desc }
func (s stubNodeConn) Health() core.NodeHealth                   { return core.NodeHealth{Online: true} }
func (s stubNodeConn) Capabilities() []core.CapabilityDescriptor { return s.desc.ApprovedCapabilities }
func (s stubNodeConn) Invoke(context.Context, string, map[string]any) (*core.CapabilityExecutionResult, error) {
	return &core.CapabilityExecutionResult{Success: true}, nil
}
func (s stubNodeConn) Close(context.Context) error { return nil }

type serverNodeStore struct {
	nodes map[string]core.NodeDescriptor
}

func (s *serverNodeStore) GetNode(_ context.Context, id string) (*core.NodeDescriptor, error) {
	if node, ok := s.nodes[id]; ok {
		copy := node
		return &copy, nil
	}
	return nil, nil
}
func (s *serverNodeStore) ListNodes(context.Context) ([]core.NodeDescriptor, error) { return nil, nil }
func (s *serverNodeStore) UpsertNode(context.Context, core.NodeDescriptor) error    { return nil }
func (s *serverNodeStore) RemoveNode(context.Context, string) error                 { return nil }
func (s *serverNodeStore) GetCredential(context.Context, string) (*core.NodeCredential, error) {
	return nil, nil
}
func (s *serverNodeStore) SaveCredential(context.Context, core.NodeCredential) error { return nil }
func (s *serverNodeStore) SavePendingPairing(context.Context, fwnode.PendingPairing) error {
	return nil
}
func (s *serverNodeStore) GetPendingPairing(context.Context, string) (*fwnode.PendingPairing, error) {
	return nil, nil
}
func (s *serverNodeStore) ListPendingPairings(context.Context) ([]fwnode.PendingPairing, error) {
	return nil, nil
}
func (s *serverNodeStore) DeletePendingPairing(context.Context, string) error { return nil }
func (s *serverNodeStore) DeleteExpiredPendingPairings(context.Context, time.Time) (int, error) {
	return 0, nil
}

type stubSessionRouter struct {
	authorized bool
}

func (s *stubSessionRouter) Route(context.Context, session.InboundMessage) (*core.SessionBoundary, error) {
	return nil, nil
}
func (s *stubSessionRouter) Authorize(_ context.Context, req session.AuthorizationRequest) error {
	s.authorized = true
	if req.Boundary == nil {
		return errors.New("boundary required")
	}
	return nil
}

type stubSessionStore struct {
	boundary *core.SessionBoundary
}

func (s *stubSessionStore) GetBoundary(context.Context, string) (*core.SessionBoundary, error) {
	return nil, nil
}
func (s *stubSessionStore) GetBoundaryBySessionID(context.Context, string) (*core.SessionBoundary, error) {
	return s.boundary, nil
}
func (s *stubSessionStore) UpsertBoundary(context.Context, string, *core.SessionBoundary) error {
	return nil
}
func (s *stubSessionStore) ListBoundaries(context.Context, string) ([]core.SessionBoundary, error) {
	return nil, nil
}
func (s *stubSessionStore) UpsertDelegation(context.Context, core.SessionDelegationRecord) error {
	return nil
}
func (s *stubSessionStore) ListDelegationsBySessionID(context.Context, string) ([]core.SessionDelegationRecord, error) {
	return nil, nil
}
func (s *stubSessionStore) ListDelegationsByTenantID(context.Context, string) ([]core.SessionDelegationRecord, error) {
	return nil, nil
}
func (s *stubSessionStore) DeleteBoundary(context.Context, string) error { return nil }
func (s *stubSessionStore) DeleteExpiredBoundaries(context.Context, time.Time) (int, error) {
	return 0, nil
}

type identityStore struct {
	enrollments map[string]*core.NodeEnrollment
}

func (s *identityStore) UpsertTenant(context.Context, core.TenantRecord) error { return nil }
func (s *identityStore) GetTenant(context.Context, string) (*core.TenantRecord, error) {
	return nil, nil
}
func (s *identityStore) ListTenants(context.Context) ([]core.TenantRecord, error) { return nil, nil }
func (s *identityStore) UpsertSubject(context.Context, core.SubjectRecord) error  { return nil }
func (s *identityStore) GetSubject(context.Context, string, core.SubjectKind, string) (*core.SubjectRecord, error) {
	return nil, nil
}
func (s *identityStore) ListSubjects(context.Context, string) ([]core.SubjectRecord, error) {
	return nil, nil
}
func (s *identityStore) UpsertExternalIdentity(context.Context, core.ExternalIdentity) error {
	return nil
}
func (s *identityStore) GetExternalIdentity(context.Context, string, core.ExternalProvider, string, string) (*core.ExternalIdentity, error) {
	return nil, nil
}
func (s *identityStore) ListExternalIdentities(context.Context, string) ([]core.ExternalIdentity, error) {
	return nil, nil
}
func (s *identityStore) UpsertNodeEnrollment(context.Context, core.NodeEnrollment) error { return nil }
func (s *identityStore) GetNodeEnrollment(_ context.Context, _, nodeID string) (*core.NodeEnrollment, error) {
	if enrollment, ok := s.enrollments[nodeID]; ok {
		copy := *enrollment
		return &copy, nil
	}
	return nil, nil
}
func (s *identityStore) ListNodeEnrollments(context.Context, string) ([]core.NodeEnrollment, error) {
	return nil, nil
}
func (s *identityStore) DeleteNodeEnrollment(context.Context, string, string) error { return nil }

type recordingDiscoveryStore struct {
	nodeAds    []core.NodeAdvertisement
	runtimeAds []core.RuntimeAdvertisement
	exportAds  []core.ExportAdvertisement
}

func (s *recordingDiscoveryStore) UpsertNodeAdvertisement(_ context.Context, ad core.NodeAdvertisement) error {
	s.nodeAds = append(s.nodeAds, ad)
	return nil
}
func (s *recordingDiscoveryStore) UpsertRuntimeAdvertisement(_ context.Context, ad core.RuntimeAdvertisement) error {
	s.runtimeAds = append(s.runtimeAds, ad)
	return nil
}
func (s *recordingDiscoveryStore) UpsertExportAdvertisement(_ context.Context, ad core.ExportAdvertisement) error {
	s.exportAds = append(s.exportAds, ad)
	return nil
}
func (s *recordingDiscoveryStore) ListNodeAdvertisements(context.Context) ([]core.NodeAdvertisement, error) {
	return append([]core.NodeAdvertisement(nil), s.nodeAds...), nil
}
func (s *recordingDiscoveryStore) ListRuntimeAdvertisements(context.Context) ([]core.RuntimeAdvertisement, error) {
	return append([]core.RuntimeAdvertisement(nil), s.runtimeAds...), nil
}
func (s *recordingDiscoveryStore) ListExportAdvertisements(context.Context) ([]core.ExportAdvertisement, error) {
	return append([]core.ExportAdvertisement(nil), s.exportAds...), nil
}
func (s *recordingDiscoveryStore) DeleteExpired(context.Context, time.Time) error { return nil }

type recordingRPCConn struct {
	writes []any
}

func (c *recordingRPCConn) WriteJSON(v any) error {
	c.writes = append(c.writes, v)
	return nil
}

func (c *recordingRPCConn) ReadJSON(v any) error { return errors.New("no reads expected") }
func (c *recordingRPCConn) Close() error         { return nil }

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return json.RawMessage(data)
}
