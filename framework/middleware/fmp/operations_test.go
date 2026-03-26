package fmp

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type telemetryRecorder struct {
	events []core.Event
}

func (t *telemetryRecorder) Emit(event core.Event) {
	t.events = append(t.events, event)
}

func TestResolveRoutesFiltersBlockedRuntimeVersion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery: &InMemoryDiscoveryStore{},
		Rollout: RolloutPolicy{
			MinImportRuntimeVersion: "2.0.0",
		},
		Now: func() time.Time { return now },
	}
	if err := svc.AdvertiseRuntime(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "mesh.local",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:               "rt-a",
			NodeID:                  "node-a",
			RuntimeVersion:          "1.5.0",
			CompatibilityClass:      "v1",
			SupportedContextClasses: []string{"workflow-runtime"},
			AttestationProfile:      "test.attestation.v1",
			AttestationClaims:       map[string]string{"node_id": "node-a"},
			Signature:               "sig-rt-a",
		},
		Signature: "sig-rt-a",
	}); err != nil {
		t.Fatalf("AdvertiseRuntime() error = %v", err)
	}
	if err := svc.AdvertiseExport(context.Background(), core.ExportAdvertisement{
		TrustDomain: "mesh.local",
		RuntimeID:   "rt-a",
		NodeID:      "node-a",
		Export: core.ExportDescriptor{
			ExportName:             "agent.resume",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			AdmissionSummary:       core.AvailabilitySpec{Available: true},
		},
	}); err != nil {
		t.Fatalf("AdvertiseExport() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:       "agent.resume",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 256,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("expected rollout-blocked routes, got %+v", routes)
	}
}

func TestTryAcceptHandoffRejectsWhenResumeLimiterFull(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	limiter := &InMemoryOperationalLimiter{Limits: OperationalLimits{MaxActiveResumeSlots: 1}}
	svc := &Service{
		Ownership: store,
		Limiter:   limiter,
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	offer := core.HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "agent.resume",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		ContextSizeBytes:   64,
		LeaseToken:         *lease,
		Expiry:             lease.Expiry,
	}
	destination := core.ExportDescriptor{
		ExportName:             "agent.resume",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
		AdmissionSummary:       core.AvailabilitySpec{Available: true},
	}
	if _, refusal, err := svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-b"); err != nil || refusal != nil {
		t.Fatalf("first TryAcceptHandoff() err=%v refusal=%+v", err, refusal)
	}
	offer2 := offer
	offer2.OfferID = "offer-2"
	if _, refusal, err := svc.TryAcceptHandoff(context.Background(), offer2, destination, "rt-c"); err != nil {
		t.Fatalf("second TryAcceptHandoff() error = %v", err)
	} else if refusal == nil || refusal.Code != core.RefusalDestinationBusy {
		t.Fatalf("expected limiter refusal, got %+v", refusal)
	}
}

func TestTryAcceptHandoffIsIdempotentForSameOfferAndRuntime(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	svc := &Service{Ownership: store}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	requireAttempt(t, store.CreateLineage(context.Background(), lineage))
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	requireAttempt(t, store.UpsertAttempt(context.Background(), attempt))
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	offer := core.HandoffOffer{
		OfferID:            lease.LeaseID,
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "agent.resume",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		ContextSizeBytes:   64,
		LeaseToken:         *lease,
		Expiry:             lease.Expiry,
	}
	destination := core.ExportDescriptor{
		ExportName:             "agent.resume",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
		AdmissionSummary:       core.AvailabilitySpec{Available: true},
	}
	first, refusal, err := svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-b")
	if err != nil || refusal != nil {
		t.Fatalf("first TryAcceptHandoff() err=%v refusal=%+v", err, refusal)
	}
	second, refusal, err := svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-b")
	if err != nil || refusal != nil {
		t.Fatalf("second TryAcceptHandoff() err=%v refusal=%+v", err, refusal)
	}
	if first.ProvisionalAttemptID != second.ProvisionalAttemptID {
		t.Fatalf("provisional attempt mismatch: %s vs %s", first.ProvisionalAttemptID, second.ProvisionalAttemptID)
	}
}

func TestTryAcceptHandoffRejectsConflictingDuplicateOffer(t *testing.T) {
	t.Parallel()

	store := &InMemoryOwnershipStore{}
	svc := &Service{Ownership: store}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	requireAttempt(t, store.CreateLineage(context.Background(), lineage))
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	requireAttempt(t, store.UpsertAttempt(context.Background(), attempt))
	lease, err := store.IssueLease(context.Background(), lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	offer := core.HandoffOffer{
		OfferID:            lease.LeaseID,
		LineageID:          lineage.LineageID,
		SourceAttemptID:    attempt.AttemptID,
		SourceRuntimeID:    attempt.RuntimeID,
		DestinationExport:  "agent.resume",
		ContextManifestRef: "ctx-1",
		ContextClass:       lineage.ContextClass,
		ContextSizeBytes:   64,
		LeaseToken:         *lease,
		Expiry:             lease.Expiry,
	}
	destination := core.ExportDescriptor{
		ExportName:             "agent.resume",
		AcceptedContextClasses: []string{"workflow-runtime"},
		RouteMode:              core.RouteModeGateway,
		AdmissionSummary:       core.AvailabilitySpec{Available: true},
	}
	if _, refusal, err := svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-b"); err != nil || refusal != nil {
		t.Fatalf("first TryAcceptHandoff() err=%v refusal=%+v", err, refusal)
	}
	var refusal *core.TransferRefusal
	_, refusal, err = svc.TryAcceptHandoff(context.Background(), offer, destination, "rt-c")
	if err != nil {
		t.Fatalf("second TryAcceptHandoff() error = %v", err)
	}
	if refusal == nil || refusal.Code != core.RefusalDuplicateHandoff {
		t.Fatalf("expected duplicate handoff refusal, got %+v", refusal)
	}
}

func requireAttempt(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForwardFederatedContextBlockedByForwardBudget(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	svc := &Service{
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Forwarder:  &fakeGatewayForwarder{},
		Limiter: &InMemoryOperationalLimiter{Limits: OperationalLimits{
			MaxForwardBytesWindow: 512,
		}},
		Now: func() time.Time { return now },
	}
	if err := svc.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{gateway},
		ExpiresAt:         now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if err := svc.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []core.SubjectRef{gateway},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway},
		RequireGatewayAuthentication: true,
		MaxTransferBytes:             4096,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}
	_, refusal, err := svc.ForwardFederatedContext(context.Background(), core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    gateway,
		DestinationExport:  "mesh://mesh.remote/agent.resume",
		OfferID:            "offer-1",
		RouteMode:          core.RouteModeGateway,
		SizeBytes:          1024,
		ContextManifestRef: "ctx-1",
		SealedContext: core.SealedContext{
			EnvelopeVersion:    "v1",
			ContextManifestRef: "ctx-1",
			CipherSuite:        "age",
			CiphertextChunks:   [][]byte{[]byte("opaque")},
			IntegrityTag:       "tag-1",
		},
	})
	if err != nil {
		t.Fatalf("ForwardFederatedContext() error = %v", err)
	}
	if refusal == nil || refusal.Code != core.RefusalTransferBudget {
		t.Fatalf("expected transfer budget refusal, got %+v", refusal)
	}
}

func TestRegisterTrustBundleEmitsTelemetryAndAudit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	telemetry := &telemetryRecorder{}
	audit := core.NewInMemoryAuditLogger(16)
	svc := &Service{
		Trust:     &InMemoryTrustBundleStore{},
		Telemetry: telemetry,
		Audit:     audit,
		Now:       func() time.Time { return now },
	}
	if err := svc.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain: "mesh.remote",
		BundleID:    "bundle-1",
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if len(telemetry.events) != 1 {
		t.Fatalf("expected telemetry event, got %d", len(telemetry.events))
	}
	if telemetry.events[0].Metadata["trust_domain"] != "mesh.remote" {
		t.Fatalf("telemetry metadata = %+v", telemetry.events[0].Metadata)
	}
	records, err := audit.Query(context.Background(), core.AuditQuery{Type: core.FrameworkEventFMPTrustRegistered})
	if err != nil {
		t.Fatalf("audit.Query() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	if records[0].Metadata["trust_domain"] != "mesh.remote" {
		t.Fatalf("audit metadata = %+v", records[0].Metadata)
	}
}

func TestBuildTransferDebugViewRedactsPayloadContents(t *testing.T) {
	t.Parallel()

	view := BuildTransferDebugView("mesh.remote", core.HandoffOffer{
		OfferID: "offer-1",
		LeaseToken: core.LeaseToken{
			LeaseID: "lease-1",
		},
	}, "rt-a", core.RouteModeGateway, core.ContextManifest{
		ContextID:        "ctx-1",
		LineageID:        "lineage-1",
		AttemptID:        "attempt-1",
		ContextClass:     "workflow-runtime",
		SchemaVersion:    "fmp.context.v1",
		SizeBytes:        128,
		ChunkCount:       2,
		SensitivityClass: core.SensitivityClassHigh,
		ObjectRefs:       []string{"obj-1", "obj-2"},
	}, core.SealedContext{
		EnvelopeVersion:    "v1",
		ContextManifestRef: "ctx-1",
		CipherSuite:        "age",
		RecipientBindings:  []string{"r1", "r2"},
		CiphertextChunks:   [][]byte{[]byte("secret-1"), []byte("secret-2")},
		ExternalObjectRefs: []string{"ref-1"},
		IntegrityTag:       "tag-1",
	})
	if view.Manifest.ObjectRefCount != 2 {
		t.Fatalf("manifest debug view = %+v", view.Manifest)
	}
	if view.Sealed.ChunkCount != 2 || view.Sealed.RecipientCount != 2 || !view.Sealed.HasIntegrityTag {
		t.Fatalf("sealed debug view = %+v", view.Sealed)
	}
}
