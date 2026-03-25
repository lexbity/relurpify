package fmp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type RuntimeRegistrationRequest struct {
	TrustDomain string                 `json:"trust_domain" yaml:"trust_domain"`
	Node        core.NodeDescriptor    `json:"node" yaml:"node"`
	Runtime     core.RuntimeDescriptor `json:"runtime" yaml:"runtime"`
	ExpiresAt   time.Time              `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Signature   string                 `json:"signature,omitempty" yaml:"signature,omitempty"`
}

func (r RuntimeRegistrationRequest) Validate() error {
	if strings.TrimSpace(r.TrustDomain) == "" {
		return fmt.Errorf("trust domain required")
	}
	if err := r.Node.Validate(); err != nil {
		return fmt.Errorf("node invalid: %w", err)
	}
	if err := r.Runtime.Validate(); err != nil {
		return fmt.Errorf("runtime invalid: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(r.Runtime.NodeID), strings.TrimSpace(r.Node.ID)) {
		return fmt.Errorf("runtime node_id must match registered node")
	}
	if strings.TrimSpace(r.Runtime.TrustDomain) != "" && !strings.EqualFold(strings.TrimSpace(r.Runtime.TrustDomain), strings.TrimSpace(r.TrustDomain)) {
		return fmt.Errorf("runtime trust domain must match registration trust domain")
	}
	if strings.TrimSpace(r.Runtime.AttestationProfile) == "" {
		return fmt.Errorf("runtime attestation profile required")
	}
	if len(r.Runtime.AttestationClaims) == 0 {
		return fmt.Errorf("runtime attestation claims required")
	}
	if strings.TrimSpace(firstNonEmpty(r.Signature, r.Runtime.Signature)) == "" {
		return fmt.Errorf("runtime registration signature required")
	}
	return nil
}

func (s *Service) RegisterRuntime(ctx context.Context, req RuntimeRegistrationRequest) error {
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	if err := req.Validate(); err != nil {
		return err
	}
	nodeAd := core.NodeAdvertisement{
		TrustDomain: req.TrustDomain,
		Node:        req.Node,
		Health: core.NodeHealth{
			Online:     true,
			Foreground: true,
			LastSeenAt: s.nowUTC(),
		},
		ExpiresAt: req.ExpiresAt,
	}
	if nodeAd.ExpiresAt.IsZero() {
		nodeAd.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	runtime := req.Runtime
	runtime.TrustDomain = req.TrustDomain
	if err := SignRuntimeDescriptor(s.Signer, &runtime); err != nil {
		return err
	}
	if strings.TrimSpace(runtime.Signature) == "" {
		runtime.Signature = firstNonEmpty(req.Signature, runtime.Signature)
	}
	if runtime.ExpiresAt.IsZero() {
		runtime.ExpiresAt = nodeAd.ExpiresAt
	}
	runtimeAd := core.RuntimeAdvertisement{
		TrustDomain: req.TrustDomain,
		Runtime:     runtime,
		ExpiresAt:   runtime.ExpiresAt,
		Signature:   runtime.Signature,
	}
	if err := s.Discovery.UpsertNodeAdvertisement(ctx, nodeAd); err != nil {
		return err
	}
	return s.Discovery.UpsertRuntimeAdvertisement(ctx, runtimeAd)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
