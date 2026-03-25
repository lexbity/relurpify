package fmp

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestPublishLocalTrustBundleAddsTrustAnchorAndRecipientKeys(t *testing.T) {
	t.Parallel()

	signer := NewEd25519SignerFromSeed([]byte("local-trust-bundle"))
	store := &InMemoryTrustBundleStore{}
	svc := &Service{
		Trust:  store,
		Signer: signer,
		Now:    func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}
	err := svc.PublishLocalTrustBundle(context.Background(), "mesh.local", "mesh.local:nexus", []core.RecipientKeyAdvertisement{
		{
			Recipient: "runtime://mesh.local/rt-1",
			KeyID:     "runtime",
			Version:   "v1",
			PublicKey: []byte("0123456789abcdef0123456789abcdef"),
			Active:    true,
		},
		{
			Recipient: "gateway://mesh.local/node-gw",
			KeyID:     "gateway-mediation",
			Version:   "v1",
			PublicKey: []byte("abcdef0123456789abcdef0123456789"),
			Active:    true,
		},
	})
	if err != nil {
		t.Fatalf("PublishLocalTrustBundle() error = %v", err)
	}
	bundle, err := store.GetTrustBundle(context.Background(), "mesh.local")
	if err != nil {
		t.Fatalf("GetTrustBundle() error = %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.Signature == "" || bundle.SignatureAlgorithm != SignatureAlgorithmEd25519 {
		t.Fatalf("expected signed trust bundle, got %+v", bundle)
	}
	if len(bundle.TrustAnchors) != 1 {
		t.Fatalf("trust anchors = %+v", bundle.TrustAnchors)
	}
	if len(bundle.RecipientKeys) != 2 {
		t.Fatalf("recipient keys = %+v", bundle.RecipientKeys)
	}
}

func TestPublishLocalTrustBundleMergesRecipientKeyUpdates(t *testing.T) {
	t.Parallel()

	store := &InMemoryTrustBundleStore{}
	svc := &Service{
		Trust: store,
		Now:   func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}
	if err := store.UpsertTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain: "mesh.local",
		BundleID:    "mesh.local:nexus",
		RecipientKeys: []core.RecipientKeyAdvertisement{{
			Recipient: "gateway://mesh.local/node-gw",
			KeyID:     "gateway-mediation",
			Version:   "v1",
			PublicKey: []byte("oldoldoldoldoldoldoldoldoldold12"),
			Active:    true,
		}},
		IssuedAt:  svc.nowUTC(),
		ExpiresAt: svc.nowUTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("UpsertTrustBundle() error = %v", err)
	}
	err := svc.PublishLocalTrustBundle(context.Background(), "mesh.local", "mesh.local:nexus", []core.RecipientKeyAdvertisement{{
		Recipient: "gateway://mesh.local/node-gw",
		KeyID:     "gateway-mediation",
		Version:   "v2",
		PublicKey: []byte("newnewnewnewnewnewnewnewnewnew12"),
		Active:    true,
	}})
	if err != nil {
		t.Fatalf("PublishLocalTrustBundle() error = %v", err)
	}
	bundle, err := store.GetTrustBundle(context.Background(), "mesh.local")
	if err != nil {
		t.Fatalf("GetTrustBundle() error = %v", err)
	}
	if bundle == nil || len(bundle.RecipientKeys) != 1 {
		t.Fatalf("bundle recipient keys = %+v", bundle)
	}
	if got := bundle.RecipientKeys[0].Version; got != "v2" {
		t.Fatalf("version = %s, want v2", got)
	}
}
