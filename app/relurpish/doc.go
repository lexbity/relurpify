// relurpish is the primary end-user terminal interface for interacting with
// Relurpify AI coding agents.
//
// It provides a full conversational TUI backed by a local Ollama LLM and an
// extensible capability provider system. Agent-executed capability calls are
// governed by the workspace manifest and normally execute through the runtime's
// sandboxed capability layer, while relurpish itself can also perform explicit
// user-driven local actions such as editor launch and session export.
//
// # Subdirectories
//
//   - tui: Bubble Tea user interface (chat, planner, debug, config, session;
//     HITL/guidance overlays; streaming output; session export).
//   - runtime: agent runtime — capability provider registration (builtin, MCP,
//     Nexus node, background delegation, browser); session orchestration;
//     Nexus HTTP client.
package main
