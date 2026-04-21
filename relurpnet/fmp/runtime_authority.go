package fmp

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func validateAuthoritativeRuntime(ad core.RuntimeAdvertisement) error {
	if err := ad.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(ad.Runtime.AttestationProfile) == "" {
		return fmt.Errorf("authoritative runtime attestation profile required")
	}
	if len(ad.Runtime.AttestationClaims) == 0 {
		return fmt.Errorf("authoritative runtime attestation claims required")
	}
	if strings.TrimSpace(firstNonEmpty(ad.Signature, ad.Runtime.Signature)) == "" {
		return fmt.Errorf("authoritative runtime signature required")
	}
	return nil
}

func (s *Service) resolveRegisteredRuntimeAdvertisement(ctx context.Context, trustDomain, runtimeID string) (*core.RuntimeAdvertisement, error) {
	if s == nil || s.Discovery == nil {
		return nil, fmt.Errorf("discovery store unavailable")
	}
	runtimes, err := s.listLiveRuntimeAds(ctx)
	if err != nil {
		return nil, err
	}
	for _, runtime := range runtimes {
		if strings.EqualFold(runtime.TrustDomain, trustDomain) && strings.EqualFold(runtime.Runtime.RuntimeID, runtimeID) {
			copy := runtime
			return &copy, nil
		}
	}
	return nil, nil
}
