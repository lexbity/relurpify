// Package identity resolves agent and node identities to their assigned roles
// and capability sets within the Relurpify trust model.
//
// IdentityResolver maps an identity (user, node, or agent) to the set of
// policies and capabilities it is permitted to exercise. IdentityStore
// provides durable persistence of tenant, subject, external identity, and
// node enrollment records across restarts, supporting the Nexus gateway's
// authentication and authorization flow.
package identity
