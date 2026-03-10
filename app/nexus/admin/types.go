package admin

import (
	"context"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/adminapi"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusgateway "github.com/lexcodex/relurpify/app/nexus/gateway"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/lexcodex/relurpify/framework/middleware/session"
)

const APIVersionV1Alpha1 = adminapi.APIVersionV1Alpha1

type AdminErrorCode string

const (
	AdminErrorUnauthorized    AdminErrorCode = "unauthorized"
	AdminErrorNotFound        AdminErrorCode = "not_found"
	AdminErrorConflict        AdminErrorCode = "conflict"
	AdminErrorPolicyDenied    AdminErrorCode = "policy_denied"
	AdminErrorInvalidArgument AdminErrorCode = "invalid_argument"
	AdminErrorInternal        AdminErrorCode = "internal"
	AdminErrorNotImplemented  AdminErrorCode = "not_implemented"
)

type AdminError struct {
	Code    AdminErrorCode `json:"code"`
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
}

func (e AdminError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return string(e.Code)
}

type PageRequest = adminapi.PageRequest
type PageResult = adminapi.PageResult
type AdminRequest = adminapi.AdminRequest
type AdminResult = adminapi.AdminResult
type HealthRequest = adminapi.HealthRequest
type HealthResult = adminapi.HealthResult
type ListNodesRequest = adminapi.ListNodesRequest
type ListNodesResult = adminapi.ListNodesResult
type GetNodeRequest = adminapi.GetNodeRequest
type GetNodeResult = adminapi.GetNodeResult
type RevokeNodeRequest = adminapi.RevokeNodeRequest
type RevokeNodeResult = adminapi.RevokeNodeResult
type ListPendingPairingsRequest = adminapi.ListPendingPairingsRequest
type ListPendingPairingsResult = adminapi.ListPendingPairingsResult
type ApprovePairingRequest = adminapi.ApprovePairingRequest
type ApprovePairingResult = adminapi.ApprovePairingResult
type RejectPairingRequest = adminapi.RejectPairingRequest
type RejectPairingResult = adminapi.RejectPairingResult
type ListSessionsRequest = adminapi.ListSessionsRequest
type ListSessionsResult = adminapi.ListSessionsResult
type GetSessionRequest = adminapi.GetSessionRequest
type GetSessionResult = adminapi.GetSessionResult
type CloseSessionRequest = adminapi.CloseSessionRequest
type CloseSessionResult = adminapi.CloseSessionResult
type ListSubjectsRequest = adminapi.ListSubjectsRequest
type SubjectInfo = adminapi.SubjectInfo
type ListSubjectsResult = adminapi.ListSubjectsResult
type ListExternalIdentitiesRequest = adminapi.ListExternalIdentitiesRequest
type ListExternalIdentitiesResult = adminapi.ListExternalIdentitiesResult
type ListTokensRequest = adminapi.ListTokensRequest
type TokenInfo = adminapi.TokenInfo
type ListTokensResult = adminapi.ListTokensResult
type IssueTokenRequest = adminapi.IssueTokenRequest
type IssueTokenResult = adminapi.IssueTokenResult
type RevokeTokenRequest = adminapi.RevokeTokenRequest
type RevokeTokenResult = adminapi.RevokeTokenResult
type ListChannelsRequest = adminapi.ListChannelsRequest
type ChannelInfo = adminapi.ChannelInfo
type ListChannelsResult = adminapi.ListChannelsResult
type RestartChannelRequest = adminapi.RestartChannelRequest
type RestartChannelResult = adminapi.RestartChannelResult
type ListPolicyRulesRequest = adminapi.ListPolicyRulesRequest
type ListPolicyRulesResult = adminapi.ListPolicyRulesResult
type SetPolicyRuleEnabledRequest = adminapi.SetPolicyRuleEnabledRequest
type SetPolicyRuleEnabledResult = adminapi.SetPolicyRuleEnabledResult
type ListEventsRequest = adminapi.ListEventsRequest
type EventInfo = adminapi.EventInfo
type ListEventsResult = adminapi.ListEventsResult
type ReadEventStreamRequest = adminapi.ReadEventStreamRequest
type ReadEventStreamResult = adminapi.ReadEventStreamResult
type ListTenantsRequest = adminapi.ListTenantsRequest
type ListTenantsResult = adminapi.ListTenantsResult
type PendingPairingInfo = adminapi.PendingPairingInfo
type SessionInfo = adminapi.SessionInfo

type AdminService interface {
	ListNodes(ctx context.Context, req ListNodesRequest) (ListNodesResult, error)
	GetNode(ctx context.Context, req GetNodeRequest) (GetNodeResult, error)
	RevokeNode(ctx context.Context, req RevokeNodeRequest) (RevokeNodeResult, error)
	ListPendingPairings(ctx context.Context, req ListPendingPairingsRequest) (ListPendingPairingsResult, error)
	ApprovePairing(ctx context.Context, req ApprovePairingRequest) (ApprovePairingResult, error)
	RejectPairing(ctx context.Context, req RejectPairingRequest) (RejectPairingResult, error)

	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
	GetSession(ctx context.Context, req GetSessionRequest) (GetSessionResult, error)
	CloseSession(ctx context.Context, req CloseSessionRequest) (CloseSessionResult, error)

	ListSubjects(ctx context.Context, req ListSubjectsRequest) (ListSubjectsResult, error)
	ListExternalIdentities(ctx context.Context, req ListExternalIdentitiesRequest) (ListExternalIdentitiesResult, error)
	ListTokens(ctx context.Context, req ListTokensRequest) (ListTokensResult, error)
	IssueToken(ctx context.Context, req IssueTokenRequest) (IssueTokenResult, error)
	RevokeToken(ctx context.Context, req RevokeTokenRequest) (RevokeTokenResult, error)

	ListChannels(ctx context.Context, req ListChannelsRequest) (ListChannelsResult, error)
	RestartChannel(ctx context.Context, req RestartChannelRequest) (RestartChannelResult, error)

	ListPolicyRules(ctx context.Context, req ListPolicyRulesRequest) (ListPolicyRulesResult, error)
	SetPolicyRuleEnabled(ctx context.Context, req SetPolicyRuleEnabledRequest) (SetPolicyRuleEnabledResult, error)

	Health(ctx context.Context, req HealthRequest) (HealthResult, error)
	ListEvents(ctx context.Context, req ListEventsRequest) (ListEventsResult, error)
	ReadEventStream(ctx context.Context, req ReadEventStreamRequest) (ReadEventStreamResult, error)

	ListTenants(ctx context.Context, req ListTenantsRequest) (ListTenantsResult, error)
}

type ServiceConfig struct {
	Nodes        fwnode.NodeStore
	NodeManager  *fwnode.Manager
	Sessions     session.Store
	Identities   identity.Store
	Tokens       TokenStore
	Policies     PolicyRuleStore
	Events       event.Log
	Materializer *nexusgateway.StateMaterializer
	Channels     *channel.Manager
	Partition    string
	Config       nexuscfg.Config
	StartedAt    time.Time
	PolicyEngine authorization.PolicyEngine
}

type TokenStore interface {
	ListTokens(ctx context.Context) ([]core.AdminTokenRecord, error)
	GetToken(ctx context.Context, id string) (*core.AdminTokenRecord, error)
	CreateToken(ctx context.Context, record core.AdminTokenRecord) error
	RevokeToken(ctx context.Context, id string, revokedAt time.Time) error
}

type PolicyRuleStore interface {
	ListRules(ctx context.Context) ([]core.PolicyRule, error)
	SetRuleEnabled(ctx context.Context, ruleID string, enabled bool) error
}
