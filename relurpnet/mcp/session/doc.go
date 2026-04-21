// Package session tracks the lifecycle state of an active MCP client session.
//
// # Session and State
//
// Session is a thread-safe state machine for a single MCP client connection,
// transitioning through: connecting → initializing → initialized, with
// optional degraded, closing, closed, and failed terminal states. Invalid
// transitions return errors.
//
// # Lifecycle methods
//
// MarkTransportEstablished advances from connecting to initializing.
// ApplyInitializeResult records the negotiated protocol version and remote
// peer info, advancing to initialized. UpdateRequestCount tracks in-flight
// request count. SetSubscription tracks resource URI subscriptions. Degrade
// marks a recoverable error; Fail records an unrecoverable failure.
// BeginClose and MarkClosed finalize the session.
//
// # Snapshot
//
// Snapshot returns a point-in-time copy of session state including provider
// ID, session ID, transport kind, negotiated version, request count,
// active subscriptions, and failure reason.
package session
