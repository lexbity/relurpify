package euclo

import (
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
)

// GetRegistrationFuncs returns AgentRegistrationFuncs for Euclo.
// These functions are called during agent initialization to register
// capabilities, prompt providers, and thoughtrecipes.
func GetRegistrationFuncs() agentenv.AgentRegistrationFuncs {
	return services.NewRegistration().AgentRegistrationFuncs()
}
