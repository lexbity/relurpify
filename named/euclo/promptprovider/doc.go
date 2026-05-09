// Package promptprovider provides euclo-specific prompt context providers.
//
// These providers supply runtime context for thoughtrecipe step prompts, exposing
// euclo's internal state (captures, plan goals, step summaries) to the
// prompt registry system.
//
// Package registration:
//
//	eucloprovider.RegisterAll(registry)
//
// The package follows the same pattern as relurpicabilities - it's
// self-contained, has no init-time side effects, and registers all
// providers during named-agent Initialize().
package promptprovider
