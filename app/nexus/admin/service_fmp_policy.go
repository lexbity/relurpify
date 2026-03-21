package admin

import (
	"context"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type fmpTrustBundleLister interface {
	ListTrustBundles(ctx context.Context) ([]core.TrustBundle, error)
}

type fmpBoundaryPolicyLister interface {
	ListBoundaryPolicies(ctx context.Context) ([]core.BoundaryPolicy, error)
}

func (s *service) ListFMPTrustBundles(ctx context.Context, req ListFMPTrustBundlesRequest) (ListFMPTrustBundlesResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return ListFMPTrustBundlesResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.Trust == nil {
		return ListFMPTrustBundlesResult{}, notImplemented("fmp trust bundle listing not implemented", nil)
	}
	lister, ok := s.cfg.FMP.Trust.(fmpTrustBundleLister)
	if !ok {
		return ListFMPTrustBundlesResult{}, notImplemented("fmp trust bundle listing not implemented", nil)
	}
	bundles, err := lister.ListTrustBundles(ctx)
	if err != nil {
		return ListFMPTrustBundlesResult{}, internalError("list fmp trust bundles failed", err, nil)
	}
	sort.Slice(bundles, func(i, j int) bool { return bundles[i].TrustDomain < bundles[j].TrustDomain })
	total := len(bundles)
	bundles = applyPage(bundles, req.Page)
	return ListFMPTrustBundlesResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		PageResult:  pageResult(total),
		Bundles:     bundles,
	}, nil
}

func (s *service) UpsertFMPTrustBundle(ctx context.Context, req UpsertFMPTrustBundleRequest) (UpsertFMPTrustBundleResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return UpsertFMPTrustBundleResult{}, err
	}
	if s.cfg.FMP == nil {
		return UpsertFMPTrustBundleResult{}, notImplemented("fmp trust bundle management not implemented", nil)
	}
	if err := s.cfg.FMP.RegisterTrustBundle(ctx, req.Bundle); err != nil {
		return UpsertFMPTrustBundleResult{}, internalError("upsert fmp trust bundle failed", err, map[string]any{"trust_domain": req.Bundle.TrustDomain})
	}
	return UpsertFMPTrustBundleResult{AdminResult: resultEnvelope(req.AdminRequest), Bundle: req.Bundle}, nil
}

func (s *service) ListFMPBoundaryPolicies(ctx context.Context, req ListFMPBoundaryPoliciesRequest) (ListFMPBoundaryPoliciesResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return ListFMPBoundaryPoliciesResult{}, err
	}
	if s.cfg.FMP == nil || s.cfg.FMP.Boundaries == nil {
		return ListFMPBoundaryPoliciesResult{}, notImplemented("fmp boundary policy listing not implemented", nil)
	}
	lister, ok := s.cfg.FMP.Boundaries.(fmpBoundaryPolicyLister)
	if !ok {
		return ListFMPBoundaryPoliciesResult{}, notImplemented("fmp boundary policy listing not implemented", nil)
	}
	policies, err := lister.ListBoundaryPolicies(ctx)
	if err != nil {
		return ListFMPBoundaryPoliciesResult{}, internalError("list fmp boundary policies failed", err, nil)
	}
	sort.Slice(policies, func(i, j int) bool { return policies[i].TrustDomain < policies[j].TrustDomain })
	total := len(policies)
	policies = applyPage(policies, req.Page)
	return ListFMPBoundaryPoliciesResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		PageResult:  pageResult(total),
		Policies:    policies,
	}, nil
}

func (s *service) SetFMPBoundaryPolicy(ctx context.Context, req SetFMPBoundaryPolicyRequest) (SetFMPBoundaryPolicyResult, error) {
	if err := authorizeGlobalFMPAdmin(req.Principal); err != nil {
		return SetFMPBoundaryPolicyResult{}, err
	}
	if s.cfg.FMP == nil {
		return SetFMPBoundaryPolicyResult{}, notImplemented("fmp boundary policy management not implemented", nil)
	}
	if err := s.cfg.FMP.SetBoundaryPolicy(ctx, req.Policy); err != nil {
		return SetFMPBoundaryPolicyResult{}, internalError("set fmp boundary policy failed", err, map[string]any{"trust_domain": req.Policy.TrustDomain})
	}
	return SetFMPBoundaryPolicyResult{AdminResult: resultEnvelope(req.AdminRequest), Policy: req.Policy}, nil
}

func authorizeGlobalFMPAdmin(principal core.AuthenticatedPrincipal) error {
	if !hasGlobalTenantScope(principal) {
		return AdminError{
			Code:    AdminErrorPolicyDenied,
			Message: "global admin scope required for mesh federation controls",
		}
	}
	return nil
}

func sanitizeTrustDomain(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
