package fmp

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type staleButLiveDiscovery struct {
	exports  []core.ExportAdvertisement
	runtimes []core.RuntimeAdvertisement
	nodes    []core.NodeAdvertisement
}

func (s *staleButLiveDiscovery) UpsertNodeAdvertisement(context.Context, core.NodeAdvertisement) error {
	return nil
}

func (s *staleButLiveDiscovery) UpsertRuntimeAdvertisement(context.Context, core.RuntimeAdvertisement) error {
	return nil
}

func (s *staleButLiveDiscovery) UpsertExportAdvertisement(context.Context, core.ExportAdvertisement) error {
	return nil
}

func (s *staleButLiveDiscovery) ListNodeAdvertisements(context.Context) ([]core.NodeAdvertisement, error) {
	return append([]core.NodeAdvertisement(nil), s.nodes...), nil
}

func (s *staleButLiveDiscovery) ListRuntimeAdvertisements(context.Context) ([]core.RuntimeAdvertisement, error) {
	return append([]core.RuntimeAdvertisement(nil), s.runtimes...), nil
}

func (s *staleButLiveDiscovery) ListExportAdvertisements(context.Context) ([]core.ExportAdvertisement, error) {
	return append([]core.ExportAdvertisement(nil), s.exports...), nil
}

func (s *staleButLiveDiscovery) DeleteExpired(context.Context, time.Time) error {
	return nil
}

func (s *staleButLiveDiscovery) ListLiveNodeAdvertisements(_ context.Context, now time.Time) ([]core.NodeAdvertisement, error) {
	var out []core.NodeAdvertisement
	for _, ad := range s.nodes {
		if ad.ExpiresAt.IsZero() || now.Before(ad.ExpiresAt) {
			out = append(out, ad)
		}
	}
	return out, nil
}

func (s *staleButLiveDiscovery) ListLiveRuntimeAdvertisements(_ context.Context, now time.Time) ([]core.RuntimeAdvertisement, error) {
	var out []core.RuntimeAdvertisement
	for _, ad := range s.runtimes {
		if ad.ExpiresAt.IsZero() || now.Before(ad.ExpiresAt) {
			out = append(out, ad)
		}
	}
	return out, nil
}

func (s *staleButLiveDiscovery) ListLiveExportAdvertisements(_ context.Context, now time.Time) ([]core.ExportAdvertisement, error) {
	var out []core.ExportAdvertisement
	for _, ad := range s.exports {
		if ad.ExpiresAt.IsZero() || now.Before(ad.ExpiresAt) {
			out = append(out, ad)
		}
	}
	return out, nil
}

func TestResolveRoutesUsesLiveDiscoveryViewWhenDeleteExpiredIsNoOp(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	discovery := &staleButLiveDiscovery{
		runtimes: []core.RuntimeAdvertisement{
			{
				TrustDomain: "mesh.local",
				Runtime: core.RuntimeDescriptor{
					RuntimeID:               "rt-expired",
					NodeID:                  "node-1",
					RuntimeVersion:          "1.0.0",
					CompatibilityClass:      "compat-a",
					SupportedContextClasses: []string{"workflow-runtime"},
					MaxContextSize:          2048,
				},
				ExpiresAt: now.Add(-time.Minute),
			},
			{
				TrustDomain: "mesh.local",
				Runtime: core.RuntimeDescriptor{
					RuntimeID:               "rt-live",
					NodeID:                  "node-2",
					RuntimeVersion:          "1.0.0",
					CompatibilityClass:      "compat-a",
					SupportedContextClasses: []string{"workflow-runtime"},
					MaxContextSize:          2048,
				},
				ExpiresAt: now.Add(time.Minute),
			},
		},
		exports: []core.ExportAdvertisement{
			{
				TrustDomain: "mesh.local",
				RuntimeID:   "rt-expired",
				NodeID:      "node-1",
				Export: core.ExportDescriptor{
					ExportName:             "agent.resume",
					AcceptedContextClasses: []string{"workflow-runtime"},
					RouteMode:              core.RouteModeGateway,
					MaxContextSize:         2048,
					AdmissionSummary:       core.AvailabilitySpec{Available: true},
				},
				ExpiresAt: now.Add(-time.Minute),
			},
			{
				TrustDomain: "mesh.local",
				RuntimeID:   "rt-live",
				NodeID:      "node-2",
				Export: core.ExportDescriptor{
					ExportName:             "agent.resume",
					AcceptedContextClasses: []string{"workflow-runtime"},
					RouteMode:              core.RouteModeGateway,
					MaxContextSize:         2048,
					AdmissionSummary:       core.AvailabilitySpec{Available: true},
				},
				ExpiresAt: now.Add(time.Minute),
			},
		},
	}
	svc := &Service{
		Discovery: discovery,
		Now:       func() time.Time { return now },
	}

	routes, err := svc.ResolveRoutes(context.Background(), RouteSelectionRequest{
		ExportName:       "agent.resume",
		ContextClass:     "workflow-runtime",
		ContextSizeBytes: 256,
	})
	if err != nil {
		t.Fatalf("ResolveRoutes() error = %v", err)
	}
	if len(routes) != 1 || routes[0].RuntimeID != "rt-live" {
		t.Fatalf("routes = %+v", routes)
	}
}

func TestResolveDestinationRuntimeRecipientUsesLiveDiscoveryView(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		Discovery: &staleButLiveDiscovery{
			exports: []core.ExportAdvertisement{
				{
					TrustDomain: "mesh.remote",
					RuntimeID:   "rt-expired",
					NodeID:      "node-1",
					Export:      core.ExportDescriptor{ExportName: "agent.resume"},
					ExpiresAt:   now.Add(-time.Minute),
				},
				{
					TrustDomain: "mesh.remote",
					RuntimeID:   "rt-live",
					NodeID:      "node-2",
					Export:      core.ExportDescriptor{ExportName: "agent.resume"},
					ExpiresAt:   now.Add(time.Minute),
				},
			},
		},
		Now: func() time.Time { return now },
	}

	recipient, err := svc.resolveDestinationRuntimeRecipient(context.Background(), core.GatewayForwardRequest{
		TrustDomain:       "mesh.remote",
		DestinationExport: "mesh://mesh.remote/agent.resume",
	})
	if err != nil {
		t.Fatalf("resolveDestinationRuntimeRecipient() error = %v", err)
	}
	if recipient != "runtime://mesh.remote/rt-live" {
		t.Fatalf("recipient = %s", recipient)
	}
}
