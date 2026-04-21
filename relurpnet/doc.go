// Package middleware groups the Nexus-facing middleware layers used to compose
// network, session, and federated runtime behavior.
//
// The subpackages under this tree own reusable boundary logic:
//
//   - channel: channel adapters and event sink integration.
//   - fmp: federated mesh protocol and handoff mechanics.
//   - gateway: authenticated gateway server and feed scoping.
//   - identity: network-facing bearer-token and subject resolution.
//   - mcp: MCP transport, protocol, and server helpers.
//   - node: node pairing and connection runtime helpers.
//   - session: session routing and tenant-bound authorization.
package middleware
