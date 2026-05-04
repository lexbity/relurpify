package promptprovider

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/prompt"
)

// RegisterAll registers all euclo-specific prompt providers with the registry.
// Safe to call multiple times - duplicate registrations are skipped.
func RegisterAll(registry prompt.Registry) error {
	if registry == nil {
		return fmt.Errorf("promptprovider: registry is nil")
	}

	providers := []struct {
		name string
		provider prompt.ContextProvider
	}{
		{"euclo.recipe_step_context", &recipeStepContextProvider{}},
		{"euclo.recipe_plan_goal", &recipePlanGoalProvider{}},
		{"euclo.recipe_prior_step_result", &recipePriorStepProvider{}},
	}

	for _, p := range providers {
		if err := registry.RegisterProvider(p.name, p.provider); err != nil {
			// Skip already registered providers (shared providers may be registered multiple times)
			if prompt.IsAlreadyRegistered(err) {
				continue
			}
			return fmt.Errorf("register provider %s: %w", p.name, err)
		}
	}

	return nil
}
