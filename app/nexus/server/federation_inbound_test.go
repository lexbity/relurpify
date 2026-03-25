package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
)

type fakeWorkflowRuntimeStore struct{}

func (fakeWorkflowRuntimeStore) QueryWorkflowRuntime(context.Context, string, string) (map[string]any, error) {
	return map[string]any{
		"status": "running",
		"step":   "inspect",
	}, nil
}

func TestFederationInboundHandlerDispatchesToRegisteredHandler(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{
		Trust:      &fwfmp.InMemoryTrustBundleStore{},
		Boundaries: &fwfmp.InMemoryBoundaryPolicyStore{},
		Forwarder:  fwfmp.NewHTTPGatewayForwarder(nil),
		Now:        func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}
	gateway := EnsureFederatedMeshGateway(mesh)
	remote := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	if err := mesh.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{remote},
		ExpiresAt:         time.Date(2026, 3, 23, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if err := mesh.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []core.SubjectRef{remote},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway},
		RequireGatewayAuthentication: true,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}
	called := false
	if err := gateway.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
		called = true
		return &core.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            true,
			ForwardedAt:       time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
		}, nil
	}); err != nil {
		t.Fatalf("RegisterExportHandler() error = %v", err)
	}

	reqBody, err := json.Marshal(core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    remote,
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
		t.Fatalf("Marshal: %v", err)
	}
	policy := fwgateway.DefaultFMPTransportPolicy(false)
	policy.Now = func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) }
	policy.NonceStore = &fwgateway.InMemoryTransportNonceStore{Now: policy.Now}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fwfmp.DefaultFederationForwardPath, bytes.NewReader(reqBody))
	req.Header.Set("X-FMP-Trust-Domain", "mesh.remote")
	req.Header.Set(fwgateway.HeaderFMPTransportProfile, fwgateway.TransportProfileHTTPTLS)
	req.Header.Set(fwgateway.HeaderFMPSessionNonce, "nonce-1")
	req.Header.Set(fwgateway.HeaderFMPSessionIssuedAt, time.Date(2026, 3, 23, 11, 59, 50, 0, time.UTC).Format(time.RFC3339Nano))
	req.Header.Set(fwgateway.HeaderFMPSessionExpiresAt, time.Date(2026, 3, 23, 12, 5, 0, 0, time.UTC).Format(time.RFC3339Nano))
	req.Header.Set(fwgateway.HeaderFMPPeerKeyID, "gw-1")
	req.TLS = &tls.ConnectionState{}
	FederationInboundHandler(mesh, policy).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected local federation handler to be called")
	}
}

func TestFederationInboundHandlerRejectsReplayNonce(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	mesh := &fwfmp.Service{
		Trust:      &fwfmp.InMemoryTrustBundleStore{},
		Boundaries: &fwfmp.InMemoryBoundaryPolicyStore{},
		Forwarder:  fwfmp.NewHTTPGatewayForwarder(nil),
		Now:        func() time.Time { return now },
	}
	gateway := EnsureFederatedMeshGateway(mesh)
	remote := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	if err := mesh.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{remote},
		IssuedAt:          now,
		ExpiresAt:         now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if err := mesh.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []core.SubjectRef{remote},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway},
		RequireGatewayAuthentication: true,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}
	if err := gateway.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
		return &core.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            true,
			ForwardedAt:       now,
		}, nil
	}); err != nil {
		t.Fatalf("RegisterExportHandler() error = %v", err)
	}
	policy := fwgateway.DefaultFMPTransportPolicy(false)
	policy.Now = func() time.Time { return now }
	policy.NonceStore = &fwgateway.InMemoryTransportNonceStore{Now: policy.Now}

	reqBody, err := json.Marshal(core.GatewayForwardRequest{
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
		t.Fatalf("Marshal: %v", err)
	}
	handler := FederationInboundHandler(mesh, policy)
	makeReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, fwfmp.DefaultFederationForwardPath, bytes.NewReader(reqBody))
		req.Header.Set("X-FMP-Trust-Domain", "mesh.remote")
		req.Header.Set(fwgateway.HeaderFMPTransportProfile, fwgateway.TransportProfileHTTPTLS)
		req.Header.Set(fwgateway.HeaderFMPSessionNonce, "nonce-1")
		req.Header.Set(fwgateway.HeaderFMPSessionIssuedAt, now.Add(-10*time.Second).Format(time.RFC3339Nano))
		req.Header.Set(fwgateway.HeaderFMPSessionExpiresAt, now.Add(5*time.Minute).Format(time.RFC3339Nano))
		req.Header.Set(fwgateway.HeaderFMPPeerKeyID, "gw-1")
		req.TLS = &tls.ConnectionState{}
		return req
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, makeReq())
	if first.Code == http.StatusForbidden {
		t.Fatalf("first request unexpectedly rejected: %s", first.Body.String())
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, makeReq())
	if second.Code != http.StatusForbidden {
		t.Fatalf("second status = %d body = %s", second.Code, second.Body.String())
	}
}

func TestFederationInboundHandlerMediatesAndResealsForLocalRuntime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	gatewayRecipient := fwfmp.QualifiedGatewayRecipient("mesh.remote", "node-gw")
	runtimeRecipient := "runtime://mesh.remote/rt-remote"
	resolver := fwfmp.StaticRecipientKeyResolver{
		gatewayRecipient: []byte("0123456789abcdef0123456789abcdef"),
		runtimeRecipient: []byte("abcdef0123456789abcdef0123456789"),
	}
	packager := fwfmp.JSONPackager{RuntimeStore: fakeWorkflowRuntimeStore{}, KeyResolver: resolver}
	lineage := core.LineageRecord{
		LineageID:        "lineage-1",
		TenantID:         "tenant-1",
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		SensitivityClass: core.SensitivityClassModerate,
		Owner:            core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-local",
		State:     core.AttemptStateRunning,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, fwfmp.RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{gatewayRecipient})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}

	mesh := &fwfmp.Service{
		Discovery:  &fwfmp.InMemoryDiscoveryStore{},
		Trust:      &fwfmp.InMemoryTrustBundleStore{},
		Boundaries: &fwfmp.InMemoryBoundaryPolicyStore{},
		Forwarder:  fwfmp.NewHTTPGatewayForwarder(nil),
		Now:        func() time.Time { return now },
		Audit:      core.NewInMemoryAuditLogger(16),
		Mediator: &fwfmp.MediationController{
			Packager:       packager,
			LocalRecipient: gatewayRecipient,
			Now:            func() time.Time { return now },
		},
	}
	if err := mesh.Discovery.UpsertRuntimeAdvertisement(context.Background(), core.RuntimeAdvertisement{
		TrustDomain: "mesh.remote",
		Runtime: core.RuntimeDescriptor{
			RuntimeID:      "rt-remote",
			NodeID:         "node-remote",
			RuntimeVersion: "1.0.0",
		},
	}); err != nil {
		t.Fatalf("UpsertRuntimeAdvertisement() error = %v", err)
	}
	if err := mesh.Discovery.UpsertExportAdvertisement(context.Background(), core.ExportAdvertisement{
		TrustDomain: "mesh.remote",
		RuntimeID:   "rt-remote",
		NodeID:      "node-remote",
		Export: core.ExportDescriptor{
			ExportName: "agent.resume",
			RouteMode:  core.RouteModeMediated,
		},
	}); err != nil {
		t.Fatalf("UpsertExportAdvertisement() error = %v", err)
	}
	gateway := EnsureFederatedMeshGateway(mesh)
	remote := core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}
	if err := mesh.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{remote},
		ExpiresAt:         now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("RegisterTrustBundle() error = %v", err)
	}
	if err := mesh.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []core.SubjectRef{remote},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeMediated},
		RequireGatewayAuthentication: true,
		AllowMediation:               true,
	}); err != nil {
		t.Fatalf("SetBoundaryPolicy() error = %v", err)
	}
	called := false
	if err := gateway.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
		called = true
		if len(req.SealedContext.RecipientBindings) != 1 || req.SealedContext.RecipientBindings[0] != runtimeRecipient {
			t.Fatalf("recipient bindings = %+v, want %s", req.SealedContext.RecipientBindings, runtimeRecipient)
		}
		var out fwfmp.PortableContextPackage
		if err := (fwfmp.JSONPackager{KeyResolver: resolver, LocalRecipient: runtimeRecipient}).UnsealPackage(context.Background(), req.SealedContext, &out); err != nil {
			t.Fatalf("UnsealPackage(runtime) error = %v", err)
		}
		if string(out.ExecutionPayload) != string(pkg.ExecutionPayload) {
			t.Fatal("mediated payload mismatch")
		}
		return &core.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            false,
			ForwardedAt:       now,
		}, nil
	}); err != nil {
		t.Fatalf("RegisterExportHandler() error = %v", err)
	}

	reqBody, err := json.Marshal(core.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    remote,
		DestinationExport:  "mesh://mesh.remote/agent.resume",
		RouteMode:          core.RouteModeMediated,
		MediationRequested: true,
		SizeBytes:          pkg.Manifest.SizeBytes,
		ContextManifestRef: pkg.Manifest.ContextID,
		SealedContext:      *sealed,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	policy := fwgateway.DefaultFMPTransportPolicy(false)
	policy.Now = func() time.Time { return now }
	policy.NonceStore = &fwgateway.InMemoryTransportNonceStore{Now: policy.Now}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fwfmp.DefaultFederationForwardPath, bytes.NewReader(reqBody))
	req.Header.Set("X-FMP-Trust-Domain", "mesh.remote")
	req.Header.Set(fwgateway.HeaderFMPTransportProfile, fwgateway.TransportProfileHTTPTLS)
	req.Header.Set(fwgateway.HeaderFMPSessionNonce, "nonce-mediate")
	req.Header.Set(fwgateway.HeaderFMPSessionIssuedAt, now.Add(-10*time.Second).Format(time.RFC3339Nano))
	req.Header.Set(fwgateway.HeaderFMPSessionExpiresAt, now.Add(5*time.Minute).Format(time.RFC3339Nano))
	req.Header.Set(fwgateway.HeaderFMPPeerKeyID, "gw-1")
	req.TLS = &tls.ConnectionState{}
	FederationInboundHandler(mesh, policy).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected local mediated federation handler to be called")
	}
}
