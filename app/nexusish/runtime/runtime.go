package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	nexusadminapi "github.com/lexcodex/relurpify/app/nexus/adminapi"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	mcpclient "github.com/lexcodex/relurpify/framework/middleware/mcp/client"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
)

type RuntimeMode string

const (
	RuntimeModeHTTP  RuntimeMode = "http"
	RuntimeModeStdio RuntimeMode = "stdio"
)

type adminClient interface {
	CallTool(context.Context, protocol.CallToolParams) (*protocol.CallToolResult, error)
	ReadResource(context.Context, string) (*protocol.ReadResourceResult, error)
	Close() error
}

type Runtime struct {
	Workspace  string
	ConfigPath string

	mu     sync.Mutex
	client adminClient
	mode   RuntimeMode
}

func New(workspace, configPath string) *Runtime {
	return &Runtime{Workspace: workspace, ConfigPath: configPath}
}

func (r *Runtime) State(ctx context.Context) (RuntimeState, error) {
	client, err := r.ensureClient(ctx)
	if err != nil {
		return RuntimeState{}, err
	}
	var payload nexusadminapi.HealthResult
	if err := r.readResourceJSON(ctx, client, "nexus://gateway/health", &payload); err == nil {
		state := healthResultToRuntimeState(payload)
		return r.enrichState(ctx, client, state), nil
	}
	return r.stateFromResources(ctx, client)
}

func (r *Runtime) ApprovePairing(ctx context.Context, pairingCode string) error {
	return r.callTool(ctx, "nexus.nodes.approve_pairing", map[string]any{"code": pairingCode})
}

func (r *Runtime) RejectPairing(ctx context.Context, pairingCode string) error {
	return r.callTool(ctx, "nexus.nodes.reject_pairing", map[string]any{"code": pairingCode})
}

func (r *Runtime) RevokeNode(ctx context.Context, nodeID string) error {
	return r.callTool(ctx, "nexus.nodes.revoke", map[string]any{"node_id": nodeID})
}

func (r *Runtime) CloseSession(ctx context.Context, sessionID string) error {
	return r.callTool(ctx, "nexus.sessions.close", map[string]any{"session_id": sessionID})
}

func (r *Runtime) IssueToken(ctx context.Context, req IssueTokenRequest) (string, error) {
	client, err := r.ensureClient(ctx)
	if err != nil {
		return "", err
	}
	result, err := client.CallTool(ctx, protocol.CallToolParams{
		Name: "nexus.tokens.issue",
		Arguments: map[string]any{
			"subject_id":  req.SubjectID,
			"scopes":      splitScope(req.Scope),
			"api_version": nexusadminapi.APIVersionV1Alpha1,
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", toolResultError(result)
	}
	tokenID, _ := result.StructuredContent["token_id"].(string)
	if token, _ := result.StructuredContent["token"].(string); strings.TrimSpace(token) != "" {
		return token, nil
	}
	return tokenID, nil
}

func (r *Runtime) RevokeToken(ctx context.Context, tokenID string) error {
	return r.callTool(ctx, "nexus.tokens.revoke", map[string]any{"token_id": tokenID})
}

func (r *Runtime) SetPolicyRuleEnabled(ctx context.Context, ruleID string, enabled bool) error {
	return r.callTool(ctx, "nexus.policy.set_rule_enabled", map[string]any{"rule_id": ruleID, "enabled": enabled})
}

func (r *Runtime) RestartChannel(ctx context.Context, channel string) error {
	return r.callTool(ctx, "nexus.channels.restart", map[string]any{"channel": channel})
}

func (r *Runtime) ListEvents(ctx context.Context, req ListEventsRequest) ([]EventInfo, error) {
	state, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EventInfo, 0, len(state.EventCounts))
	for eventType, count := range state.EventCounts {
		out = append(out, EventInfo{Type: eventType, Count: count})
	}
	return out, nil
}

func (r *Runtime) ensureClient(ctx context.Context) (adminClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	cfg, err := r.loadConfig()
	if err != nil {
		return nil, err
	}
	if client, err := r.connectHTTP(ctx, cfg); err == nil {
		r.client = client
		r.mode = RuntimeModeHTTP
		return r.client, nil
	}
	client, err := r.connectStdio(ctx, cfg)
	if err != nil {
		return nil, err
	}
	r.client = client
	r.mode = RuntimeModeStdio
	return r.client, nil
}

func (r *Runtime) loadConfig() (nexuscfg.Config, error) {
	paths := config.New(r.Workspace)
	configPath := r.ConfigPath
	if configPath == "" {
		configPath = paths.NexusConfigFile()
	}
	return nexuscfg.Load(configPath)
}

func (r *Runtime) connectHTTP(ctx context.Context, cfg nexuscfg.Config) (adminClient, error) {
	token := selectAdminToken(cfg)
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("no admin token configured")
	}
	client, err := newHTTPMCPClient(ctx, httpAdminURL(cfg), token)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (r *Runtime) connectStdio(ctx context.Context, cfg nexuscfg.Config) (adminClient, error) {
	command, args := localAdminCommand(r.Workspace, r.ConfigPath, selectAdminToken(cfg))
	return mcpclient.ConnectStdio(ctx, nil, mcpclient.StdioConfig{
		Command:      command,
		Args:         args,
		ProviderID:   "nexus-admin",
		SessionID:    "nexusish-admin",
		RemoteTarget: "stdio://nexus-admin",
		LocalPeer:    protocol.PeerInfo{Name: "nexusish", Version: "v1alpha1"},
	})
}

func httpAdminURL(cfg nexuscfg.Config) string {
	bind := strings.TrimSpace(cfg.Gateway.Bind)
	switch {
	case bind == "":
		bind = "127.0.0.1:8090"
	case strings.HasPrefix(bind, ":"):
		bind = "127.0.0.1" + bind
	}
	return "http://" + bind + "/admin/mcp"
}

func selectAdminToken(cfg nexuscfg.Config) string {
	for _, entry := range cfg.Gateway.Auth.Tokens {
		role := strings.ToLower(strings.TrimSpace(entry.Role))
		if role == "admin" || role == "operator" {
			return strings.TrimSpace(entry.Token)
		}
		for _, scope := range entry.Scopes {
			scope = strings.ToLower(strings.TrimSpace(scope))
			if scope == "gateway:admin" || scope == "nexus:admin" || scope == "nexus:operator" {
				return strings.TrimSpace(entry.Token)
			}
		}
	}
	return ""
}

func localAdminCommand(workspace, configPath, token string) (string, []string) {
	args := []string{"admin", "mcp", "--workspace", workspace}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	if token != "" {
		args = append(args, "--token", token)
	}
	if _, err := exec.LookPath("nexus"); err == nil {
		return "nexus", args
	}
	return "go", append([]string{"run", "./app/nexus"}, args...)
}

func healthResultToRuntimeState(h nexusadminapi.HealthResult) RuntimeState {
	return RuntimeState{
		Online:           h.Online,
		PID:              h.PID,
		BindAddr:         h.BindAddr,
		Uptime:           time.Duration(h.UptimeSeconds) * time.Second,
		LastSeq:          h.LastSeq,
		TenantID:         h.TenantID,
		PairedNodes:      toNodeInfos(h.PairedNodes),
		PendingPairings:  append([]PendingPairingInfo(nil), h.PendingPairings...),
		Channels:         append([]ChannelInfo(nil), h.Channels...),
		ActiveSessions:   append([]SessionInfo(nil), h.ActiveSessions...),
		SecurityWarnings: append([]string(nil), h.SecurityWarnings...),
		EventCounts:      cloneCounts(h.EventCounts),
	}
}

func cloneCounts(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (r *Runtime) callTool(ctx context.Context, name string, args map[string]any) error {
	client, err := r.ensureClient(ctx)
	if err != nil {
		return err
	}
	if args == nil {
		args = map[string]any{}
	}
	args["api_version"] = nexusadminapi.APIVersionV1Alpha1
	result, err := client.CallTool(ctx, protocol.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err
	}
	if result.IsError {
		return toolResultError(result)
	}
	return nil
}

func (r *Runtime) readResourceJSON(ctx context.Context, client adminClient, uri string, target any) error {
	read, err := client.ReadResource(ctx, uri)
	if err != nil {
		return err
	}
	if len(read.Contents) == 0 || strings.TrimSpace(read.Contents[0].Text) == "" {
		return fmt.Errorf("empty resource: %s", uri)
	}
	return json.Unmarshal([]byte(read.Contents[0].Text), target)
}

func (r *Runtime) stateFromResources(ctx context.Context, client adminClient) (RuntimeState, error) {
	var (
		nodes     nexusadminapi.ListNodesResult
		pending   nexusadminapi.ListPendingPairingsResult
		channels  nexusadminapi.ListChannelsResult
		sessions  nexusadminapi.ListSessionsResult
		events    nexusadminapi.ListEventsResult
		subjects  nexusadminapi.ListSubjectsResult
		externals nexusadminapi.ListExternalIdentitiesResult
		tokens    nexusadminapi.ListTokensResult
		policies  nexusadminapi.ListPolicyRulesResult
	)
	if err := r.readResourceJSON(ctx, client, "nexus://nodes/enrolled", &nodes); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://nodes/pending", &pending); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://channels/status", &channels); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://sessions/active", &sessions); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://gateway/events", &events); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://identity/subjects", &subjects); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://identity/externals", &externals); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://tokens/list", &tokens); err != nil {
		return RuntimeState{}, err
	}
	if err := r.readResourceJSON(ctx, client, "nexus://policy/rules", &policies); err != nil {
		return RuntimeState{}, err
	}
	counts := make(map[string]uint64, len(events.Events))
	for _, event := range events.Events {
		counts[event.Type] = event.Count
	}
	return RuntimeState{
		Online:          true,
		PairedNodes:     toNodeInfos(nodes.Nodes),
		PendingPairings: append([]PendingPairingInfo(nil), pending.Pairings...),
		Channels:        append([]ChannelInfo(nil), channels.Channels...),
		ActiveSessions:  toSessionInfos(sessions.Sessions),
		Subjects:        append([]SubjectInfo(nil), subjects.Subjects...),
		ExternalIDs:     toExternalIdentityInfos(externals.Identities),
		Tokens:          append([]TokenInfo(nil), tokens.Tokens...),
		PolicyRules:     toPolicyRuleInfos(policies.Rules),
		EventCounts:     counts,
	}, nil
}

func (r *Runtime) enrichState(ctx context.Context, client adminClient, state RuntimeState) RuntimeState {
	var (
		subjects  nexusadminapi.ListSubjectsResult
		externals nexusadminapi.ListExternalIdentitiesResult
		tokens    nexusadminapi.ListTokensResult
		policies  nexusadminapi.ListPolicyRulesResult
	)
	if err := r.readResourceJSON(ctx, client, "nexus://identity/subjects", &subjects); err == nil {
		state.Subjects = append([]SubjectInfo(nil), subjects.Subjects...)
	}
	if err := r.readResourceJSON(ctx, client, "nexus://identity/externals", &externals); err == nil {
		state.ExternalIDs = toExternalIdentityInfos(externals.Identities)
	}
	if err := r.readResourceJSON(ctx, client, "nexus://tokens/list", &tokens); err == nil {
		state.Tokens = append([]TokenInfo(nil), tokens.Tokens...)
	}
	if err := r.readResourceJSON(ctx, client, "nexus://policy/rules", &policies); err == nil {
		state.PolicyRules = toPolicyRuleInfos(policies.Rules)
	}
	return state
}

func toNodeInfos(in []core.NodeDescriptor) []NodeInfo {
	out := make([]NodeInfo, 0, len(in))
	for _, node := range in {
		out = append(out, NodeInfo{
			ID:         node.ID,
			Name:       node.Name,
			Platform:   string(node.Platform),
			TenantID:   node.TenantID,
			TrustClass: string(node.TrustClass),
		})
	}
	return out
}

func toPolicyRuleInfos(in []core.PolicyRule) []PolicyRuleInfo {
	out := make([]PolicyRuleInfo, 0, len(in))
	for _, rule := range in {
		out = append(out, PolicyRuleInfo{
			ID:       rule.ID,
			Name:     rule.Name,
			Priority: rule.Priority,
			Enabled:  rule.Enabled,
			Action:   rule.Effect.Action,
			Reason:   rule.Effect.Reason,
		})
	}
	return out
}

func toSessionInfos(in []core.SessionBoundary) []SessionInfo {
	out := make([]SessionInfo, 0, len(in))
	for _, session := range in {
		out = append(out, SessionInfo{ID: session.SessionID})
	}
	return out
}

func toExternalIdentityInfos(in []core.ExternalIdentity) []ExternalIdentityInfo {
	out := make([]ExternalIdentityInfo, 0, len(in))
	for _, identity := range in {
		out = append(out, ExternalIdentityInfo{
			TenantID:    identity.TenantID,
			Provider:    string(identity.Provider),
			AccountID:   identity.AccountID,
			ExternalID:  identity.ExternalID,
			SubjectID:   identity.Subject.ID,
			DisplayName: identity.DisplayName,
		})
	}
	return out
}

func splitScope(scope string) []string {
	fields := strings.FieldsFunc(scope, func(r rune) bool {
		return r == ',' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func toolResultError(result *protocol.CallToolResult) error {
	if result == nil {
		return fmt.Errorf("nil tool result")
	}
	if raw, ok := result.StructuredContent["error"].(map[string]any); ok {
		if message, ok := raw["message"].(string); ok && strings.TrimSpace(message) != "" {
			return fmt.Errorf("%s", message)
		}
	}
	if len(result.Content) > 0 && strings.TrimSpace(result.Content[0].Text) != "" {
		return fmt.Errorf("%s", result.Content[0].Text)
	}
	return fmt.Errorf("admin tool failed")
}
