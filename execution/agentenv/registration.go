package agentenv

// AgentRegistrationFuncs provides function pointers for agent self-registration.
// This pattern allows named agents to register their capabilities and prompt providers
// without creating circular dependencies. Framework knows nothing about specific agents,
// and agents import only framework/agentenv (not ayenitd).
//
// Registration functions are called by the composition root after framework services
// are built but before the agent is initialized. All functions are optional (nil means no registration).
type AgentRegistrationFuncs struct {
	// RegisterCapabilities is called after the capability bundle is built.
	// Agents register their capability handlers (e.g., relurpic capabilities for euclo).
	RegisterCapabilities func(AgentContext) error

	// RegisterPromptProviders is called after the prompt registry is built.
	// Agents register their prompt providers (e.g., paradigm and euclo providers for euclo).
	RegisterPromptProviders func(AgentContext) error

	// LoadThoughtRecipes is called during initialization.
	// Agents load and return their thoughtrecipe registries.
	// TODO: Move thoughtrecipe registry into AgentContext in future iteration.
	LoadThoughtRecipes func() (interface{}, error)
}
