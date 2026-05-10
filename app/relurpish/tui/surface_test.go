package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeChatPane struct{}

func (fakeChatPane) Init() tea.Cmd                                                 { return nil }
func (fakeChatPane) Update(msg tea.Msg) (ChatPaner, tea.Cmd)                       { return fakeChatPane{}, nil }
func (fakeChatPane) View() string                                                  { return "" }
func (fakeChatPane) SetSize(int, int)                                              {}
func (fakeChatPane) SetSubTab(SubTabID)                                            {}
func (fakeChatPane) ActiveSubTab() SubTabID                                        { return "" }
func (fakeChatPane) HandleInputSubmit(string) tea.Cmd                              { return nil }
func (fakeChatPane) HasActiveRuns() bool                                           { return false }
func (fakeChatPane) StartRun(string) (tea.Cmd, string)                             { return nil, "" }
func (fakeChatPane) Undo() bool                                                    { return false }
func (fakeChatPane) Redo() bool                                                    { return false }
func (fakeChatPane) ToggleCompact()                                                {}
func (fakeChatPane) Cleanup()                                                      {}
func (fakeChatPane) AppendMessage(Message)                                         {}
func (fakeChatPane) ClearMessages()                                                {}
func (fakeChatPane) Messages() []Message                                           { return nil }
func (fakeChatPane) SetSearchFilter(string)                                        {}
func (fakeChatPane) ScrollUp()                                                     {}
func (fakeChatPane) PageDown()                                                     {}
func (fakeChatPane) PageUp()                                                       {}
func (fakeChatPane) AddSystemMessage(string)                                       {}
func (fakeChatPane) RollbackLastUndo()                                             {}
func (fakeChatPane) PushUndoSnapshot([]Message)                                    {}
func (fakeChatPane) HITLService() HITLServiceIface                                 { return nil }
func (fakeChatPane) StartRunWithMetadata(string, map[string]any) (tea.Cmd, string) { return nil, "" }
func (fakeChatPane) StartRunSilent(string) (tea.Cmd, string)                       { return nil, "" }
func (fakeChatPane) SetCompactRunID(string, int)                                   {}
func (fakeChatPane) AllowParallel() bool                                           { return false }
func (fakeChatPane) SetAllowParallel(bool)                                         {}
func (fakeChatPane) LastPrompt() string                                            { return "" }
func (fakeChatPane) StopLatestRun() tea.Cmd                                        { return nil }
func (fakeChatPane) RetryLastRun() tea.Cmd                                         { return nil }
func (fakeChatPane) ApplyPendingChanges(ChangeStatus) int                          { return 0 }
func (fakeChatPane) MutateMessages(func([]Message))                                {}
func (fakeChatPane) AddFile(string) tea.Cmd                                        { return nil }

type fakeSurface struct {
	name         string
	chat         ChatPaner
	resolveHit   *int
	tabCount     int
	commandCount int
}

func (s *fakeSurface) Name() string { return s.name }

func (s *fakeSurface) RegisterTabs(reg *TabRegistry) {
	s.tabCount++
	reg.Register(TabDefinition{ID: TabChat, Label: "chat"})
}

func (s *fakeSurface) RegisterCommands(reg *CommandRegistry) {
	s.commandCount++
	reg.Register(Command{Name: "surface-cmd", Usage: "/surface-cmd", Handler: func(m *RootModel, args []string) (*RootModel, tea.Cmd) {
		return m, nil
	}})
}

func (s *fakeSurface) NewChat(RuntimeAdapter, *AgentContext, *Session, *NotificationQueue) ChatPaner {
	if s.chat != nil {
		return s.chat
	}
	return fakeChatPane{}
}

func (s *fakeSurface) InitialTab() TabID                                        { return TabChat }
func (s *fakeSurface) InitialSubTab(TabID) SubTabID                             { return "" }
func (s *fakeSurface) RenderNotification(item NotificationItem) string          { return item.Msg }
func (s *fakeSurface) HandleFrame(context.Context, *RootModel, SurfaceFrameMsg) {}

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
	defaultSurface := &fakeSurface{name: "default"}
	registry := NewSurfaceRegistry(defaultSurface)
	custom := &fakeSurface{name: "euclo"}
	registry.Register("euclo", custom)

	if got := registry.Resolve("euclo"); got != custom {
		t.Fatalf("expected euclo surface, got %#v", got)
	}
	if got := registry.Resolve("unknown"); got != defaultSurface {
		t.Fatalf("expected default surface, got %#v", got)
	}
}

func TestActivateSurfaceCachesPerAgent(t *testing.T) {
	surface := &fakeSurface{name: "euclo", chat: fakeChatPane{}}
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
