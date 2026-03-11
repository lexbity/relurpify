// relurpish is the primary end-user terminal interface for interacting with
// Relurpify AI coding agents.
//
// It provides a full conversational TUI backed by a local Ollama LLM and an
// extensible capability provider system. All agent-executed actions are
// governed by the workspace manifest, sandboxed via gVisor, and can connect
// to remote capabilities through the Nexus gateway.
//
// # Subdirectories
//
//   - tui: Bubble Tea user interface (chat, tasks, session, settings, tools
//     panes; HITL approval UI; streaming output; session export).
//   - runtime: agent runtime — capability provider registration (builtin, MCP,
//     Nexus node, background delegation, browser); session orchestration;
//     Nexus HTTP client.
package main
