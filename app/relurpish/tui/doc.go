// Package tui implements the relurpish Bubble Tea terminal user interface.
//
// # Panes
//
// The TUI is organised into five primary panes, each accessible via a tab bar:
//
//   - Chat: conversational agent interaction with streaming output.
//   - Tasks: list of current and past tasks with status and progress.
//   - Session: session history, context usage, and conversation export.
//   - Settings: workspace and model configuration.
//   - Tools: live view of capability policies and HITL approval history.
//
// # HITL approval
//
// When the agent requests a capability that requires human approval, hitl.go
// displays an inline prompt in the chat pane. The operator responds with:
// [y] approve once, [s] approve for the session, [a] always allow, [n] deny.
//
// # Streaming
//
// streaming.go drives incremental display of LLM output as it arrives,
// updating the chat pane without blocking the event loop.
//
// # Background tasks
//
// When a task is explicitly delegated to the background (via Nexus), the
// notification bar receives live status updates and the Tasks pane shows
// in-progress background work.
package tui
