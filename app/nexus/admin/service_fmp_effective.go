package admin

import (
	"context"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type fmpTrustBundleGetter interface {
	GetTrustBundle(ctx context.Context, trustDomain string) (*core.TrustBundle, error)
}

type fmpBoundaryPolicyGetter interface {
	GetBoundaryPolicy(ctx context.Context, trustDomain string) (*core.BoundaryPolicy, error)
}

func (s *service) GetEffectiveFMPFederationPolicy(ctx context.Context, req GetEffectiveFMPFederationPolicyRequest) (GetEffectiveFMPFederationPolicyResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return GetEffectiveFMPFederationPolicyResult{}, err
	}
	trustDomain := sanitizeTrustDomain(req.TrustDomain)
	if trustDomain == "" {
		return GetEffectiveFMPFederationPolicyResult{}, invalidArgument("trust domain required", map[string]any{"field": "trust_domain"})
	}

	policyInfo := TenantFMPFederationPolicyInfo{TenantID: tenantID}
	if s.cfg.FMPFederation != nil {
		policy, err := s.cfg.FMPFederation.GetTenantFederationPolicy(ctx, tenantID)
		if err != nil {
			return GetEffectiveFMPFederationPolicyResult{}, internalError("get tenant federation policy failed", err, map[string]any{"tenant_id": tenantID})
		}
		if policy != nil {
			policyInfo = tenantFederationInfoFromPolicy(*policy)
		}
	}

	info := EffectiveFMPFederationPolicyInfo{
		TenantID:           tenantID,
		TrustDomain:        trustDomain,
		TenantPolicy:       policyInfo,
		AllowedTrustDomain: len(policyInfo.AllowedTrustDomains) == 0 || containsFold(policyInfo.AllowedTrustDomains, trustDomain),
		AllowMediation:     policyInfo.AllowMediation,
		MaxTransferBytes:   policyInfo.MaxTransferBytes,
	}

	if len(policyInfo.AllowedRouteModes) > 0 {
		info.AllowedRouteModes = append([]string(nil), policyInfo.AllowedRouteModes...)
	} else {
		info.AllowedRouteModes = []string{string(core.RouteModeDirect), string(core.RouteModeGateway), string(core.RouteModeMediated)}
	}
	sort.Strings(info.AllowedRouteModes)

	if s.cfg.FMP != nil && s.cfg.FMP.Trust != nil {
		getter, ok := s.cfg.FMP.Trust.(fmpTrustBundleGetter)
		if ok {
			bundle, err := getter.GetTrustBundle(ctx, trustDomain)
			if err != nil {
				return GetEffectiveFMPFederationPolicyResult{}, internalError("get trust bundle failed", err, map[string]any{"trust_domain": trustDomain})
			}
			if bundle != nil {
				info.TrustBundlePresent = true
				info.TrustBundle = bundle
			}
		}
	}

	if s.cfg.FMP != nil && s.cfg.FMP.Boundaries != nil {
		getter, ok := s.cfg.FMP.Boundaries.(fmpBoundaryPolicyGetter)
		if ok {
			policy, err := getter.GetBoundaryPolicy(ctx, trustDomain)
			if err != nil {
				return GetEffectiveFMPFederationPolicyResult{}, internalError("get boundary policy failed", err, map[string]any{"trust_domain": trustDomain})
			}
			if policy != nil {
				info.BoundaryPolicyPresent = true
				info.BoundaryPolicy = policy
				info.RequireGatewayAuth = policy.RequireGatewayAuthentication
				info.AcceptedSourceDomains = append([]string(nil), policy.AcceptedSourceDomains...)
				info.AcceptedSourceIdentities = append([]core.SubjectRef(nil), policy.AcceptedSourceIdentities...)
				if len(policy.AllowedRouteModes) > 0 {
					info.AllowedRouteModes = intersectStrings(info.AllowedRouteModes, routeModeStrings(policy.AllowedRouteModes))
				}
				if policy.AllowMediation == false {
					info.AllowMediation = false
				}
				if policy.MaxTransferBytes > 0 && (info.MaxTransferBytes == 0 || policy.MaxTransferBytes < info.MaxTransferBytes) {
					info.MaxTransferBytes = policy.MaxTransferBytes
				}
			}
		}
	}

	if !containsFold(info.AllowedRouteModes, string(core.RouteModeMediated)) {
		info.AllowMediation = false
	}
	sort.Strings(info.AcceptedSourceDomains)

	return GetEffectiveFMPFederationPolicyResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Policy:      info,
	}, nil
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func intersectStrings(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, value := range a {
		if containsFold(b, value) {
			out = append(out, value)
		}
	}
	return out
}
