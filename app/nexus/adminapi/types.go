package adminapi

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	rexnexus "github.com/lexcodex/relurpify/named/rex/nexus"
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
	TenantID    string           `json:"tenant_id"`
	Kind        core.SubjectKind `json:"kind"`
	ID          string           `json:"id"`
	DisplayName string           `json:"display_name,omitempty"`
	Roles       []string         `json:"roles,omitempty"`
}

type TokenInfo struct {
	ID          string           `json:"id"`
	Name        string           `json:"name,omitempty"`
	TenantID    string           `json:"tenant_id,omitempty"`
	SubjectKind core.SubjectKind `json:"subject_kind,omitempty"`
	SubjectID   string           `json:"subject_id,omitempty"`
	Scope       []string         `json:"scope,omitempty"`
	IssuedAt    time.Time        `json:"issued_at"`
	ExpiresAt   *time.Time       `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time       `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time       `json:"revoked_at,omitempty"`
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

type FMPContinuationInfo struct {
	LineageID           string                `json:"lineage_id"`
	TenantID            string                `json:"tenant_id"`
	TaskClass           string                `json:"task_class"`
	ContextClass        string                `json:"context_class"`
	Owner               core.SubjectRef       `json:"owner"`
	SessionID           string                `json:"session_id,omitempty"`
	TrustClass          core.TrustClass       `json:"trust_class,omitempty"`
	CurrentOwnerAttempt string                `json:"current_owner_attempt,omitempty"`
	CurrentOwnerRuntime string                `json:"current_owner_runtime,omitempty"`
	LineageVersion      int64                 `json:"lineage_version,omitempty"`
	UpdatedAt           time.Time             `json:"updated_at,omitempty"`
	SensitivityClass    core.SensitivityClass `json:"sensitivity_class,omitempty"`
}

type HealthResult struct {
	AdminResult
	Online            bool                  `json:"online"`
	PID               int                   `json:"pid"`
	BindAddr          string                `json:"bind_addr"`
	UptimeSeconds     int64                 `json:"uptime_seconds"`
	TenantID          string                `json:"tenant_id"`
	LastSeq           uint64                `json:"last_seq"`
	PairedNodes       []core.NodeDescriptor `json:"paired_nodes"`
	PendingPairings   []PendingPairingInfo  `json:"pending_pairings"`
	Channels          []ChannelInfo         `json:"channels"`
	ActiveSessions    []SessionInfo         `json:"active_sessions"`
	SecurityWarnings  []string              `json:"security_warnings"`
	ReadinessWarnings []string              `json:"readiness_warnings,omitempty"`
	EventCounts       map[string]uint64     `json:"event_counts"`
	RexRuntime        *rexnexus.Projection  `json:"rex_runtime,omitempty"`
}

type DescribeRexRuntimeRequest struct{ AdminRequest }

type DescribeRexRuntimeResult struct {
	AdminResult
	Registration rexnexus.Registration `json:"registration"`
	Runtime      rexnexus.Projection   `json:"runtime"`
}

type ReadRexAdminSnapshotRequest struct{ AdminRequest }

type ReadRexAdminSnapshotResult struct {
	AdminResult
	Snapshot rexnexus.AdminSnapshot `json:"snapshot"`
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

type UpdateNodeCapabilitiesRequest struct {
	AdminRequest
	NodeID       string                      `json:"node_id"`
	Capabilities []core.CapabilityDescriptor `json:"capabilities,omitempty"`
}

type UpdateNodeCapabilitiesResult struct {
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

type GrantSessionDelegationRequest struct {
	AdminRequest
	SessionID   string                  `json:"session_id"`
	SubjectKind core.SubjectKind        `json:"subject_kind"`
	SubjectID   string                  `json:"subject_id"`
	Operations  []core.SessionOperation `json:"operations,omitempty"`
	ExpiresAt   *time.Time              `json:"expires_at,omitempty"`
}

type GrantSessionDelegationResult struct {
	AdminResult
	Delegation core.SessionDelegationRecord `json:"delegation"`
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

type CreateSubjectRequest struct {
	AdminRequest
	SubjectTenantID string           `json:"subject_tenant_id,omitempty"`
	SubjectKind     core.SubjectKind `json:"subject_kind"`
	SubjectID       string           `json:"subject_id"`
	DisplayName     string           `json:"display_name,omitempty"`
	Roles           []string         `json:"roles,omitempty"`
}

type CreateSubjectResult struct {
	AdminResult
	Subject SubjectInfo `json:"subject"`
}

type BindExternalIdentityRequest struct {
	AdminRequest
	SubjectTenantID string                `json:"subject_tenant_id,omitempty"`
	Provider        core.ExternalProvider `json:"provider"`
	AccountID       string                `json:"account_id,omitempty"`
	ExternalID      string                `json:"external_id"`
	SubjectKind     core.SubjectKind      `json:"subject_kind"`
	SubjectID       string                `json:"subject_id"`
	DisplayName     string                `json:"display_name,omitempty"`
	ProviderLabel   string                `json:"provider_label,omitempty"`
}

type BindExternalIdentityResult struct {
	AdminResult
	Identity core.ExternalIdentity `json:"identity"`
}

type ListExternalIdentitiesRequest struct {
	AdminRequest
	Page        PageRequest      `json:"page,omitempty"`
	SubjectKind core.SubjectKind `json:"subject_kind,omitempty"`
	SubjectID   string           `json:"subject_id,omitempty"`
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
	SubjectTenantID string           `json:"subject_tenant_id,omitempty"`
	SubjectKind     core.SubjectKind `json:"subject_kind,omitempty"`
	SubjectID       string           `json:"subject_id"`
	Scopes          []string         `json:"scopes,omitempty"`
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

type ListFMPContinuationsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListFMPContinuationsResult struct {
	AdminResult
	PageResult
	Continuations []FMPContinuationInfo `json:"continuations"`
}

type ReadFMPContinuationAuditRequest struct {
	AdminRequest
	LineageID string `json:"lineage_id"`
	Limit     int    `json:"limit,omitempty"`
}

type ReadFMPContinuationAuditResult struct {
	AdminResult
	Lineage      *FMPContinuationInfo         `json:"lineage,omitempty"`
	Events       []core.FrameworkEvent        `json:"events"`
	AuditChain   []core.AuditChainEntry       `json:"audit_chain,omitempty"`
	Verification *core.AuditChainVerification `json:"verification,omitempty"`
}

type VerifyFMPAuditTrailRequest struct {
	AdminRequest
	LineageID string `json:"lineage_id"`
	Limit     int    `json:"limit,omitempty"`
}

type VerifyFMPAuditTrailResult struct {
	AdminResult
	Lineage      *FMPContinuationInfo        `json:"lineage,omitempty"`
	Verification core.AuditChainVerification `json:"verification"`
}

type ListFMPTrustBundlesRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListFMPTrustBundlesResult struct {
	AdminResult
	PageResult
	Bundles []core.TrustBundle `json:"bundles"`
}

type UpsertFMPTrustBundleRequest struct {
	AdminRequest
	Bundle core.TrustBundle `json:"bundle"`
}

type UpsertFMPTrustBundleResult struct {
	AdminResult
	Bundle core.TrustBundle `json:"bundle"`
}

type ListFMPBoundaryPoliciesRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListFMPBoundaryPoliciesResult struct {
	AdminResult
	PageResult
	Policies []core.BoundaryPolicy `json:"policies"`
}

type SetFMPBoundaryPolicyRequest struct {
	AdminRequest
	Policy core.BoundaryPolicy `json:"policy"`
}

type SetFMPBoundaryPolicyResult struct {
	AdminResult
	Policy core.BoundaryPolicy `json:"policy"`
}

type TenantFMPExportInfo struct {
	TenantID   string    `json:"tenant_id"`
	ExportName string    `json:"export_name"`
	Enabled    bool      `json:"enabled"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type ListTenantFMPExportsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListTenantFMPExportsResult struct {
	AdminResult
	PageResult
	Exports []TenantFMPExportInfo `json:"exports"`
}

type SetTenantFMPExportRequest struct {
	AdminRequest
	ExportName string `json:"export_name"`
	Enabled    bool   `json:"enabled"`
}

type SetTenantFMPExportResult struct {
	AdminResult
	Export TenantFMPExportInfo `json:"export"`
}

type TenantFMPFederationPolicyInfo struct {
	TenantID            string    `json:"tenant_id"`
	AllowedTrustDomains []string  `json:"allowed_trust_domains,omitempty"`
	AllowedRouteModes   []string  `json:"allowed_route_modes,omitempty"`
	AllowMediation      bool      `json:"allow_mediation,omitempty"`
	MaxTransferBytes    int64     `json:"max_transfer_bytes,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

type GetTenantFMPFederationPolicyRequest struct {
	AdminRequest
}

type GetTenantFMPFederationPolicyResult struct {
	AdminResult
	Policy TenantFMPFederationPolicyInfo `json:"policy"`
}

type SetTenantFMPFederationPolicyRequest struct {
	AdminRequest
	AllowedTrustDomains []string `json:"allowed_trust_domains,omitempty"`
	AllowedRouteModes   []string `json:"allowed_route_modes,omitempty"`
	AllowMediation      bool     `json:"allow_mediation,omitempty"`
	MaxTransferBytes    int64    `json:"max_transfer_bytes,omitempty"`
}

type SetTenantFMPFederationPolicyResult struct {
	AdminResult
	Policy TenantFMPFederationPolicyInfo `json:"policy"`
}

type EffectiveFMPFederationPolicyInfo struct {
	TenantID                 string                        `json:"tenant_id"`
	TrustDomain              string                        `json:"trust_domain"`
	TenantPolicy             TenantFMPFederationPolicyInfo `json:"tenant_policy"`
	TrustBundlePresent       bool                          `json:"trust_bundle_present"`
	TrustBundle              *core.TrustBundle             `json:"trust_bundle,omitempty"`
	BoundaryPolicyPresent    bool                          `json:"boundary_policy_present"`
	BoundaryPolicy           *core.BoundaryPolicy          `json:"boundary_policy,omitempty"`
	AllowedTrustDomain       bool                          `json:"allowed_trust_domain"`
	AllowedRouteModes        []string                      `json:"allowed_route_modes,omitempty"`
	AllowMediation           bool                          `json:"allow_mediation"`
	MaxTransferBytes         int64                         `json:"max_transfer_bytes,omitempty"`
	RequireGatewayAuth       bool                          `json:"require_gateway_authentication,omitempty"`
	AcceptedSourceDomains    []string                      `json:"accepted_source_domains,omitempty"`
	AcceptedSourceIdentities []core.SubjectRef             `json:"accepted_source_identities,omitempty"`
}

type GetEffectiveFMPFederationPolicyRequest struct {
	AdminRequest
	TrustDomain string `json:"trust_domain"`
}

type GetEffectiveFMPFederationPolicyResult struct {
	AdminResult
	Policy EffectiveFMPFederationPolicyInfo `json:"policy"`
}

type ListTenantsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type TenantInfo struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"display_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
}

type ListTenantsResult struct {
	AdminResult
	PageResult
	Tenants []TenantInfo `json:"tenants"`
}

type GetTenantRequest struct {
	AdminRequest
	TenantLookupID string `json:"tenant_lookup_id"`
}

type GetTenantResult struct {
	AdminResult
	Tenant *TenantInfo `json:"tenant,omitempty"`
}

type SetTenantEnabledRequest struct {
	AdminRequest
	TenantLookupID string `json:"tenant_lookup_id"`
	Enabled        bool   `json:"enabled"`
}

type SetTenantEnabledResult struct {
	AdminResult
	TenantLookupID string `json:"tenant_lookup_id"`
	Enabled        bool   `json:"enabled"`
}

type NodeEnrollmentInfo struct {
	TenantID       string          `json:"tenant_id"`
	NodeID         string          `json:"node_id"`
	Owner          core.SubjectRef `json:"owner"`
	TrustClass     core.TrustClass `json:"trust_class"`
	KeyID          string          `json:"key_id,omitempty"`
	PairedAt       time.Time       `json:"paired_at"`
	LastVerifiedAt time.Time       `json:"last_verified_at,omitempty"`
	AuthMethod     core.AuthMethod `json:"auth_method,omitempty"`
}

type ListNodeEnrollmentsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListNodeEnrollmentsResult struct {
	AdminResult
	PageResult
	Enrollments []NodeEnrollmentInfo `json:"enrollments"`
}

type RevokeNodeEnrollmentRequest struct {
	AdminRequest
	NodeID string `json:"node_id"`
}

type RevokeNodeEnrollmentResult struct {
	AdminResult
	NodeID string `json:"node_id"`
}

type ListSessionDelegationsRequest struct {
	AdminRequest
	Page PageRequest `json:"page,omitempty"`
}

type ListSessionDelegationsResult struct {
	AdminResult
	PageResult
	Delegations []core.SessionDelegationRecord `json:"delegations"`
}

// Phase 6.4: Compatibility Window Management

type ListFMPCompatibilityWindowsRequest struct {
	AdminRequest
}

type ListFMPCompatibilityWindowsResult struct {
	AdminResult
	Windows []FMPCompatibilityWindow `json:"windows"`
}

type FMPCompatibilityWindow struct {
	ContextClass      string `json:"context_class"`
	MinSchemaVersion  string `json:"min_schema_version,omitempty"`
	MaxSchemaVersion  string `json:"max_schema_version,omitempty"`
	MinRuntimeVersion string `json:"min_runtime_version,omitempty"`
	MaxRuntimeVersion string `json:"max_runtime_version,omitempty"`
}

type SetFMPCompatibilityWindowRequest struct {
	AdminRequest
	Window FMPCompatibilityWindow `json:"window"`
}

type SetFMPCompatibilityWindowResult struct {
	AdminResult
	Window FMPCompatibilityWindow `json:"window"`
}

type DeleteFMPCompatibilityWindowRequest struct {
	AdminRequest
	ContextClass string `json:"context_class"`
}

type DeleteFMPCompatibilityWindowResult struct {
	AdminResult
	ContextClass string `json:"context_class"`
}

// Phase 6.5: Circuit Breaker Management

type ListFMPCircuitBreakersRequest struct {
	AdminRequest
}

type ListFMPCircuitBreakersResult struct {
	AdminResult
	CircuitBreakers []FMPCircuitBreakerStatus `json:"circuit_breakers"`
}

type FMPCircuitBreakerStatus struct {
	TrustDomain string  `json:"trust_domain"`
	State       string  `json:"state"`
	ErrorRate   float64 `json:"error_rate"`
	Requests    int     `json:"requests"`
	TrippedAt   *int64  `json:"tripped_at,omitempty"`  // Unix timestamp in ns
	RecoveryAt  *int64  `json:"recovery_at,omitempty"` // Unix timestamp in ns
}

type SetFMPCircuitBreakerConfigRequest struct {
	AdminRequest
	TrustDomain         string  `json:"trust_domain"`
	ErrorThreshold      float64 `json:"error_threshold"`
	MinRequests         int     `json:"min_requests"`
	WindowDurationSec   int     `json:"window_duration_sec"`
	RecoveryDurationSec int     `json:"recovery_duration_sec"`
}

type SetFMPCircuitBreakerConfigResult struct {
	AdminResult
	TrustDomain string `json:"trust_domain"`
}

type ResetFMPCircuitBreakerRequest struct {
	AdminRequest
	TrustDomain string `json:"trust_domain"`
}

type ResetFMPCircuitBreakerResult struct {
	AdminResult
	TrustDomain string `json:"trust_domain"`
}

// Phase 7.2: SLO Signals

type ReadRexSLOSignalsRequest struct {
	AdminRequest
}

type ReadRexSLOSignalsResult struct {
	AdminResult
	TotalWorkflows     int      `json:"total_workflows"`
	RunningWorkflows   int      `json:"running_workflows"`
	CompletedWorkflows int      `json:"completed_workflows"`
	FailedWorkflows    int      `json:"failed_workflows"`
	RecoverySensitive  int      `json:"recovery_sensitive"`
	DegradedWorkflows  []string `json:"degraded_workflow_ids,omitempty"`
	CachedAt           int64    `json:"cached_at"` // Unix timestamp in ns
}
