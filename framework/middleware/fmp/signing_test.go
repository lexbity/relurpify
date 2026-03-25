package fmp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestProtocolSigningRoundTrip(t *testing.T) {
	t.Parallel()

	signer := NewEd25519SignerFromSeed([]byte("phase5-signing-roundtrip"))
	verifier := &Ed25519Verifier{PublicKey: signer.PublicKey()}
	manifest := core.ContextManifest{
		ContextID:      "ctx-1",
		LineageID:      "lineage-1",
		AttemptID:      "attempt-1",
		ContextClass:   "workflow-runtime",
		SchemaVersion:  "fmp.context.v1",
		ContentHash:    "hash",
		TransferMode:   core.TransferModeInline,
		EncryptionMode: core.EncryptionModeEndToEnd,
	}
	if err := SignContextManifest(signer, &manifest); err != nil {
		t.Fatalf("SignContextManifest() error = %v", err)
	}
	if manifest.SignatureAlgorithm != SignatureAlgorithmEd25519 || manifest.Signature == "" {
		t.Fatalf("manifest signature missing: %+v", manifest)
	}
	if err := VerifyContextManifest(verifier, manifest); err != nil {
		t.Fatalf("VerifyContextManifest() error = %v", err)
	}
	manifest.ContextClass = "tampered"
	if err := VerifyContextManifest(verifier, manifest); err == nil {
		t.Fatal("VerifyContextManifest() error = nil, want tamper detection")
	}
}

func TestServiceSignsAndVerifiesContinuationFlow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	signer := NewEd25519SignerFromSeed([]byte("phase5-service-signing"))
	store := &InMemoryOwnershipStore{}
	runtimeEndpoint := &fakeRuntimeEndpoint{
		descriptor: core.RuntimeDescriptor{
			RuntimeID:               "rt-b",
			NodeID:                  "node-b",
			RuntimeVersion:          "1.0.0",
			SupportedContextClasses: []string{"workflow-runtime"},
			CompatibilityClass:      "v1",
			AttestationProfile:      "test",
			AttestationClaims:       map[string]string{"k": "v"},
			SignatureAlgorithm:      SignatureAlgorithmEd25519,
			Signature:               "placeholder",
		},
		createdAttempt: &core.AttemptRecord{
			AttemptID: "lineage-1:rt-b:resume",
			LineageID: "lineage-1",
			RuntimeID: "rt-b",
			State:     core.AttemptStateResumePending,
			StartTime: now,
		},
		issuedReceipt: &core.ResumeReceipt{
			ReceiptID:         "receipt-1",
			LineageID:         "lineage-1",
			AttemptID:         "lineage-1:rt-b:resume",
			RuntimeID:         "rt-b",
			ImportedContextID: "lineage-1:attempt-a",
			StartTime:         now,
			Status:            core.ReceiptStatusRunning,
		},
	}
	sourceSvc := &Service{
		Ownership: store,
		Packager: JSONPackager{
			RuntimeStore:      fakeWorkflowRuntimeStore{},
			KeyResolver:       testRecipientKeys(),
			DefaultRecipients: []string{"runtime://mesh-a/node-b/rt-b"},
			LocalRecipient:    "runtime://mesh-a/node-b/rt-b",
			Signer:            signer,
		},
		Log:       &memoryEventLog{},
		Partition: "local",
		LeaseTTL:  time.Minute,
		Now:       func() time.Time { return now },
		Signer:    signer,
		Verifier:  &Ed25519Verifier{PublicKey: signer.PublicKey()},
	}
	destSvc := &Service{
		Ownership: store,
		Runtime:   runtimeEndpoint,
		Log:       &memoryEventLog{},
		Partition: "local",
		LeaseTTL:  time.Minute,
		Now:       func() time.Time { return now },
		Signer:    signer,
		Verifier:  &Ed25519Verifier{PublicKey: signer.PublicKey()},
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := sourceSvc.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{AttemptID: "attempt-a", LineageID: lineage.LineageID, RuntimeID: "rt-a", State: core.AttemptStateRunning, StartTime: now}
	if err := store.UpsertAttempt(context.Background(), source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	offer, pkg, sealed, err := sourceSvc.OfferHandoff(context.Background(), lineage.LineageID, source.AttemptID, "exp.run", "issuer", RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("OfferHandoff() error = %v", err)
	}
	if offer.Signature == "" || offer.LeaseToken.Signature == "" || pkg.Manifest.Signature == "" {
		t.Fatalf("expected signed continuation artifacts: offer=%+v lease=%+v manifest=%+v", *offer, offer.LeaseToken, pkg.Manifest)
	}
	if _, _, refusal, err := destSvc.ResumeHandoff(context.Background(), *offer, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b", pkg.Manifest, *sealed); err != nil || refusal != nil {
		t.Fatalf("ResumeHandoff() err=%v refusal=%+v", err, refusal)
	}

	tampered := *offer
	tampered.DestinationExport = "exp.other"
	_, refusal, err := sourceSvc.TryAcceptHandoff(context.Background(), tampered, core.ExportDescriptor{
		ExportName:             "exp.run",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
	}, "rt-b")
	if err != nil {
		t.Fatalf("TryAcceptHandoff(tampered) err=%v", err)
	}
	if refusal == nil || !strings.Contains(refusal.Message, "signature") {
		t.Fatalf("expected signature refusal, got %+v", refusal)
	}
}

func TestServiceSignsRuntimeAndExportAdvertisements(t *testing.T) {
	t.Parallel()

	signer := NewEd25519SignerFromSeed([]byte("phase5-advertisement-signing"))
	store := &InMemoryDiscoveryStore{}
	svc := &Service{
		Discovery: store,
		Signer:    signer,
		Now:       func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}
	err := svc.RegisterRuntime(context.Background(), RuntimeRegistrationRequest{
		TrustDomain: "mesh.local",
		Node: core.NodeDescriptor{
			ID:         "node-1",
			TenantID:   "tenant-1",
			Name:       "Node One",
			Platform:   core.NodePlatformHeadless,
			TrustClass: core.TrustClassRemoteApproved,
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-1"},
			PairedAt:   time.Date(2026, 3, 23, 11, 0, 0, 0, time.UTC),
		},
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-1",
			NodeID:                  "node-1",
			RuntimeVersion:          "1.0.0",
			SupportedContextClasses: []string{"workflow-runtime"},
			CompatibilityClass:      "compat-a",
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-1"},
			Signature:               "legacy-placeholder",
		},
		Signature: "legacy-placeholder",
	})
	if err != nil {
		t.Fatalf("RegisterRuntime() error = %v", err)
	}
	runtimes, err := store.ListRuntimeAdvertisements(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeAdvertisements() error = %v", err)
	}
	if len(runtimes) != 1 || runtimes[0].Runtime.SignatureAlgorithm != SignatureAlgorithmEd25519 || runtimes[0].Runtime.Signature == "" {
		t.Fatalf("unexpected signed runtime advertisement: %+v", runtimes)
	}

	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "mesh.local",
		RuntimeID:   "rt-1",
		NodeID:      "node-1",
		Export: core.ExportDescriptor{
			ExportName:             "exp.run",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	exports, err := store.ListExportAdvertisements(context.Background())
	if err != nil {
		t.Fatalf("ListExportAdvertisements() error = %v", err)
	}
	if len(exports) != 1 || exports[0].Export.SignatureAlgorithm != SignatureAlgorithmEd25519 || exports[0].Export.Signature == "" {
		t.Fatalf("unexpected signed export advertisement: %+v", exports)
	}
}

func TestImportFederatedAdvertisementsVerifiesTrustAnchorSignature(t *testing.T) {
	t.Parallel()

	remoteSigner := NewEd25519SignerFromSeed([]byte("phase5-remote-ad-signing"))
	svc := &Service{
		Discovery:  &InMemoryDiscoveryStore{},
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Nexus: NexusAdapter{
			Federation: fakeTenantFederationLookup{policies: map[string]core.TenantFederationPolicy{
				"tenant-1": {TenantID: "tenant-1", AllowedTrustDomains: []string{"mesh.remote"}},
			}},
		},
		Now: func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	err := svc.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{gateway},
		TrustAnchors:      []string{trustAnchorForPublicKey(SignatureAlgorithmEd25519, remoteSigner.PublicKey())},
		IssuedAt:          time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
		ExpiresAt:         time.Date(2026, 3, 23, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if err := svc.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []core.SubjectRef{gateway},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway},
		RequireGatewayAuthentication: true,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}

	runtimeAd := core.RuntimeAdvertisement{
		TrustDomain: "mesh.remote",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-remote",
			NodeID:                  "node-remote",
			RuntimeVersion:          "1.0.0",
			CompatibilityClass:      "compat-a",
			SupportedContextClasses: []string{"workflow-runtime"},
			MaxContextSize:          2048,
			AttestationProfile:      "remote.runtime.v1",
			AttestationClaims:       map[string]string{"node_id": "node-remote"},
		},
	}
	if err := SignRuntimeDescriptor(remoteSigner, &runtimeAd.Runtime); err != nil {
		t.Fatalf("SignRuntimeDescriptor() error = %v", err)
	}
	runtimeAd.Signature = runtimeAd.Runtime.Signature
	if err := svc.ImportFederatedRuntimeAdvertisement(context.Background(), gateway, runtimeAd, "mesh.remote"); err != nil {
		t.Fatalf("ImportFederatedRuntimeAdvertisement() error = %v", err)
	}

	exportAd := core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		RuntimeID:   "rt-remote",
		NodeID:      "node-remote",
		Export: core.ExportDescriptor{
			ExportName:             "agent.resume",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			MaxContextSize:         2048,
		},
	}
	if err := SignExportDescriptor(remoteSigner, &exportAd.Export); err != nil {
		t.Fatalf("SignExportDescriptor() error = %v", err)
	}
	exportAd.Signature = exportAd.Export.Signature
	if err := svc.ImportFederatedExportAdvertisement(context.Background(), gateway, exportAd, "mesh.remote"); err != nil {
		t.Fatalf("ImportFederatedExportAdvertisement() error = %v", err)
	}

	tampered := exportAd
	tampered.Export.ExportName = "agent.other"
	if err := svc.ImportFederatedExportAdvertisement(context.Background(), gateway, tampered, "mesh.remote"); err == nil {
		t.Fatal("ImportFederatedExportAdvertisement() error = nil, want signature failure")
	}
}
