// Package services provides Euclo's framework service registrations.
//
// Euclo integrates with runtime composition through the
// agentenv.AgentRegistrationFuncs pattern defined in execution/agentenv. This
// package implements the registration functions that wire Euclo's capabilities,
// prompt providers, and thoughtrecipes into the workspace environment.
//
// The registrations are called by the composition root (ayenitd) during workspace
// initialization, avoiding circular dependencies between named/euclo and ayenitd.
package services
