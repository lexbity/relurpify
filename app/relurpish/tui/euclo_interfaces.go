package tui

import (
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
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
	// UpdateSidebarFromFrame inspects the frame content; if it is a
	// ContextProposalContent it updates the context sidebar entries.
	UpdateSidebarFromFrame(frame interaction.InteractionFrame)
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

// ArchaeoPaner is the interface for the archaeology tab pane. It handles both
// the explore subtab (full-width feed of blob proposals) and the plan subtab
// (live plan + staged blob sidebar, Phase 5).
type ArchaeoPaner interface {
	Update(msg tea.Msg) (ArchaeoPaner, tea.Cmd)
	View() string
	SetSize(w, h int)
	SetSubTab(id SubTabID)
	HandleInputSubmit(value string) tea.Cmd
	// SetBlobEmojiEnabled toggles emoji vs letter-badge rendering.
	SetBlobEmojiEnabled(enabled bool)
	// StagedBlobs returns the current set of staged blobs from the explore feed.
	StagedBlobs() []StagedBlobEntry
	// PromoteAll stages all proposed blobs in the current explore run.
	PromoteAll()
}

// EucloPlugin provides factory functions for creating euclo-specific panes and
// registering euclo tabs. It is passed to RunWithEuclo so that main.go can wire
// the euclo agent without creating a circular import.
type EucloPlugin struct {
	NewChat    func(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) ChatPaner
	NewPlanner func() PlannerPaner
	NewDebug   func() DebugPaner
	SetupTabs  func(reg *TabRegistry)
}
