package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusgateway "github.com/lexcodex/relurpify/app/nexus/gateway"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
)

func (s *service) ListChannels(_ context.Context, req ListChannelsRequest) (ListChannelsResult, error) {
	state := nexusgateway.StateSnapshot{}
	if s.cfg.Materializer != nil {
		state = s.cfg.Materializer.State()
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
		activity := state.ChannelActivity[name]
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
	state := nexusgateway.StateSnapshot{}
	if s.cfg.Materializer != nil {
		state = s.cfg.Materializer.State()
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
		TenantID:         defaultTenant(req.TenantID),
		LastSeq:          state.LastSeq,
		PairedNodes:      nodes.Nodes,
		PendingPairings:  pairings.Pairings,
		Channels:         channelResult.Channels,
		ActiveSessions:   activeSessions,
		SecurityWarnings: buildSecurityWarnings(s.cfg.Config, len(pairings.Pairings)),
		EventCounts:      copyEventCounts(state.EventTypeCounts),
	}, nil
}

func (s *service) ListEvents(ctx context.Context, req ListEventsRequest) (ListEventsResult, error) {
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
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	events, err := s.cfg.Events.Read(ctx, s.cfg.Partition, req.AfterSeq, limit, false)
	if err != nil {
		return ReadEventStreamResult{}, internalError("read event stream failed", err, map[string]any{"after_seq": req.AfterSeq, "limit": limit})
	}
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

func buildSecurityWarnings(cfg nexuscfg.Config, pendingPairings int) []string {
	var warnings []string
	if bind := strings.TrimSpace(cfg.Gateway.Bind); bind != "" && !isLoopbackBind(bind) {
		warnings = append(warnings, fmt.Sprintf("Gateway bind %q is not loopback-only.", bind))
	}
	if cfg.Nodes.AutoApproveLocal {
		warnings = append(warnings, "Local node auto-approval is enabled.")
	}
	if pendingPairings > 0 {
		warnings = append(warnings, fmt.Sprintf("%d node pairing request(s) are pending approval.", pendingPairings))
	}
	if len(cfg.Channels) == 0 {
		warnings = append(warnings, "No channels are configured; gateway surface may be incomplete.")
	}
	return warnings
}

func isLoopbackBind(bind string) bool {
	switch {
	case bind == "":
		return true
	case strings.HasPrefix(bind, ":"):
		return true
	case strings.HasPrefix(bind, "127.0.0.1:"):
		return true
	case strings.HasPrefix(bind, "localhost:"):
		return true
	case strings.HasPrefix(bind, "[::1]:"):
		return true
	default:
		return false
	}
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

func hashAdminToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
