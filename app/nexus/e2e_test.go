package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusserver "github.com/lexcodex/relurpify/app/nexus/server"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	mcpprotocol "github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	mcpserver "github.com/lexcodex/relurpify/framework/middleware/mcp/server"
	"github.com/lexcodex/relurpify/testsuite/nexustest"
	"github.com/stretchr/testify/require"
)

type nexusHarness struct {
	t             *testing.T
	ctx           context.Context
	cancel        context.CancelFunc
	server        *httptest.Server
	eventLog      *db.SQLiteEventLog
	sessionStore  *db.SQLiteSessionStore
	identityStore *db.SQLiteIdentityStore
	nodeStore     *db.SQLiteNodeStore
	tokenStore    *db.SQLiteAdminTokenStore
	policyStore   *db.FilePolicyRuleStore
	policyFile    string
	adapter       *channel.TestChannelAdapter
}

func newNexusHarness(t *testing.T, cfg nexuscfg.Config) *nexusHarness {
	t.Helper()

	dir := t.TempDir()
	eventLog, err := db.NewSQLiteEventLog(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(dir, "sessions.db"))
	require.NoError(t, err)
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(dir, "identities.db"))
	require.NoError(t, err)
	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(dir, "nodes.db"))
	require.NoError(t, err)
	tokenStore, err := db.NewSQLiteAdminTokenStore(filepath.Join(dir, "admin_tokens.db"))
	require.NoError(t, err)
	policyFile := filepath.Join(dir, "policy_rules.yaml")
	policyStore, err := db.NewFilePolicyRuleStore(policyFile)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	adapter := channel.NewTestChannelAdapter("webchat")
	handler, err := (&nexusserver.NexusApp{
		EventLog:          eventLog,
		SessionStore:      sessionStore,
		IdentityStore:     identityStore,
		NodeStore:         nodeStore,
		TokenStore:        tokenStore,
		PolicyStore:       policyStore,
		Config:            cfg,
		Partition:         nexusEventPartition,
		ChannelAdapters:   []channel.Adapter{adapter},
		PrincipalResolver: gatewayPrincipalResolver(cfg.Gateway.Auth, tokenStore),
		VerifyNodeConnection: func(ctx context.Context, store identity.Store, principal fwgateway.ConnectionPrincipal, info fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
			return verifyGatewayNodeChallenge(ctx, store, principal, info, conn)
		},
	}).Handler(ctx)
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	h := &nexusHarness{
		t:             t,
		ctx:           ctx,
		cancel:        cancel,
		server:        server,
		eventLog:      eventLog,
		sessionStore:  sessionStore,
		identityStore: identityStore,
		nodeStore:     nodeStore,
		tokenStore:    tokenStore,
		policyStore:   policyStore,
		policyFile:    policyFile,
		adapter:       adapter,
	}
	t.Cleanup(func() {
		server.Close()
		cancel()
		require.NoError(t, nodeStore.Close())
		require.NoError(t, tokenStore.Close())
		require.NoError(t, identityStore.Close())
		require.NoError(t, sessionStore.Close())
		require.NoError(t, eventLog.Close())
	})
	return h
}

func (h *nexusHarness) gatewayURL() string {
	return h.server.URL + "/gateway"
}

func (h *nexusHarness) adminMCPURL() string {
	return h.server.URL + "/admin/mcp"
}

func (h *nexusHarness) newClient(t *testing.T, token, role string) *nexustest.TestGatewayClient {
	t.Helper()
	client := nexustest.NewTestGatewayClient()
	require.NoError(t, client.Connect(h.gatewayURL(), token, role))
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func (h *nexusHarness) adminMCPCall(t *testing.T, token, sessionID string, envelope map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.adminMCPURL(), strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if sessionID != "" {
		req.Header.Set(mcpserver.SessionHeader, sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	payload := map[string]any{}
	if resp.StatusCode != http.StatusAccepted {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	}
	return resp, payload
}

func (h *nexusHarness) waitForEvent(t *testing.T, client *nexustest.TestGatewayClient, eventType string, timeout time.Duration) core.FrameworkEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-client.Events():
			require.True(t, ok, "gateway event stream closed before %s", eventType)
			if ev.Type == eventType {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", eventType)
		}
	}
}

func (h *nexusHarness) assertNoEventOfType(t *testing.T, client *nexustest.TestGatewayClient, eventType string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ev := <-client.Events():
		if ev.Type == eventType {
			t.Fatalf("unexpected event %s", ev.Type)
		}
	case <-timer.C:
	}
}

func (h *nexusHarness) waitForOutbound(t *testing.T, timeout time.Duration) channel.OutboundMessage {
	t.Helper()
	select {
	case msg := <-h.adapter.Outbound():
		return msg
	case <-time.After(timeout):
		t.Fatal("timed out waiting for outbound message")
		return channel.OutboundMessage{}
	}
}

func (h *nexusHarness) countEvents(t *testing.T, eventType string) int {
	t.Helper()
	events, err := h.eventLog.Read(context.Background(), nexusEventPartition, 0, 256, false)
	require.NoError(t, err)
	count := 0
	for _, ev := range events {
		if ev.Type == eventType {
			count++
		}
	}
	return count
}

func (h *nexusHarness) waitForLoggedEvent(t *testing.T, eventType string, predicate func(core.FrameworkEvent) bool) core.FrameworkEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, err := h.eventLog.Read(context.Background(), nexusEventPartition, 0, 256, false)
		require.NoError(t, err)
		for _, ev := range events {
			if ev.Type != eventType {
				continue
			}
			if predicate == nil || predicate(ev) {
				return ev
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for logged event %s", eventType)
	return core.FrameworkEvent{}
}

func (h *nexusHarness) appendEvents(t *testing.T, events ...core.FrameworkEvent) []uint64 {
	t.Helper()
	seqs, err := h.eventLog.Append(context.Background(), nexusEventPartition, events)
	require.NoError(t, err)
	return seqs
}

func (h *nexusHarness) waitForBoundary(t *testing.T, predicate func(core.SessionBoundary) bool) core.SessionBoundary {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		boundaries, err := h.sessionStore.ListBoundaries(context.Background(), nexusEventPartition)
		require.NoError(t, err)
		for _, boundary := range boundaries {
			if predicate == nil || predicate(boundary) {
				return boundary
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for session boundary")
	return core.SessionBoundary{}
}

func (h *nexusHarness) collectEvents(t *testing.T, client *nexustest.TestGatewayClient, quietPeriod time.Duration) []core.FrameworkEvent {
	t.Helper()
	var out []core.FrameworkEvent
	timer := time.NewTimer(quietPeriod)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-client.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(quietPeriod)
		case <-timer.C:
			return out
		}
	}
}

func (h *nexusHarness) waitForCapabilities(t *testing.T, tenantID string, expected int) []core.CapabilityDescriptor {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client := nexustest.NewTestGatewayClient()
		require.NoError(t, client.Connect(h.gatewayURL(), "agent-a", "agent"))
		caps := client.Capabilities()
		require.NoError(t, client.Close())
		if len(caps) == expected {
			return caps
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d capabilities for tenant %s", expected, tenantID)
	return nil
}

func (h *nexusHarness) seedBoundary(t *testing.T, boundary *core.SessionBoundary) {
	t.Helper()
	key := core.SessionBoundaryKey(boundary.Scope, boundary.Partition, boundary.ChannelID, boundary.PeerID, "")
	require.NoError(t, h.sessionStore.UpsertBoundary(context.Background(), key, boundary))
}

func (h *nexusHarness) enrollNode(t *testing.T, tenantID string, cred core.NodeCredential) {
	t.Helper()
	cred.TenantID = tenantID
	require.NoError(t, h.identityStore.UpsertNodeEnrollment(context.Background(), core.NodeEnrollment{
		TenantID:   tenantID,
		NodeID:     cred.DeviceID,
		TrustClass: core.TrustClassRemoteApproved,
		Owner: core.SubjectRef{
			TenantID: tenantID,
			Kind:     core.SubjectKindNode,
			ID:       cred.DeviceID,
		},
		PublicKey:  append([]byte(nil), cred.PublicKey...),
		KeyID:      cred.KeyID,
		PairedAt:   cred.IssuedAt,
		AuthMethod: core.AuthMethodNodeChallenge,
	}))
}

func testGatewayConfig() nexuscfg.Config {
	return nexuscfg.Config{
		Gateway: nexuscfg.GatewayConfig{
			Path: "/gateway",
			Auth: nexuscfg.GatewayAuthConfig{
				Enabled: true,
				Tokens: []nexuscfg.GatewayTokenAuth{
					{Token: "agent-a", TenantID: "tenant-a", Role: "agent", SubjectID: "svc-a"},
					{Token: "agent-b", TenantID: "tenant-b", Role: "agent", SubjectID: "svc-b"},
					{Token: "admin-a", TenantID: "tenant-a", Role: "admin", SubjectID: "admin-a", Scopes: []string{"gateway:admin"}},
					{Token: "node-a", TenantID: "tenant-a", Role: "node", SubjectID: "node-a"},
					{Token: "node-missing", TenantID: "tenant-a", Role: "node", SubjectID: "node-missing"},
				},
			},
		},
	}
}

func TestAgentConnectsWithValidToken(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	clientA := h.newClient(t, "agent-a", "agent")
	clientB := h.newClient(t, "agent-a", "agent")

	require.NotEmpty(t, clientA.SessionID())
	require.NotEmpty(t, clientB.SessionID())
	require.NotEqual(t, clientA.SessionID(), clientB.SessionID())
}

func TestAgentRejectedWithBadToken(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	_, resp, err := websocket.DefaultDialer.Dial(strings.Replace(h.gatewayURL(), "http://", "ws://", 1), http.Header{
		"Authorization": []string{"Bearer bad-token"},
	})
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAgentRejectedWithoutToken(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	_, resp, err := websocket.DefaultDialer.Dial(strings.Replace(h.gatewayURL(), "http://", "ws://", 1), nil)
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAgentCannotSpoofActorID(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(h.gatewayURL(), "http://", "ws://", 1), http.Header{
		"Authorization": []string{"Bearer agent-a"},
	})
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          "agent",
		"actor_id":      "admin",
		"last_seen_seq": 0,
	}))
	var connected map[string]any
	require.NoError(t, conn.ReadJSON(&connected))

	ev := h.waitForLoggedEvent(t, core.FrameworkEventSessionCreated, func(ev core.FrameworkEvent) bool {
		return ev.Actor.Kind == "agent"
	})
	require.Equal(t, "svc-a", ev.Actor.ID)
	require.Equal(t, "tenant-a", ev.Actor.TenantID)
}

func TestAgentRoleLockedToToken(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(h.gatewayURL(), "http://", "ws://", 1), http.Header{
		"Authorization": []string{"Bearer agent-a"},
	})
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          "operator",
		"last_seen_seq": 0,
	}))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestBroadcastRespectsTenantIsolation(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	tenantA := h.newClient(t, "agent-a", "agent")
	tenantB := h.newClient(t, "agent-b", "agent")
	admin := h.newClient(t, "admin-a", "admin")

	_, err := h.eventLog.Append(context.Background(), nexusEventPartition, []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
		Partition: nexusEventPartition,
		Payload:   json.RawMessage(`{"text":"tenant-a"}`),
	}})
	require.NoError(t, err)

	ev := h.waitForEvent(t, tenantA, core.FrameworkEventMessageInbound, 2*time.Second)
	require.Equal(t, "tenant-a", ev.Actor.TenantID)
	h.assertNoEventOfType(t, tenantB, core.FrameworkEventMessageInbound, 250*time.Millisecond)
	adminEvent := h.waitForEvent(t, admin, core.FrameworkEventMessageInbound, 2*time.Second)
	require.Equal(t, "tenant-a", adminEvent.Actor.TenantID)
}

func TestAdminMCPHealthResource(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	initResp, initPayload := h.adminMCPCall(t, "admin-a", "", map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": mcpprotocol.InitializeRequest{
			ProtocolVersion: mcpprotocol.Revision20250618,
			ClientInfo:      mcpprotocol.PeerInfo{Name: "test-client", Version: "1.0.0"},
		},
	})
	require.Equal(t, http.StatusOK, initResp.StatusCode)
	require.NotNil(t, initPayload["result"])

	sessionID := initResp.Header.Get(mcpserver.SessionHeader)
	require.NotEmpty(t, sessionID)

	notifyResp, _ := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	require.Equal(t, http.StatusAccepted, notifyResp.StatusCode)

	readResp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "resources/read",
		"params":  mcpprotocol.ReadResourceParams{URI: "nexus://gateway/health"},
	})
	require.Equal(t, http.StatusOK, readResp.StatusCode)
	result := payload["result"].(map[string]any)
	contents := result["contents"].([]any)
	require.NotEmpty(t, contents)
	content := contents[0].(map[string]any)
	require.Equal(t, "application/json", content["mimeType"])
}

func TestAdminMPCListsNexusishResources(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	initResp, initPayload := h.adminMCPCall(t, "admin-a", "", map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": mcpprotocol.InitializeRequest{
			ProtocolVersion: mcpprotocol.Revision20250618,
			ClientInfo:      mcpprotocol.PeerInfo{Name: "test-client", Version: "1.0.0"},
		},
	})
	require.Equal(t, http.StatusOK, initResp.StatusCode)
	require.NotNil(t, initPayload["result"])

	sessionID := initResp.Header.Get(mcpserver.SessionHeader)
	require.NotEmpty(t, sessionID)

	notifyResp, _ := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	require.Equal(t, http.StatusAccepted, notifyResp.StatusCode)

	resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "resources/list",
		"params":  map[string]any{},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	result := payload["result"].(map[string]any)
	resources := result["resources"].([]any)
	uris := make([]string, 0, len(resources))
	for _, resource := range resources {
		uris = append(uris, resource.(map[string]any)["uri"].(string))
	}
	require.Contains(t, uris, "nexus://gateway/health")
	require.Contains(t, uris, "nexus://nodes/enrolled")
	require.Contains(t, uris, "nexus://nodes/pending")
	require.Contains(t, uris, "nexus://channels/status")
	require.Contains(t, uris, "nexus://sessions/active")
	require.Contains(t, uris, "nexus://events/stream")
	require.Contains(t, uris, "nexus://identity/subjects")
	require.Contains(t, uris, "nexus://identity/externals")
}

func TestAdminMCPResourcesAndRevokeTool(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	node := core.NodeDescriptor{
		ID:         "device-1",
		TenantID:   "tenant-a",
		Name:       "Device 1",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassRemoteApproved,
		PairedAt:   time.Now().UTC(),
		Owner: core.SubjectRef{
			TenantID: "tenant-a",
			Kind:     core.SubjectKindNode,
			ID:       "device-1",
		},
	}
	require.NoError(t, h.nodeStore.UpsertNode(context.Background(), node))
	h.enrollNode(t, "tenant-a", core.NodeCredential{
		DeviceID:  "device-1",
		PublicKey: []byte("pk"),
		IssuedAt:  time.Now().UTC(),
	})
	require.NoError(t, h.identityStore.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:    "tenant-a",
		Provider:    core.ExternalProviderWebchat,
		AccountID:   "workspace-a",
		ExternalID:  "ext-1",
		DisplayName: "User 1",
		Subject: core.SubjectRef{
			TenantID: "tenant-a",
			Kind:     core.SubjectKindUser,
			ID:       "user-1",
		},
	}))
	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:      "session-1",
		RoutingKey:     core.SessionBoundaryKey(core.SessionScopePerChannelPeer, nexusEventPartition, "webchat", "peer-1", ""),
		TenantID:       "tenant-a",
		Scope:          core.SessionScopePerChannelPeer,
		Partition:      nexusEventPartition,
		ChannelID:      "webchat",
		PeerID:         "peer-1",
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
	})
	h.appendEvents(t, core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
		Partition: nexusEventPartition,
		Payload:   json.RawMessage(`{"text":"hello"}`),
	})

	initResp, initPayload := h.adminMCPCall(t, "admin-a", "", map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": mcpprotocol.InitializeRequest{
			ProtocolVersion: mcpprotocol.Revision20250618,
			ClientInfo:      mcpprotocol.PeerInfo{Name: "test-client", Version: "1.0.0"},
		},
	})
	require.Equal(t, http.StatusOK, initResp.StatusCode)
	require.NotNil(t, initPayload["result"])

	sessionID := initResp.Header.Get(mcpserver.SessionHeader)
	require.NotEmpty(t, sessionID)

	notifyResp, _ := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	require.Equal(t, http.StatusAccepted, notifyResp.StatusCode)

	readJSON := func(uri string) map[string]any {
		t.Helper()
		resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
			"jsonrpc": "2.0",
			"id":      uri,
			"method":  "resources/read",
			"params":  mcpprotocol.ReadResourceParams{URI: uri},
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		result := payload["result"].(map[string]any)
		contents := result["contents"].([]any)
		require.NotEmpty(t, contents)
		text := contents[0].(map[string]any)["text"].(string)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &decoded))
		return decoded
	}

	channels := readJSON("nexus://channels/status")
	require.NotEmpty(t, channels["channels"])

	sessions := readJSON("nexus://sessions/active")
	require.Len(t, sessions["sessions"], 1)

	events := readJSON("nexus://events/stream?after_seq=0")
	require.NotEmpty(t, events["events"])

	subjects := readJSON("nexus://identity/subjects")
	require.NotEmpty(t, subjects["subjects"])
	externals := readJSON("nexus://identity/externals")
	require.NotEmpty(t, externals["identities"])

	resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "revoke-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name:      "nexus.nodes.revoke",
			Arguments: map[string]any{"node_id": "device-1", "api_version": "v1alpha1"},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	result := payload["result"].(map[string]any)
	if isError, ok := result["isError"].(bool); ok {
		require.False(t, isError)
	}
	nodeAfter, err := h.nodeStore.GetNode(context.Background(), "device-1")
	require.NoError(t, err)
	require.Nil(t, nodeAfter)
}

func TestAdminMCPCloseSessionAndManageTokensAndPolicies(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:      "session-close-me",
		RoutingKey:     core.SessionBoundaryKey(core.SessionScopePerChannelPeer, nexusEventPartition, "webchat", "peer-close", ""),
		TenantID:       "tenant-a",
		Scope:          core.SessionScopePerChannelPeer,
		Partition:      nexusEventPartition,
		ChannelID:      "webchat",
		PeerID:         "peer-close",
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
	})
	require.NoError(t, os.WriteFile(h.policyFile, []byte(`
- id: rule-a
  name: Rule A
  priority: 100
  enabled: true
  effect:
    action: allow
`), 0o644))

	initResp, initPayload := h.adminMCPCall(t, "admin-a", "", map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": mcpprotocol.InitializeRequest{
			ProtocolVersion: mcpprotocol.Revision20250618,
			ClientInfo:      mcpprotocol.PeerInfo{Name: "test-client", Version: "1.0.0"},
		},
	})
	require.Equal(t, http.StatusOK, initResp.StatusCode)
	require.NotNil(t, initPayload["result"])
	sessionID := initResp.Header.Get(mcpserver.SessionHeader)
	require.NotEmpty(t, sessionID)

	notifyResp, _ := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	require.Equal(t, http.StatusAccepted, notifyResp.StatusCode)

	resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "close-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name:      "nexus.sessions.close",
			Arguments: map[string]any{"session_id": "session-close-me", "api_version": "v1alpha1"},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	boundary, err := h.sessionStore.GetBoundaryBySessionID(context.Background(), "session-close-me")
	require.NoError(t, err)
	require.Nil(t, boundary)

	resp, payload = h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "token-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.tokens.issue",
			Arguments: map[string]any{
				"subject_id":  "subject-a",
				"scopes":      []string{"nexus:operator"},
				"api_version": "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	issued := payload["result"].(map[string]any)["structuredContent"].(map[string]any)
	tokenID := issued["token_id"].(string)
	tokenValue := issued["token"].(string)
	require.NotEmpty(t, tokenID)
	require.NotEmpty(t, tokenValue)

	initResp, initPayload = h.adminMCPCall(t, tokenValue, "", map[string]any{
		"jsonrpc": "2.0",
		"id":      "dyn-1",
		"method":  "initialize",
		"params": mcpprotocol.InitializeRequest{
			ProtocolVersion: mcpprotocol.Revision20250618,
			ClientInfo:      mcpprotocol.PeerInfo{Name: "dyn-client", Version: "1.0.0"},
		},
	})
	require.Equal(t, http.StatusOK, initResp.StatusCode)
	require.NotNil(t, initPayload["result"])

	readJSON := func(uri string) map[string]any {
		t.Helper()
		resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
			"jsonrpc": "2.0",
			"id":      uri,
			"method":  "resources/read",
			"params":  mcpprotocol.ReadResourceParams{URI: uri},
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		result := payload["result"].(map[string]any)
		contents := result["contents"].([]any)
		text := contents[0].(map[string]any)["text"].(string)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &decoded))
		return decoded
	}

	tokens := readJSON("nexus://tokens/list")
	require.Len(t, tokens["tokens"], 1)

	resp, _ = h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "token-2",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name:      "nexus.tokens.revoke",
			Arguments: map[string]any{"token_id": tokenID, "api_version": "v1alpha1"},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	tokens = readJSON("nexus://tokens/list")
	tokenEntry := tokens["tokens"].([]any)[0].(map[string]any)
	require.NotEmpty(t, tokenEntry["revoked_at"])

	policies := readJSON("nexus://policy/rules")
	require.Len(t, policies["rules"], 1)
	require.Equal(t, true, policies["rules"].([]any)[0].(map[string]any)["enabled"])

	resp, _ = h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "policy-2",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name:      "nexus.policy.set_rule_enabled",
			Arguments: map[string]any{"rule_id": "rule-a", "enabled": false, "api_version": "v1alpha1"},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	policies = readJSON("nexus://policy/rules")
	require.Equal(t, false, policies["rules"].([]any)[0].(map[string]any)["enabled"])

	resp, _ = h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "chan-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name:      "nexus.channels.restart",
			Arguments: map[string]any{"channel": "webchat", "api_version": "v1alpha1"},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	channels := readJSON("nexus://channels/status")
	require.NotEmpty(t, channels["channels"])
}

func TestInboundMessageCreatesSession(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	require.NoError(t, h.identityStore.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "local",
		Provider:   core.ExternalProviderWebchat,
		AccountID:  "workspace-a",
		ExternalID: "peer-1",
		Subject: core.SubjectRef{
			TenantID: "local",
			Kind:     core.SubjectKindUser,
			ID:       "user-1",
		},
	}))

	require.NoError(t, h.adapter.SendInbound(context.Background(), channel.InboundMessage{
		Channel: "webchat",
		Account: "workspace-a",
		Sender: channel.Identity{
			ChannelID: "peer-1",
		},
		Conversation: channel.Conversation{
			Kind: "dm",
			ID:   "conv-1",
		},
		Content: channel.MessageContent{Text: "hello"},
	}))

	created := h.waitForLoggedEvent(t, core.FrameworkEventSessionCreated, func(ev core.FrameworkEvent) bool {
		return ev.Actor.Kind == "system" && ev.Actor.ID == "session-router"
	})
	require.Equal(t, "local", created.Actor.TenantID)

	boundary := h.waitForBoundary(t, func(boundary core.SessionBoundary) bool {
		return boundary.ChannelID == "webchat" && boundary.PeerID == "conv-1"
	})
	require.Equal(t, "local", boundary.TenantID)
	require.Equal(t, "user-1", boundary.Owner.ID)
	require.Equal(t, core.TrustClassRemoteApproved, boundary.TrustClass)
	require.NotEmpty(t, boundary.SessionID)

	routed := h.waitForLoggedEvent(t, core.FrameworkEventSessionMessage, func(ev core.FrameworkEvent) bool {
		return ev.Actor.ID == boundary.SessionID
	})
	var payload struct {
		SessionKey string `json:"session_key"`
		Content    struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(routed.Payload, &payload))
	require.Equal(t, boundary.SessionID, payload.SessionKey)
	require.Equal(t, "hello", payload.Content.Text)
}

func TestInboundMessageRoutesToExistingSession(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	require.NoError(t, h.identityStore.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "local",
		Provider:   core.ExternalProviderWebchat,
		AccountID:  "workspace-a",
		ExternalID: "peer-1",
		Subject: core.SubjectRef{
			TenantID: "local",
			Kind:     core.SubjectKindUser,
			ID:       "user-1",
		},
	}))

	sendInbound := func(text string) {
		t.Helper()
		require.NoError(t, h.adapter.SendInbound(context.Background(), channel.InboundMessage{
			Channel: "webchat",
			Account: "workspace-a",
			Sender: channel.Identity{
				ChannelID: "peer-1",
			},
			Conversation: channel.Conversation{
				Kind: "dm",
				ID:   "conv-1",
			},
			Content: channel.MessageContent{Text: text},
		}))
	}

	sendInbound("first")
	first := h.waitForLoggedEvent(t, core.FrameworkEventSessionMessage, func(ev core.FrameworkEvent) bool {
		var payload struct {
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		return json.Unmarshal(ev.Payload, &payload) == nil && payload.Content.Text == "first"
	})
	sendInbound("second")
	second := h.waitForLoggedEvent(t, core.FrameworkEventSessionMessage, func(ev core.FrameworkEvent) bool {
		var payload struct {
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		return json.Unmarshal(ev.Payload, &payload) == nil && payload.Content.Text == "second"
	})

	var firstPayload struct {
		SessionKey string `json:"session_key"`
	}
	var secondPayload struct {
		SessionKey string `json:"session_key"`
	}
	require.NoError(t, json.Unmarshal(first.Payload, &firstPayload))
	require.NoError(t, json.Unmarshal(second.Payload, &secondPayload))
	require.Equal(t, firstPayload.SessionKey, secondPayload.SessionKey)

	boundaries, err := h.sessionStore.ListBoundaries(context.Background(), nexusEventPartition)
	require.NoError(t, err)
	require.Len(t, boundaries, 1)
}

func TestUnresolvedExternalIdentityGetsIsolatedTenant(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	require.NoError(t, h.adapter.SendInbound(context.Background(), channel.InboundMessage{
		Channel: "webchat",
		Account: "workspace-a",
		Sender: channel.Identity{
			ChannelID: "anonymous-peer",
		},
		Conversation: channel.Conversation{
			Kind: "dm",
			ID:   "conv-unresolved",
		},
		Content: channel.MessageContent{Text: "who am i"},
	}))

	boundary := h.waitForBoundary(t, func(boundary core.SessionBoundary) bool {
		return boundary.ChannelID == "webchat" && boundary.PeerID == "conv-unresolved"
	})
	require.Equal(t, "__relurpify_unresolved_external__", boundary.TenantID)
	require.Empty(t, boundary.Owner.ID)
	require.Equal(t, core.TrustClassRemoteDeclared, boundary.TrustClass)

	routed := h.waitForLoggedEvent(t, core.FrameworkEventSessionMessage, func(ev core.FrameworkEvent) bool {
		return ev.Actor.ID == boundary.SessionID
	})
	require.Equal(t, "__relurpify_unresolved_external__", routed.Actor.TenantID)
}

func TestNodeConnectsAndRegistersCapabilities(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())
	nodeCaps := []core.CapabilityDescriptor{{
		ID:            "camera.capture",
		Name:          "camera.capture",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}}
	device, err := nexustest.NewTestNodeDevice("node-a", nodeCaps)
	require.NoError(t, err)
	h.enrollNode(t, "tenant-a", device.Credential())
	require.NoError(t, device.Connect(context.Background(), h.gatewayURL(), "node-a", "Node A", core.NodePlatformLinux))
	t.Cleanup(func() {
		_ = device.Close()
	})

	caps := h.waitForCapabilities(t, "tenant-a", 1)
	require.Len(t, caps, 1)
	require.Equal(t, "camera.capture", caps[0].ID)
	require.Equal(t, "node:node-a", caps[0].Source.ProviderID)
	require.Equal(t, core.TrustClassRemoteApproved, caps[0].TrustClass)
}

func TestNodeDisconnectClearsCapabilities(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())
	device, err := nexustest.NewTestNodeDevice("node-a", []core.CapabilityDescriptor{{
		ID:   "camera.capture",
		Name: "camera.capture",
		Kind: core.CapabilityKindTool,
	}})
	require.NoError(t, err)
	h.enrollNode(t, "tenant-a", device.Credential())
	require.NoError(t, device.Connect(context.Background(), h.gatewayURL(), "node-a", "Node A", core.NodePlatformLinux))
	require.Len(t, h.waitForCapabilities(t, "tenant-a", 1), 1)

	require.NoError(t, device.Close())
	require.Eventually(t, func() bool {
		client := nexustest.NewTestGatewayClient()
		if err := client.Connect(h.gatewayURL(), "agent-a", "agent"); err != nil {
			return false
		}
		defer func() { _ = client.Close() }()
		return len(client.Capabilities()) == 0
	}, 2*time.Second, 20*time.Millisecond)
}

func TestAgentInvokesNodeCapability(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())
	device, err := nexustest.NewTestNodeDevice("node-a", []core.CapabilityDescriptor{{
		ID:   "camera.capture",
		Name: "camera.capture",
		Kind: core.CapabilityKindTool,
	}})
	require.NoError(t, err)
	h.enrollNode(t, "tenant-a", device.Credential())
	device.SetInvokeHandler(func(invocation nexustest.TestNodeInvocation) *core.CapabilityExecutionResult {
		return &core.CapabilityExecutionResult{
			Success: true,
			Data: map[string]any{
				"device": invocation.CapabilityID,
				"args":   invocation.Args,
			},
		}
	})
	require.NoError(t, device.Connect(context.Background(), h.gatewayURL(), "node-a", "Node A", core.NodePlatformLinux))
	t.Cleanup(func() {
		_ = device.Close()
	})

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:  "sess-owned",
		TenantID:   "tenant-a",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-1",
		Owner:      core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	})

	client := h.newClient(t, "agent-a", "agent")
	require.Len(t, client.Capabilities(), 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := client.InvokeCapability(ctx, "sess-owned", "camera.capture", map[string]any{"quality": "high"})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "camera.capture", result.Data["device"])

	select {
	case invocation := <-device.Invocations():
		require.Equal(t, "camera.capture", invocation.CapabilityID)
		require.Equal(t, "high", invocation.Args["quality"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for node invocation")
	}
}

func TestNodeRejectedWithBadSignature(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	device, err := nexustest.NewTestNodeDevice("node-a", nil)
	require.NoError(t, err)
	h.enrollNode(t, "tenant-a", device.Credential())

	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(h.gatewayURL(), "http://", "ws://", 1), http.Header{
		"Authorization": []string{"Bearer node-a"},
	})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          "node",
		"last_seen_seq": 0,
		"node_id":       "node-a",
		"node_name":     "Node A",
		"node_platform": string(core.NodePlatformLinux),
	}))

	var connected map[string]any
	require.NoError(t, conn.ReadJSON(&connected))
	require.Equal(t, "connected", connected["type"])

	var challenge map[string]any
	require.NoError(t, conn.ReadJSON(&challenge))
	require.Equal(t, "node.challenge", challenge["type"])

	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":      "node.challenge.response",
		"signature": "ZmFrZS1zaWduYXR1cmU",
	}))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestNodeRejectedWithMissingEnrollment(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(h.gatewayURL(), "http://", "ws://", 1), http.Header{
		"Authorization": []string{"Bearer node-missing"},
	})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          "node",
		"last_seen_seq": 0,
		"node_id":       "node-missing",
		"node_name":     "Missing Node",
		"node_platform": string(core.NodePlatformLinux),
	}))

	var connected map[string]any
	require.NoError(t, conn.ReadJSON(&connected))
	require.Equal(t, "connected", connected["type"])

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestReplayDeliversMissedEvents(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	initialSeqs := h.appendEvents(t,
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"one"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"two"}`),
		},
	)

	newSeqs := h.appendEvents(t,
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"three"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"four"}`),
		},
	)

	reconnect := nexustest.NewTestGatewayClient()
	require.NoError(t, reconnect.ConnectWithLastSeen(h.gatewayURL(), "agent-a", "agent", initialSeqs[len(initialSeqs)-1]))
	replayed := h.collectEvents(t, reconnect, 150*time.Millisecond)
	require.NoError(t, reconnect.Close())

	replayedSeqs := map[uint64]struct{}{}
	for _, ev := range replayed {
		if ev.Type == core.FrameworkEventMessageInbound {
			replayedSeqs[ev.Seq] = struct{}{}
		}
	}
	for _, seq := range newSeqs {
		_, ok := replayedSeqs[seq]
		require.True(t, ok, "expected replayed seq %d", seq)
	}
}

func TestReplayExcludesForeignTenantEvents(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	seqs := h.appendEvents(t,
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"baseline"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"tenant-a"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-b"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"text":"tenant-b"}`),
		},
	)

	client := nexustest.NewTestGatewayClient()
	require.NoError(t, client.ConnectWithLastSeen(h.gatewayURL(), "agent-a", "agent", seqs[0]))
	replayed := h.collectEvents(t, client, 150*time.Millisecond)
	require.NoError(t, client.Close())

	delivered := map[uint64]struct{}{}
	for _, ev := range replayed {
		if ev.Type == core.FrameworkEventMessageInbound {
			delivered[ev.Seq] = struct{}{}
			require.Equal(t, "tenant-a", ev.Actor.TenantID)
		}
	}
	_, ok := delivered[seqs[1]]
	require.True(t, ok, "expected tenant-a replay seq %d", seqs[1])
	_, ok = delivered[seqs[2]]
	require.False(t, ok, "did not expect tenant-b replay seq %d", seqs[2])
}

func TestConnectionProducesSessionCreatedEvent(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	_ = h.newClient(t, "agent-a", "agent")
	ev := h.waitForLoggedEvent(t, core.FrameworkEventSessionCreated, func(ev core.FrameworkEvent) bool {
		return ev.Actor.ID == "svc-a"
	})
	require.Equal(t, "agent", ev.Actor.Kind)
	require.Equal(t, "tenant-a", ev.Actor.TenantID)
}

func TestAgentSendsOutboundToOwnedSession(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())
	client := h.newClient(t, "agent-a", "agent")

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:  "sess-owned",
		TenantID:   "tenant-a",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-1",
		Owner:      core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	})

	before := h.countEvents(t, core.FrameworkEventMessageOutbound)
	require.NoError(t, client.SendOutbound("sess-owned", "hello from owner"))
	msg := h.waitForOutbound(t, 2*time.Second)
	require.Equal(t, "webchat", msg.Channel)
	require.Equal(t, "conv-1", msg.ConversationID)
	require.Equal(t, "hello from owner", msg.Content.Text)
	require.Eventually(t, func() bool {
		return h.countEvents(t, core.FrameworkEventMessageOutbound) == before+1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestAgentCannotSendToUnownedSession(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())
	client := h.newClient(t, "agent-b", "agent")

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:  "sess-foreign",
		TenantID:   "tenant-a",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-2",
		Owner:      core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	})

	before := h.countEvents(t, core.FrameworkEventMessageOutbound)
	require.NoError(t, client.SendOutbound("sess-foreign", "should fail"))
	select {
	case msg := <-h.adapter.Outbound():
		t.Fatalf("unexpected outbound message: %+v", msg)
	case <-time.After(250 * time.Millisecond):
	}
	require.Equal(t, before, h.countEvents(t, core.FrameworkEventMessageOutbound))
}

func TestAgentCannotSendToNonexistentSession(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())
	client := h.newClient(t, "agent-a", "agent")

	before := h.countEvents(t, core.FrameworkEventMessageOutbound)
	require.NoError(t, client.SendOutbound("sess-missing", "missing"))
	select {
	case msg := <-h.adapter.Outbound():
		t.Fatalf("unexpected outbound message: %+v", msg)
	case <-time.After(250 * time.Millisecond):
	}
	require.Equal(t, before, h.countEvents(t, core.FrameworkEventMessageOutbound))
}
