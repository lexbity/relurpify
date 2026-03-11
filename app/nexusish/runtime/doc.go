// Package runtime provides the typed HTTP client used by nexusish to
// communicate with the Nexus gateway admin API.
//
// Runtime wraps the low-level HTTP client (http_client.go) with typed methods
// for each admin API endpoint: listing nodes, sessions, capabilities, security
// policies, identities, and the live event stream.
//
// ClientState holds the runtime's current view of the gateway — the last
// fetched snapshot of nodes, sessions, and events — and is updated on each
// poll cycle by the TUI's update loop.
package runtime
