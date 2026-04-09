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
// # Trust concepts in Nexus
//
// Nexus works with three distinct trust-related concepts. They operate at
// different layers and must not be conflated:
//
// Node enrollment trust (framework/core.TrustClass on NodeEnrollment) —
// records the result of the node pairing handshake. Answers: "how was this
// node authenticated when it joined the fabric?"
//
//   - RemoteApproved: completed the challenge/response pairing handshake.
//   - WorkspaceTrusted: declared in the local workspace manifest.
//
// Event ingress origin (named/rex/events.IngressOrigin on CanonicalEvent) —
// classifies where an inbound event came from. Answers: "how trusted is the
// source of this event?" Used by the gateway to gate which event types are
// allowed from which origins.
//
//   - OriginInternal: originated inside the Nexus fabric.
//   - OriginPeer: from an authenticated enrolled node.
//   - OriginExternal: from an unauthenticated external source.
//
// FMP sensitivity (framework/core.SensitivityClass on FMP routing types) —
// classifies how sensitive a payload is for transport routing decisions.
// Answers: "how should this data be handled in transit?"
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
