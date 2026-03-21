package fmp

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type fakeGatewayForwarder struct {
	lastRequest core.GatewayForwardRequest
	result      *core.GatewayForwardResult
}

func (f *fakeGatewayForwarder) ForwardSealedContext(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
	f.lastRequest = req
	if f.result != nil {
		return f.result, nil
	}
	return nil, nil
}

func TestImportFederatedExportAdvertisementRequiresTrustedGateway(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery:  &InMemoryDiscoveryStore{},
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Now:        func() time.Time { return now },
	}
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
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
	if err := svc.ImportFederatedRuntimeAdvertisement(context.Background(), gateway, core.RuntimeAdvertisement{
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
			Signature:               "sig-remote",
		},
		Signature: "sig-remote",
	}, "mesh.remote"); err != nil {
		t.Fatalf("ImportFederatedRuntimeAdvertisement() error = %v", err)
	}
	if err := svc.ImportFederatedExportAdvertisement(context.Background(), gateway, core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		Export: core.ExportDescriptor{
			ExportName:             "agent.resume",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
			MaxContextSize:         2048,
		},
		RuntimeID: "rt-remote",
		NodeID:    "node-remote",
	}, "mesh.remote"); err != nil {
		t.Fatalf("ImportFederatedExportAdvertisement() error = %v", err)
	}
	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:       "mesh://mesh.remote/agent.resume",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 512,
		AllowRemote:      true,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 1 || !routes[0].Imported || routes[0].TrustDomain != "mesh.remote" {
		t.Fatalf("unexpected routes = %+v", routes)
	}
}

func TestImportFederatedExportAdvertisementRejectsUntrustedMesh(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery:  &InMemoryDiscoveryStore{},
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Now:        func() time.Time { return now },
	}
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	err := svc.ImportFederatedExportAdvertisement(context.Background(), gateway, core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		Export: core.ExportDescriptor{
			ExportName:             "agent.resume",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
		},
	}, "mesh.remote")
	if err == nil {
		t.Fatal("expected untrusted mesh import to fail")
	}
}

func TestImportFederatedRuntimeAdvertisementRejectsMissingAttestation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery:  &InMemoryDiscoveryStore{},
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Now:        func() time.Time { return now },
	}
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	if err := svc.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{gateway},
		ExpiresAt:         now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	err := svc.ImportFederatedRuntimeAdvertisement(context.Background(), gateway, core.RuntimeAdvertisement{
		TrustDomain: "mesh.remote",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:      "rt-remote",
			NodeID:         "node-remote",
			RuntimeVersion: "1.0.0",
		},
	}, "mesh.remote")
	if err == nil {
		t.Fatal("expected federated runtime import to fail without attestation")
	}
}

func TestImportFederatedExportAdvertisementRequiresRegisteredRuntime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery:  &InMemoryDiscoveryStore{},
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Now:        func() time.Time { return now },
	}
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
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
	err := svc.ImportFederatedExportAdvertisement(context.Background(), gateway, core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		RuntimeID:   "rt-missing",
		NodeID:      "node-remote",
		Export: core.ExportDescriptor{
			ExportName:             "agent.resume",
			AcceptedContextClasses: []string{"workflow-runtime"},
			RouteMode:              core.RouteModeGateway,
		},
	}, "mesh.remote")
	if err == nil {
		t.Fatal("expected federated export import to fail without registered runtime")
	}
}

func TestForwardFederatedContextDefaultsToOpaqueGatewayPassThrough(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	forwarder := &fakeGatewayForwarder{}
	svc := &Service{
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Forwarder:  forwarder,
		Now:        func() time.Time { return now },
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
	result, refusal, err := svc.ForwardFederatedContext(context.Background(), core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    gateway,
		DestinationExport:  "mesh://mesh.remote/agent.resume",
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
	if refusal != nil {
		t.Fatalf("ForwardFederatedContext() refusal = %+v", refusal)
	}
	if result == nil || !result.Opaque || result.RouteMode != core.RouteModeGateway {
		t.Fatalf("unexpected forward result = %+v", result)
	}
	if forwarder.lastRequest.ContextManifestRef != "ctx-1" {
		t.Fatalf("forwarder request = %+v", forwarder.lastRequest)
	}
}

func TestForwardFederatedContextRejectsMediationUnlessAllowed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	svc := &Service{
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Forwarder:  &fakeGatewayForwarder{},
		Now:        func() time.Time { return now },
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
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway, core.RouteModeMediated},
		RequireGatewayAuthentication: true,
		MaxTransferBytes:             4096,
		AllowMediation:               false,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}
	_, refusal, err := svc.ForwardFederatedContext(context.Background(), core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    gateway,
		DestinationExport:  "mesh://mesh.remote/agent.resume",
		RouteMode:          core.RouteModeMediated,
		MediationRequested: true,
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
	if refusal == nil || refusal.Code != core.RefusalUnauthorized {
		t.Fatalf("expected mediation refusal, got %+v", refusal)
	}
}

func TestLocalGatewayForwarderDispatchesRegisteredExportHandler(t *testing.T) {
	t.Parallel()

	forwarder := NewLocalGatewayForwarder()
	called := false
	if err := forwarder.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
		called = true
		return &core.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            false,
			ForwardedAt:       time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		}, nil
	}); err != nil {
		t.Fatalf("RegisterExportHandler() error = %v", err)
	}
	result, err := forwarder.ForwardSealedContext(context.Background(), core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"},
		DestinationExport:  "mesh://mesh.remote/agent.resume",
		RouteMode:          core.RouteModeGateway,
		SizeBytes:          128,
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
		t.Fatalf("ForwardSealedContext() error = %v", err)
	}
	if !called {
		t.Fatal("registered handler was not called")
	}
	if result == nil || result.DestinationExport != "mesh://mesh.remote/agent.resume" {
		t.Fatalf("unexpected result = %+v", result)
	}
}

func TestForwardFederatedContextRejectsTenantDisallowedTrustDomain(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	svc := &Service{
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Forwarder:  &fakeGatewayForwarder{},
		Nexus: NexusAdapter{
			Federation: fakeTenantFederationLookup{policies: map[string]core.TenantFederationPolicy{
				"tenant-1": {
					TenantID:            "tenant-1",
					AllowedTrustDomains: []string{"mesh.allowed"},
				},
			}},
		},
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
		RouteMode:          core.RouteModeGateway,
		SizeBytes:          128,
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
	if refusal == nil || refusal.Code != core.RefusalUnauthorized {
		t.Fatalf("expected tenant trust-domain refusal, got %+v", refusal)
	}
}

func TestForwardFederatedContextRejectsTenantTransferBudget(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	gateway := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	svc := &Service{
		Trust:      &InMemoryTrustBundleStore{},
		Boundaries: &InMemoryBoundaryPolicyStore{},
		Forwarder:  &fakeGatewayForwarder{},
		Nexus: NexusAdapter{
			Federation: fakeTenantFederationLookup{policies: map[string]core.TenantFederationPolicy{
				"tenant-1": {
					TenantID:          "tenant-1",
					AllowedRouteModes: []core.RouteMode{core.RouteModeGateway},
					MaxTransferBytes:  64,
				},
			}},
		},
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
		RouteMode:          core.RouteModeGateway,
		SizeBytes:          128,
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
		t.Fatalf("expected tenant transfer budget refusal, got %+v", refusal)
	}
}
