// Package rex implements the Nexus-managed named runtime for hybrid workflow
// execution across the existing Relurpify agent and framework layers.
//
// # Trust concepts in this package
//
// Rex uses two distinct trust-related concepts. They must not be confused:
//
// IngressOrigin (named/rex/events) — classifies where an inbound canonical
// event came from and how its source was authenticated. This is an ingress
// routing concept, not a capability authorization concept.
//
//   - OriginInternal: event originated within the Nexus fabric itself.
//   - OriginPeer: event came from an authenticated enrolled peer node.
//   - OriginExternal: event came from an unauthenticated external source.
//
// framework/core.TrustClass — governs capability execution authorization
// inside the agent runtime. Rex passes this through from the framework when
// constructing tasks and contexts, but does not define it. See framework/core
// for the full definition.
//
// These two concepts operate at different layers and must not be substituted
// for one another. IngressOrigin gates what enters the system; TrustClass
// gates what the system is allowed to execute once a task is running.
package rex
