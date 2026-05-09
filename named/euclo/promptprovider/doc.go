// Package promptprovider provides Euclo-specific prompt context providers.
//
// These providers expose structured clarification and execution state to prompt
// templates. They are read-only views over state and do not participate in route
// policy.
//
// Package registration:
//
//	promptprovider.RegisterAll(registry)
//
// The package is self-contained, has no init-time side effects, and registers
// all providers during named-agent initialization.
package promptprovider
