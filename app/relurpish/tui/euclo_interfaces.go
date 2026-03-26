package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ChatPaner is the interface that euclotui.ChatPane must implement so that the
// RootModel can hold it without importing the euclotui package.
type ChatPaner interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (ChatPaner, tea.Cmd)
	View() string
	SetSize(w, h int)
	SetSubTab(id SubTabID)
	ActiveSubTab() SubTabID
	HandleInputSubmit(value string) tea.Cmd
	HasActiveRuns() bool
	StartRun(description string) (tea.Cmd, string)
	Undo() bool
	Redo() bool
	ToggleCompact()
	Cleanup()
	// Feed operations (replaces direct feed.xxx access in model.go)
	AppendMessage(msg Message)
	ClearMessages()
	Messages() []Message
	SetSearchFilter(filter string)
	ScrollUp()
	PageDown()
	PageUp()
	// System message helpers
	AddSystemMessage(text string)
	// Undo rollback when compact fails
	RollbackLastUndo()
	// PushUndoSnapshot snapshots msgs onto the undo stack and clears redo.
	PushUndoSnapshot(msgs []Message)
	// HITLService returns the underlying HITL service for use by model.go HITL handlers.
	HITLService() HITLServiceIface

	// Methods used by commands.go that previously accessed concrete fields directly.

	// StartRunWithMetadata begins a run with extra metadata (used by /rerun, /resume).
	StartRunWithMetadata(prompt string, extra map[string]any) (tea.Cmd, string)
	// StartRunSilent begins a run without adding a user message (used by /compact).
	StartRunSilent(prompt string) (tea.Cmd, string)
	// SetCompactRunID stores the compact run ID and message count (used by /compact).
	SetCompactRunID(runID string, msgCount int)
	// AllowParallel returns whether parallel runs are enabled.
	AllowParallel() bool
	// SetAllowParallel sets the parallel run mode.
	SetAllowParallel(v bool)
	// LastPrompt returns the last submitted prompt text.
	LastPrompt() string
	// StopLatestRun cancels the most recently started run.
	StopLatestRun() tea.Cmd
	// RetryLastRun restarts the most recent prompt.
	RetryLastRun() tea.Cmd
	// ApplyPendingChanges bulk-approves or rejects pending file changes.
	ApplyPendingChanges(status ChangeStatus) int
	// MutateMessages calls fn with a mutable reference to each message, then
	// refreshes the feed. Used by /diff and similar commands that patch messages.
	MutateMessages(fn func(msgs []Message))
	// AddFile adds a file reference to the pane's context.
	AddFile(path string) tea.Cmd
}

// PlannerPaner is the interface that euclotui.PlannerPane must implement.
type PlannerPaner interface {
	Update(msg tea.Msg) (PlannerPaner, tea.Cmd)
	View() string
	SetSize(w, h int)
	SetSubTab(id SubTabID)
	HandleInputSubmit(value string) tea.Cmd
}

// DebugPaner is the interface that euclotui.DebugPane must implement.
type DebugPaner interface {
	Update(msg tea.Msg) (DebugPaner, tea.Cmd)
	View() string
	SetSize(w, h int)
	SetSubTab(id SubTabID)
	HandleInputSubmit(value string) tea.Cmd
}

// EucloEmitter is the interface for an object that can resolve interaction
// responses — implemented by TUIFrameEmitter in the euclotui package.
// It embeds interaction.FrameEmitter so it can be passed directly to
// RuntimeAdapter.SetInteractionEmitter.
type EucloEmitter interface {
	interaction.FrameEmitter
	Resolve(response interaction.UserResponse)
}

// EucloPlugin provides factory functions for creating euclo-specific panes and
// registering euclo tabs. It is passed to RunWithEuclo so that main.go can wire
// the euclo agent without creating a circular import.
type EucloPlugin struct {
	NewChat    func(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) ChatPaner
	NewPlanner func() PlannerPaner
	NewDebug   func() DebugPaner
	SetupTabs  func(reg *TabRegistry)
	NewEmitter func(program *tea.Program) EucloEmitter
}
