package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/adminapi"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	"github.com/stretchr/testify/require"
)

type recordingAdminClient struct {
	resources map[string]any
	results   map[string]*protocol.CallToolResult
	calls     []protocol.CallToolParams
	readURIs  []string
}

func (r *recordingAdminClient) CallTool(_ context.Context, params protocol.CallToolParams) (*protocol.CallToolResult, error) {
	r.calls = append(r.calls, params)
	if result, ok := r.results[params.Name]; ok {
		return result, nil
	}
	return &protocol.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil
}

func (r *recordingAdminClient) ReadResource(_ context.Context, uri string) (*protocol.ReadResourceResult, error) {
	r.readURIs = append(r.readURIs, uri)
	value, ok := r.resources[uri]
	if !ok {
		return nil, fmt.Errorf("missing resource: %s", uri)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &protocol.ReadResourceResult{Contents: []protocol.ContentBlock{{Type: "text", Text: string(data)}}}, nil
}

func (r *recordingAdminClient) Close() error { return nil }

func TestRuntimeHelperFunctions(t *testing.T) {
	require.Equal(t, "http://127.0.0.1:8090/admin/mcp", httpAdminURL(nexuscfg.Config{}))
	require.Equal(t, "http://127.0.0.1:8091/admin/mcp", httpAdminURL(nexuscfg.Config{Gateway: nexuscfg.GatewayConfig{Bind: ":8091"}}))
	require.Equal(t, []string{"a", "b", "c"}, splitScope("a, b c"))

	counts := cloneCounts(map[string]uint64{"a": 1})
	counts["a"] = 2
	require.Equal(t, uint64(1), cloneCounts(map[string]uint64{"a": 1})["a"])

	require.Equal(t, "tok-1", selectAdminToken(nexuscfg.Config{Gateway: nexuscfg.GatewayConfig{Auth: nexuscfg.GatewayAuthConfig{Tokens: []nexuscfg.GatewayTokenAuth{{Role: "admin", Token: "tok-1"}}}}}))
	require.Equal(t, "tok-2", selectAdminToken(nexuscfg.Config{Gateway: nexuscfg.GatewayConfig{Auth: nexuscfg.GatewayAuthConfig{Tokens: []nexuscfg.GatewayTokenAuth{{Scopes: []string{"nexus:operator"}, Token: "tok-2"}}}}}))

	_, args := localAdminCommand("/ws", "/cfg", "secret")
	require.Contains(t, args, "--workspace")
	require.Contains(t, args, "/ws")
	require.Contains(t, args, "--config")
	require.Contains(t, args, "/cfg")
	require.Contains(t, args, "--token")
	require.Contains(t, args, "secret")

	noop := &protocol.CallToolResult{StructuredContent: map[string]any{"error": map[string]any{"message": "boom"}}}
	require.ErrorContains(t, toolResultError(noop), "boom")
	require.ErrorContains(t, toolResultError(&protocol.CallToolResult{Content: []protocol.ContentBlock{{Text: "failed"}}}), "failed")
	require.ErrorContains(t, toolResultError(&protocol.CallToolResult{}), "admin tool failed")
	require.ErrorContains(t, toolResultError(nil), "nil tool result")

	dir := t.TempDir()
	fakeNexus := filepath.Join(dir, "nexus")
	require.NoError(t, os.WriteFile(fakeNexus, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })
	require.NoError(t, os.Setenv("PATH", dir))
	cmd, _ := localAdminCommand("/workspace", "", "")
	require.Equal(t, "nexus", cmd)
}

func TestRuntimeStateAndToolCalls(t *testing.T) {
	client := &recordingAdminClient{
		resources: map[string]any{
			"nexus://gateway/health": map[string]any{
				"online":            true,
				"pid":               22,
				"bind_addr":         "127.0.0.1:8090",
				"uptime_seconds":    12,
				"tenant_id":         "tenant-a",
				"last_seq":          9,
				"paired_nodes":      []map[string]any{{"id": "node-1", "name": "Node 1", "platform": "linux", "trust_class": "remote_approved"}},
				"pending_pairings":  []map[string]any{{"code": "PAIR-1"}},
				"channels":          []map[string]any{{"name": "webchat", "configured": true, "connected": true}},
				"active_sessions":   []map[string]any{{"id": "session-1"}},
				"security_warnings": []string{"warn-a"},
				"event_counts":      map[string]uint64{"message.inbound.v1": 3},
			},
			"nexus://identity/subjects":  map[string]any{"subjects": []map[string]any{{"tenant_id": "tenant-a", "kind": "user", "id": "subject-1"}}},
			"nexus://identity/externals": map[string]any{"identities": []map[string]any{{"tenant_id": "tenant-a", "provider": "webchat", "account_id": "acct-1", "external_id": "ext-1", "subject": map[string]any{"id": "subject-1"}, "display_name": "User 1"}}},
			"nexus://tokens/list":        map[string]any{"tokens": []map[string]any{{"id": "tok-1", "name": "subject-1"}}},
			"nexus://policy/rules":       map[string]any{"rules": []map[string]any{{"id": "rule-1", "name": "Rule 1", "priority": 1, "enabled": true, "effect": map[string]any{"action": "allow", "reason": "ok"}}}},
			"nexus://nodes/enrolled":     map[string]any{"nodes": []map[string]any{{"id": "node-1", "name": "Node 1", "platform": "linux", "trust_class": "remote_approved"}}},
			"nexus://nodes/pending":      map[string]any{"pairings": []map[string]any{{"code": "PAIR-1"}}},
			"nexus://channels/status":    map[string]any{"channels": []map[string]any{{"name": "webchat", "configured": true, "connected": true}}},
			"nexus://sessions/active":    map[string]any{"sessions": []map[string]any{{"session_id": "session-1"}}},
			"nexus://gateway/events":     map[string]any{"events": []map[string]any{{"type": "message.inbound.v1", "count": 3}}},
		},
		results: map[string]*protocol.CallToolResult{
			"nexus.identity.create_subject": &protocol.CallToolResult{StructuredContent: map[string]any{"subject_id": "subject-1"}},
			"nexus.tokens.issue":            &protocol.CallToolResult{StructuredContent: map[string]any{"token": "issued-token"}},
			"nexus.policy.set_rule_enabled": &protocol.CallToolResult{StructuredContent: map[string]any{"ok": true}},
		},
	}

	rt := &Runtime{client: client}

	state, err := rt.State(context.Background())
	require.NoError(t, err)
	require.True(t, state.Online)
	require.Equal(t, "tenant-a", state.TenantID)
	require.Len(t, state.PairedNodes, 1)
	require.Len(t, state.PendingPairings, 1)
	require.Len(t, state.Channels, 1)
	require.Len(t, state.ActiveSessions, 1)
	require.Len(t, state.Subjects, 1)
	require.Len(t, state.ExternalIDs, 1)
	require.Len(t, state.Tokens, 1)
	require.Len(t, state.PolicyRules, 1)
	require.Equal(t, uint64(3), state.EventCounts["message.inbound.v1"])

	token, err := rt.IssueToken(context.Background(), IssueTokenRequest{
		SubjectTenantID: "tenant-a",
		SubjectID:       "subject-1",
		Scope:           "nexus:admin, nexus:operator",
	})
	require.NoError(t, err)
	require.Equal(t, "issued-token", token)

	require.NoError(t, rt.SetPolicyRuleEnabled(context.Background(), "rule-1", true))
	require.GreaterOrEqual(t, len(client.calls), 3)
	require.Equal(t, "nexus.identity.create_subject", client.calls[0].Name)
	require.Equal(t, []string{"nexus:admin", "nexus:operator"}, client.calls[1].Arguments["scopes"])
	require.Equal(t, true, client.calls[len(client.calls)-1].Arguments["enabled"])
	require.Equal(t, adminapi.APIVersionV1Alpha1, client.calls[len(client.calls)-1].Arguments["api_version"])
}

func TestRuntimeResponseTransforms(t *testing.T) {
	state := healthResultToRuntimeState(adminapi.HealthResult{
		Online:           true,
		PID:              3,
		BindAddr:         "127.0.0.1:8090",
		UptimeSeconds:    7,
		LastSeq:          11,
		TenantID:         "tenant",
		PairedNodes:      []core.NodeDescriptor{{ID: "node-1"}},
		PendingPairings:  []adminapi.PendingPairingInfo{{Code: "PAIR-1"}},
		Channels:         []adminapi.ChannelInfo{{Name: "webchat"}},
		ActiveSessions:   []adminapi.SessionInfo{{ID: "session-1"}},
		SecurityWarnings: []string{"warn"},
		EventCounts:      map[string]uint64{"a": 1},
	})
	require.True(t, state.Online)
	require.Equal(t, 7*time.Second, state.Uptime)
	require.Equal(t, "node-1", state.PairedNodes[0].ID)

	require.Equal(t, []NodeInfo{{ID: "node-1", Name: "", Platform: "", TenantID: "", TrustClass: ""}}, toNodeInfos([]core.NodeDescriptor{{ID: "node-1"}}))
	require.Equal(t, []SessionInfo{{ID: "session-1"}}, toSessionInfos([]core.SessionBoundary{{SessionID: "session-1"}}))
	require.Equal(t, []ExternalIdentityInfo{{TenantID: "tenant", Provider: "webchat", AccountID: "acct", ExternalID: "ext", SubjectID: "subject-1", DisplayName: "User"}}, toExternalIdentityInfos([]core.ExternalIdentity{{TenantID: "tenant", Provider: core.ExternalProviderWebchat, AccountID: "acct", ExternalID: "ext", Subject: core.SubjectRef{ID: "subject-1"}, DisplayName: "User"}}))
	require.Equal(t, []PolicyRuleInfo{{ID: "rule-1", Name: "Rule 1", Priority: 1, Enabled: true, Action: "allow", Reason: "ok"}}, toPolicyRuleInfos([]core.PolicyRule{{ID: "rule-1", Name: "Rule 1", Priority: 1, Enabled: true, Effect: core.PolicyEffect{Action: "allow", Reason: "ok"}}}))
	require.Equal(t, map[string]uint64{"a": 1}, cloneCounts(map[string]uint64{"a": 1}))
}
