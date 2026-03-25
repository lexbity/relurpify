package fmp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func (s *Service) PublishLocalTrustBundle(ctx context.Context, trustDomain, bundleID string, recipientKeys []core.RecipientKeyAdvertisement) error {
	if s == nil || s.Trust == nil {
		return fmt.Errorf("trust bundle store unavailable")
	}
	trustDomain = strings.TrimSpace(trustDomain)
	if trustDomain == "" {
		return fmt.Errorf("trust domain required")
	}
	if strings.TrimSpace(bundleID) == "" {
		bundleID = trustDomain + ":local"
	}
	existing, err := s.Trust.GetTrustBundle(ctx, trustDomain)
	if err != nil {
		return err
	}
	bundle := core.TrustBundle{
		TrustDomain: trustDomain,
		BundleID:    bundleID,
		IssuedAt:    s.nowUTC(),
		ExpiresAt:   s.nowUTC().Add(24 * time.Hour),
	}
	if existing != nil {
		bundle = *existing
		bundle.BundleID = firstNonEmpty(bundleID, bundle.BundleID)
		bundle.IssuedAt = s.nowUTC()
		if bundle.ExpiresAt.IsZero() || !bundle.ExpiresAt.After(bundle.IssuedAt) {
			bundle.ExpiresAt = bundle.IssuedAt.Add(24 * time.Hour)
		}
	}
	if anchor, ok := TrustAnchorForSigner(s.Signer); ok && !containsFoldString(bundle.TrustAnchors, anchor) {
		bundle.TrustAnchors = append(bundle.TrustAnchors, anchor)
	}
	bundle.RecipientKeys = mergeRecipientKeys(bundle.RecipientKeys, recipientKeys)
	return s.RegisterTrustBundle(ctx, bundle)
}

func mergeRecipientKeys(existing, added []core.RecipientKeyAdvertisement) []core.RecipientKeyAdvertisement {
	out := make([]core.RecipientKeyAdvertisement, 0, len(existing)+len(added))
	index := map[string]int{}
	for _, key := range existing {
		normalized := normalizeRecipientKey(key)
		if normalized.Recipient == "" || len(normalized.PublicKey) == 0 {
			continue
		}
		index[recipientKeyIndex(normalized)] = len(out)
		out = append(out, normalized)
	}
	for _, key := range added {
		normalized := normalizeRecipientKey(key)
		if normalized.Recipient == "" || len(normalized.PublicKey) == 0 {
			continue
		}
		keyID := recipientKeyIndex(normalized)
		if idx, ok := index[keyID]; ok {
			out[idx] = normalized
			continue
		}
		index[keyID] = len(out)
		out = append(out, normalized)
	}
	return out
}

func normalizeRecipientKey(key core.RecipientKeyAdvertisement) core.RecipientKeyAdvertisement {
	key.Recipient = strings.TrimSpace(key.Recipient)
	key.KeyID = strings.TrimSpace(key.KeyID)
	key.Version = strings.TrimSpace(key.Version)
	key.PublicKey = append([]byte(nil), key.PublicKey...)
	if !key.Active && key.RevokedAt.IsZero() {
		key.Active = true
	}
	return key
}

func recipientKeyIndex(key core.RecipientKeyAdvertisement) string {
	return strings.ToLower(strings.TrimSpace(key.Recipient)) + "::" + strings.ToLower(strings.TrimSpace(key.KeyID))
}
