package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	nexusgateway "github.com/lexcodex/relurpify/app/nexus/gateway"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
)

func (s *service) ListChannels(ctx context.Context, req ListChannelsRequest) (ListChannelsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListChannelsResult{}, err
	}
	projection, err := s.tenantRuntimeProjection(ctx, tenantID)
	if err != nil {
		return ListChannelsResult{}, err
	}
	statuses := map[string]channel.AdapterStatus{}
	if s.cfg.Channels != nil {
		statuses = s.cfg.Channels.Status()
	}
	names := make(map[string]struct{}, len(s.cfg.Config.Channels)+len(statuses))
	for name := range s.cfg.Config.Channels {
		names[name] = struct{}{}
	}
	for name := range statuses {
		names[name] = struct{}{}
	}
	channels := make([]ChannelInfo, 0, len(names))
	for name := range names {
		activity := projection.ChannelActivity[name]
		status := statuses[name]
		_, configured := s.cfg.Config.Channels[name]
		channels = append(channels, ChannelInfo{
			Name:       name,
			Configured: configured,
			Connected:  status.Connected,
			LastError:  status.LastError,
			Reconnects: status.Reconnects,
			Inbound:    activity.Inbound,
			Outbound:   activity.Outbound,
		})
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].Name < channels[j].Name })
	channels = applyPage(channels, req.Page)
	return ListChannelsResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(channels)), Channels: channels}, nil
}

func (s *service) RestartChannel(ctx context.Context, req RestartChannelRequest) (RestartChannelResult, error) {
	if _, err := authorizeTenant(req.Principal, req.TenantID); err != nil {
		return RestartChannelResult{}, err
	}
	if s.cfg.Channels == nil {
		return RestartChannelResult{}, notImplemented("restart channel not implemented", nil)
	}
	channelName := strings.TrimSpace(req.Channel)
	if channelName == "" {
		return RestartChannelResult{}, invalidArgument("channel required", map[string]any{"field": "channel"})
	}
	if err := s.cfg.Channels.Restart(ctx, channelName); err != nil {
		return RestartChannelResult{}, internalError("restart channel failed", err, map[string]any{"channel": channelName})
	}
	return RestartChannelResult{AdminResult: resultEnvelope(req.AdminRequest), Channel: channelName}, nil
}

func (s *service) ListPolicyRules(ctx context.Context, req ListPolicyRulesRequest) (ListPolicyRulesResult, error) {
	if _, err := authorizeTenant(req.Principal, req.TenantID); err != nil {
		return ListPolicyRulesResult{}, err
	}
	if s.cfg.Policies == nil {
		return ListPolicyRulesResult{}, notImplemented("list policy rules not implemented", nil)
	}
	rules, err := s.cfg.Policies.ListRules(ctx)
	if err != nil {
		return ListPolicyRulesResult{}, internalError("list policy rules failed", err, nil)
	}
	rules = applyPage(rules, req.Page)
	return ListPolicyRulesResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(rules)), Rules: rules}, nil
}

func (s *service) SetPolicyRuleEnabled(ctx context.Context, req SetPolicyRuleEnabledRequest) (SetPolicyRuleEnabledResult, error) {
	if _, err := authorizeTenant(req.Principal, req.TenantID); err != nil {
		return SetPolicyRuleEnabledResult{}, err
	}
	if s.cfg.Policies == nil {
		return SetPolicyRuleEnabledResult{}, notImplemented("set policy rule enabled not implemented", nil)
	}
	if strings.TrimSpace(req.RuleID) == "" {
		return SetPolicyRuleEnabledResult{}, invalidArgument("rule_id required", map[string]any{"field": "rule_id"})
	}
	if err := s.cfg.Policies.SetRuleEnabled(ctx, req.RuleID, req.Enabled); err != nil {
		if os.IsNotExist(err) {
			return SetPolicyRuleEnabledResult{}, notFound("policy rule not found", map[string]any{"rule_id": req.RuleID})
		}
		return SetPolicyRuleEnabledResult{}, internalError("set policy rule failed", err, map[string]any{"rule_id": req.RuleID})
	}
	return SetPolicyRuleEnabledResult{AdminResult: resultEnvelope(req.AdminRequest), RuleID: req.RuleID, Enabled: req.Enabled}, nil
}

func (s *service) Health(ctx context.Context, req HealthRequest) (HealthResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return HealthResult{}, err
	}
	state := nexusgateway.StateSnapshot{}
	if s.cfg.Materializer != nil {
		state = s.cfg.Materializer.State()
	}
	projection, err := s.tenantRuntimeProjection(ctx, tenantID)
	if err != nil {
		return HealthResult{}, err
	}
	nodes, err := s.ListNodes(ctx, ListNodesRequest{AdminRequest: req.AdminRequest})
	if err != nil {
		return HealthResult{}, err
	}
	pairings, err := s.ListPendingPairings(ctx, ListPendingPairingsRequest{AdminRequest: req.AdminRequest})
	if err != nil {
		return HealthResult{}, err
	}
	sessions, err := s.ListSessions(ctx, ListSessionsRequest{AdminRequest: req.AdminRequest})
	if err != nil {
		return HealthResult{}, err
	}
	channelResult, err := s.ListChannels(ctx, ListChannelsRequest{AdminRequest: req.AdminRequest})
	if err != nil {
		return HealthResult{}, err
	}
	activeSessions := make([]SessionInfo, 0, len(sessions.Sessions))
	for _, boundary := range sessions.Sessions {
		activeSessions = append(activeSessions, SessionInfo{
			ID:   boundary.SessionID,
			Role: state.ActiveSessions[boundary.SessionID].Role,
		})
	}
	return HealthResult{
		AdminResult:      resultEnvelope(req.AdminRequest),
		Online:           true,
		PID:              os.Getpid(),
		BindAddr:         s.cfg.Config.Gateway.Bind,
		UptimeSeconds:    int64(time.Since(s.cfg.StartedAt).Seconds()),
		TenantID:         tenantID,
		LastSeq:          projection.LastSeq,
		PairedNodes:      nodes.Nodes,
		PendingPairings:  pairings.Pairings,
		Channels:         channelResult.Channels,
		ActiveSessions:   activeSessions,
		SecurityWarnings: s.cfg.Config.SecurityWarnings(len(pairings.Pairings)),
		EventCounts:      copyEventCounts(projection.EventTypeCounts),
	}, nil
}

func (s *service) ListEvents(ctx context.Context, req ListEventsRequest) (ListEventsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListEventsResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	afterSeq, err := decodeCursor(req.Cursor)
	if err != nil {
		return ListEventsResult{}, invalidArgument("invalid cursor", map[string]any{"cursor": req.Cursor})
	}
	events, err := s.cfg.Events.Read(ctx, s.cfg.Partition, afterSeq, limit, false)
	if err != nil {
		return ListEventsResult{}, internalError("list events failed", err, nil)
	}
	events = filterEventsByTenant(events, tenantID)
	counts := make(map[string]uint64)
	for _, ev := range events {
		counts[ev.Type]++
	}
	result := ListEventsResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		PageResult: PageResult{
			NextCursor: nextCursor(events),
			Total:      -1,
		},
	}
	for eventType, count := range counts {
		result.Events = append(result.Events, EventInfo{Type: eventType, Count: count})
	}
	sort.Slice(result.Events, func(i, j int) bool {
		if result.Events[i].Count == result.Events[j].Count {
			return result.Events[i].Type < result.Events[j].Type
		}
		return result.Events[i].Count > result.Events[j].Count
	})
	return result, nil
}

func (s *service) ReadEventStream(ctx context.Context, req ReadEventStreamRequest) (ReadEventStreamResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ReadEventStreamResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	events, err := s.cfg.Events.Read(ctx, s.cfg.Partition, req.AfterSeq, limit, false)
	if err != nil {
		return ReadEventStreamResult{}, internalError("read event stream failed", err, map[string]any{"after_seq": req.AfterSeq, "limit": limit})
	}
	events = filterEventsByTenant(events, tenantID)
	result := ReadEventStreamResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		AfterSeq:    req.AfterSeq,
		Events:      append([]core.FrameworkEvent(nil), events...),
	}
	if len(events) > 0 {
		result.NextAfterSeq = events[len(events)-1].Seq
	}
	return result, nil
}

func MarshalJSONContent(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *service) tenantRuntimeProjection(ctx context.Context, tenantID string) (nexusgateway.StateSnapshot, error) {
	if s.cfg.Materializer != nil && strings.TrimSpace(tenantID) != "" {
		return s.cfg.Materializer.StateForTenant(tenantID), nil
	}
	projection := nexusgateway.StateSnapshot{
		ChannelActivity: map[string]nexusgateway.ChannelState{},
		EventTypeCounts: map[string]uint64{},
	}
	if s.cfg.Events == nil || strings.TrimSpace(tenantID) == "" {
		return projection, nil
	}
	events, err := s.cfg.Events.Read(ctx, s.cfg.Partition, 0, 0, false)
	if err != nil {
		return nexusgateway.StateSnapshot{}, internalError("read tenant runtime projection failed", err, map[string]any{"tenant_id": tenantID})
	}
	for _, ev := range filterEventsByTenant(events, tenantID) {
		if ev.Seq > projection.LastSeq {
			projection.LastSeq = ev.Seq
		}
		projection.EventTypeCounts[ev.Type]++
		switch ev.Type {
		case core.FrameworkEventMessageInbound:
			if channelName := adminChannelFromPayload(ev.Payload); channelName != "" {
				state := projection.ChannelActivity[channelName]
				state.Inbound++
				projection.ChannelActivity[channelName] = state
			}
		case core.FrameworkEventMessageOutbound:
			if channelName := adminChannelFromPayload(ev.Payload); channelName != "" {
				state := projection.ChannelActivity[channelName]
				state.Outbound++
				projection.ChannelActivity[channelName] = state
			}
		}
	}
	return projection, nil
}

func adminChannelFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var envelope struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return envelope.Channel
}

func newAdminToken() (string, string, error) {
	var tokenBytes [24]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", "", err
	}
	var idBytes [12]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", "", err
	}
	return "tok_" + hex.EncodeToString(idBytes[:]), "nexus_" + hex.EncodeToString(tokenBytes[:]), nil
}

// HashToken returns the SHA-256 hex digest of token. Used for secure
// storage and lookup of bearer tokens without keeping the plaintext.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
