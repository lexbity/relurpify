package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// DryRun resolves a route request without executing it and returns the ranked candidate set.
func DryRun(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *capability.CapabilityRegistry, recipes *recipepkg.RecipeRegistry) (*DryRunReport, error) {
	return dryRun(ctx, env, req, caps, recipes)
}
