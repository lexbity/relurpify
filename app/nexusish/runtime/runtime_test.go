package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	"github.com/stretchr/testify/require"
)

type stubAdminClient struct {
	resources map[string]any
}

func (s stubAdminClient) CallTool(context.Context, protocol.CallToolParams) (*protocol.CallToolResult, error) {
	return nil, fmt.Errorf("unexpected tool call")
}

func (s stubAdminClient) ReadResource(_ context.Context, uri string) (*protocol.ReadResourceResult, error) {
	value, ok := s.resources[uri]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &protocol.ReadResourceResult{
		Contents: []protocol.ContentBlock{{
			Type: "text",
			Text: string(data),
		}},
	}, nil
}

func (s stubAdminClient) Close() error { return nil }

func TestStateEnrichesHealthWithIdentitySecurityResources(t *testing.T) {
	rt := &Runtime{}
	client := stubAdminClient{resources: map[string]any{
		"nexus://gateway/health": map[string]any{
			"online":            true,
			"pid":               12,
			"bind_addr":         "127.0.0.1:8090",
			"uptime_seconds":    10,
			"tenant_id":         "local",
			"last_seq":          4,
			"paired_nodes":      []map[string]any{{"id": "node-1", "name": "Node 1", "platform": "linux", "trust_class": "remote_approved"}},
			"pending_pairings":  []map[string]any{},
			"channels":          []map[string]any{{"name": "webchat", "configured": true, "connected": true, "inbound": 1, "outbound": 2}},
			"active_sessions":   []map[string]any{{"id": "session-1", "role": "agent"}},
			"security_warnings": []string{"warning-a"},
			"event_counts":      map[string]uint64{"session.created.v1": 1},
		},
		"nexus://identity/subjects": map[string]any{
			"subjects": []map[string]any{{"tenant_id": "local", "kind": "node", "id": "node-1"}},
		},
		"nexus://identity/externals": map[string]any{
			"identities": []map[string]any{{"tenant_id": "local", "provider": "webchat", "account_id": "acct-1", "external_id": "ext-1", "display_name": "User 1", "subject": map[string]any{"id": "user-1"}}},
		},
		"nexus://tokens/list": map[string]any{
			"tokens": []map[string]any{{"id": "tok-1", "name": "subject-a", "subject_id": "subject-a", "scope": []string{"nexus:admin"}}},
		},
		"nexus://policy/rules": map[string]any{
			"rules": []map[string]any{{"id": "rule-1", "name": "Rule 1", "priority": 10, "enabled": true, "effect": map[string]any{"action": "allow", "reason": "ok"}}},
		},
	}}
	rt.client = client

	state, err := rt.State(context.Background())
	require.NoError(t, err)
	require.Len(t, state.Subjects, 1)
	require.Len(t, state.ExternalIDs, 1)
	require.Len(t, state.Tokens, 1)
	require.Len(t, state.PolicyRules, 1)
	require.True(t, state.Channels[0].Connected)
	require.Equal(t, "Rule 1", state.PolicyRules[0].Name)
}

func TestStateFallsBackToResourceGraph(t *testing.T) {
	rt := &Runtime{}
	client := stubAdminClient{resources: map[string]any{
		"nexus://nodes/enrolled": map[string]any{
			"nodes": []map[string]any{{"id": "node-1", "name": "Node 1", "platform": "linux", "trust_class": "remote_approved"}},
		},
		"nexus://nodes/pending": map[string]any{"pairings": []map[string]any{}},
		"nexus://channels/status": map[string]any{
			"channels": []map[string]any{{"name": "webchat", "configured": true, "connected": true, "reconnects": 1, "inbound": 3, "outbound": 4}},
		},
		"nexus://sessions/active": map[string]any{
			"sessions": []map[string]any{{"session_id": "session-1"}},
		},
		"nexus://gateway/events": map[string]any{
			"events": []map[string]any{{"type": "message.inbound.v1", "count": 3}},
		},
		"nexus://identity/subjects": map[string]any{
			"subjects": []map[string]any{{"tenant_id": "local", "kind": "node", "id": "node-1"}},
		},
		"nexus://identity/externals": map[string]any{
			"identities": []map[string]any{{"tenant_id": "local", "provider": "webchat", "account_id": "acct-1", "external_id": "ext-1", "subject": map[string]any{"id": "user-1"}}},
		},
		"nexus://tokens/list": map[string]any{
			"tokens": []map[string]any{{"id": "tok-1", "name": "subject-a", "subject_id": "subject-a", "scope": []string{"nexus:admin"}}},
		},
		"nexus://policy/rules": map[string]any{
			"rules": []map[string]any{{"id": "rule-1", "name": "Rule 1", "priority": 10, "enabled": true, "effect": map[string]any{"action": "allow"}}},
		},
	}}
	rt.client = client

	state, err := rt.State(context.Background())
	require.NoError(t, err)
	require.True(t, state.Online)
	require.Len(t, state.ActiveSessions, 1)
	require.Len(t, state.ExternalIDs, 1)
	require.Equal(t, "session-1", state.ActiveSessions[0].ID)
	require.Len(t, state.Channels, 1)
	require.Equal(t, uint64(3), state.EventCounts["message.inbound.v1"])
}
