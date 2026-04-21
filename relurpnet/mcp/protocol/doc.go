// Package protocol defines the wire-format types for the Model Context Protocol
// (MCP), supporting revisions 2025-06-18 and 2025-11-25.
//
// # Handshake
//
// InitializeRequest and InitializeResult carry the protocol version, PeerInfo
// (name and version string), and a capability map exchanged during the MCP
// initialization handshake.
//
// # Capabilities
//
// Tool, Prompt (with PromptArgument), and Resource are the three MCP capability
// kinds. Each has corresponding List* result types and call/get/read parameter
// and result types: CallToolParams/CallToolResult, GetPromptParams/GetPromptResult,
// ReadResourceParams/ReadResourceResult, and ResourceSubscribeParams.
//
// # Content
//
// ContentBlock is the universal content envelope used in prompt messages,
// resource contents, and tool results. It carries type, text, structured data,
// URI, MIME type, blob, and name fields.
//
// # Sampling and elicitation
//
// CreateMessageParams/CreateMessageResult support the sampling/createMessage
// server-to-client request. ElicitationParams/ElicitationResult support the
// elicitation/create request introduced in revision 2025-11-25.
//
// # Version constants
//
// Revision20250618 and Revision20251125 are the canonical revision strings.
// NormalizeRevision trims whitespace for safe comparison.
package protocol
