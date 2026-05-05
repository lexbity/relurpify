// Package ayenitd provides service lifecycle management for Relurpify.
// It is analogous to systemd/init: it starts services in dependency order, holds them alive,
// and shuts them down cleanly on exit.
//
// COMPLETED ARCHITECTURAL TRANSITION:
// - Composition logic has been moved to framework/agentenv (Open, BootstrapAgentRuntime)
// - Framework services are now in framework/services/ and framework/agentenv/
// - Duplicate files (capability_bundle.go, prompt_registry.go, agentenv_interfaces.go, scheduler.go) have been deleted
// - Type alias shims have been removed
// - ayenitd is now a pure service runner with no composition logic
//
// Entry points should now use framework/agentenv.Open for workspace initialization.
package ayenitd
