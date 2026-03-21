package server

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
)

func TestEnsureFederatedMeshGatewayInstallsLocalForwarder(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{}
	gateway := EnsureFederatedMeshGateway(mesh)
	if gateway == nil {
		t.Fatal("EnsureFederatedMeshGateway() returned nil")
	}
	if gateway.Forwarder == nil {
		t.Fatal("EnsureFederatedMeshGateway() did not install forwarder")
	}
	if _, ok := mesh.Forwarder.(*fwfmp.LocalGatewayForwarder); !ok {
		t.Fatalf("mesh forwarder type = %T, want *fwfmp.LocalGatewayForwarder", mesh.Forwarder)
	}
}

func TestFederatedMeshGatewayRegistersHandlerAndForwards(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{}
	gateway := EnsureFederatedMeshGateway(mesh)
	called := false
	if err := gateway.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
		called = true
		return &core.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            true,
		}, nil
	}); err != nil {
		t.Fatalf("RegisterExportHandler() error = %v", err)
	}
	result, err := gateway.Forwarder.ForwardSealedContext(context.Background(), core.GatewayForwardRequest{
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
		t.Fatal("registered federation handler was not called")
	}
	if result == nil || result.DestinationExport != "mesh://mesh.remote/agent.resume" {
		t.Fatalf("unexpected result = %+v", result)
	}
}
