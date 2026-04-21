// Package session provides session routing and event-sink integration that
// isolates agent conversation state per channel peer or thread.
//
// # Router
//
// Router routes an InboundMessage to an existing or newly created
// SessionBoundary using a composite key derived from the session scope,
// partition, channel ID, peer ID, and thread ID. DefaultRouter is backed by a
// Store and an optional PolicyEngine. Authorize enforces ownership and tenant
// boundaries before allowing an actor to resume a session.
//
// # SessionSink
//
// SessionSink implements the channel.EventSink interface. On each inbound
// message event it appends the raw event to the event log, resolves the sender
// identity via an optional identity.Resolver, routes the message to a
// SessionBoundary through the Router, and appends a session.message event
// carrying the resolved session key and normalized content. Unresolved external
// sessions are admitted up to a configurable cap.
//
// # Store
//
// Store is the persistence interface for SessionBoundary records; it supports
// upsert, lookup by routing key or session ID, list, delete, and TTL-based
// expiry sweeps.
package session
