package adminapi

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const APIVersionV1Alpha1 = "v1alpha1"

type PageRequest struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type PageResult struct {
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total"`
}

type AdminRequest struct {
	APIVersion string
	Principal  core.AuthenticatedPrincipal
	TenantID   string
	RequestID  string
}

type AdminResult struct {
	APIVersion string `json:"api_version"`
	RequestID  string `json:"request_id,omitempty"`
}

type HealthRequest struct{ AdminRequest }

type PendingPairingInfo struct {
	Code      string    `json:"code"`
	DeviceID  string    `json:"device_id"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionInfo struct {
	ID   string `json:"id"`
	Role string `json:"role,omitempty"`
}

type SubjectInfo struct {
	TenantID string           `json:"tenant_id"`
	Kind     core.SubjectKind `json:"kind"`
	ID       string           `json:"id"`
}

type TokenInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name,omitempty"`
	SubjectID  string     `json:"subject_id,omitempty"`
	Scope      []string   `json:"scope,omitempty"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type ChannelInfo struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	Reconnects int    `json:"reconnects,omitempty"`
	Inbound    uint64 `json:"inbound"`
	Outbound   uint64 `json:"outbound"`
}

type EventInfo struct {
	Type  string `json:"type"`
	Count uint64 `json:"count"`
}

type HealthResult struct {
	AdminResult
	Online           bool                  `json:"online"`
	PID              int                   `json:"pid"`
	BindAddr         string                `json:"bind_addr"`
	UptimeSeconds    int64                 `json:"uptime_seconds"`
	TenantID         string                `json:"tenant_id"`
	LastSeq          uint64                `json:"last_seq"`
	PairedNodes      []core.NodeDescriptor `json:"paired_nodes"`
	PendingPairings  []PendingPairingInfo  `json:"pending_pairings"`
	Channels         []ChannelInfo         `json:"channels"`
	ActiveSessions   []SessionInfo         `json:"active_sessions"`
	SecurityWarnings []string              `json:"security_warnings"`
	EventCounts      map[string]uint64     `json:"event_counts"`
}

type ListNodesRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListNodesResult struct {
	AdminResult
	PageResult
	Nodes []core.NodeDescriptor `json:"nodes"`
}

type GetNodeRequest struct {
	AdminRequest
	NodeID string `json:"node_id"`
}

type GetNodeResult struct {
	AdminResult
	Node *core.NodeDescriptor `json:"node,omitempty"`
}

type RevokeNodeRequest struct {
	AdminRequest
	NodeID string `json:"node_id"`
}

type RevokeNodeResult struct {
	AdminResult
	NodeID string `json:"node_id"`
}

type ListPendingPairingsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListPendingPairingsResult struct {
	AdminResult
	PageResult
	Pairings []PendingPairingInfo `json:"pairings"`
}

type ApprovePairingRequest struct {
	AdminRequest
	Code string `json:"code"`
}

type ApprovePairingResult struct {
	AdminResult
	NodeID   string    `json:"node_id"`
	PairedAt time.Time `json:"paired_at"`
}

type RejectPairingRequest struct {
	AdminRequest
	Code string `json:"code"`
}

type RejectPairingResult struct {
	AdminResult
	Code string `json:"code"`
}

type ListSessionsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListSessionsResult struct {
	AdminResult
	PageResult
	Sessions []core.SessionBoundary `json:"sessions"`
}

type GetSessionRequest struct {
	AdminRequest
	SessionID string `json:"session_id"`
}

type GetSessionResult struct {
	AdminResult
	Session *core.SessionBoundary `json:"session,omitempty"`
}

type CloseSessionRequest struct {
	AdminRequest
	SessionID string `json:"session_id"`
}

type CloseSessionResult struct {
	AdminResult
	SessionID string `json:"session_id"`
}

type ListSubjectsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListSubjectsResult struct {
	AdminResult
	PageResult
	Subjects []SubjectInfo `json:"subjects"`
}

type ListExternalIdentitiesRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListExternalIdentitiesResult struct {
	AdminResult
	PageResult
	Identities []core.ExternalIdentity `json:"identities"`
}

type ListTokensRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListTokensResult struct {
	AdminResult
	PageResult
	Tokens []TokenInfo `json:"tokens"`
}

type IssueTokenRequest struct {
	AdminRequest
	SubjectID string   `json:"subject_id"`
	Scopes    []string `json:"scopes,omitempty"`
}

type IssueTokenResult struct {
	AdminResult
	TokenID string `json:"token_id"`
	Token   string `json:"token,omitempty"`
}

type RevokeTokenRequest struct {
	AdminRequest
	TokenID string `json:"token_id"`
}

type RevokeTokenResult struct {
	AdminResult
	TokenID string `json:"token_id"`
}

type ListChannelsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListChannelsResult struct {
	AdminResult
	PageResult
	Channels []ChannelInfo `json:"channels"`
}

type RestartChannelRequest struct {
	AdminRequest
	Channel string `json:"channel"`
}

type RestartChannelResult struct {
	AdminResult
	Channel string `json:"channel"`
}

type ListPolicyRulesRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListPolicyRulesResult struct {
	AdminResult
	PageResult
	Rules []core.PolicyRule `json:"rules"`
}

type SetPolicyRuleEnabledRequest struct {
	AdminRequest
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"`
}

type SetPolicyRuleEnabledResult struct {
	AdminResult
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"`
}

type ListEventsRequest struct {
	AdminRequest
	PageRequest
}

type ListEventsResult struct {
	AdminResult
	PageResult
	Events []EventInfo `json:"events"`
}

type ReadEventStreamRequest struct {
	AdminRequest
	AfterSeq uint64 `json:"after_seq,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type ReadEventStreamResult struct {
	AdminResult
	AfterSeq     uint64                `json:"after_seq,omitempty"`
	NextAfterSeq uint64                `json:"next_after_seq,omitempty"`
	Events       []core.FrameworkEvent `json:"events"`
}

type ListTenantsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListTenantsResult struct {
	AdminResult
	PageResult
	Tenants []string `json:"tenants"`
}
