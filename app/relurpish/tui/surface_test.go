package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type stubChatPane struct{}

func (stubChatPane) Init() tea.Cmd                                                 { return nil }
func (stubChatPane) Update(msg tea.Msg) (ChatPaner, tea.Cmd)                       { return stubChatPane{}, nil }
func (stubChatPane) View() string                                                  { return "" }
func (stubChatPane) SetSize(int, int)                                              {}
func (stubChatPane) SetSubTab(SubTabID)                                            {}
func (stubChatPane) ActiveSubTab() SubTabID                                        { return "" }
func (stubChatPane) HandleInputSubmit(string) tea.Cmd                              { return nil }
func (stubChatPane) HasActiveRuns() bool                                           { return false }
func (stubChatPane) StartRun(string) (tea.Cmd, string)                             { return nil, "" }
func (stubChatPane) Undo() bool                                                    { return false }
func (stubChatPane) Redo() bool                                                    { return false }
func (stubChatPane) ToggleCompact()                                                {}
func (stubChatPane) Cleanup()                                                      {}
func (stubChatPane) AppendMessage(Message)                                         {}
func (stubChatPane) ClearMessages()                                                {}
func (stubChatPane) Messages() []Message                                           { return nil }
func (stubChatPane) SetSearchFilter(string)                                        {}
func (stubChatPane) ScrollUp()                                                     {}
func (stubChatPane) PageDown()                                                     {}
func (stubChatPane) PageUp()                                                       {}
func (stubChatPane) AddSystemMessage(string)                                       {}
func (stubChatPane) RollbackLastUndo()                                             {}
func (stubChatPane) PushUndoSnapshot([]Message)                                    {}
func (stubChatPane) HITLService() HITLServiceIface                                 { return nil }
func (stubChatPane) StartRunWithMetadata(string, map[string]any) (tea.Cmd, string) { return nil, "" }
func (stubChatPane) StartRunSilent(string) (tea.Cmd, string)                       { return nil, "" }
func (stubChatPane) SetCompactRunID(string, int)                                   {}
func (stubChatPane) AllowParallel() bool                                           { return false }
func (stubChatPane) SetAllowParallel(bool)                                         {}
func (stubChatPane) LastPrompt() string                                            { return "" }
func (stubChatPane) StopLatestRun() tea.Cmd                                        { return nil }
func (stubChatPane) RetryLastRun() tea.Cmd                                         { return nil }
func (stubChatPane) ApplyPendingChanges(ChangeStatus) int                          { return 0 }
func (stubChatPane) MutateMessages(func([]Message))                                {}
func (stubChatPane) AddFile(string) tea.Cmd                                        { return nil }

type stubSurface struct {
	name         string
	chat         ChatPaner
	resolveHit   *int
	tabCount     int
	commandCount int
}

func (s *stubSurface) Name() string { return s.name }

func (s *stubSurface) RegisterTabs(reg *TabRegistry) {
	s.tabCount++
	reg.Register(TabDefinition{ID: TabChat, Label: "chat"})
}

func (s *stubSurface) RegisterCommands(reg *CommandRegistry) {
	s.commandCount++
	reg.Register(Command{Name: "surface-cmd", Usage: "/surface-cmd", Handler: func(m *RootModel, args []string) (*RootModel, tea.Cmd) {
		return m, nil
	}})
}

func (s *stubSurface) NewChat(RuntimeAdapter, *AgentContext, *Session, *NotificationQueue) ChatPaner {
	if s.chat != nil {
		return s.chat
	}
	return stubChatPane{}
}

func (s *stubSurface) InitialTab() TabID                                        { return TabChat }
func (s *stubSurface) InitialSubTab(TabID) SubTabID                             { return "" }
func (s *stubSurface) RenderNotification(item NotificationItem) string          { return item.Msg }
func (s *stubSurface) HandleFrame(context.Context, *RootModel, SurfaceFrameMsg) {}

type countingFactory struct {
	shared        AgentSurface
	resolveCount  int
	resolveByName map[string]int
}

func (f *countingFactory) Resolve(agentName string) AgentSurface {
	f.resolveCount++
	if f.resolveByName == nil {
		f.resolveByName = make(map[string]int)
	}
	f.resolveByName[normalizeSurfaceKey(agentName)]++
	return f.shared
}

func TestSurfaceRegistryResolveFallsBack(t *testing.T) {
	defaultSurface := &stubSurface{name: "default"}
	registry := NewSurfaceRegistry(defaultSurface)
	custom := &stubSurface{name: "euclo"}
	registry.Register("euclo", custom)

	if got := registry.Resolve("euclo"); got != custom {
		t.Fatalf("expected euclo surface, got %#v", got)
	}
	if got := registry.Resolve("unknown"); got != defaultSurface {
		t.Fatalf("expected default surface, got %#v", got)
	}
}

func TestActivateSurfaceCachesPerAgent(t *testing.T) {
	surface := &stubSurface{name: "euclo", chat: stubChatPane{}}
	factory := &countingFactory{shared: surface}

	m := newRootModel(nil, factory)
	if factory.resolveCount != 1 {
		t.Fatalf("expected 1 resolve during init, got %d", factory.resolveCount)
	}

	startCount := factory.resolveCount
	m.activateSurface("euclo")
	m.activateSurface("euclo")

	if got := factory.resolveCount - startCount; got != 1 {
		t.Fatalf("expected cached activation to resolve euclo once, got %d resolves", got)
	}
	if m.activeSurface != surface {
		t.Fatalf("expected active surface to remain cached instance")
	}
	if got := factory.resolveByName["euclo"]; got != 1 {
		t.Fatalf("expected euclo to be resolved once, got %d", got)
	}
}
