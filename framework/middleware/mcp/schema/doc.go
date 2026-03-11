// Package schema converts MCP wire-format JSON schema maps into the
// framework's core.Schema type.
//
// FromMap accepts the raw map[string]any produced by JSON-decoding an MCP
// tool's inputSchema or outputSchema field and returns a *core.Schema. It
// recursively converts object properties and array items. Supported schema
// fields: type, title, description, format, properties, items, required,
// enum, and default. Nil or empty input returns a nil schema without error.
package schema
