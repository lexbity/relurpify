package server

import (
	"context"
	"path/filepath"
	"testing"

	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
)

func TestNexusAppEnsuresFMPPersistenceAndForwardResolver(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	app := &NexusApp{
		Workspace:  workspace,
		FMPService: &fwfmp.Service{},
		Config: nexuscfg.Config{
			Gateway: nexuscfg.GatewayConfig{
				Federation: nexuscfg.GatewayFederationConfig{
					Endpoints: map[string]string{"mesh.remote": "https://mesh.remote.test"},
				},
			},
		},
	}
	if err := app.ensureFMPPersistence(); err != nil {
		t.Fatalf("ensureFMPPersistence() error = %v", err)
	}
	if err := app.ensureFMPTransportPolicy(); err != nil {
		t.Fatalf("ensureFMPTransportPolicy() error = %v", err)
	}
	if app.FMPService.Trust == nil || app.FMPService.Boundaries == nil {
		t.Fatalf("fmp persistence not installed: %+v", app.FMPService)
	}
	if _, ok := app.fmpTransportPolicy().NonceStore.(*fwgateway.InMemoryTransportNonceStore); ok {
		t.Fatal("expected persistent transport nonce store")
	}
	gateway := EnsureFederatedMeshGateway(app.FMPService)
	gateway.Forwarder.Resolver = fwfmp.StaticFederationEndpointResolver(app.Config.Gateway.Federation.Endpoints)
	gateway.Forwarder.TransportPolicy = app.fmpTransportPolicy()
	endpoint, ok := gateway.Forwarder.Resolver.ResolveFederationEndpoint(context.Background(), "mesh.remote")
	if !ok || endpoint != "https://mesh.remote.test" {
		t.Fatalf("endpoint = %q ok=%v", endpoint, ok)
	}
	if _, err := filepath.Abs(workspace); err != nil {
		t.Fatalf("Abs: %v", err)
	}
	_ = core.PolicyDecisionAllow("ok")
}

func TestRexRuntimePublishesLocalTrustBundle(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	signer := fwfmp.NewEd25519SignerFromSeed([]byte("nexus-app-local-bundle"))
	mesh := &fwfmp.Service{
		Trust:  &fwfmp.InMemoryTrustBundleStore{},
		Signer: signer,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rexProvider, err := NewRexRuntimeProvider(ctx, workspace)
	if err != nil {
		t.Fatalf("NewRexRuntimeProvider() error = %v", err)
	}
	defer rexProvider.Close()
	rexProvider.AttachFMPService(mesh)
	if err := rexProvider.PublishFMPTrustBundle(ctx, mesh); err != nil {
		t.Fatalf("PublishFMPTrustBundle() error = %v", err)
	}
	bundle, err := mesh.Trust.GetTrustBundle(context.Background(), "local")
	if err != nil {
		t.Fatalf("GetTrustBundle() error = %v", err)
	}
	if bundle == nil {
		t.Fatal("expected local trust bundle")
	}
	if len(bundle.RecipientKeys) < 2 {
		t.Fatalf("recipient keys = %+v", bundle.RecipientKeys)
	}
}
