package admin

import (
	"context"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func (s *service) GetTenantFMPFederationPolicy(ctx context.Context, req GetTenantFMPFederationPolicyRequest) (GetTenantFMPFederationPolicyResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return GetTenantFMPFederationPolicyResult{}, err
	}
	if s.cfg.FMPFederation == nil {
		return GetTenantFMPFederationPolicyResult{}, notImplemented("tenant fmp federation controls not implemented", nil)
	}
	policy, err := s.cfg.FMPFederation.GetTenantFederationPolicy(ctx, tenantID)
	if err != nil {
		return GetTenantFMPFederationPolicyResult{}, internalError("get tenant fmp federation policy failed", err, map[string]any{"tenant_id": tenantID})
	}
	info := TenantFMPFederationPolicyInfo{TenantID: tenantID}
	if policy != nil {
		info = tenantFederationInfoFromPolicy(*policy)
	}
	sort.Strings(info.AllowedTrustDomains)
	return GetTenantFMPFederationPolicyResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Policy:      info,
	}, nil
}

func (s *service) SetTenantFMPFederationPolicy(ctx context.Context, req SetTenantFMPFederationPolicyRequest) (SetTenantFMPFederationPolicyResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return SetTenantFMPFederationPolicyResult{}, err
	}
	if s.cfg.FMPFederation == nil {
		return SetTenantFMPFederationPolicyResult{}, notImplemented("tenant fmp federation controls not implemented", nil)
	}
	policy := core.TenantFederationPolicy{
		TenantID:            tenantID,
		AllowedTrustDomains: append([]string(nil), req.AllowedTrustDomains...),
		AllowedRouteModes:   make([]core.RouteMode, 0, len(req.AllowedRouteModes)),
		AllowMediation:      req.AllowMediation,
		MaxTransferBytes:    req.MaxTransferBytes,
		UpdatedAt:           time.Now().UTC(),
	}
	for i := range policy.AllowedTrustDomains {
		policy.AllowedTrustDomains[i] = strings.TrimSpace(policy.AllowedTrustDomains[i])
	}
	for _, mode := range req.AllowedRouteModes {
		policy.AllowedRouteModes = append(policy.AllowedRouteModes, core.RouteMode(strings.TrimSpace(mode)))
	}
	if err := policy.Validate(); err != nil {
		return SetTenantFMPFederationPolicyResult{}, invalidArgument("tenant federation policy invalid", map[string]any{"cause": err.Error()})
	}
	if err := s.cfg.FMPFederation.SetTenantFederationPolicy(ctx, policy); err != nil {
		return SetTenantFMPFederationPolicyResult{}, internalError("set tenant fmp federation policy failed", err, map[string]any{"tenant_id": tenantID})
	}
	return SetTenantFMPFederationPolicyResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Policy:      tenantFederationInfoFromPolicy(policy),
	}, nil
}

func tenantFederationInfoFromPolicy(policy core.TenantFederationPolicy) TenantFMPFederationPolicyInfo {
	return TenantFMPFederationPolicyInfo{
		TenantID:            policy.TenantID,
		AllowedTrustDomains: append([]string(nil), policy.AllowedTrustDomains...),
		AllowedRouteModes:   routeModeStrings(policy.AllowedRouteModes),
		AllowMediation:      policy.AllowMediation,
		MaxTransferBytes:    policy.MaxTransferBytes,
		UpdatedAt:           policy.UpdatedAt,
	}
}

func routeModeStrings(modes []core.RouteMode) []string {
	if len(modes) == 0 {
		return nil
	}
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		out = append(out, string(mode))
	}
	sort.Strings(out)
	return out
}
