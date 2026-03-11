package admin

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
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

type setPolicyRuleEnabledArgs struct {
	APIVersion string `json:"api_version,omitempty"`
	RuleID     string `json:"rule_id"`
	Enabled    bool   `json:"enabled"`
}

func handleListPendingPairings(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleApprovePairing(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleRejectPairing(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleListNodes(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleGetNode(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleUpdateNodeCapabilities(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleListEvents(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleRevokeNode(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleCloseSession(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleGrantSessionDelegation(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		expiresAt, err := timeArg(args, "expires_at")
		if err != nil {
			return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "expires_at invalid", Detail: map[string]any{"field": "expires_at", "cause": err.Error()}}
		}
		result, err := svc.GrantSessionDelegation(ctx, GrantSessionDelegationRequest{
			AdminRequest: requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SessionID:    stringArg(args, "session_id", ""),
			SubjectKind:  core.SubjectKind(stringArg(args, "subject_kind", "")),
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

func handleRestartChannel(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleIssueToken(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.IssueToken(ctx, IssueTokenRequest{
			AdminRequest:    requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SubjectTenantID: stringArg(args, "subject_tenant_id", ""),
			SubjectKind:     core.SubjectKind(stringArg(args, "subject_kind", "")),
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

func handleCreateSubject(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.CreateSubject(ctx, CreateSubjectRequest{
			AdminRequest:    requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SubjectTenantID: stringArg(args, "subject_tenant_id", ""),
			SubjectKind:     core.SubjectKind(stringArg(args, "subject_kind", "")),
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

func handleBindExternalIdentity(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
	switch version {
	case APIVersionV1Alpha1:
		result, err := svc.BindExternalIdentity(ctx, BindExternalIdentityRequest{
			AdminRequest:    requestEnvelope(principal, version, tenantFromPrincipal(principal)),
			SubjectTenantID: stringArg(args, "subject_tenant_id", ""),
			Provider:        core.ExternalProvider(stringArg(args, "provider", "")),
			AccountID:       stringArg(args, "account_id", ""),
			ExternalID:      stringArg(args, "external_id", ""),
			SubjectKind:     core.SubjectKind(stringArg(args, "subject_kind", "")),
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

func handleRevokeToken(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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

func handleSetPolicyRuleEnabled(ctx context.Context, svc AdminService, principal core.AuthenticatedPrincipal, version string, args map[string]any) (*protocol.CallToolResult, error) {
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
