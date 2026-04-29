package server

import (
	"context"
	"fmt"

	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

type FederatedMeshGateway struct {
	Mesh      *fwfmp.Service
	Forwarder *fwfmp.HTTPGatewayForwarder
}

func EnsureFederatedMeshGateway(mesh *fwfmp.Service) *FederatedMeshGateway {
	if mesh == nil {
		return nil
	}
	forwarder, ok := mesh.Forwarder.(*fwfmp.HTTPGatewayForwarder)
	if !ok || forwarder == nil {
		forwarder = fwfmp.NewHTTPGatewayForwarder(nil)
		mesh.Forwarder = forwarder
	}
	return &FederatedMeshGateway{
		Mesh:      mesh,
		Forwarder: forwarder,
	}
}

func (g *FederatedMeshGateway) RegisterExportHandler(trustDomain, exportName string, handler fwfmp.FederatedExportHandler) error {
	if g == nil || g.Forwarder == nil {
		return fmt.Errorf("federated mesh gateway unavailable")
	}
	return g.Forwarder.RegisterExportHandler(trustDomain, exportName, handler)
}

func (g *FederatedMeshGateway) ImportAdvertisements(ctx context.Context, gateway identity.SubjectRef, ads []fwfmp.ExportAdvertisement, sourceDomain string) error {
	if g == nil || g.Mesh == nil {
		return fmt.Errorf("federated mesh gateway unavailable")
	}
	for _, ad := range ads {
		if err := g.Mesh.ImportFederatedExportAdvertisement(ctx, gateway, ad, sourceDomain); err != nil {
			return err
		}
	}
	return nil
}

func (g *FederatedMeshGateway) ForwardSealedContext(ctx context.Context, req fwfmp.GatewayForwardRequest) (*fwfmp.GatewayForwardResult, *fwfmp.TransferRefusal, error) {
	if g == nil || g.Mesh == nil {
		return nil, nil, fmt.Errorf("federated mesh gateway unavailable")
	}
	return g.Mesh.ForwardFederatedContext(ctx, req)
}
