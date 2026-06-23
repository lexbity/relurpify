// Package ayenitd provides service lifecycle management for Relurpify.
//
// It starts long-lived services in dependency order, keeps them alive for the
// owning process, and shuts them down cleanly on exit. Workspace composition
// lives in the app composition root; ayenitd owns service startup and teardown
// only.
package ayenitd
