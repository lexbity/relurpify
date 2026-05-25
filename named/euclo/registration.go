package euclo

import (
	"errors"
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/agents/promptprovider"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	eucloprovider "codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
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
	if env.Config == nil || env.Config.AgentSpec == nil {
		return fmt.Errorf("agent spec required for relurpic capability registration")
	}
	if err := relurpicabilities.RegisterAll(env, env.Config.AgentSpec.Capabilities.Relurpic); err != nil {
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

// loadEucloThoughtRecipes scans the Euclo DSL source tree and falls back to an empty
// registry if the workspace does not currently contain thoughtrecipe sources.
func loadEucloThoughtRecipes() (interface{}, error) {
	loader := thoughtrecipepkg.NewLoader()
	result, err := loader.LoadWorkspace(".")
	if err == nil && result != nil {
		return result, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &thoughtrecipepkg.LoadResult{
		Registry: thoughtrecipepkg.NewThoughtRecipeRegistry(),
	}, nil
}
