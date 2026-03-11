// Package gateway implements the Nexus gateway HTTP/WebSocket server and
// deterministic event replay for connected clients.
//
// # Server
//
// Server upgrades HTTP connections, resolves the authenticated
// ConnectionPrincipal from a bearer token, and runs the per-connection message
// loop. Supported client frame types include: connect, ping/pong,
// session.close, admin.snapshot, message.outbound, and capability.invoke.
// Node connections are handed off to a HandleNodeConnection hook after
// optional challenge verification.
//
// A background broadcast loop tails the event log and pushes new events to all
// connected clients permitted to receive them. Slow clients are evicted when
// their bounded send queue fills.
//
// # Replay
//
// When a client reconnects with a non-zero LastSeenSeq field in its connect
// frame, the server reads missed events from the event log and delivers them
// as replay_event frames followed by replay_complete before resuming live
// streaming. Access control applies to replayed events identically to live
// events.
//
// # Principal and authorization
//
// ConnectionPrincipal carries the resolved role, EventActor, and optional
// AuthenticatedPrincipal for an active connection. PrincipalResolver must be
// set on the Server; the gateway rejects unauthenticated connections.
// Admin principals receive all events regardless of tenant scope.
package gateway
