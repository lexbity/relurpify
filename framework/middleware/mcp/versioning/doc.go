// Package versioning implements MCP protocol version negotiation between a
// client and a server.
//
// # SupportMatrix
//
// SupportMatrix declares which MCP protocol revisions this implementation
// supports. DefaultSupportMatrix returns a matrix with revision 2025-06-18 as
// the primary version, listing its feature families (base-protocol, tools,
// prompts, resources) and transport kinds (stdio, streamable-http).
//
// # ChooseRevision and Negotiate
//
// ChooseRevision selects the best revision from a caller-supplied preference
// list, falling back to the primary revision if none are supported. Negotiate
// verifies that the peer's declared revision matches the chosen revision and
// returns a NegotiationResult with the requested/negotiated version strings
// and a flag indicating whether the negotiated version is the primary.
//
// # RevisionSupport
//
// RevisionSupport describes a single supported revision: feature families,
// transport kinds, deprecated/unsupported methods, content shape variants,
// and test fixture coverage tag.
package versioning
