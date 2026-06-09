package agentenv

// AgentRegistrationFuncs provides function pointers for agent self-registration.
// This is a transitional workspace-open contract that will be replaced by
// app-composed registration products. app/envcomposition owns the concrete
// wiring; execution/agentenv only provides the call-through bridge.
//
// Registration functions are called after workspace services are built but before
// the agent is initialized. All functions are optional.
type AgentRegistrationFuncs struct {
	// RegisterCapabilities is called after the capability bundle is built.
	// Agents register their capability handlers.
	RegisterCapabilities func(AgentContext) error

	// RegisterPromptProviders is called after the prompt registry is built.
	// Agents register their prompt providers.
	RegisterPromptProviders func(AgentContext) error

	// LoadThoughtRecipes is called during initialization.
	// Agents load and return their thoughtrecipe registries.
	LoadThoughtRecipes func() (interface{}, error)
}
