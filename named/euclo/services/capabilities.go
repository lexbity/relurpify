package services

import (
    "codeburg.org/lexbit/relurpify/framework/agentenv"
    "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// defaultCapabilityRegistrar implements CapabilityRegistrar using Euclo's relurpic capabilities.
type defaultCapabilityRegistrar struct{}

func (r *defaultCapabilityRegistrar) RegisterAll(env agentenv.WorkspaceEnvironment) error {
    // If the environment's registry is nil, skip registration.
    if env.Registry == nil {
        return nil
    }
    return relurpicabilities.RegisterAll(env)
}
