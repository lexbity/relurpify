// Package server implements an MCP server that exposes agent capabilities over
// stdio and HTTP transports.
//
// # Service and Exporter
//
// Service is the MCP server implementation constructed with New, taking a
// PeerInfo, an Exporter, and optional lifecycle Hooks. Exporter is the
// interface Service delegates capability operations to: ListTools, CallTool,
// ListPrompts, GetPrompt, ListResources, and ReadResource.
//
// # ServeConn
//
// ServeConn runs the full MCP session lifecycle on an io.ReadWriteCloser:
// initialization handshake (version negotiation), per-session resource
// subscription tracking, method dispatch to the Exporter, and Hook firing on
// session open, initialization, and close. Multiple concurrent sessions are
// tracked independently.
//
// # Resource push
//
// NotifyResourceUpdated sends a notifications/resources/updated frame to all
// initialized sessions that have subscribed to the given URI.
package server
