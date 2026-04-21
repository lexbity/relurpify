package server

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/app/nexus/adapters/webchat"
	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex"
	rexconfig "codeburg.org/lexbit/relurpify/named/rex/config"
	rexdelegates "codeburg.org/lexbit/relurpify/named/rex/delegates"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
	"github.com/stretchr/testify/require"
)

type fakeCallableRuntime struct {
	caps        []core.Capability
	projection  rexnexus.Projection
	executeResp *core.Result
	executeErr  error
	lastTask    *core.Task
	lastState   *core.Context
}

func (f *fakeCallableRuntime) Execute(_ context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	f.lastTask = task
	f.lastState = state
	if f.executeResp == nil {
		f.executeResp = &core.Result{Success: true, Data: map[string]any{"ok": true}}
	}
	return f.executeResp, f.executeErr
}

func (f *fakeCallableRuntime) RuntimeProjection() rexnexus.Projection {
	return f.projection
}

func (f *fakeCallableRuntime) Capabilities() []core.Capability {
	return append([]core.Capability(nil), f.caps...)
}

type fakeTrustStore struct {
	bundles map[string]core.TrustBundle
}

func (f *fakeTrustStore) UpsertTrustBundle(_ context.Context, bundle core.TrustBundle) error {
	if f.bundles == nil {
		f.bundles = map[string]core.TrustBundle{}
	}
	f.bundles[bundle.TrustDomain] = bundle
	return nil
}

func (f *fakeTrustStore) GetTrustBundle(_ context.Context, trustDomain string) (*core.TrustBundle, error) {
	if bundle, ok := f.bundles[trustDomain]; ok {
		copy := bundle
		return &copy, nil
	}
	return nil, nil
}

func TestServerPhase4HandlerAndAdapterHelpers(t *testing.T) {
	t.Parallel()

	t.Run("handler validation", func(t *testing.T) {
		t.Parallel()

		_, err := (&NexusApp{}).Handler(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "event log required")
	})

	t.Run("transport policy and adapter registration", func(t *testing.T) {
		t.Parallel()

		app := &NexusApp{Config: nexuscfg.Config{Gateway: nexuscfg.GatewayConfig{Bind: ":8080"}}}
		require.NotNil(t, app.fmpTransportPolicy())
		require.Same(t, app.FMPTransportPolicy, app.fmpTransportPolicy())

		manager := channel.NewManager(nil, nil)
		cfg := nexuscfg.Config{
			Channels: map[string]map[string]any{
				"webchat":  {"enabled": true},
				"telegram": {"enabled": true},
				"discord":  {"enabled": true},
			},
		}
		require.NoError(t, registerConfiguredAdapters(manager, cfg, &webchat.Adapter{}))
		require.Len(t, manager.Status(), 3)
		require.Error(t, manager.Register(&webchat.Adapter{}))

		raw := channelConfigs(nexuscfg.Config{Channels: map[string]map[string]any{
			"webchat": {"enabled": true},
			"bad":     {"enabled": func() {}},
		}})
		require.Contains(t, raw, "webchat")
		require.NotContains(t, raw, "bad")
		require.Nil(t, channelConfigs(nexuscfg.Config{}))

		require.True(t, enabled(nil, "missing", true))
		require.False(t, enabled(map[string]map[string]any{"webchat": {"enabled": false}}, "webchat", true))
	})

	t.Run("federated mesh gateway", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, EnsureFederatedMeshGateway(nil))

		mesh := &fwfmp.Service{}
		gateway := EnsureFederatedMeshGateway(mesh)
		require.NotNil(t, gateway)
		require.IsType(t, &fwfmp.HTTPGatewayForwarder{}, mesh.Forwarder)

		require.Error(t, (&FederatedMeshGateway{}).RegisterExportHandler("mesh.remote", "agent.resume", func(context.Context, core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
			return nil, nil
		}))
		require.Error(t, (&FederatedMeshGateway{}).ImportAdvertisements(context.Background(), core.SubjectRef{}, nil, "mesh.remote"))
		_, _, err := (&FederatedMeshGateway{}).ForwardSealedContext(context.Background(), core.GatewayForwardRequest{})
		require.Error(t, err)

		require.NoError(t, gateway.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req core.GatewayForwardRequest) (*core.GatewayForwardResult, error) {
			return &core.GatewayForwardResult{TrustDomain: req.TrustDomain, DestinationExport: req.DestinationExport, RouteMode: req.RouteMode, Opaque: true}, nil
		}))
		result, refusal, err := gateway.ForwardSealedContext(context.Background(), core.GatewayForwardRequest{
			TenantID:           "tenant-a",
			LineageID:          "lineage-1",
			TrustDomain:        "mesh.remote",
			SourceDomain:       "mesh.local",
			GatewayIdentity:    core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "gw-1"},
			DestinationExport:  "mesh://mesh.remote/agent.resume",
			RouteMode:          core.RouteModeGateway,
			ContextManifestRef: "ctx-1",
			SealedContext: core.SealedContext{
				EnvelopeVersion:    "v1",
				ContextManifestRef: "ctx-1",
				CipherSuite:        "age",
				CiphertextChunks:   [][]byte{[]byte("opaque")},
				IntegrityTag:       "tag-1",
			},
		})
		require.NoError(t, err)
		if result == nil {
			require.NotNil(t, refusal)
		} else {
			require.True(t, result.Opaque)
		}
	})
}

func TestServerPhase4RexRuntimeProviderHelpers(t *testing.T) {
	t.Parallel()

	provider := &RexRuntimeProvider{}
	require.Equal(t, rexnexus.Registration{}, provider.Registration())
	_, err := provider.AdminSnapshot(context.Background())
	require.Error(t, err)
	_, err = provider.InvokeCapability(context.Background(), "", "", map[string]any{"instruction": "do work"})
	require.Error(t, err)
	_, _, err = provider.ReadSLOSignals(context.Background())
	require.Error(t, err)

	fakeRuntime := &fakeCallableRuntime{
		caps: []core.Capability{core.CapabilityExecute},
		projection: rexnexus.Projection{
			Health:     rexruntime.HealthHealthy,
			WorkflowID: "wf-1",
			RunID:      "run-1",
		},
	}
	provider = &RexRuntimeProvider{
		Adapter:       rexnexus.NewAdapter("rex", fakeRuntime, nil),
		WorkflowStore: nil,
	}

	registrations := provider.Registration()
	require.Equal(t, "rex", registrations.Name)
	require.Len(t, registrations.Capabilities, 1)

	snapshot, err := provider.AdminSnapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, rexruntime.HealthHealthy, snapshot.Runtime.Health)

	result, err := provider.InvokeCapability(context.Background(), "sess-1", "tenant-a", map[string]any{
		"instruction": "build it",
		"task_id":     "task-1",
		"workflow_id": "wf-1",
		"run_id":      "run-1",
		"context":     map[string]any{"a": 1},
		"metadata":    map[string]any{"b": "2"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "sess-1", fakeRuntime.lastState.GetString("gateway.session_id"))
	require.Equal(t, "tenant-a", fakeRuntime.lastState.GetString("gateway.tenant_id"))
	require.Equal(t, "task-1", fakeRuntime.lastTask.ID)

	_, err = provider.InvokeCapability(context.Background(), "", "", map[string]any{})
	require.Error(t, err)

	signerStore := &fakeTrustStore{}
	fmpService := &fwfmp.Service{Trust: signerStore}
	require.NoError(t, provider.PublishFMPTrustBundle(context.Background(), fmpService))
	bundle, err := signerStore.GetTrustBundle(context.Background(), "local")
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.Len(t, bundle.RecipientKeys, 2)
	require.True(t, bundle.Validate() == nil)

	require.NoError(t, provider.PublishFMPTrustBundle(context.Background(), nil))

	agent := &rex.Agent{
		Runtime:   rexruntime.New(rexconfig.Default(), nil),
		Delegates: &rexdelegates.Registry{},
	}
	workItem := rexnexusWorkItem("wf-1", "run-1", &core.Task{ID: "task-1"}, core.NewContext(), agent)
	require.Error(t, workItem.Execute(context.Background(), workItem))
}

func TestServerPhase4NodeRuntimeHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, "runtime-1", fallbackNodeRuntimeID(core.NodeDescriptor{ID: "node-1"}, fwgateway.NodeConnectInfo{RuntimeID: "runtime-1"}))
	require.Equal(t, "node-1:default", fallbackNodeRuntimeID(core.NodeDescriptor{ID: "node-1"}, fwgateway.NodeConnectInfo{}))
	require.Equal(t, "0.0.0", fallbackRuntimeVersion(fwgateway.NodeConnectInfo{}))
	require.Equal(t, "default", fallbackCompatibilityClass(fwgateway.NodeConnectInfo{}))
	require.Equal(t, "nexus-register:node-1:runtime-1:1.2.3:peer-1:http.tls.v1", runtimeRegistrationSignatureFromValues("node-1", "runtime-1", "1.2.3", "peer-1", "http.tls.v1"))
	require.Equal(t, map[string]string{"x": "1"}, copyNodeTags(map[string]string{"x": "1"}))
	require.Nil(t, copyNodeTags(nil))

	store := &serverNodeStore{
		nodes: map[string]core.NodeDescriptor{
			"node-1": {ID: "node-1", ApprovedCapabilities: []core.CapabilityDescriptor{{ID: "cap-1"}}},
		},
	}
	manager := &fwnode.Manager{Store: store}
	require.Len(t, connectedNodeCapabilities(context.Background(), manager, "node-1"), 1)
	require.Nil(t, connectedNodeCapabilities(context.Background(), manager, ""))
	require.Nil(t, ConnectedNodeCapabilitiesForTest(context.Background(), manager, ""))
	require.Len(t, ListNodeCapabilities(manager, fwgateway.ConnectionPrincipal{Actor: core.EventActor{TenantID: "tenant-a"}}), 0)
	require.Equal(t, "tenant-a", NormalizeTenantID("tenant-a"))
	require.Equal(t, DefaultTenantID, NormalizeTenantID(""))

	require.Equal(t, "session store unavailable", mustErrString(t, func() error {
		_, err := InvokeAuthorizedNodeCapability(context.Background(), nil, nil, manager, fwgateway.ConnectionPrincipal{}, "sess-1", "cap-1", nil)
		return err
	}))

	router := &stubSessionRouter{}
	sessionStore := &stubSessionStore{boundary: &core.SessionBoundary{SessionID: "sess-1"}}
	principal := fwgateway.ConnectionPrincipal{Authenticated: true, Actor: core.EventActor{TenantID: "tenant-a"}}
	_, err := InvokeAuthorizedNodeCapability(context.Background(), router, sessionStore, manager, principal, "sess-1", "cap-1", map[string]any{"x": 1})
	require.Error(t, err)
	require.True(t, router.authorized)
}
