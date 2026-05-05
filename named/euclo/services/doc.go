// Package services provides Euclo's framework service registrations.
//
// Euclo integrates with the framework through the agentenv.AgentRegistrationFuncs
// pattern defined in framework/agentenv. This package implements the registration
// functions that wire Euclo's capabilities, prompt providers, and recipes into the
// workspace environment.
//
// The registrations are called by the composition root (ayenitd) during workspace
// initialization, avoiding circular dependencies between named/euclo and ayenitd.
package services
