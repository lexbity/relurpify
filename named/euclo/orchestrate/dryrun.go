package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// DryRun resolves a route request without executing it and returns the ranked candidate set.
func DryRun(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *capability.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) (*DryRunReport, error) {
	return dryRun(ctx, env, req, caps, thoughtrecipes)
}
