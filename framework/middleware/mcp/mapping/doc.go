// Package mapping translates between MCP wire-format capability types and the
// framework's internal CapabilityDescriptor.
//
// # Importing
//
// ImportedToolDescriptor, ImportedPromptDescriptor, and ImportedResourceDescriptor
// convert protocol.Tool, protocol.Prompt, and protocol.Resource values from a
// remote MCP server into CapabilityDescriptors. Each receives an
// "mcp:<providerID>:<kind>:<name>" ID, RuntimeFamily provider, remote scope,
// session affinity, the supplied TrustClass, and MCP-specific annotations.
//
// # Exporting
//
// ExportedTool, ExportedPrompt, and ExportedResource convert local
// CapabilityDescriptors into the protocol.Tool, protocol.Prompt, and
// protocol.Resource shapes expected by MCP clients.
//
// # Content block conversion
//
// contentBlocksFromCore and CoreContentBlocksFromProtocol translate between
// framework core.ContentBlock and protocol.ContentBlock, handling text,
// structured, resource link, embedded resource, binary reference, and error
// block variants.
package mapping
