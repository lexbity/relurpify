package services

import (
    "codeburg.org/lexbit/relurpify/agents/promptprovider"
    "codeburg.org/lexbit/relurpify/framework/agentenv"
    eucloprovider "codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
)

// defaultPromptRegistrar implements PromptRegistrar using Euclo's prompt providers.
type defaultPromptRegistrar struct{}

func (r *defaultPromptRegistrar) RegisterAll(env agentenv.WorkspaceEnvironment) error {
    if env.PromptRegistry == nil {
        return nil // No registry to register with.
    }
    // Register generic paradigm providers.
    if err := promptprovider.RegisterAll(env.PromptRegistry); err != nil {
        return err
    }
    // Register Euclo-specific providers.
    if err := eucloprovider.RegisterAll(env.PromptRegistry); err != nil {
        return err
    }
    return nil
}
