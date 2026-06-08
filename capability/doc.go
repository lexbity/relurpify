// Package capability defines the top-level capability domain boundary.
//
// The concrete capability surface is split by owner:
// descriptor owns capability metadata, handler owns invocation interfaces,
// result owns execution results and content blocks, provider owns external
// provider contracts, registry owns registration/invocation, sandbox owns
// command isolation, and ports owns consumer-facing tool ports.
package capability
