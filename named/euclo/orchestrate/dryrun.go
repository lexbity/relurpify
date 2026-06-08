package orchestrate

import (
	"context"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// DryRun resolves a route request without executing it and returns the ranked candidate set.
func DryRun(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *registry.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) (*DryRunReport, error) {
	return dryRun(ctx, env, req, caps, thoughtrecipes)
}
