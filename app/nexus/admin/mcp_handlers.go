package admin

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
)

type listPendingPairingsArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type approvePairingArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Code       string `json:"code"`
}

type rejectPairingArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Code       string `json:"code"`
}

type listNodesArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type getNodeArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	NodeID     string `json:"node_id"`
}

type listEventsArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type listFMPContinuationsArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type readFMPContinuationAuditArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	LineageID  string `json:"lineage_id"`
	Limit      int    `json:"limit,omitempty"`
}

type verifyFMPAuditTrailArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	LineageID  string `json:"lineage_id"`
	Limit      int    `json:"limit,omitempty"`
}

type listFMPTrustBundlesArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type upsertFMPTrustBundleArgs struct {
	APIVersion string            `json:"api_version,omitempty"`
	Bundle     fwfmp.TrustBundle `json:"bundle"`
}

type listFMPBoundaryPoliciesArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type setFMPBoundaryPolicyArgs struct {
	APIVersion string               `json:"api_version,omitempty"`
	Policy     fwfmp.BoundaryPolicy `json:"policy"`
}

type listTenantFMPExportsArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type setTenantFMPExportArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	ExportName string `json:"export_name"`
	Enabled    bool   `json:"enabled"`
}

type getTenantFMPFederationPolicyArgs struct {
	APIVersion string `json:"api_version,omitempty"`
}

type setTenantFMPFederationPolicyArgs struct {
	APIVersion          string   `json:"api_version,omitempty"`
	AllowedTrustDomains []string `json:"allowed_trust_domains,omitempty"`
	AllowedRouteModes   []string `json:"allowed_route_modes,omitempty"`
	AllowMediation      bool     `json:"allow_mediation,omitempty"`
	MaxTransferBytes    int64    `json:"max_transfer_bytes,omitempty"`
}

type getEffectiveFMPFederationPolicyArgs struct {
	APIVersion  string `json:"api_version,omitempty"`
	TrustDomain string `json:"trust_domain"`
}

type updateNodeCapabilitiesArgs struct {
	APIVersion   string                      `json:"api_version,omitempty"`
	NodeID       string                      `json:"node_id"`
	Capabilities []core.CapabilityDescriptor `json:"capabilities,omitempty"`
}

type revokeNodeArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	NodeID     string `json:"node_id"`
}

type closeSessionArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	SessionID  string `json:"session_id"`
}

type grantSessionDelegationArgs struct {
	APIVersion  string   `json:"api_version,omitempty"`
	SessionID   string   `json:"session_id"`
	SubjectKind string   `json:"subject_kind"`
	SubjectID   string   `json:"subject_id"`
	Operations  []string `json:"operations,omitempty"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
}

type restartChannelArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	Channel    string `json:"channel"`
}

type issueTokenArgs struct {
	APIVersion      string   `json:"api_version,omitempty"`
	SubjectTenantID string   `json:"subject_tenant_id,omitempty"`
	SubjectKind     string   `json:"subject_kind,omitempty"`
	SubjectID       string   `json:"subject_id"`
	Scopes          []string `json:"scopes,omitempty"`
}

type createSubjectArgs struct {
	APIVersion      string   `json:"api_version,omitempty"`
	SubjectTenantID string   `json:"subject_tenant_id,omitempty"`
	SubjectKind     string   `json:"subject_kind"`
	SubjectID       string   `json:"subject_id"`
	DisplayName     string   `json:"display_name,omitempty"`
	Roles           []string `json:"roles,omitempty"`
}

type bindExternalIdentityArgs struct {
	APIVersion      string `json:"api_version,omitempty"`
	SubjectTenantID string `json:"subject_tenant_id,omitempty"`
	Provider        string `json:"provider"`
	AccountID       string `json:"account_id,omitempty"`
	ExternalID      string `json:"external_id"`
	SubjectKind     string `json:"subject_kind"`
	SubjectID       string `json:"subject_id"`
	DisplayName     string `json:"display_name,omitempty"`
	ProviderLabel   string `json:"provider_label,omitempty"`
}

type revokeTokenArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	TokenID    string `json:"token_id"`
}

type describeRexRuntimeArgs struct {
	APIVersion string `json:"api_version,omitempty"`
}

type readRexAdminSnapshotArgs struct {
	APIVersion string `json:"api_version,omitempty"`
}

type setPolicyRuleEnabledArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	RuleID     string `json:"rule_id"`
	Enabled    bool   `json:"enabled"`
}

// Phase 6.4: Compatibility Window args

type listFMPCompatibilityWindowsArgs struct {
	APIVersion string `json:"api_version,omitempty"`
}

type setFMPCompatibilityWindowArgs struct {
	APIVersion        string `json:"api_version,omitempty"`
	ContextClass      string `json:"context_class"`
	MinSchemaVersion  string `json:"min_schema_version,omitempty"`
	MaxSchemaVersion  string `json:"max_schema_version,omitempty"`
	MinRuntimeVersion string `json:"min_runtime_version,omitempty"`
	MaxRuntimeVersion string `json:"max_runtime_version,omitempty"`
}

type deleteFMPCompatibilityWindowArgs struct {
	APIVersion   string `json:"api_version,omitempty"`
	ContextClass string `json:"context_class"`
}

// Phase 6.5: Circuit Breaker args

type listFMPCircuitBreakersArgs struct {
	APIVersion string `json:"api_version,omitempty"`
}

type setFMPCircuitBreakerConfigArgs struct {
	APIVersion          string  `json:"api_version,omitempty"`
	TrustDomain         string  `json:"trust_domain"`
	ErrorThreshold      float64 `json:"error_threshold"`
	MinRequests         int     `json:"min_requests"`
	WindowDurationSec   int     `json:"window_duration_sec,omitempty"`
	RecoveryDurationSec int     `json:"recovery_duration_sec,omitempty"`
}

type resetFMPCircuitBreakerArgs struct {
	APIVersion  string `json:"api_version,omitempty"`
	TrustDomain string `json:"trust_domain"`
}

// Phase 7.2: SLO Signals args

type readRexSLOSignalsArgs struct {
	APIVersion string `json:"api_version,omitempty"`
}

func handleListPendingPairings(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		req := ListPendingPairingsRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), Page: PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)}}
		result, err := svc.ListPendingPairings(ctx, req)
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleApprovePairing(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ApprovePairing(ctx, ApprovePairingRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), Code: stringArg(args, "code", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleRejectPairing(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.RejectPairing(ctx, RejectPairingRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), Code: stringArg(args, "code", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleListNodes(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListNodes(ctx, ListNodesRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), Page: PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)}})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleGetNode(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.GetNode(ctx, GetNodeRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), NodeID: stringArg(args, "node_id", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleUpdateNodeCapabilities(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.UpdateNodeCapabilities(ctx, UpdateNodeCapabilitiesRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			NodeID:       stringArg(args, "node_id", ""),
			Capabilities: capabilityDescriptorsArg(args, "capabilities"),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleListEvents(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListEvents(ctx, ListEventsRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), PageRequest: PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)}})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleDescribeRexRuntime(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, _ map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.DescribeRexRuntime(ctx, DescribeRexRuntimeRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal))})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleReadRexAdminSnapshot(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, _ map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ReadRexAdminSnapshot(ctx, ReadRexAdminSnapshotRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal))})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleListFMPContinuations(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListFMPContinuations(ctx, ListFMPContinuationsRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Page:         PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)},
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleReadFMPContinuationAudit(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ReadFMPContinuationAudit(ctx, ReadFMPContinuationAuditRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			LineageID:    stringArg(args, "lineage_id", ""),
			Limit:        intArg(args, "limit", 0),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleVerifyFMPAuditTrail(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.VerifyFMPAuditTrail(ctx, VerifyFMPAuditTrailRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			LineageID:    stringArg(args, "lineage_id", ""),
			Limit:        intArg(args, "limit", 0),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleListFMPTrustBundles(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListFMPTrustBundles(ctx, ListFMPTrustBundlesRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Page:         PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)},
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleUpsertFMPTrustBundle(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		bundle, err := trustBundleArg(args, "bundle")
		if err != nil {
			return nil, err
		}
		result, err := svc.UpsertFMPTrustBundle(ctx, UpsertFMPTrustBundleRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Bundle:       bundle,
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleListFMPBoundaryPolicies(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListFMPBoundaryPolicies(ctx, ListFMPBoundaryPoliciesRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Page:         PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)},
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleSetFMPBoundaryPolicy(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		policy, err := boundaryPolicyArg(args, "policy")
		if err != nil {
			return nil, err
		}
		result, err := svc.SetFMPBoundaryPolicy(ctx, SetFMPBoundaryPolicyRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Policy:       policy,
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleListTenantFMPExports(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListTenantFMPExports(ctx, ListTenantFMPExportsRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Page:         PageRequest{Cursor: stringArg(args, "cursor", ""), Limit: intArg(args, "limit", 0)},
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleSetTenantFMPExport(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.SetTenantFMPExport(ctx, SetTenantFMPExportRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			ExportName:   stringArg(args, "export_name", ""),
			Enabled:      boolArg(args, "enabled", false),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleGetTenantFMPFederationPolicy(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.GetTenantFMPFederationPolicy(ctx, GetTenantFMPFederationPolicyRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleSetTenantFMPFederationPolicy(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.SetTenantFMPFederationPolicy(ctx, SetTenantFMPFederationPolicyRequest{
			AdminRequest:        requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			AllowedTrustDomains: stringListArg(args, "allowed_trust_domains"),
			AllowedRouteModes:   stringListArg(args, "allowed_route_modes"),
			AllowMediation:      boolArg(args, "allow_mediation", false),
			MaxTransferBytes:    int64(intArg(args, "max_transfer_bytes", 0)),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleGetEffectiveFMPFederationPolicy(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.GetEffectiveFMPFederationPolicy(ctx, GetEffectiveFMPFederationPolicyRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			TrustDomain:  stringArg(args, "trust_domain", ""),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleRevokeNode(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.RevokeNode(ctx, RevokeNodeRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), NodeID: stringArg(args, "node_id", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleCloseSession(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.CloseSession(ctx, CloseSessionRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), SessionID: stringArg(args, "session_id", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleGrantSessionDelegation(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		expiresAt, err := timeArg(args, "expires_at")
		if err != nil {
			return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "expires_at invalid", Detail: map[string]any{"field": "expires_at", "cause": err.Error()}}
		}
		result, err := svc.GrantSessionDelegation(ctx, GrantSessionDelegationRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SessionID:    stringArg(args, "session_id", ""),
			SubjectKind:  identity.SubjectKind(stringArg(args, "subject_kind", "")),
			SubjectID:    stringArg(args, "subject_id", ""),
			Operations:   sessionOperationsArg(args, "operations"),
			ExpiresAt:    expiresAt,
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleRestartChannel(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.RestartChannel(ctx, RestartChannelRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), Channel: stringArg(args, "channel", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleIssueToken(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.IssueToken(ctx, IssueTokenRequest{
			AdminRequest:    requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SubjectTenantID: stringArg(args, "subject_tenant_id", ""),
			SubjectKind:     identity.SubjectKind(stringArg(args, "subject_kind", "")),
			SubjectID:       stringArg(args, "subject_id", ""),
			Scopes:          stringListArg(args, "scopes"),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleCreateSubject(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.CreateSubject(ctx, CreateSubjectRequest{
			AdminRequest:    requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SubjectTenantID: stringArg(args, "subject_tenant_id", ""),
			SubjectKind:     identity.SubjectKind(stringArg(args, "subject_kind", "")),
			SubjectID:       stringArg(args, "subject_id", ""),
			DisplayName:     stringArg(args, "display_name", ""),
			Roles:           stringListArg(args, "roles"),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleBindExternalIdentity(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.BindExternalIdentity(ctx, BindExternalIdentityRequest{
			AdminRequest:    requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SubjectTenantID: stringArg(args, "subject_tenant_id", ""),
			Provider:        identity.ExternalProvider(stringArg(args, "provider", "")),
			AccountID:       stringArg(args, "account_id", ""),
			ExternalID:      stringArg(args, "external_id", ""),
			SubjectKind:     identity.SubjectKind(stringArg(args, "subject_kind", "")),
			SubjectID:       stringArg(args, "subject_id", ""),
			DisplayName:     stringArg(args, "display_name", ""),
			ProviderLabel:   stringArg(args, "provider_label", ""),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleRevokeToken(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.RevokeToken(ctx, RevokeTokenRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), TokenID: stringArg(args, "token_id", "")})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleSetPolicyRuleEnabled(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.SetPolicyRuleEnabled(ctx, SetPolicyRuleEnabledRequest{AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)), RuleID: stringArg(args, "rule_id", ""), Enabled: boolArg(args, "enabled", false)})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

// Phase 6.4: Compatibility Window handlers

func handleListFMPCompatibilityWindows(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListFMPCompatibilityWindows(ctx, ListFMPCompatibilityWindowsRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleSetFMPCompatibilityWindow(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.SetFMPCompatibilityWindow(ctx, SetFMPCompatibilityWindowRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			Window: FMPCompatibilityWindow{
				ContextClass:      stringArg(args, "context_class", ""),
				MinSchemaVersion:  stringArg(args, "min_schema_version", ""),
				MaxSchemaVersion:  stringArg(args, "max_schema_version", ""),
				MinRuntimeVersion: stringArg(args, "min_runtime_version", ""),
				MaxRuntimeVersion: stringArg(args, "max_runtime_version", ""),
			},
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleDeleteFMPCompatibilityWindow(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.DeleteFMPCompatibilityWindow(ctx, DeleteFMPCompatibilityWindowRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			ContextClass: stringArg(args, "context_class", ""),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

// Phase 6.5: Circuit Breaker handlers

func handleListFMPCircuitBreakers(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ListFMPCircuitBreakers(ctx, ListFMPCircuitBreakersRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleSetFMPCircuitBreakerConfig(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.SetFMPCircuitBreakerConfig(ctx, SetFMPCircuitBreakerConfigRequest{
			AdminRequest:        requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			TrustDomain:         stringArg(args, "trust_domain", ""),
			ErrorThreshold:      floatArg(args, "error_threshold", 0.5),
			MinRequests:         intArg(args, "min_requests", 10),
			WindowDurationSec:   intArg(args, "window_duration_sec", 60),
			RecoveryDurationSec: intArg(args, "recovery_duration_sec", 30),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

func handleResetFMPCircuitBreaker(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ResetFMPCircuitBreaker(ctx, ResetFMPCircuitBreakerRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			TrustDomain:  stringArg(args, "trust_domain", ""),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}

// Phase 7.2: SLO Signals handler

func handleReadRexSLOSignals(ctx context.Context, svc AdminService, principal identity.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.ReadRexSLOSignals(ctx, ReadRexSLOSignalsRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
		})
		if err != nil {
			return nil, err
		}
		return structuredResult(result)
	default:
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "unsupported API version", Detail: map[string]any{"api_version": version}}
	}
}
