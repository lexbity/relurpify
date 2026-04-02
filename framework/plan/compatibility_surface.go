package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
)

type CompatibilityBackendResolver interface {
	BackendID() string
	Supports(agentenv.CompatibilitySurfaceRequest) bool
	ExtractSurface(context.Context, agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error)
}

type CompatibilitySurfacePlanner struct {
	resolvers []CompatibilityBackendResolver
}

func NewCompatibilitySurfacePlanner(resolvers ...CompatibilityBackendResolver) *CompatibilitySurfacePlanner {
	return &CompatibilitySurfacePlanner{resolvers: append([]CompatibilityBackendResolver(nil), resolvers...)}
}

func (p *CompatibilitySurfacePlanner) ExtractSurface(ctx context.Context, req agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error) {
	resolver := p.selectResolver(req)
	if resolver == nil {
		return agentenv.CompatibilitySurface{}, false, nil
	}
	surface, ok, err := resolver.ExtractSurface(ctx, req)
	if err != nil || !ok {
		return agentenv.CompatibilitySurface{}, ok, err
	}
	if surface.Metadata == nil {
		surface.Metadata = map[string]any{}
	}
	surface.Metadata["backend"] = resolver.BackendID()
	surface.Metadata["source"] = firstNonEmpty(strings.TrimSpace(stringValueAny(surface.Metadata["source"])), "framework_plan")
	return surface, true, nil
}

func (p *CompatibilitySurfacePlanner) selectResolver(req agentenv.CompatibilitySurfaceRequest) CompatibilityBackendResolver {
	for _, resolver := range p.resolvers {
		if resolver == nil {
			continue
		}
		if resolver.Supports(req) {
			return resolver
		}
	}
	return nil
}

func stringValueAny(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
