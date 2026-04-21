// Package node manages WebSocket connections to remote agent nodes within the
// Nexus gateway.
//
// NodeManager tracks the lifecycle of each connected node: pairing,
// authentication, capability advertisement, and graceful disconnect.
// ws_connection.go owns the per-node WebSocket connection, framing messages
// and surfacing structured events to the gateway's session router.
// credential.go stores and rotates node authentication credentials.
package node
