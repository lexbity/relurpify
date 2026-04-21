package admin

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/adminapi"
	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	nexusgateway "codeburg.org/lexbit/relurpify/app/nexus/gateway"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/identity"
	rexcontrolplane "codeburg.org/lexbit/relurpify/named/rex/controlplane"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
	"codeburg.org/lexbit/relurpify/relurpnet/session"
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
type UpdateNodeCapabilitiesRequest = adminapi.UpdateNodeCapabilitiesRequest
type UpdateNodeCapabilitiesResult = adminapi.UpdateNodeCapabilitiesResult
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
type GrantSessionDelegationRequest = adminapi.GrantSessionDelegationRequest
type GrantSessionDelegationResult = adminapi.GrantSessionDelegationResult
type ListSubjectsRequest = adminapi.ListSubjectsRequest
type SubjectInfo = adminapi.SubjectInfo
type ListSubjectsResult = adminapi.ListSubjectsResult
type CreateSubjectRequest = adminapi.CreateSubjectRequest
type CreateSubjectResult = adminapi.CreateSubjectResult
type BindExternalIdentityRequest = adminapi.BindExternalIdentityRequest
type BindExternalIdentityResult = adminapi.BindExternalIdentityResult
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
type FMPContinuationInfo = adminapi.FMPContinuationInfo
type ListFMPContinuationsRequest = adminapi.ListFMPContinuationsRequest
type ListFMPContinuationsResult = adminapi.ListFMPContinuationsResult
type ReadFMPContinuationAuditRequest = adminapi.ReadFMPContinuationAuditRequest
type ReadFMPContinuationAuditResult = adminapi.ReadFMPContinuationAuditResult
type VerifyFMPAuditTrailRequest = adminapi.VerifyFMPAuditTrailRequest
type VerifyFMPAuditTrailResult = adminapi.VerifyFMPAuditTrailResult
type ListFMPTrustBundlesRequest = adminapi.ListFMPTrustBundlesRequest
type ListFMPTrustBundlesResult = adminapi.ListFMPTrustBundlesResult
type UpsertFMPTrustBundleRequest = adminapi.UpsertFMPTrustBundleRequest
type UpsertFMPTrustBundleResult = adminapi.UpsertFMPTrustBundleResult
type ListFMPBoundaryPoliciesRequest = adminapi.ListFMPBoundaryPoliciesRequest
type ListFMPBoundaryPoliciesResult = adminapi.ListFMPBoundaryPoliciesResult
type SetFMPBoundaryPolicyRequest = adminapi.SetFMPBoundaryPolicyRequest
type SetFMPBoundaryPolicyResult = adminapi.SetFMPBoundaryPolicyResult
type TenantFMPExportInfo = adminapi.TenantFMPExportInfo
type ListTenantFMPExportsRequest = adminapi.ListTenantFMPExportsRequest
type ListTenantFMPExportsResult = adminapi.ListTenantFMPExportsResult
type SetTenantFMPExportRequest = adminapi.SetTenantFMPExportRequest
type SetTenantFMPExportResult = adminapi.SetTenantFMPExportResult
type TenantFMPFederationPolicyInfo = adminapi.TenantFMPFederationPolicyInfo
type GetTenantFMPFederationPolicyRequest = adminapi.GetTenantFMPFederationPolicyRequest
type GetTenantFMPFederationPolicyResult = adminapi.GetTenantFMPFederationPolicyResult
type SetTenantFMPFederationPolicyRequest = adminapi.SetTenantFMPFederationPolicyRequest
type SetTenantFMPFederationPolicyResult = adminapi.SetTenantFMPFederationPolicyResult
type EffectiveFMPFederationPolicyInfo = adminapi.EffectiveFMPFederationPolicyInfo
type GetEffectiveFMPFederationPolicyRequest = adminapi.GetEffectiveFMPFederationPolicyRequest
type GetEffectiveFMPFederationPolicyResult = adminapi.GetEffectiveFMPFederationPolicyResult
type ListTenantsRequest = adminapi.ListTenantsRequest
type TenantInfo = adminapi.TenantInfo
type ListTenantsResult = adminapi.ListTenantsResult
type GetTenantRequest = adminapi.GetTenantRequest
type GetTenantResult = adminapi.GetTenantResult
type SetTenantEnabledRequest = adminapi.SetTenantEnabledRequest
type SetTenantEnabledResult = adminapi.SetTenantEnabledResult
type NodeEnrollmentInfo = adminapi.NodeEnrollmentInfo
type ListNodeEnrollmentsRequest = adminapi.ListNodeEnrollmentsRequest
type ListNodeEnrollmentsResult = adminapi.ListNodeEnrollmentsResult
type RevokeNodeEnrollmentRequest = adminapi.RevokeNodeEnrollmentRequest
type RevokeNodeEnrollmentResult = adminapi.RevokeNodeEnrollmentResult
type ListSessionDelegationsRequest = adminapi.ListSessionDelegationsRequest
type ListSessionDelegationsResult = adminapi.ListSessionDelegationsResult
type PendingPairingInfo = adminapi.PendingPairingInfo
type SessionInfo = adminapi.SessionInfo
type DescribeRexRuntimeRequest = adminapi.DescribeRexRuntimeRequest
type DescribeRexRuntimeResult = adminapi.DescribeRexRuntimeResult
type ReadRexAdminSnapshotRequest = adminapi.ReadRexAdminSnapshotRequest
type ReadRexAdminSnapshotResult = adminapi.ReadRexAdminSnapshotResult

type RexRuntime interface {
	Registration() rexnexus.Registration
	RuntimeProjection() rexnexus.Projection
	AdminSnapshot(context.Context) (rexnexus.AdminSnapshot, error)
}

type AdminService interface {
	ListNodes(ctx context.Context, req ListNodesRequest) (ListNodesResult, error)
	GetNode(ctx context.Context, req GetNodeRequest) (GetNodeResult, error)
	UpdateNodeCapabilities(ctx context.Context, req UpdateNodeCapabilitiesRequest) (UpdateNodeCapabilitiesResult, error)
	RevokeNode(ctx context.Context, req RevokeNodeRequest) (RevokeNodeResult, error)
	ListPendingPairings(ctx context.Context, req ListPendingPairingsRequest) (ListPendingPairingsResult, error)
	ApprovePairing(ctx context.Context, req ApprovePairingRequest) (ApprovePairingResult, error)
	RejectPairing(ctx context.Context, req RejectPairingRequest) (RejectPairingResult, error)

	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
	GetSession(ctx context.Context, req GetSessionRequest) (GetSessionResult, error)
	CloseSession(ctx context.Context, req CloseSessionRequest) (CloseSessionResult, error)
	GrantSessionDelegation(ctx context.Context, req GrantSessionDelegationRequest) (GrantSessionDelegationResult, error)

	ListSubjects(ctx context.Context, req ListSubjectsRequest) (ListSubjectsResult, error)
	CreateSubject(ctx context.Context, req CreateSubjectRequest) (CreateSubjectResult, error)
	BindExternalIdentity(ctx context.Context, req BindExternalIdentityRequest) (BindExternalIdentityResult, error)
	ListExternalIdentities(ctx context.Context, req ListExternalIdentitiesRequest) (ListExternalIdentitiesResult, error)
	ListTokens(ctx context.Context, req ListTokensRequest) (ListTokensResult, error)
	IssueToken(ctx context.Context, req IssueTokenRequest) (IssueTokenResult, error)
	RevokeToken(ctx context.Context, req RevokeTokenRequest) (RevokeTokenResult, error)

	ListChannels(ctx context.Context, req ListChannelsRequest) (ListChannelsResult, error)
	RestartChannel(ctx context.Context, req RestartChannelRequest) (RestartChannelResult, error)

	ListPolicyRules(ctx context.Context, req ListPolicyRulesRequest) (ListPolicyRulesResult, error)
	SetPolicyRuleEnabled(ctx context.Context, req SetPolicyRuleEnabledRequest) (SetPolicyRuleEnabledResult, error)

	Health(ctx context.Context, req HealthRequest) (HealthResult, error)
	DescribeRexRuntime(ctx context.Context, req DescribeRexRuntimeRequest) (DescribeRexRuntimeResult, error)
	ReadRexAdminSnapshot(ctx context.Context, req ReadRexAdminSnapshotRequest) (ReadRexAdminSnapshotResult, error)
	ListEvents(ctx context.Context, req ListEventsRequest) (ListEventsResult, error)
	ReadEventStream(ctx context.Context, req ReadEventStreamRequest) (ReadEventStreamResult, error)
	ListFMPContinuations(ctx context.Context, req ListFMPContinuationsRequest) (ListFMPContinuationsResult, error)
	ReadFMPContinuationAudit(ctx context.Context, req ReadFMPContinuationAuditRequest) (ReadFMPContinuationAuditResult, error)
	VerifyFMPAuditTrail(ctx context.Context, req VerifyFMPAuditTrailRequest) (VerifyFMPAuditTrailResult, error)
	ListFMPTrustBundles(ctx context.Context, req ListFMPTrustBundlesRequest) (ListFMPTrustBundlesResult, error)
	UpsertFMPTrustBundle(ctx context.Context, req UpsertFMPTrustBundleRequest) (UpsertFMPTrustBundleResult, error)
	ListFMPBoundaryPolicies(ctx context.Context, req ListFMPBoundaryPoliciesRequest) (ListFMPBoundaryPoliciesResult, error)
	SetFMPBoundaryPolicy(ctx context.Context, req SetFMPBoundaryPolicyRequest) (SetFMPBoundaryPolicyResult, error)
	ListTenantFMPExports(ctx context.Context, req ListTenantFMPExportsRequest) (ListTenantFMPExportsResult, error)
	SetTenantFMPExport(ctx context.Context, req SetTenantFMPExportRequest) (SetTenantFMPExportResult, error)
	GetTenantFMPFederationPolicy(ctx context.Context, req GetTenantFMPFederationPolicyRequest) (GetTenantFMPFederationPolicyResult, error)
	SetTenantFMPFederationPolicy(ctx context.Context, req SetTenantFMPFederationPolicyRequest) (SetTenantFMPFederationPolicyResult, error)
	GetEffectiveFMPFederationPolicy(ctx context.Context, req GetEffectiveFMPFederationPolicyRequest) (GetEffectiveFMPFederationPolicyResult, error)

	ListTenants(ctx context.Context, req ListTenantsRequest) (ListTenantsResult, error)
	GetTenant(ctx context.Context, req GetTenantRequest) (GetTenantResult, error)
	SetTenantEnabled(ctx context.Context, req SetTenantEnabledRequest) (SetTenantEnabledResult, error)

	ListNodeEnrollments(ctx context.Context, req ListNodeEnrollmentsRequest) (ListNodeEnrollmentsResult, error)
	RevokeNodeEnrollment(ctx context.Context, req RevokeNodeEnrollmentRequest) (RevokeNodeEnrollmentResult, error)

	ListSessionDelegations(ctx context.Context, req ListSessionDelegationsRequest) (ListSessionDelegationsResult, error)

	// Phase 6.4: Compatibility Window Management
	ListFMPCompatibilityWindows(ctx context.Context, req ListFMPCompatibilityWindowsRequest) (ListFMPCompatibilityWindowsResult, error)
	SetFMPCompatibilityWindow(ctx context.Context, req SetFMPCompatibilityWindowRequest) (SetFMPCompatibilityWindowResult, error)
	DeleteFMPCompatibilityWindow(ctx context.Context, req DeleteFMPCompatibilityWindowRequest) (DeleteFMPCompatibilityWindowResult, error)

	// Phase 6.5: Circuit Breaker Management
	ListFMPCircuitBreakers(ctx context.Context, req ListFMPCircuitBreakersRequest) (ListFMPCircuitBreakersResult, error)
	SetFMPCircuitBreakerConfig(ctx context.Context, req SetFMPCircuitBreakerConfigRequest) (SetFMPCircuitBreakerConfigResult, error)
	ResetFMPCircuitBreaker(ctx context.Context, req ResetFMPCircuitBreakerRequest) (ResetFMPCircuitBreakerResult, error)

	// Phase 7.2: SLO Signals
	ReadRexSLOSignals(ctx context.Context, req ReadRexSLOSignalsRequest) (ReadRexSLOSignalsResult, error)
}

type ServiceConfig struct {
	Nodes         fwnode.NodeStore
	NodeManager   *fwnode.Manager
	Sessions      session.Store
	Identities    identity.Store
	Tokens        TokenStore
	Policies      PolicyRuleStore
	FMPExports    TenantFMPExportStore
	FMPFederation TenantFMPFederationPolicyStore
	Events        event.Log
	Materializer  *nexusgateway.StateMaterializer
	Channels      *channel.Manager
	FMP           *fwfmp.Service
	Partition     string
	Config        nexuscfg.Config
	StartedAt     time.Time
	PolicyEngine  authorization.PolicyEngine
	RexRuntime    RexRuntime
	// Phase 7.2: Rex runtime for SLO signals
	RexProvider RexSLOProvider
}

type RexSLOProvider interface {
	ReadSLOSignals(context.Context) (rexcontrolplane.SLOSignals, int64, error)
}

type TokenStore interface {
	ListTokens(ctx context.Context) ([]core.AdminTokenRecord, error)
	GetToken(ctx context.Context, id string) (*core.AdminTokenRecord, error)
	GetTokenByHash(ctx context.Context, tokenHash string) (*core.AdminTokenRecord, error)
	CreateToken(ctx context.Context, record core.AdminTokenRecord) error
	RevokeToken(ctx context.Context, id string, revokedAt time.Time) error
}

type PolicyRuleStore interface {
	ListRules(ctx context.Context) ([]core.PolicyRule, error)
	SetRuleEnabled(ctx context.Context, ruleID string, enabled bool) error
}

type TenantFMPExportStore interface {
	ListTenantExports(ctx context.Context, tenantID string) ([]TenantFMPExportInfo, error)
	SetTenantExportEnabled(ctx context.Context, tenantID, exportName string, enabled bool) error
	IsExportEnabled(ctx context.Context, tenantID, exportName string) (bool, bool, error)
}

type TenantFMPFederationPolicyStore interface {
	GetTenantFederationPolicy(ctx context.Context, tenantID string) (*core.TenantFederationPolicy, error)
	SetTenantFederationPolicy(ctx context.Context, policy core.TenantFederationPolicy) error
}

// Type aliases for Phase 6.4 and 6.5 admin API types
type ListFMPCompatibilityWindowsRequest = adminapi.ListFMPCompatibilityWindowsRequest
type ListFMPCompatibilityWindowsResult = adminapi.ListFMPCompatibilityWindowsResult
type FMPCompatibilityWindow = adminapi.FMPCompatibilityWindow
type SetFMPCompatibilityWindowRequest = adminapi.SetFMPCompatibilityWindowRequest
type SetFMPCompatibilityWindowResult = adminapi.SetFMPCompatibilityWindowResult
type DeleteFMPCompatibilityWindowRequest = adminapi.DeleteFMPCompatibilityWindowRequest
type DeleteFMPCompatibilityWindowResult = adminapi.DeleteFMPCompatibilityWindowResult

type ListFMPCircuitBreakersRequest = adminapi.ListFMPCircuitBreakersRequest
type ListFMPCircuitBreakersResult = adminapi.ListFMPCircuitBreakersResult
type FMPCircuitBreakerStatus = adminapi.FMPCircuitBreakerStatus
type SetFMPCircuitBreakerConfigRequest = adminapi.SetFMPCircuitBreakerConfigRequest
type SetFMPCircuitBreakerConfigResult = adminapi.SetFMPCircuitBreakerConfigResult
type ResetFMPCircuitBreakerRequest = adminapi.ResetFMPCircuitBreakerRequest
type ResetFMPCircuitBreakerResult = adminapi.ResetFMPCircuitBreakerResult

type ReadRexSLOSignalsRequest = adminapi.ReadRexSLOSignalsRequest
type ReadRexSLOSignalsResult = adminapi.ReadRexSLOSignalsResult
