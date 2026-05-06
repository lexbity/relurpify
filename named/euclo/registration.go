package euclo

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/agents/promptprovider"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	eucloprovider "codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
	"codeburg.org/lexbit/relurpify/named/euclo/recipetemplates"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
)

// GetRegistrationFuncs returns AgentRegistrationFuncs for Euclo.
// These functions are called by the composition root to register Euclo's
// capabilities and prompt providers without creating a circular dependency
// between named/euclo and ayenitd.
func GetRegistrationFuncs() agentenv.AgentRegistrationFuncs {
	return services.NewRegistration().AgentRegistrationFuncs()
}

// registerEucloCapabilities registers all relurpic capability handlers.
// Relurpic capabilities are subagent-backed and caller-owned: Euclo registers
// them when it receives the WorkspaceEnvironment.
func registerEucloCapabilities(env agentenv.WorkspaceEnvironment) error {
	if err := relurpicabilities.RegisterAll(env); err != nil {
		return fmt.Errorf("register relurpic capabilities: %w", err)
	}
	return nil
}

// registerEucloPromptProviders registers Euclo's prompt providers.
// This includes both paradigm providers (generic) and Euclo-specific providers.
func registerEucloPromptProviders(env agentenv.WorkspaceEnvironment) error {
	if env.PromptRegistry == nil {
		return nil
	}
	if err := promptprovider.RegisterAll(env.PromptRegistry); err != nil {
		return fmt.Errorf("register paradigm prompt providers: %w", err)
	}
	if err := eucloprovider.RegisterAll(env.PromptRegistry); err != nil {
		return fmt.Errorf("register euclo prompt providers: %w", err)
	}
	return nil
}

// loadEucloRecipes loads all Euclo recipe templates.
// Returns a recipe.RecipeRegistry. The registry is loaded from the Euclo
// recipe templates package and returned to the caller for wiring.
func loadEucloRecipes() (interface{}, error) {
	registry, err := recipetemplates.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load euclo recipe templates: %w", err)
	}
	return registry, nil
}
