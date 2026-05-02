package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

func TestEnsureFederatedMeshGatewayInstallsHTTPForwarder(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{}
	gateway := EnsureFederatedMeshGateway(mesh)
	if gateway == nil {
		t.Fatal("EnsureFederatedMeshGateway() returned nil")
	}
	if gateway.Forwarder == nil {
		t.Fatal("EnsureFederatedMeshGateway() did not install forwarder")
	}
	if _, ok := mesh.Forwarder.(*fwfmp.HTTPGatewayForwarder); !ok {
		t.Fatalf("mesh forwarder type = %T, want *fwfmp.HTTPGatewayForwarder", mesh.Forwarder)
	}
}

func TestFederatedMeshGatewayRegistersHandlerAndForwards(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{}
	gateway := EnsureFederatedMeshGateway(mesh)
	called := false
	if err := gateway.RegisterExportHandler("mesh.remote", "agent.resume", func(_ context.Context, req fwfmp.GatewayForwardRequest) (*fwfmp.GatewayForwardResult, error) {
		called = true
		return &fwfmp.GatewayForwardResult{
			TrustDomain:       req.TrustDomain,
			DestinationExport: req.DestinationExport,
			RouteMode:         req.RouteMode,
			Opaque:            true,
		}, nil
	}); err != nil {
		t.Fatalf("RegisterExportHandler() error = %v", err)
	}
	result, err := gateway.Forwarder.ForwardSealedContext(context.Background(), fwfmp.GatewayForwardRequest{
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.remote",
		GatewayIdentity:    identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "gw-1"},
		DestinationExport:  "mesh://mesh.remote/agent.resume",
		RouteMode:          fwfmp.RouteModeGateway,
		SizeBytes:          128,
		ContextManifestRef: "ctx-1",
		SealedContext: fwfmp.SealedContext{
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

func TestHTTPGatewayForwarderPostsToRemoteFederationEndpoint(t *testing.T) {
	t.Parallel()

	var got fwfmp.GatewayForwardRequest
	forwarder := fwfmp.NewHTTPGatewayForwarder(fwfmp.StaticFederationEndpointResolver{
		"mesh.remote": "https://mesh.remote.test",
	})
	forwarder.TransportPolicy = fwgateway.DefaultFMPTransportPolicy(false)
	forwarder.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != fwfmp.DefaultFederationForwardPath {
			t.Fatalf("path = %s", req.URL.Path)
		}
		if got := req.Header.Get("X-FMP-Trust-Domain"); got != "mesh.remote" {
			t.Fatalf("trust domain header = %q", got)
		}
		if got := req.Header.Get(fwgateway.HeaderFMPTransportProfile); got != fwgateway.TransportProfileHTTPTLS {
			t.Fatalf("transport profile = %q", got)
		}
		if req.Header.Get(fwgateway.HeaderFMPSessionNonce) == "" {
			t.Fatal("missing transport nonce header")
		}
		if got := req.Header.Get(fwgateway.HeaderFMPPeerKeyID); got != "gw-local" {
			t.Fatalf("peer key id = %q", got)
		}
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		body, err := json.Marshal(fwfmp.GatewayForwardTransportResponse{
			Result: &fwfmp.GatewayForwardResult{
				TrustDomain:       got.TrustDomain,
				DestinationExport: got.DestinationExport,
				RouteMode:         got.RouteMode,
				Opaque:            true,
				ForwardedAt:       time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
			},
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}
	result, err := forwarder.ForwardSealedContext(context.Background(), fwfmp.GatewayForwardRequest{
		TenantID:           "tenant-1",
		TrustDomain:        "mesh.remote",
		SourceDomain:       "mesh.local",
		GatewayIdentity:    identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "gw-local"},
		DestinationExport:  "mesh://mesh.remote/agent.resume",
		RouteMode:          fwfmp.RouteModeGateway,
		SizeBytes:          128,
		ContextManifestRef: "ctx-1",
		SealedContext: fwfmp.SealedContext{
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
	if result == nil || got.TrustDomain != "mesh.remote" {
		t.Fatalf("result = %+v got = %+v", result, got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
