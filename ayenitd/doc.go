// Package ayenitd provides service lifecycle management for Relurpify.
//
// It starts long-lived services in dependency order, keeps them alive for the
// owning process, and shuts them down cleanly on exit. Workspace composition
// lives in execution/agentenv; ayenitd owns service startup and teardown only.
package ayenitd
