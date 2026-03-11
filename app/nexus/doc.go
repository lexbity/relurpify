// nexus is the Relurpify gateway server that coordinates distributed agent
// nodes.
//
// Nexus acts as the central hub in a mesh of agent nodes. Each node (typically
// a relurpish instance on a remote machine) registers with Nexus, advertises
// its capabilities, and receives task delegations routed by the gateway.
//
// # Responsibilities
//
//   - Node pairing and authentication (node_auth.go).
//   - Capability routing: forwarding requests to the correct node.
//   - Event streaming: aggregating execution events from all nodes into a
//     unified observability stream.
//   - Admin surface: management API exposed over MCP (see admin/, adminapi/).
//
// # Subdirectories
//
//   - admin: admin domain and MCP handler surface.
//   - adminapi: request/response type contracts for the admin API.
//   - bootstrap: startup dependency wiring and config resolution.
//   - config: Nexus-specific configuration types.
//   - gateway: event materializer (events → OTel spans / audit log).
//   - server: HTTP handler composition and node connection helpers.
//   - status: health and status monitoring.
package main
