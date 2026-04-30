// Package tui implements the relurpish Bubble Tea terminal user interface.
//
// # Panes
//
// The TUI is organised around five primary panes exposed by the root tab bar:
//
//   - Chat: conversational agent interaction and streamed execution output.
//   - Config: agent-specific policies, capabilities, prompts, tools, and contract data.
//   - Session: workspace files, pending changes, live runtime state, and queued tasks.
//
// # Guidance and HITL
//
// Guidance requests, approvals, and deferred observations are surfaced through
// the shared overlay and notification flow. Operators can approve once, approve
// for the session, persist policy when applicable, or deny.
//
// # Streaming
//
// streaming.go drives incremental display of LLM output as it arrives,
// updating the active shell without blocking the event loop.
//
// # Background work
//
// Background and queued tasks are surfaced through the session pane and
// notification bar, while runtime workflow and provider state remain available
// from the session live views.
package tui
