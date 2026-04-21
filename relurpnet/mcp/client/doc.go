// Package client implements an MCP client that connects to an MCP server over
// a stdio transport and invokes tools, prompts, and resources.
//
// # ConnectStdio
//
// ConnectStdio launches an MCP server subprocess, performs the MCP
// initialization handshake (version negotiation, initialize request,
// notifications/initialized), and returns a ready Client. StdioConfig carries
// the command, args, working directory, environment, provider and session IDs,
// preferred protocol versions, and local peer info.
//
// # Client
//
// Client exposes ListTools, ListPrompts, ListResources, CallTool, ReadResource,
// SubscribeResource, and UnsubscribeResource over JSON-RPC 2.0 multiplexed on
// a single stdio connection. Concurrent calls are matched to responses by
// correlation ID. SetNotificationHandler receives server-to-client
// notification methods. SetRequestHandler handles server-initiated sampling
// and elicitation requests. SessionSnapshot returns a point-in-time view of
// session state. Close drains pending calls and shuts down the transport.
package client
