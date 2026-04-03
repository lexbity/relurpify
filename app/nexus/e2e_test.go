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
	nexusadmin "github.com/lexcodex/relurpify/app/nexus/admin"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusdb "github.com/lexcodex/relurpify/app/nexus/db"
	nexusserver "github.com/lexcodex/relurpify/app/nexus/server"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/memory"
	memdb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	mcpprotocol "github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	mcpserver "github.com/lexcodex/relurpify/framework/middleware/mcp/server"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	rexcontrolplane "github.com/lexcodex/relurpify/named/rex/controlplane"
	rexnexus "github.com/lexcodex/relurpify/named/rex/nexus"
	rexreconcile "github.com/lexcodex/relurpify/named/rex/reconcile"
	"github.com/lexcodex/relurpify/testsuite/nexustest"
	"github.com/stretchr/testify/require"
)

type nexusHarness struct {
	t              *testing.T
	ctx            context.Context
	cancel         context.CancelFunc
	server         *httptest.Server
	workspace      string
	eventLog       *nexusdb.SQLiteEventLog
	sessionStore   *nexusdb.SQLiteSessionStore
	identityStore  *nexusdb.SQLiteIdentityStore
	nodeStore      *nexusdb.SQLiteNodeStore
	tokenStore     *nexusdb.SQLiteAdminTokenStore
	policyStore    *memdb.FilePolicyRuleStore
	policyFile     string
	adapter        *channel.TestChannelAdapter
	app            *nexusserver.NexusApp
	rexRuntime     *nexusserver.RexRuntimeProvider
	rexEventBridge *nexusserver.RexEventBridge
	fmpService     *fwfmp.Service
	fmpExportStore nexusadmin.TenantFMPExportStore
}

type nexusHarnessOptions struct {
	enableRex       bool
	rexRuntime      *nexusserver.RexRuntimeProvider
	rexEventBridge  *nexusserver.RexEventBridge
	fmpService      *fwfmp.Service
	fmpExportStore  nexusadmin.TenantFMPExportStore
	nodeManager     *fwnode.Manager
	channelManager  *channel.Manager
	channelAdapters []channel.Adapter
	startedAt       time.Time
}

func newNexusHarness(t *testing.T, cfg nexuscfg.Config) *nexusHarness {
	return newNexusHarnessWithOptions(t, cfg, nexusHarnessOptions{})
}

func newNexusHarnessWithOptions(t *testing.T, cfg nexuscfg.Config, opts nexusHarnessOptions) *nexusHarness {
	t.Helper()

	dir := t.TempDir()
	workspace := ""
	if opts.enableRex {
		workspace = filepath.Join(dir, "workspace")
	}
	eventLog, err := nexusdb.NewSQLiteEventLog(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	sessionStore, err := nexusdb.NewSQLiteSessionStore(filepath.Join(dir, "sessions.db"))
	require.NoError(t, err)
	identityStore, err := nexusdb.NewSQLiteIdentityStore(filepath.Join(dir, "identities.db"))
	require.NoError(t, err)
	nodeStore, err := nexusdb.NewSQLiteNodeStore(filepath.Join(dir, "nodes.db"))
	require.NoError(t, err)
	tokenStore, err := nexusdb.NewSQLiteAdminTokenStore(filepath.Join(dir, "admin_tokens.db"))
	require.NoError(t, err)
	policyFile := filepath.Join(dir, "policy_rules.yaml")
	policyStore, err := memdb.NewFilePolicyRuleStore(policyFile)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	adapter := channel.NewTestChannelAdapter("webchat")
	channelAdapters := opts.channelAdapters
	if len(channelAdapters) == 0 {
		channelAdapters = []channel.Adapter{adapter}
	}
	rexRuntime := opts.rexRuntime
	ownsRexRuntime := false
	if rexRuntime == nil && opts.enableRex {
		rexRuntime, err = nexusserver.NewRexRuntimeProvider(ctx, workspace)
		require.NoError(t, err)
		ownsRexRuntime = true
	}
	app := &nexusserver.NexusApp{
		EventLog:          eventLog,
		SessionStore:      sessionStore,
		IdentityStore:     identityStore,
		NodeStore:         nodeStore,
		TokenStore:        tokenStore,
		PolicyStore:       policyStore,
		Config:            cfg,
		Partition:         nexusEventPartition,
		Workspace:         workspace,
		ChannelAdapters:   channelAdapters,
		RexRuntime:        rexRuntime,
		RexEventBridge:    opts.rexEventBridge,
		NodeManager:       opts.nodeManager,
		ChannelManager:    opts.channelManager,
		FMPService:        opts.fmpService,
		FMPExportStore:    opts.fmpExportStore,
		StartedAt:         opts.startedAt,
		PrincipalResolver: gatewayPrincipalResolver(cfg.Gateway.Auth, tokenStore, identityStore),
		VerifyNodeConnection: func(ctx context.Context, store identity.Store, principal fwgateway.ConnectionPrincipal, info fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
			return verifyGatewayNodeChallenge(ctx, store, principal, info, conn)
		},
	}
	handler, err := app.Handler(ctx)
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	h := &nexusHarness{
		t:              t,
		ctx:            ctx,
		cancel:         cancel,
		server:         server,
		workspace:      workspace,
		eventLog:       eventLog,
		sessionStore:   sessionStore,
		identityStore:  identityStore,
		nodeStore:      nodeStore,
		tokenStore:     tokenStore,
		policyStore:    policyStore,
		policyFile:     policyFile,
		adapter:        adapter,
		app:            app,
		rexRuntime:     rexRuntime,
		rexEventBridge: app.RexEventBridge,
		fmpService:     app.FMPService,
		fmpExportStore: app.FMPExportStore,
	}
	t.Cleanup(func() {
		server.Close()
		cancel()
		if ownsRexRuntime && rexRuntime != nil {
			rexRuntime.Close()
		}
		require.NoError(t, nodeStore.Close())
		require.NoError(t, tokenStore.Close())
		require.NoError(t, identityStore.Close())
		require.NoError(t, sessionStore.Close())
		require.NoError(t, eventLog.Close())
	})
	return h
}

type lineageLister interface {
	ListLineages(context.Context) ([]core.LineageRecord, error)
}

type trustBundleLister interface {
	ListTrustBundles(context.Context) ([]core.TrustBundle, error)
}

type activeAttemptLister interface {
	ListActiveAttemptsByLineage(context.Context, string) ([]core.AttemptRecord, error)
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

func (h *nexusHarness) waitForRexWorkflow(t *testing.T, workflowID string) (*memory.WorkflowRecord, *memory.WorkflowRunRecord) {
	t.Helper()
	require.NotNil(t, h.rexRuntime)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		workflow, workflowOK, err := h.rexRuntime.WorkflowStore.GetWorkflow(context.Background(), workflowID)
		require.NoError(t, err)
		run, runOK, err := h.rexRuntime.WorkflowStore.GetRun(context.Background(), workflowID+":run")
		require.NoError(t, err)
		if workflowOK && runOK {
			return workflow, run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for rex workflow %s", workflowID)
	return nil, nil
}

func (h *nexusHarness) waitForRexArtifact(t *testing.T, workflowID, runID, kind string) memory.WorkflowArtifactRecord {
	t.Helper()
	require.NotNil(t, h.rexRuntime)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		artifacts, err := h.rexRuntime.WorkflowStore.ListWorkflowArtifacts(context.Background(), workflowID, runID)
		require.NoError(t, err)
		for _, artifact := range artifacts {
			if artifact.Kind == kind {
				return artifact
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for rex artifact %s for workflow %s", kind, workflowID)
	return memory.WorkflowArtifactRecord{}
}

func (h *nexusHarness) listRexWorkflows(t *testing.T, limit int) []memory.WorkflowRecord {
	t.Helper()
	require.NotNil(t, h.rexRuntime)
	workflows, err := h.rexRuntime.WorkflowStore.ListWorkflows(context.Background(), limit)
	require.NoError(t, err)
	return workflows
}

func (h *nexusHarness) listRexEvents(t *testing.T, workflowID string, limit int) []memory.WorkflowEventRecord {
	t.Helper()
	require.NotNil(t, h.rexRuntime)
	events, err := h.rexRuntime.WorkflowStore.ListEvents(context.Background(), workflowID, limit)
	require.NoError(t, err)
	return events
}

func (h *nexusHarness) listRexArtifacts(t *testing.T, workflowID, runID string) []memory.WorkflowArtifactRecord {
	t.Helper()
	require.NotNil(t, h.rexRuntime)
	artifacts, err := h.rexRuntime.WorkflowStore.ListWorkflowArtifacts(context.Background(), workflowID, runID)
	require.NoError(t, err)
	return artifacts
}

func (h *nexusHarness) findLoggedEvents(t *testing.T, eventType string, predicate func(core.FrameworkEvent) bool) []core.FrameworkEvent {
	t.Helper()
	events, err := h.eventLog.Read(context.Background(), nexusEventPartition, 0, 256, false)
	require.NoError(t, err)
	var matched []core.FrameworkEvent
	for _, ev := range events {
		if ev.Type != eventType {
			continue
		}
		if predicate == nil || predicate(ev) {
			matched = append(matched, ev)
		}
	}
	return matched
}

func (h *nexusHarness) listFMPLineages(t *testing.T) []core.LineageRecord {
	t.Helper()
	require.NotNil(t, h.fmpService)
	lister, ok := h.fmpService.Ownership.(lineageLister)
	require.True(t, ok, "fmp ownership store does not support ListLineages")
	lineages, err := lister.ListLineages(context.Background())
	require.NoError(t, err)
	return lineages
}

func (h *nexusHarness) getFMPAttempt(t *testing.T, attemptID string) *core.AttemptRecord {
	t.Helper()
	require.NotNil(t, h.fmpService)
	attempt, ok, err := h.fmpService.Ownership.GetAttempt(context.Background(), attemptID)
	require.NoError(t, err)
	require.True(t, ok, "attempt %s not found", attemptID)
	return attempt
}

func (h *nexusHarness) getFMPLineage(t *testing.T, lineageID string) *core.LineageRecord {
	t.Helper()
	require.NotNil(t, h.fmpService)
	lineage, ok, err := h.fmpService.Ownership.GetLineage(context.Background(), lineageID)
	require.NoError(t, err)
	require.True(t, ok, "lineage %s not found", lineageID)
	return lineage
}

func (h *nexusHarness) listFMPActiveAttempts(t *testing.T, lineageID string) []core.AttemptRecord {
	t.Helper()
	require.NotNil(t, h.fmpService)
	lister, ok := h.fmpService.Ownership.(activeAttemptLister)
	require.True(t, ok, "fmp ownership store does not support ListActiveAttemptsByLineage")
	attempts, err := lister.ListActiveAttemptsByLineage(context.Background(), lineageID)
	require.NoError(t, err)
	return attempts
}

func (h *nexusHarness) listFMPTrustBundles(t *testing.T) []core.TrustBundle {
	t.Helper()
	require.NotNil(t, h.fmpService)
	lister, ok := h.fmpService.Trust.(trustBundleLister)
	require.True(t, ok, "fmp trust store does not support ListTrustBundles")
	bundles, err := lister.ListTrustBundles(context.Background())
	require.NoError(t, err)
	return bundles
}

func (h *nexusHarness) listTenantFMPExports(t *testing.T, tenantID string) []nexusadmin.TenantFMPExportInfo {
	t.Helper()
	require.NotNil(t, h.fmpExportStore)
	exports, err := h.fmpExportStore.ListTenantExports(context.Background(), tenantID)
	require.NoError(t, err)
	return exports
}

func (h *nexusHarness) admissionAuditRecords() []rexcontrolplane.AuditRecord {
	if h == nil || h.rexRuntime == nil || h.rexRuntime.AdmissionAudit == nil {
		return nil
	}
	return h.rexRuntime.AdmissionAudit.Records()
}

func (h *nexusHarness) waitForAdmissionAudit(t *testing.T, predicate func(rexcontrolplane.AuditRecord) bool) rexcontrolplane.AuditRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, record := range h.admissionAuditRecords() {
			if predicate == nil || predicate(record) {
				return record
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for admission audit record")
	return rexcontrolplane.AuditRecord{}
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

func requireOneWorkflowOnly(t *testing.T, workflows []memory.WorkflowRecord, workflowID string) {
	t.Helper()
	require.Len(t, workflows, 1)
	require.Equal(t, workflowID, workflows[0].WorkflowID)
}

func requireCursorAdvanced(t *testing.T, before, after uint64) {
	t.Helper()
	require.Greater(t, after, before, "expected cursor to advance")
}

func requireNoDuplicateAttempt(t *testing.T, attempts []core.AttemptRecord, attemptID string) {
	t.Helper()
	count := 0
	for _, attempt := range attempts {
		if attempt.AttemptID == attemptID {
			count++
		}
	}
	require.Equal(t, 1, count, "expected exactly one attempt %s", attemptID)
}

func requireSameTenantSessionPropagated(t *testing.T, tenantID, sessionID string, values map[string]any) {
	t.Helper()
	candidates := []string{
		valueAtPath(values, "tenant_id"),
		valueAtPath(values, "gateway.tenant_id"),
		valueAtPath(values, "gateway.session_tenant_id"),
	}
	require.Contains(t, candidates, tenantID, "tenant id not propagated in %+v", values)
	sessionCandidates := []string{
		valueAtPath(values, "session_id"),
		valueAtPath(values, "gateway.session_id"),
		valueAtPath(values, "session_key"),
	}
	require.Contains(t, sessionCandidates, sessionID, "session id not propagated in %+v", values)
}

func valueAtPath(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if raw, ok := values[key]; ok {
		if text, ok := raw.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	if !strings.Contains(key, ".") {
		return ""
	}
	parts := strings.Split(key, ".")
	current := values
	for i, part := range parts {
		raw, ok := current[part]
		if !ok {
			return ""
		}
		if i == len(parts)-1 {
			text, _ := raw.(string)
			return strings.TrimSpace(text)
		}
		next, ok := raw.(map[string]any)
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func (h *nexusHarness) seedBoundary(t *testing.T, boundary *core.SessionBoundary) {
	t.Helper()
	key := core.SessionBoundaryKey(boundary.Scope, boundary.Partition, boundary.ChannelID, boundary.PeerID, "")
	require.NoError(t, h.sessionStore.UpsertBoundary(context.Background(), key, boundary))
}

func (h *nexusHarness) enrollNode(t *testing.T, tenantID string, cred core.NodeCredential) {
	h.enrollNodeWithApprovedCapabilities(t, tenantID, cred, nil)
}

func (h *nexusHarness) enrollNodeWithApprovedCapabilities(t *testing.T, tenantID string, cred core.NodeCredential, caps []core.CapabilityDescriptor) {
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
	require.NoError(t, h.nodeStore.UpsertNode(context.Background(), core.NodeDescriptor{
		ID:                   cred.DeviceID,
		TenantID:             tenantID,
		Name:                 cred.DeviceID,
		Platform:             core.NodePlatformHeadless,
		TrustClass:           core.TrustClassRemoteApproved,
		PairedAt:             cred.IssuedAt,
		Owner:                core.SubjectRef{TenantID: tenantID, Kind: core.SubjectKindNode, ID: cred.DeviceID},
		ApprovedCapabilities: append([]core.CapabilityDescriptor(nil), caps...),
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

func TestBroadcastRestrictsRuntimeFeedToAuthorizedSessions(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	tenantA := h.newClient(t, "agent-a", "agent")
	tenantB := h.newClient(t, "agent-b", "agent")
	admin := h.newClient(t, "admin-a", "admin")

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

	_, err := h.eventLog.Append(context.Background(), nexusEventPartition, []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventSessionMessage,
		Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
		Partition: nexusEventPartition,
		Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"tenant-a"}`),
	}, {
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
		Partition: nexusEventPartition,
		Payload:   json.RawMessage(`{"text":"tenant-a"}`),
	}})
	require.NoError(t, err)

	ev := h.waitForEvent(t, tenantA, core.FrameworkEventSessionMessage, 2*time.Second)
	require.Equal(t, "tenant-a", ev.Actor.TenantID)
	h.assertNoEventOfType(t, tenantA, core.FrameworkEventMessageInbound, 250*time.Millisecond)
	h.assertNoEventOfType(t, tenantB, core.FrameworkEventSessionMessage, 250*time.Millisecond)
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

func TestAdminMCPReadResourceDeniesCrossTenantQuery(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:      "session-tenant-b",
		RoutingKey:     core.SessionBoundaryKey(core.SessionScopePerChannelPeer, nexusEventPartition, "webchat", "peer-b", ""),
		TenantID:       "tenant-b",
		Scope:          core.SessionScopePerChannelPeer,
		Partition:      nexusEventPartition,
		ChannelID:      "webchat",
		PeerID:         "peer-b",
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
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

	resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "cross-tenant-read",
		"method":  "resources/read",
		"params":  mcpprotocol.ReadResourceParams{URI: "nexus://sessions/active?tenant=tenant-b"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	errObj := payload["error"].(map[string]any)
	require.NotNil(t, errObj)
	message, _ := errObj["message"].(string)
	require.Contains(t, strings.ToLower(message), "cross-tenant")
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
		"id":      "subject-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.identity.create_subject",
			Arguments: map[string]any{
				"subject_tenant_id": "tenant-a",
				"subject_kind":      string(core.SubjectKindServiceAccount),
				"subject_id":        "subject-a",
				"display_name":      "Subject A",
				"roles":             []string{"operator"},
				"api_version":       "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

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

func TestAdminMCPBindExternalIdentityEnablesResolvedRouting(t *testing.T) {
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
		"id":      "subject-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.identity.create_subject",
			Arguments: map[string]any{
				"subject_tenant_id": "tenant-a",
				"subject_kind":      string(core.SubjectKindUser),
				"subject_id":        "user-a",
				"display_name":      "User A",
				"api_version":       "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, payload["result"])

	resp, payload = h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "bind-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.identity.bind_external",
			Arguments: map[string]any{
				"subject_tenant_id": "tenant-a",
				"provider":          string(core.ExternalProviderWebchat),
				"account_id":        "workspace-a",
				"external_id":       "peer-bound",
				"subject_kind":      string(core.SubjectKindUser),
				"subject_id":        "user-a",
				"display_name":      "Peer Bound",
				"api_version":       "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, payload["result"])

	require.NoError(t, h.adapter.SendInbound(context.Background(), channel.InboundMessage{
		Channel: "webchat",
		Account: "workspace-a",
		Sender: channel.Identity{
			ChannelID: "peer-bound",
		},
		Conversation: channel.Conversation{
			Kind: "dm",
			ID:   "conv-bound",
		},
		Content: channel.MessageContent{Text: "hello from bound peer"},
	}))

	boundary := h.waitForBoundary(t, func(boundary core.SessionBoundary) bool {
		return boundary.ChannelID == "webchat" && boundary.PeerID == "conv-bound"
	})
	require.Equal(t, "tenant-a", boundary.TenantID)
	require.Equal(t, "user-a", boundary.Owner.ID)
	require.Equal(t, core.TrustClassRemoteApproved, boundary.TrustClass)
	require.NotNil(t, boundary.Binding)
	require.Equal(t, "peer-bound", boundary.Binding.ExternalUserID)
}

func TestDelegatedServiceAccountCanSendToOwnedSession(t *testing.T) {
	h := newNexusHarness(t, testGatewayConfig())

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:      "sess-owned",
		RoutingKey:     core.SessionBoundaryKey(core.SessionScopePerChannelPeer, nexusEventPartition, "webchat", "peer-1", ""),
		TenantID:       "tenant-a",
		Scope:          core.SessionScopePerChannelPeer,
		Partition:      nexusEventPartition,
		ChannelID:      "webchat",
		PeerID:         "peer-1",
		Owner:          core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
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

	resp, _ := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "subject-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.identity.create_subject",
			Arguments: map[string]any{
				"subject_tenant_id": "tenant-a",
				"subject_kind":      string(core.SubjectKindServiceAccount),
				"subject_id":        "delegate-a",
				"display_name":      "Delegate A",
				"api_version":       "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, payload := h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "token-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.tokens.issue",
			Arguments: map[string]any{
				"subject_tenant_id": "tenant-a",
				"subject_kind":      string(core.SubjectKindServiceAccount),
				"subject_id":        "delegate-a",
				"scopes":            []string{"nexus:operator"},
				"api_version":       "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	issued := payload["result"].(map[string]any)["structuredContent"].(map[string]any)
	delegateToken := issued["token"].(string)
	require.NotEmpty(t, delegateToken)

	resp, _ = h.adminMCPCall(t, "admin-a", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      "delegate-1",
		"method":  "tools/call",
		"params": mcpprotocol.CallToolParams{
			Name: "nexus.sessions.grant_delegation",
			Arguments: map[string]any{
				"session_id":   "sess-owned",
				"subject_kind": string(core.SubjectKindServiceAccount),
				"subject_id":   "delegate-a",
				"operations":   []string{string(core.SessionOperationSend)},
				"api_version":  "v1alpha1",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	client := h.newClient(t, delegateToken, "operator")
	require.NoError(t, client.SendOutbound("sess-owned", "delegated hello"))

	outbound := h.waitForOutbound(t, 2*time.Second)
	require.Equal(t, "webchat", outbound.Channel)
	require.Equal(t, "peer-1", outbound.ConversationID)
	require.Equal(t, "delegated hello", outbound.Content.Text)
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

func TestInboundMessageStartsRexWorkflow(t *testing.T) {
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{enableRex: true})

	require.NoError(t, h.identityStore.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "local",
		Provider:   core.ExternalProviderWebchat,
		AccountID:  "workspace-a",
		ExternalID: "peer-rex-1",
		Subject: core.SubjectRef{
			TenantID: "local",
			Kind:     core.SubjectKindUser,
			ID:       "user-rex-1",
		},
	}))

	require.NoError(t, h.adapter.SendInbound(context.Background(), channel.InboundMessage{
		Channel: "webchat",
		Account: "workspace-a",
		Sender: channel.Identity{
			ChannelID: "peer-rex-1",
		},
		Conversation: channel.Conversation{
			Kind: "dm",
			ID:   "conv-rex-1",
		},
		Content: channel.MessageContent{Text: "draft a deployment plan"},
	}))

	boundary := h.waitForBoundary(t, func(boundary core.SessionBoundary) bool {
		return boundary.ChannelID == "webchat" && boundary.PeerID == "conv-rex-1"
	})
	workflowID := "rex-session:" + boundary.SessionID
	workflow, run := h.waitForRexWorkflow(t, workflowID)

	require.Equal(t, workflowID, workflow.WorkflowID)
	require.True(t, strings.HasPrefix(workflow.TaskID, "session:"+boundary.SessionID+":"), "task id = %q", workflow.TaskID)
	require.Equal(t, core.TaskTypeAnalysis, workflow.TaskType)
	require.Equal(t, "draft a deployment plan", workflow.Instruction)
	require.Equal(t, workflowID+":run", run.RunID)
	require.Equal(t, workflowID, run.WorkflowID)

	require.Eventually(t, func() bool {
		events, err := h.rexRuntime.WorkflowStore.ListEvents(context.Background(), workflowID, 10)
		require.NoError(t, err)
		return len(events) >= 2
	}, 3*time.Second, 20*time.Millisecond)
}

func TestInboundMessagesReuseExistingRexWorkflow(t *testing.T) {
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{enableRex: true})

	require.NoError(t, h.identityStore.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "local",
		Provider:   core.ExternalProviderWebchat,
		AccountID:  "workspace-a",
		ExternalID: "peer-rex-2",
		Subject: core.SubjectRef{
			TenantID: "local",
			Kind:     core.SubjectKindUser,
			ID:       "user-rex-2",
		},
	}))

	sendInbound := func(text string) {
		t.Helper()
		require.NoError(t, h.adapter.SendInbound(context.Background(), channel.InboundMessage{
			Channel: "webchat",
			Account: "workspace-a",
			Sender: channel.Identity{
				ChannelID: "peer-rex-2",
			},
			Conversation: channel.Conversation{
				Kind: "dm",
				ID:   "conv-rex-2",
			},
			Content: channel.MessageContent{Text: text},
		}))
	}

	sendInbound("first rex message")
	firstBoundary := h.waitForBoundary(t, func(boundary core.SessionBoundary) bool {
		return boundary.ChannelID == "webchat" && boundary.PeerID == "conv-rex-2"
	})
	workflowID := "rex-session:" + firstBoundary.SessionID
	_, _ = h.waitForRexWorkflow(t, workflowID)

	sendInbound("second rex message")
	secondSessionMessage := h.waitForLoggedEvent(t, core.FrameworkEventSessionMessage, func(ev core.FrameworkEvent) bool {
		var payload struct {
			SessionKey string `json:"session_key"`
			Content    struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		return json.Unmarshal(ev.Payload, &payload) == nil &&
			payload.SessionKey == firstBoundary.SessionID &&
			payload.Content.Text == "second rex message"
	})
	require.Equal(t, firstBoundary.SessionID, secondSessionMessage.Actor.ID)

	require.Eventually(t, func() bool {
		workflows, err := h.rexRuntime.WorkflowStore.ListWorkflows(context.Background(), 10)
		require.NoError(t, err)
		if len(workflows) != 1 {
			return false
		}
		if workflows[0].WorkflowID != workflowID {
			return false
		}
		events, err := h.rexRuntime.WorkflowStore.ListEvents(context.Background(), workflowID, 10)
		require.NoError(t, err)
		return len(events) >= 4
	}, 3*time.Second, 20*time.Millisecond)
}

func TestConcurrentTenantInboundMessagesStayIsolatedAcrossRexWorkflows(t *testing.T) {
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{enableRex: true})

	boundaryA := core.SessionBoundary{
		SessionID:  "sess-tenant-a",
		TenantID:   "tenant-a",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-shared",
		Owner:      core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	}
	boundaryB := core.SessionBoundary{
		SessionID:  "sess-tenant-b",
		TenantID:   "tenant-b",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-shared",
		Owner:      core.SubjectRef{TenantID: "tenant-b", Kind: core.SubjectKindServiceAccount, ID: "svc-b"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	}
	h.seedBoundary(t, &boundaryA)
	h.seedBoundary(t, &boundaryB)

	appendErrs := make(chan error, 2)
	go func() {
		_, err := h.eventLog.Append(context.Background(), nexusEventPartition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-tenant-a", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-tenant-a","channel":"webchat","conversation_id":"conv-shared","sender_id":"user-a","content":{"text":"same prompt"}}`),
		}})
		appendErrs <- err
	}()
	go func() {
		_, err := h.eventLog.Append(context.Background(), nexusEventPartition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-tenant-b", TenantID: "tenant-b"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-tenant-b","channel":"webchat","conversation_id":"conv-shared","sender_id":"user-b","content":{"text":"same prompt"}}`),
		}})
		appendErrs <- err
	}()
	require.NoError(t, <-appendErrs)
	require.NoError(t, <-appendErrs)

	require.Equal(t, "svc-a", boundaryA.Owner.ID)
	require.Equal(t, "svc-b", boundaryB.Owner.ID)

	workflowIDA := "rex-session:" + boundaryA.SessionID
	workflowIDB := "rex-session:" + boundaryB.SessionID
	_, runA := h.waitForRexWorkflow(t, workflowIDA)
	_, runB := h.waitForRexWorkflow(t, workflowIDB)
	require.NotEqual(t, runA.RunID, runB.RunID)

	require.Eventually(t, func() bool {
		workflows := h.listRexWorkflows(t, 10)
		if len(workflows) != 2 {
			return false
		}
		seen := map[string]bool{}
		for _, workflow := range workflows {
			seen[workflow.WorkflowID] = true
		}
		return seen[workflowIDA] && seen[workflowIDB]
	}, 3*time.Second, 20*time.Millisecond)

	eventsA := h.listRexEvents(t, workflowIDA, 10)
	eventsB := h.listRexEvents(t, workflowIDB, 10)
	require.NotEmpty(t, eventsA)
	require.NotEmpty(t, eventsB)
	for _, event := range eventsA {
		require.Equal(t, workflowIDA, event.WorkflowID)
	}
	for _, event := range eventsB {
		require.Equal(t, workflowIDB, event.WorkflowID)
	}
	require.Equal(t, workflowIDA, runA.WorkflowID)
	require.Equal(t, workflowIDB, runB.WorkflowID)
}

func TestStaleWorkflowSignalAfterCompletionIsRejectedWithoutMutation(t *testing.T) {
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{enableRex: true})

	now := time.Now().UTC().Truncate(time.Second)
	workflowID := "wf-stale-signal"
	runID := "run-stale-signal"

	require.NoError(t, h.rexRuntime.WorkflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-stale-signal",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "completed workflow",
		Status:      memory.WorkflowRunStatusCompleted,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, h.rexRuntime.WorkflowStore.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:          runID,
		WorkflowID:     workflowID,
		Status:         memory.WorkflowRunStatusCompleted,
		RuntimeVersion: "rex-test",
		StartedAt:      now.Add(-time.Minute),
		FinishedAt:     &now,
	}))
	require.NoError(t, h.rexRuntime.WorkflowStore.AppendEvent(context.Background(), memory.WorkflowEventRecord{
		EventID:    "evt-stale-signal-finished",
		WorkflowID: workflowID,
		RunID:      runID,
		EventType:  "rex.run.finished",
		Message:    "completed",
		CreatedAt:  now,
	}))

	beforeEvents := h.listRexEvents(t, workflowID, 10)

	h.appendEvents(t, core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      "rex.workflow.signal.v1",
		Actor:     core.EventActor{Kind: "agent", ID: "svc-a", TenantID: "tenant-a"},
		Partition: nexusEventPartition,
		Payload: json.RawMessage(`{
			"workflow_id":"wf-stale-signal",
			"run_id":"run-stale-signal",
			"expected_signal":"resume",
			"signal":"resume"
		}`),
	})

	workflow, ok, err := h.rexRuntime.WorkflowStore.GetWorkflow(context.Background(), workflowID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, memory.WorkflowRunStatusCompleted, workflow.Status)

	run, ok, err := h.rexRuntime.WorkflowStore.GetRun(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, memory.WorkflowRunStatusCompleted, run.Status)

	afterEvents := h.listRexEvents(t, workflowID, 10)
	require.Len(t, afterEvents, len(beforeEvents))
}

func TestFMPResumeIntoRexUsesFullAppHarness(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ownership := &fwfmp.InMemoryOwnershipStore{}
	signer := fwfmp.NewEd25519SignerFromSeed([]byte("e2e-full-app-fmp-rex"))
	exportStore, err := nexusdb.NewSQLiteFMPExportStore(filepath.Join(t.TempDir(), "tenant_exports.db"))
	require.NoError(t, err)
	defer exportStore.Close()
	require.NoError(t, exportStore.SetTenantExportEnabled(context.Background(), "tenant-1", "exp.run", true))

	mesh := &fwfmp.Service{
		Ownership: ownership,
		Signer:    signer,
		Now:       func() time.Time { return now },
	}
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{
		enableRex:      true,
		fmpService:     mesh,
		fmpExportStore: exportStore,
	})

	require.NoError(t, h.identityStore.UpsertTenant(context.Background(), core.TenantRecord{ID: "tenant-1", CreatedAt: now}))
	require.NoError(t, h.identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1", CreatedAt: now}))
	require.NoError(t, h.identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1", CreatedAt: now}))

	liveNow := now
	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:      "sess-import-1",
		RoutingKey:     "tenant-1:webchat:conv-import-1",
		TenantID:       "tenant-1",
		Partition:      nexusEventPartition,
		Scope:          core.SessionScopePerChannelPeer,
		ChannelID:      "webchat",
		PeerID:         "conv-import-1",
		Owner:          core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      liveNow,
		LastActivityAt: liveNow,
	})
	require.NoError(t, h.sessionStore.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		SessionID:  "sess-import-1",
		TenantID:   "tenant-1",
		Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
		Operations: []core.SessionOperation{core.SessionOperationResume},
		CreatedAt:  liveNow,
	}))

	require.Eventually(t, func() bool {
		exports := h.listTenantFMPExports(t, "tenant-1")
		return len(exports) == 1 && exports[0].ExportName == "exp.run" && exports[0].Enabled
	}, 2*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool {
		bundles := h.listFMPTrustBundles(t)
		for _, bundle := range bundles {
			if bundle.TrustDomain == "local" {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond)

	sourceSvc := &fwfmp.Service{
		Ownership: ownership,
		Packager: fwfmp.JSONPackager{
			RuntimeStore: importedWorkflowRuntimeStore{},
			KeyResolver: &fwfmp.TrustBundleRecipientKeyResolver{
				Trust: h.fmpService.Trust,
			},
			DefaultRecipients: []string{"runtime://local/rex"},
			LocalRecipient:    "runtime://mesh-a/source/rt-a",
		},
		Signer: signer,
		Now:    func() time.Time { return now },
	}

	lineage := core.LineageRecord{
		LineageID:    "lineage-e2e",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		SessionID:    "sess-import-1",
		TrustClass:   core.TrustClassRemoteApproved,
		Delegations: []core.SessionDelegationRecord{{
			SessionID:  "sess-import-1",
			TenantID:   "tenant-1",
			Grantee:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "delegate-1"},
			Operations: []core.SessionOperation{core.SessionOperationResume},
			CreatedAt:  liveNow,
		}},
	}
	require.NoError(t, sourceSvc.CreateLineage(context.Background(), lineage))
	require.NoError(t, ownership.UpsertAttempt(context.Background(), core.AttemptRecord{
		AttemptID: "attempt-source-e2e",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-source",
		State:     core.AttemptStateRunning,
		StartTime: now,
	}))

	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, "attempt-source-e2e", "exp.run", "issuer", fwfmp.RuntimeQuery{WorkflowID: "wf-import", RunID: "run-import"})
	require.NoError(t, err)

	executed, commit, authorized, refusal, err := h.fmpService.ResumeHandoffForNode(context.Background(), *offer, core.ExportDescriptor{
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
	require.Equal(t, "lineage-e2e", commit.LineageID)
	require.Equal(t, "lineage-e2e:rex:resume", commit.NewAttemptID)

	require.Eventually(t, func() bool {
		workflow, ok, err := h.rexRuntime.WorkflowStore.GetWorkflow(context.Background(), "wf-import")
		require.NoError(t, err)
		if !ok || workflow == nil {
			return false
		}
		run, ok, err := h.rexRuntime.WorkflowStore.GetRun(context.Background(), commit.NewAttemptID)
		require.NoError(t, err)
		return ok && run != nil && run.WorkflowID == workflow.WorkflowID
	}, 5*time.Second, 20*time.Millisecond)

	var artifacts []memory.WorkflowArtifactRecord
	require.Eventually(t, func() bool {
		artifacts = h.listRexArtifacts(t, "wf-import", commit.NewAttemptID)
		kinds := map[string]bool{}
		for _, artifact := range artifacts {
			kinds[artifact.Kind] = true
		}
		return kinds["rex.fmp_import"] && kinds["rex.fmp_lineage"] && kinds["rex.task_request"]
	}, 5*time.Second, 20*time.Millisecond)
	importArtifact := requireArtifactKind(t, artifacts, "rex.fmp_import")
	lineageArtifact := requireArtifactKind(t, artifacts, "rex.fmp_lineage")
	taskArtifact := requireArtifactKind(t, artifacts, "rex.task_request")

	var imported map[string]any
	require.NoError(t, json.Unmarshal([]byte(importArtifact.InlineRawText), &imported))
	require.Equal(t, commit.NewAttemptID, imported["attempt_id"])

	var binding rexnexus.LineageBinding
	require.NoError(t, json.Unmarshal([]byte(lineageArtifact.InlineRawText), &binding))
	require.Equal(t, "lineage-e2e", binding.LineageID)
	require.Equal(t, commit.NewAttemptID, binding.AttemptID)

	var taskPayload map[string]any
	require.NoError(t, json.Unmarshal([]byte(taskArtifact.InlineRawText), &taskPayload))
	stateMap, _ := taskPayload["state"].(map[string]any)
	taskMap, _ := taskPayload["task"].(map[string]any)
	taskContext, _ := taskMap["context"].(map[string]any)
	requireSameTenantSessionPropagated(t, "tenant-1", "sess-import-1", stateMap)
	requireSameTenantSessionPropagated(t, "tenant-1", "sess-import-1", taskContext)

	resumeAttempt := h.getFMPAttempt(t, commit.NewAttemptID)
	require.Contains(t, []core.AttemptState{core.AttemptStateRunning, core.AttemptStateCompleted, core.AttemptStateFailed}, resumeAttempt.State)
	sourceAttempt := h.getFMPAttempt(t, "attempt-source-e2e")
	require.Equal(t, core.AttemptStateCommittedRemote, sourceAttempt.State)
	require.True(t, sourceAttempt.Fenced)
	lineageRecord := h.getFMPLineage(t, "lineage-e2e")
	require.Equal(t, commit.NewAttemptID, lineageRecord.CurrentOwnerAttempt)

	events := h.listRexEvents(t, "wf-import", 10)
	require.NotEmpty(t, events)
}

func TestFMPControlPlaneEventsUpdateRexBindingThroughAppBridge(t *testing.T) {
	now := time.Date(2026, 4, 2, 20, 0, 0, 0, time.UTC)
	mesh := &fwfmp.Service{
		Ownership: &fwfmp.InMemoryOwnershipStore{},
		Signer:    fwfmp.NewEd25519SignerFromSeed([]byte("e2e-full-app-control-plane")),
		Now:       func() time.Time { return now },
	}
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{
		enableRex:  true,
		fmpService: mesh,
	})

	seedRexBinding(t, h, "wf-control", "attempt-control", rexnexus.LineageBinding{
		LineageID: "lineage-control",
		AttemptID: "attempt-control",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: now.Add(-1 * time.Minute),
	})
	seedRexBinding(t, h, "wf-other", "attempt-other", rexnexus.LineageBinding{
		LineageID: "lineage-other",
		AttemptID: "attempt-other",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: now.Add(-1 * time.Minute),
	})

	h.appendEvents(t,
		core.FrameworkEvent{
			Type:      core.FrameworkEventFMPHandoffAccepted,
			Partition: nexusEventPartition,
			Timestamp: now,
			Payload: mustJSONPayload(t, map[string]any{
				"lineage_id": "lineage-control",
				"attempt_id": "attempt-control",
			}),
		},
		core.FrameworkEvent{
			Type:      core.FrameworkEventFMPResumeCommitted,
			Partition: nexusEventPartition,
			Timestamp: now.Add(1 * time.Second),
			Payload: mustJSONPayload(t, map[string]any{
				"lineage_id":  "lineage-control",
				"old_attempt": "attempt-control",
				"new_attempt": "attempt-next",
			}),
		},
		core.FrameworkEvent{
			Type:      core.FrameworkEventFMPFenceIssued,
			Partition: nexusEventPartition,
			Timestamp: now.Add(2 * time.Second),
			Payload: mustJSONPayload(t, map[string]any{
				"lineage_id": "lineage-control",
				"attempt_id": "attempt-control",
			}),
		},
	)

	var controlBinding rexnexus.LineageBinding
	require.Eventually(t, func() bool {
		artifact, ok := findArtifactKind(h.listRexArtifacts(t, "wf-control", "attempt-control"), "rex.fmp_lineage")
		if !ok {
			return false
		}
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &controlBinding); err != nil {
			return false
		}
		return controlBinding.State == string(core.AttemptStateFenced)
	}, 5*time.Second, 20*time.Millisecond)

	var controlEvents []memory.WorkflowEventRecord
	require.Eventually(t, func() bool {
		controlEvents = h.listRexEvents(t, "wf-control", 10)
		return len(controlEvents) == 3
	}, 5*time.Second, 20*time.Millisecond)
	controlEventTypes := []string{controlEvents[0].EventType, controlEvents[1].EventType, controlEvents[2].EventType}
	require.ElementsMatch(t, []string{
		core.FrameworkEventFMPHandoffAccepted,
		core.FrameworkEventFMPResumeCommitted,
		core.FrameworkEventFMPFenceIssued,
	}, controlEventTypes)

	var otherBinding rexnexus.LineageBinding
	otherArtifact := h.waitForRexArtifact(t, "wf-other", "attempt-other", "rex.fmp_lineage")
	require.NoError(t, json.Unmarshal([]byte(otherArtifact.InlineRawText), &otherBinding))
	require.Equal(t, string(core.AttemptStateRunning), otherBinding.State)
	otherEvents := h.listRexEvents(t, "wf-other", 10)
	require.Empty(t, otherEvents)
}

func TestFMPFenceEventIsStateIdempotentThroughAppBridge(t *testing.T) {
	now := time.Date(2026, 4, 2, 21, 0, 0, 0, time.UTC)
	mesh := &fwfmp.Service{
		Ownership: &fwfmp.InMemoryOwnershipStore{},
		Signer:    fwfmp.NewEd25519SignerFromSeed([]byte("e2e-full-app-fence-idempotent")),
		Now:       func() time.Time { return now },
	}
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{
		enableRex:  true,
		fmpService: mesh,
	})

	seedRexBinding(t, h, "wf-fence", "attempt-fence", rexnexus.LineageBinding{
		LineageID: "lineage-fence",
		AttemptID: "attempt-fence",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: now.Add(-1 * time.Minute),
	})

	h.appendEvents(t,
		core.FrameworkEvent{
			Type:      core.FrameworkEventFMPFenceIssued,
			Partition: nexusEventPartition,
			Timestamp: now,
			Payload: mustJSONPayload(t, map[string]any{
				"lineage_id": "lineage-fence",
				"attempt_id": "attempt-fence",
			}),
		},
		core.FrameworkEvent{
			Type:      core.FrameworkEventFMPFenceIssued,
			Partition: nexusEventPartition,
			Timestamp: now.Add(1 * time.Second),
			Payload: mustJSONPayload(t, map[string]any{
				"lineage_id": "lineage-fence",
				"attempt_id": "attempt-fence",
			}),
		},
	)

	var binding rexnexus.LineageBinding
	require.Eventually(t, func() bool {
		var ok bool
		binding, ok = readRexBinding(t, h, "wf-fence", "attempt-fence")
		return ok && binding.State == string(core.AttemptStateFenced)
	}, 5*time.Second, 20*time.Millisecond)
	require.False(t, binding.UpdatedAt.Before(now))

	var events []memory.WorkflowEventRecord
	require.Eventually(t, func() bool {
		events = h.listRexEvents(t, "wf-fence", 10)
		return len(events) == 2
	}, 5*time.Second, 20*time.Millisecond)
	for _, event := range events {
		require.Equal(t, core.FrameworkEventFMPFenceIssued, event.EventType)
	}
}

func TestFMPControlPlaneEventWithMismatchedBindingDoesNotMutateWorkflow(t *testing.T) {
	now := time.Date(2026, 4, 2, 21, 30, 0, 0, time.UTC)
	mesh := &fwfmp.Service{
		Ownership: &fwfmp.InMemoryOwnershipStore{},
		Signer:    fwfmp.NewEd25519SignerFromSeed([]byte("e2e-full-app-control-mismatch")),
		Now:       func() time.Time { return now },
	}
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{
		enableRex:  true,
		fmpService: mesh,
	})

	initial := rexnexus.LineageBinding{
		LineageID: "lineage-bound",
		AttemptID: "attempt-bound",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: now.Add(-1 * time.Minute),
	}
	seedRexBinding(t, h, "wf-mismatch", "attempt-bound", initial)

	h.appendEvents(t, core.FrameworkEvent{
		Type:      core.FrameworkEventFMPResumeCommitted,
		Partition: nexusEventPartition,
		Timestamp: now,
		Payload: mustJSONPayload(t, map[string]any{
			"lineage_id":  "lineage-unrelated",
			"old_attempt": "attempt-unrelated",
			"new_attempt": "attempt-new",
		}),
	})

	require.Never(t, func() bool {
		events := h.listRexEvents(t, "wf-mismatch", 10)
		if len(events) != 0 {
			return true
		}
		binding, ok := readRexBinding(t, h, "wf-mismatch", "attempt-bound")
		if !ok {
			return true
		}
		return binding.State != initial.State || !binding.UpdatedAt.Equal(initial.UpdatedAt)
	}, 250*time.Millisecond, 20*time.Millisecond)

	binding, ok := readRexBinding(t, h, "wf-mismatch", "attempt-bound")
	require.True(t, ok)
	require.Equal(t, initial.LineageID, binding.LineageID)
	require.Equal(t, initial.AttemptID, binding.AttemptID)
	require.Equal(t, initial.State, binding.State)
	require.Equal(t, initial.UpdatedAt, binding.UpdatedAt)
	require.Empty(t, h.listRexEvents(t, "wf-mismatch", 10))
}

func TestFMPAuthoritativeReconcilerSuppressesRetryForStaleRexBinding(t *testing.T) {
	now := time.Date(2026, 4, 2, 22, 0, 0, 0, time.UTC)
	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{
		Ownership: ownership,
		Signer:    fwfmp.NewEd25519SignerFromSeed([]byte("e2e-full-app-reconcile-authority")),
		Now:       func() time.Time { return now },
	}
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{
		enableRex:  true,
		fmpService: mesh,
	})
	h.fmpService.Log = h.eventLog

	require.NoError(t, ownership.CreateLineage(context.Background(), core.LineageRecord{
		LineageID:    "lineage-reconcile",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}))
	require.NoError(t, ownership.UpsertAttempt(context.Background(), core.AttemptRecord{
		AttemptID:        "attempt-reconcile",
		LineageID:        "lineage-reconcile",
		RuntimeID:        "rex",
		State:            core.AttemptStateFailed,
		Fenced:           true,
		FencingEpoch:     7,
		StartTime:        now.Add(-2 * time.Minute),
		LastProgressTime: now.Add(-1 * time.Minute),
	}))

	seedRexBinding(t, h, "wf-reconcile", "attempt-reconcile", rexnexus.LineageBinding{
		LineageID: "lineage-reconcile",
		AttemptID: "attempt-reconcile",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: now.Add(-3 * time.Minute),
	})

	record := h.rexRuntime.Agent.RecordAmbiguity("wf-reconcile", "attempt-reconcile", "binding-disagrees-with-fmp")
	require.Equal(t, "lineage-reconcile", record.LineageID)
	require.Equal(t, "attempt-reconcile", record.AttemptID)
	require.Equal(t, int64(7), record.FencingEpoch)
	require.False(t, h.rexRuntime.Agent.ShouldRetryAmbiguity(record))

	binding, ok := readRexBinding(t, h, "wf-reconcile", "attempt-reconcile")
	require.True(t, ok)
	require.Equal(t, string(core.AttemptStateRunning), binding.State)
	require.Empty(t, h.listRexEvents(t, "wf-reconcile", 10))
}

func TestFMPReconciliationOutcomeUpdatesBindingAndOwnership(t *testing.T) {
	now := time.Date(2026, 4, 2, 22, 30, 0, 0, time.UTC)
	ownership := &fwfmp.InMemoryOwnershipStore{}
	mesh := &fwfmp.Service{
		Ownership: ownership,
		Signer:    fwfmp.NewEd25519SignerFromSeed([]byte("e2e-full-app-reconcile-outcome")),
		Now:       func() time.Time { return now },
	}
	h := newNexusHarnessWithOptions(t, testGatewayConfig(), nexusHarnessOptions{
		enableRex:  true,
		fmpService: mesh,
	})

	require.NoError(t, ownership.CreateLineage(context.Background(), core.LineageRecord{
		LineageID:    "lineage-outcome",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}))
	require.NoError(t, ownership.UpsertAttempt(context.Background(), core.AttemptRecord{
		AttemptID:        "attempt-outcome",
		LineageID:        "lineage-outcome",
		RuntimeID:        "rex",
		State:            core.AttemptStateRunning,
		StartTime:        now.Add(-2 * time.Minute),
		LastProgressTime: now.Add(-1 * time.Minute),
	}))

	seedRexBinding(t, h, "wf-outcome", "attempt-outcome", rexnexus.LineageBinding{
		LineageID: "lineage-outcome",
		AttemptID: "attempt-outcome",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: now.Add(-3 * time.Minute),
	})

	record := h.rexRuntime.Agent.RecordAmbiguity("wf-outcome", "attempt-outcome", "operator-confirmed-terminal")
	resolved := h.rexRuntime.Agent.ResolveAmbiguity(record, rexreconcile.OutcomeTerminal, "authoritative FMP terminal")
	require.Equal(t, rexreconcile.StatusTerminal, resolved.Status)
	require.False(t, h.rexRuntime.Agent.ShouldRetryAmbiguity(resolved))

	require.Eventually(t, func() bool {
		attempt := h.getFMPAttempt(t, "attempt-outcome")
		if attempt.State != core.AttemptStateFailed {
			return false
		}
		binding, ok := readRexBinding(t, h, "wf-outcome", "attempt-outcome")
		return ok && binding.State == string(core.AttemptStateFailed)
	}, 5*time.Second, 20*time.Millisecond)
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

func requireArtifactKind(t *testing.T, artifacts []memory.WorkflowArtifactRecord, kind string) memory.WorkflowArtifactRecord {
	t.Helper()
	if artifact, ok := findArtifactKind(artifacts, kind); ok {
		return artifact
	}
	t.Fatalf("artifact kind %s not found", kind)
	return memory.WorkflowArtifactRecord{}
}

func findArtifactKind(artifacts []memory.WorkflowArtifactRecord, kind string) (memory.WorkflowArtifactRecord, bool) {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact, true
		}
	}
	return memory.WorkflowArtifactRecord{}, false
}

func readRexBinding(t *testing.T, h *nexusHarness, workflowID, runID string) (rexnexus.LineageBinding, bool) {
	t.Helper()
	artifact, ok := findArtifactKind(h.listRexArtifacts(t, workflowID, runID), "rex.fmp_lineage")
	if !ok {
		return rexnexus.LineageBinding{}, false
	}
	var binding rexnexus.LineageBinding
	require.NoError(t, json.Unmarshal([]byte(artifact.InlineRawText), &binding))
	return binding, true
}

func seedRexBinding(t *testing.T, h *nexusHarness, workflowID, runID string, binding rexnexus.LineageBinding) {
	t.Helper()
	require.NotNil(t, h.rexRuntime)
	require.NoError(t, h.rexRuntime.WorkflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task:" + runID,
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "seeded workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   binding.UpdatedAt,
		UpdatedAt:   binding.UpdatedAt,
	}))
	require.NoError(t, h.rexRuntime.WorkflowStore.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      runID,
		WorkflowID: workflowID,
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  binding.UpdatedAt,
	}))
	raw, err := json.Marshal(binding)
	require.NoError(t, err)
	require.NoError(t, h.rexRuntime.WorkflowStore.UpsertWorkflowArtifact(context.Background(), memory.WorkflowArtifactRecord{
		ArtifactID:      runID + ":fmp-lineage",
		WorkflowID:      workflowID,
		RunID:           runID,
		Kind:            "rex.fmp_lineage",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "rex fmp lineage binding",
		InlineRawText:   string(raw),
		SummaryMetadata: map[string]any{"lineage_id": binding.LineageID, "attempt_id": binding.AttemptID, "state": binding.State},
		CreatedAt:       binding.UpdatedAt,
	}))
}

func mustJSONPayload(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}

type importedWorkflowRuntimeStore struct{}

func (importedWorkflowRuntimeStore) QueryWorkflowRuntime(context.Context, string, string) (map[string]any, error) {
	return map[string]any{
		"workflow_id": "wf-import",
		"run_id":      "run-import",
		"task": map[string]any{
			"id":          "task-import",
			"type":        string(core.TaskTypeAnalysis),
			"instruction": "inspect imported workflow state",
			"context": map[string]any{
				"workflow_id":        "wf-import",
				"gateway.session_id": "sess-import-1",
				"gateway.tenant_id":  "tenant-1",
				"session_id":         "sess-import-1",
				"tenant_id":          "tenant-1",
			},
			"metadata": map[string]any{
				"origin": "fmp-test",
			},
		},
		"state": map[string]any{
			"workflow_id":        "wf-import",
			"run_id":             "run-import",
			"gateway.session_id": "sess-import-1",
			"gateway.tenant_id":  "tenant-1",
			"session_id":         "sess-import-1",
			"tenant_id":          "tenant-1",
		},
		"events": []string{"checkpointed"},
	}, nil
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
	h.enrollNodeWithApprovedCapabilities(t, "tenant-a", device.Credential(), nodeCaps)
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
	h.enrollNodeWithApprovedCapabilities(t, "tenant-a", device.Credential(), []core.CapabilityDescriptor{{
		ID:   "camera.capture",
		Name: "camera.capture",
		Kind: core.CapabilityKindTool,
	}})
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
	h.enrollNodeWithApprovedCapabilities(t, "tenant-a", device.Credential(), []core.CapabilityDescriptor{{
		ID:   "camera.capture",
		Name: "camera.capture",
		Kind: core.CapabilityKindTool,
	}})
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
		"type":                      "connect",
		"version":                   "1.0",
		"role":                      "node",
		"last_seen_seq":             0,
		"node_id":                   "node-a",
		"node_name":                 "Node A",
		"node_platform":             string(core.NodePlatformLinux),
		"trust_domain":              "local",
		"runtime_id":                "node-a-runtime",
		"runtime_version":           "test",
		"compatibility_class":       "test",
		"supported_context_classes": []string{"workflow-runtime"},
		"transport_profile":         fwgateway.TransportProfileWebSocketLoopback,
		"session_nonce":             "nonce-node-a",
		"session_issued_at":         time.Now().UTC(),
		"session_expires_at":        time.Now().UTC().Add(5 * time.Minute),
		"peer_key_id":               "node-a-peer",
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
		"type":                      "connect",
		"version":                   "1.0",
		"role":                      "node",
		"last_seen_seq":             0,
		"node_id":                   "node-missing",
		"node_name":                 "Missing Node",
		"node_platform":             string(core.NodePlatformLinux),
		"trust_domain":              "local",
		"runtime_id":                "node-missing-runtime",
		"runtime_version":           "test",
		"compatibility_class":       "test",
		"supported_context_classes": []string{"workflow-runtime"},
		"transport_profile":         fwgateway.TransportProfileWebSocketLoopback,
		"session_nonce":             "nonce-node-missing",
		"session_issued_at":         time.Now().UTC(),
		"session_expires_at":        time.Now().UTC().Add(5 * time.Minute),
		"peer_key_id":               "node-missing-peer",
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

	initialSeqs := h.appendEvents(t,
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"one"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"two"}`),
		},
	)

	newSeqs := h.appendEvents(t,
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"three"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"four"}`),
		},
	)

	reconnect := nexustest.NewTestGatewayClient()
	require.NoError(t, reconnect.ConnectWithLastSeen(h.gatewayURL(), "agent-a", "agent", initialSeqs[len(initialSeqs)-1]))
	replayed := h.collectEvents(t, reconnect, 150*time.Millisecond)
	require.NoError(t, reconnect.Close())

	replayedSeqs := map[uint64]struct{}{}
	for _, ev := range replayed {
		if ev.Type == core.FrameworkEventSessionMessage {
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

	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:  "sess-owned",
		TenantID:   "tenant-a",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-owned",
		Owner:      core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	})
	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:  "sess-foreign",
		TenantID:   "tenant-a",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-foreign",
		Owner:      core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-other"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	})
	h.seedBoundary(t, &core.SessionBoundary{
		SessionID:  "sess-tenant-b",
		TenantID:   "tenant-b",
		Scope:      core.SessionScopePerChannelPeer,
		Partition:  nexusEventPartition,
		ChannelID:  "webchat",
		PeerID:     "conv-b",
		Owner:      core.SubjectRef{TenantID: "tenant-b", Kind: core.SubjectKindServiceAccount, ID: "svc-b"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	})

	seqs := h.appendEvents(t,
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"baseline"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-owned", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-owned","text":"tenant-a"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-foreign", TenantID: "tenant-a"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-foreign","text":"tenant-a-foreign"}`),
		},
		core.FrameworkEvent{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Actor:     core.EventActor{Kind: "session", ID: "sess-tenant-b", TenantID: "tenant-b"},
			Partition: nexusEventPartition,
			Payload:   json.RawMessage(`{"session_key":"sess-tenant-b","text":"tenant-b"}`),
		},
	)

	client := nexustest.NewTestGatewayClient()
	require.NoError(t, client.ConnectWithLastSeen(h.gatewayURL(), "agent-a", "agent", seqs[0]))
	replayed := h.collectEvents(t, client, 150*time.Millisecond)
	require.NoError(t, client.Close())

	delivered := map[uint64]struct{}{}
	for _, ev := range replayed {
		if ev.Type == core.FrameworkEventSessionMessage {
			delivered[ev.Seq] = struct{}{}
			require.Equal(t, "tenant-a", ev.Actor.TenantID)
		}
	}
	_, ok := delivered[seqs[1]]
	require.True(t, ok, "expected tenant-a replay seq %d", seqs[1])
	_, ok = delivered[seqs[2]]
	require.False(t, ok, "did not expect foreign-owner replay seq %d", seqs[2])
	_, ok = delivered[seqs[3]]
	require.False(t, ok, "did not expect tenant-b replay seq %d", seqs[3])
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
