// Package ports holds the canonical tool port interfaces (Tool, ToolParameter,
// ToolResult, CommandRunner, etc.) and the consumer-defined State interface
// that capability declares for the context envelope.
//
// capability/ports is the single home for tool abstraction types used across
// Relurpify. No other domain defines its own tool interface.
//
// Consumer-defined port in this package:
//
//	State  — capability/ports.State is the minimal envelope interface that
//	         capability handlers receive; context/Envelope satisfies it (P9–P10).
package ports
