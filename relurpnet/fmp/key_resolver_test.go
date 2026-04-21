package fmp

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestTrustBundleRecipientKeyResolverReturnsActiveKeys(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	store := &InMemoryTrustBundleStore{}
	err := store.UpsertTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain: "mesh.remote",
		BundleID:    "bundle-1",
		RecipientKeys: []core.RecipientKeyAdvertisement{
			{
				Recipient: "runtime://mesh.remote/node-1/rt-1",
				KeyID:     "old",
				Version:   "v1",
				PublicKey: []byte("11111111111111112222222222222222"),
				Active:    true,
				ExpiresAt: now.Add(time.Hour),
			},
			{
				Recipient: "runtime://mesh.remote/node-1/rt-1",
				KeyID:     "new",
				Version:   "v2",
				PublicKey: []byte("33333333333333334444444444444444"),
				Active:    true,
				ExpiresAt: now.Add(2 * time.Hour),
			},
			{
				Recipient: "runtime://mesh.remote/node-1/rt-1",
				KeyID:     "revoked",
				Version:   "v0",
				PublicKey: []byte("55555555555555556666666666666666"),
				Active:    true,
				ExpiresAt: now.Add(time.Hour),
				RevokedAt: now.Add(-time.Minute),
			},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("UpsertTrustBundle() error = %v", err)
	}

	resolver := &TrustBundleRecipientKeyResolver{
		Trust: store,
		Now:   func() time.Time { return now },
	}
	keys, err := resolver.ResolveRecipientKeys(context.Background(), "runtime://mesh.remote/node-1/rt-1")
	if err != nil {
		t.Fatalf("ResolveRecipientKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %+v", keys)
	}
	if keys[0].KeyID != "new" || keys[1].KeyID != "old" {
		t.Fatalf("unexpected key order: %+v", keys)
	}
}
