package fmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// TrustBundleStore is part of the Phase 1 frozen FMP surface.
type TrustBundleStore interface {
	UpsertTrustBundle(ctx context.Context, bundle core.TrustBundle) error
	GetTrustBundle(ctx context.Context, trustDomain string) (*core.TrustBundle, error)
}

// BoundaryPolicyStore is part of the Phase 1 frozen FMP surface.
type BoundaryPolicyStore interface {
	UpsertBoundaryPolicy(ctx context.Context, policy core.BoundaryPolicy) error
	GetBoundaryPolicy(ctx context.Context, trustDomain string) (*core.BoundaryPolicy, error)
}

// GatewayForwarder is part of the Phase 1 frozen FMP surface.
// Later phases should replace the current abstraction-only state with a real
// Nexus and gateway-backed forwarding implementation.
type GatewayForwarder interface {
	ForwardSealedContext(ctx context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error)
}

type InMemoryTrustBundleStore struct {
	mu      sync.RWMutex
	bundles map[string]core.TrustBundle
}

func (s *InMemoryTrustBundleStore) ListTrustBundles(_ context.Context) ([]core.TrustBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.TrustBundle, 0, len(s.bundles))
	for _, bundle := range s.bundles {
		out = append(out, bundle)
	}
	return out, nil
}

func (s *InMemoryTrustBundleStore) UpsertTrustBundle(_ context.Context, bundle core.TrustBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bundles == nil {
		s.bundles = map[string]core.TrustBundle{}
	}
	s.bundles[strings.ToLower(strings.TrimSpace(bundle.TrustDomain))] = bundle
	return nil
}

func (s *InMemoryTrustBundleStore) GetTrustBundle(_ context.Context, trustDomain string) (*core.TrustBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.bundles[strings.ToLower(strings.TrimSpace(trustDomain))]
	if !ok {
		return nil, nil
	}
	copy := bundle
	return &copy, nil
}

type InMemoryBoundaryPolicyStore struct {
	mu       sync.RWMutex
	policies map[string]core.BoundaryPolicy
}

func (s *InMemoryBoundaryPolicyStore) ListBoundaryPolicies(_ context.Context) ([]core.BoundaryPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.BoundaryPolicy, 0, len(s.policies))
	for _, policy := range s.policies {
		out = append(out, policy)
	}
	return out, nil
}

func (s *InMemoryBoundaryPolicyStore) UpsertBoundaryPolicy(_ context.Context, policy core.BoundaryPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policies == nil {
		s.policies = map[string]core.BoundaryPolicy{}
	}
	s.policies[strings.ToLower(strings.TrimSpace(policy.TrustDomain))] = policy
	return nil
}

func (s *InMemoryBoundaryPolicyStore) GetBoundaryPolicy(_ context.Context, trustDomain string) (*core.BoundaryPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policy, ok := s.policies[strings.ToLower(strings.TrimSpace(trustDomain))]
	if !ok {
		return nil, nil
	}
	copy := policy
	return &copy, nil
}

func (s *Service) RegisterTrustBundle(ctx context.Context, bundle core.TrustBundle) error {
	if s.Trust == nil {
		return fmt.Errorf("trust bundle store unavailable")
	}
	if bundle.IssuedAt.IsZero() {
		bundle.IssuedAt = s.nowUTC()
	}
	if err := SignTrustBundle(s.Signer, &bundle); err != nil {
		return err
	}
	if err := s.Trust.UpsertTrustBundle(ctx, bundle); err != nil {
		return err
	}
	s.emit(ctx, core.FrameworkEventFMPTrustRegistered, core.SubjectRef{}, map[string]any{
		"trust_domain": bundle.TrustDomain,
		"bundle_id":    bundle.BundleID,
	})
	return nil
}

func (s *Service) SetBoundaryPolicy(ctx context.Context, policy core.BoundaryPolicy) error {
	if s.Boundaries == nil {
		return fmt.Errorf("boundary policy store unavailable")
	}
	return s.Boundaries.UpsertBoundaryPolicy(ctx, policy)
}

func (s *Service) ImportFederatedNodeAdvertisement(ctx context.Context, gateway core.SubjectRef, ad core.NodeAdvertisement, sourceDomain string) error {
	if err := ad.Validate(); err != nil {
		return err
	}
	if err := s.authorizeFederatedAdvertisement(ctx, gateway, ad.TrustDomain, sourceDomain, core.RouteModeGateway, 0); err != nil {
		return err
	}
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	if ad.ExpiresAt.IsZero() {
		ad.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	return s.Discovery.UpsertNodeAdvertisement(ctx, ad)
}

func (s *Service) ImportFederatedRuntimeAdvertisement(ctx context.Context, gateway core.SubjectRef, ad core.RuntimeAdvertisement, sourceDomain string) error {
	if err := validateAuthoritativeRuntime(ad); err != nil {
		return err
	}
	if err := s.verifyFederatedRuntimeAdvertisement(ctx, ad); err != nil {
		return err
	}
	if err := s.authorizeFederatedAdvertisement(ctx, gateway, ad.TrustDomain, sourceDomain, core.RouteModeGateway, ad.Runtime.MaxContextSize); err != nil {
		return err
	}
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	if ad.ExpiresAt.IsZero() {
		ad.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	return s.Discovery.UpsertRuntimeAdvertisement(ctx, ad)
}

func (s *Service) ImportFederatedExportAdvertisement(ctx context.Context, gateway core.SubjectRef, ad core.ExportAdvertisement, sourceDomain string) error {
	if err := ad.Validate(); err != nil {
		return err
	}
	if err := s.verifyFederatedExportAdvertisement(ctx, ad); err != nil {
		return err
	}
	routeMode := resolveRouteMode(ad.Export)
	if err := s.authorizeFederatedAdvertisement(ctx, gateway, ad.TrustDomain, sourceDomain, routeMode, ad.Export.MaxContextSize); err != nil {
		return err
	}
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	ad.Imported = true
	if !ad.Export.AdmissionSummary.Available && strings.TrimSpace(ad.Export.AdmissionSummary.Reason) == "" {
		ad.Export.AdmissionSummary.Available = true
	}
	if ad.ExpiresAt.IsZero() {
		ad.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	runtimeAd, err := s.resolveRegisteredRuntimeAdvertisement(ctx, ad.TrustDomain, ad.RuntimeID)
	if err != nil {
		return err
	}
	if runtimeAd == nil {
		return fmt.Errorf("federated export runtime %s in trust domain %s is not registered", ad.RuntimeID, ad.TrustDomain)
	}
	if !strings.EqualFold(runtimeAd.Runtime.NodeID, ad.NodeID) {
		return fmt.Errorf("federated export node_id must match registered runtime node_id")
	}
	if err := s.Discovery.UpsertExportAdvertisement(ctx, ad); err != nil {
		return err
	}
	s.emit(ctx, core.FrameworkEventFMPFederationImport, gateway, map[string]any{
		"trust_domain": ad.TrustDomain,
		"export_name":  ad.Export.ExportName,
		"runtime_id":   ad.RuntimeID,
	})
	return nil
}

func (s *Service) verifyFederatedRuntimeAdvertisement(ctx context.Context, ad core.RuntimeAdvertisement) error {
	if strings.TrimSpace(ad.Runtime.SignatureAlgorithm) == "" {
		return nil
	}
	if s == nil || s.Trust == nil {
		return fmt.Errorf("trust bundle store unavailable")
	}
	bundle, err := s.Trust.GetTrustBundle(ctx, ad.TrustDomain)
	if err != nil {
		return err
	}
	if bundle == nil {
		return fmt.Errorf("trust bundle not found for %s", ad.TrustDomain)
	}
	verifier, ok := verifierForTrustBundle(*bundle, ad.Runtime.SignatureAlgorithm)
	if !ok {
		return fmt.Errorf("trust bundle verifier unavailable for %s", ad.TrustDomain)
	}
	return VerifyRuntimeDescriptor(verifier, ad.Runtime)
}

func (s *Service) verifyFederatedExportAdvertisement(ctx context.Context, ad core.ExportAdvertisement) error {
	if strings.TrimSpace(ad.Export.SignatureAlgorithm) == "" {
		return nil
	}
	if s == nil || s.Trust == nil {
		return fmt.Errorf("trust bundle store unavailable")
	}
	bundle, err := s.Trust.GetTrustBundle(ctx, ad.TrustDomain)
	if err != nil {
		return err
	}
	if bundle == nil {
		return fmt.Errorf("trust bundle not found for %s", ad.TrustDomain)
	}
	verifier, ok := verifierForTrustBundle(*bundle, ad.Export.SignatureAlgorithm)
	if !ok {
		return fmt.Errorf("trust bundle verifier unavailable for %s", ad.TrustDomain)
	}
	return VerifyExportDescriptor(verifier, ad.Export)
}

func (s *Service) ForwardFederatedContext(ctx context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, *core.TransferRefusal, error) {
	if s.Forwarder == nil {
		return nil, nil, fmt.Errorf("gateway forwarder unavailable")
	}
	if err := req.Validate(); err != nil {
		return nil, nil, err
	}
	if refusal := s.allowForwardBudget(ctx, fallbackMessage(req.OfferID, req.ContextManifestRef), req.SizeBytes); refusal != nil {
		return nil, refusal, nil
	}
	if refusal := s.authorizeFederatedForward(ctx, req); refusal != nil {
		return nil, refusal, nil
	}
	if s.CircuitBreakers != nil {
		if state, err := s.CircuitBreakers.GetState(ctx, req.TrustDomain); err == nil && state == CircuitOpen {
			return nil, &core.TransferRefusal{
				Code:    core.RefusalAdmissionClosed,
				Message: fmt.Sprintf("circuit breaker open for trust domain %s", req.TrustDomain),
				RetryAt: s.nowUTC().Add(30 * time.Second),
			}, nil
		}
	}
	if req.MediationRequested {
		mediated, refusal, err := s.Mediator.MediateForward(ctx, s, req)
		if err != nil {
			return nil, nil, err
		}
		if refusal != nil {
			return nil, refusal, nil
		}
		req = mediated
	}
	result, err := s.Forwarder.ForwardSealedContext(ctx, req)
	if err != nil {
		if s.CircuitBreakers != nil {
			_ = s.CircuitBreakers.RecordFailure(ctx, req.TrustDomain, s.nowUTC())
		}
		return nil, nil, err
	}
	if result == nil {
		result = &core.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            !req.MediationRequested,
			ForwardedAt:       s.nowUTC(),
		}
	}
	if result.ForwardedAt.IsZero() {
		result.ForwardedAt = s.nowUTC()
	}
	if !req.MediationRequested {
		result.Opaque = true
	}
	if err := result.Validate(); err != nil {
		if s.CircuitBreakers != nil {
			_ = s.CircuitBreakers.RecordFailure(ctx, req.TrustDomain, s.nowUTC())
		}
		return nil, nil, err
	}
	if s.CircuitBreakers != nil {
		_ = s.CircuitBreakers.RecordSuccess(ctx, req.TrustDomain, s.nowUTC())
	}
	s.emit(ctx, core.FrameworkEventFMPGatewayForwarded, req.GatewayIdentity, map[string]any{
		"trust_domain":       req.TrustDomain,
		"destination_export": req.DestinationExport,
		"route_mode":         req.RouteMode,
		"opaque":             result.Opaque,
	})
	return result, nil, nil
}

func (s *Service) authorizeFederatedAdvertisement(ctx context.Context, gateway core.SubjectRef, trustDomain, sourceDomain string, routeMode core.RouteMode, sizeBytes int64) error {
	req := core.GatewayForwardRequest{
		TrustDomain:        trustDomain,
		SourceDomain:       sourceDomain,
		GatewayIdentity:    gateway,
		DestinationExport:  "discovery.import",
		RouteMode:          routeMode,
		SizeBytes:          sizeBytes,
		ContextManifestRef: "discovery",
		SealedContext: core.SealedContext{
			EnvelopeVersion:    "v1",
			ContextManifestRef: "discovery",
			CipherSuite:        "metadata-only",
			IntegrityTag:       "discovery",
		},
	}
	if refusal := s.authorizeFederatedForward(ctx, req); refusal != nil {
		return fmt.Errorf("federated advertisement denied: %s", refusal.Message)
	}
	return nil
}

func (s *Service) authorizeFederatedForward(ctx context.Context, req core.GatewayForwardRequest) *core.TransferRefusal {
	lineage, tenantID, refusal, err := s.resolveForwardFederationContext(ctx, req)
	if err != nil {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: err.Error()}
	}
	if refusal != nil {
		return refusal
	}
	if refusal := s.validateTenantFederationPolicy(ctx, tenantID, req.TrustDomain, req.RouteMode, req.SizeBytes); refusal != nil {
		return refusal
	}
	if lineage != nil && len(lineage.AllowedFederationTargets) > 0 && !containsFoldString(lineage.AllowedFederationTargets, req.TrustDomain) {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "trust domain not allowed by lineage federation policy"}
	}
	bundle, refusal, err := s.resolveActiveTrustBundle(ctx, req.TrustDomain)
	if err != nil {
		return &core.TransferRefusal{Code: core.RefusalUntrustedPeer, Message: err.Error()}
	}
	if refusal != nil {
		return refusal
	}
	policy, refusal, err := s.resolveBoundaryPolicy(ctx, req.TrustDomain)
	if err != nil {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: err.Error()}
	}
	if refusal != nil {
		return refusal
	}
	if policy.RequireGatewayAuthentication {
		if !subjectAllowed(req.GatewayIdentity, bundle.GatewayIdentities) {
			return &core.TransferRefusal{Code: core.RefusalUntrustedPeer, Message: "gateway identity not trusted for bundle"}
		}
	}
	if len(policy.AcceptedSourceDomains) > 0 && !containsFoldString(policy.AcceptedSourceDomains, req.SourceDomain) {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "source domain not allowed by boundary policy"}
	}
	if len(policy.AcceptedSourceIdentities) > 0 && !subjectAllowed(req.GatewayIdentity, policy.AcceptedSourceIdentities) {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "gateway identity not allowed by boundary policy"}
	}
	if len(policy.AllowedRouteModes) > 0 && !containsRouteMode(policy.AllowedRouteModes, req.RouteMode) {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "route mode not allowed by boundary policy"}
	}
	if req.MediationRequested && !policy.AllowMediation {
		return &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "mediation mode not allowed by boundary policy"}
	}
	if policy.MaxTransferBytes > 0 && req.SizeBytes > policy.MaxTransferBytes {
		return &core.TransferRefusal{Code: core.RefusalTransferBudget, Message: "transfer exceeds boundary policy budget"}
	}
	return nil
}

func (s *Service) resolveForwardFederationContext(ctx context.Context, req core.GatewayForwardRequest) (*core.LineageRecord, string, *core.TransferRefusal, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	if strings.EqualFold(strings.TrimSpace(req.DestinationExport), "discovery.import") {
		return nil, "", nil, nil
	}
	if strings.TrimSpace(req.LineageID) == "" {
		if tenantID == "" {
			return nil, "", &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "tenant id required for federated forward"}, nil
		}
		return nil, tenantID, nil, nil
	}
	if s.Ownership == nil {
		return nil, "", nil, fmt.Errorf("ownership store unavailable")
	}
	lineage, ok, err := s.Ownership.GetLineage(ctx, req.LineageID)
	if err != nil {
		return nil, "", nil, err
	}
	if !ok {
		return nil, "", &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "lineage not found for federated forward"}, nil
	}
	if tenantID != "" && !strings.EqualFold(tenantID, lineage.TenantID) {
		return nil, "", &core.TransferRefusal{Code: core.RefusalUnauthorized, Message: "tenant id does not match lineage tenant"}, nil
	}
	return lineage, lineage.TenantID, nil, nil
}

func (s *Service) resolveActiveTrustBundle(ctx context.Context, trustDomain string) (*core.TrustBundle, *core.TransferRefusal, error) {
	if s.Trust == nil {
		return nil, nil, fmt.Errorf("trust bundle store unavailable")
	}
	bundle, err := s.Trust.GetTrustBundle(ctx, trustDomain)
	if err != nil {
		return nil, nil, err
	}
	if bundle == nil {
		return nil, &core.TransferRefusal{Code: core.RefusalUntrustedPeer, Message: "trust bundle not found"}, nil
	}
	if !bundle.ExpiresAt.IsZero() && s.nowUTC().After(bundle.ExpiresAt) {
		return nil, &core.TransferRefusal{Code: core.RefusalUntrustedPeer, Message: "trust bundle expired"}, nil
	}
	return bundle, nil, nil
}

func (s *Service) resolveBoundaryPolicy(ctx context.Context, trustDomain string) (*core.BoundaryPolicy, *core.TransferRefusal, error) {
	if s.Boundaries == nil {
		return &core.BoundaryPolicy{TrustDomain: trustDomain}, nil, nil
	}
	policy, err := s.Boundaries.GetBoundaryPolicy(ctx, trustDomain)
	if err != nil {
		return nil, nil, err
	}
	if policy == nil {
		return &core.BoundaryPolicy{TrustDomain: trustDomain}, nil, nil
	}
	return policy, nil, nil
}

func containsRouteMode(values []core.RouteMode, want core.RouteMode) bool {
	for _, value := range values {
		if strings.EqualFold(string(value), string(want)) {
			return true
		}
	}
	return false
}

func subjectAllowed(subject core.SubjectRef, allowed []core.SubjectRef) bool {
	for _, candidate := range allowed {
		if candidate == subject {
			return true
		}
	}
	return false
}
