// nexusish is a terminal TUI dashboard for managing and monitoring a running
// Nexus gateway instance.
//
// Intended for Nexus administrators and developers, nexusish provides a
// real-time view of connected nodes, active sessions, the capability registry,
// security policies, identity records, and the live event stream.
//
// It communicates with Nexus exclusively through the admin HTTP API
// (see app/nexusish/runtime) and does not require direct access to the
// gateway's internal state.
//
// # Subdirectories
//
//   - tui: Bubble Tea application (panes, chrome, navigation, styles).
//   - runtime: typed HTTP client for the Nexus admin API.
package main
