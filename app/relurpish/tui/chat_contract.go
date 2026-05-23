package tui

import (
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
)

// ChatSystemMsg adds a system line to the chat feed.
type ChatSystemMsg struct{ Text string }

// chatSystemMsg remains the package-local constructor form used throughout the
// TUI package.
type chatSystemMsg = ChatSystemMsg

// ChatPaner is the core chat surface contract used by the host shell and
// agent-specific surfaces.
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
	AppendMessage(msg Message)
	ClearMessages()
	Messages() []Message
	SetSearchFilter(filter string)
	ScrollUp()
	PageDown()
	PageUp()
	AddSystemMessage(text string)
	RollbackLastUndo()
	PushUndoSnapshot(msgs []Message)
	HITLService() HITLServiceIface
	StartRunWithMetadata(prompt string, extra map[string]any) (tea.Cmd, string)
	StartRunSilent(prompt string) (tea.Cmd, string)
	SetCompactRunID(runID string, msgCount int)
	AllowParallel() bool
	SetAllowParallel(v bool)
	LastPrompt() string
	StopLatestRun() tea.Cmd
	RetryLastRun() tea.Cmd
	ApplyPendingChanges(status ChangeStatus) int
	MutateMessages(fn func(msgs []Message))
	AddFile(path string) tea.Cmd
}

// ChatSidebarController exposes the surface-local sidebar operations that the
// host shell needs to keep in sync with the active chat pane.
type ChatSidebarController interface {
	ToggleSidebar()
	AddFileToSidebar(path string) error
	RemoveFileFromSidebar(path string)
	UpdateSidebarFromFrame(frame interaction.InteractionFrame)
}

// TabAwarePane lets the host tell a pane which main tab is currently active.
// Euclo uses this to switch its Region 1 layout without a separate host stack.
type TabAwarePane interface {
	SetActiveTab(id TabID)
}
