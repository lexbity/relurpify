package tui

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeChatPane struct {
	width  int
	height int
}

func (p *fakeChatPane) Init() tea.Cmd { return nil }
func (p *fakeChatPane) Update(msg tea.Msg) (ChatPaner, tea.Cmd) {
	return p, nil
}
func (p *fakeChatPane) View() string                                                  { return "" }
func (p *fakeChatPane) SetSize(w, h int)                                              { p.width, p.height = w, h }
func (p *fakeChatPane) SetSubTab(SubTabID)                                            {}
func (p *fakeChatPane) ActiveSubTab() SubTabID                                        { return "" }
func (p *fakeChatPane) HandleInputSubmit(string) tea.Cmd                              { return nil }
func (p *fakeChatPane) HasActiveRuns() bool                                           { return false }
func (p *fakeChatPane) StartRun(string) (tea.Cmd, string)                             { return nil, "" }
func (p *fakeChatPane) Undo() bool                                                    { return false }
func (p *fakeChatPane) Redo() bool                                                    { return false }
func (p *fakeChatPane) ToggleCompact()                                                {}
func (p *fakeChatPane) Cleanup()                                                      {}
func (p *fakeChatPane) AppendMessage(Message)                                         {}
func (p *fakeChatPane) ClearMessages()                                                {}
func (p *fakeChatPane) Messages() []Message                                           { return nil }
func (p *fakeChatPane) SetSearchFilter(string)                                        {}
func (p *fakeChatPane) ScrollUp()                                                     {}
func (p *fakeChatPane) PageDown()                                                     {}
func (p *fakeChatPane) PageUp()                                                       {}
func (p *fakeChatPane) AddSystemMessage(string)                                       {}
func (p *fakeChatPane) RollbackLastUndo()                                             {}
func (p *fakeChatPane) PushUndoSnapshot([]Message)                                    {}
func (p *fakeChatPane) HITLService() HITLServiceIface                                 { return nil }
func (p *fakeChatPane) StartRunWithMetadata(string, map[string]any) (tea.Cmd, string) { return nil, "" }
func (p *fakeChatPane) StartRunSilent(string) (tea.Cmd, string)                       { return nil, "" }
func (p *fakeChatPane) SetCompactRunID(string, int)                                   {}
func (p *fakeChatPane) AllowParallel() bool                                           { return false }
func (p *fakeChatPane) SetAllowParallel(bool)                                         {}
func (p *fakeChatPane) LastPrompt() string                                            { return "" }
func (p *fakeChatPane) StopLatestRun() tea.Cmd                                        { return nil }
func (p *fakeChatPane) RetryLastRun() tea.Cmd                                         { return nil }
func (p *fakeChatPane) ApplyPendingChanges(ChangeStatus) int                          { return 0 }
func (p *fakeChatPane) MutateMessages(func([]Message))                                {}
func (p *fakeChatPane) AddFile(string) tea.Cmd                                        { return nil }

type fakeSurface struct {
	name         string
	chat         ChatPaner

	tabCount     int
	commandCount int
	tabs         []TabDefinition
	doctorReport DoctorReport
	doctorStatus string
}

func (s *fakeSurface) Name() string { return s.name }

func (s *fakeSurface) RegisterTabs(reg *TabRegistry) {
	s.tabCount++
	if len(s.tabs) > 0 {
		for _, tab := range s.tabs {
			reg.Register(tab)
		}
		return
	}
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
	return &fakeChatPane{}
}

func (s *fakeSurface) NewInput(RuntimeAdapter, *AgentContext, *Session) InputSurface {
	return nil
}

func (s *fakeSurface) NewNav(RuntimeAdapter, *AgentContext, *Session) NavSurface {
	return nil
}

func (s *fakeSurface) NewRegion1(RuntimeAdapter, *AgentContext, *Session, *SessionStore, *NotificationQueue) Region1Surface {
	return s
}

func (s *fakeSurface) InitialTab() TabID                                        { return TabChat }
func (s *fakeSurface) InitialSubTab(TabID) SubTabID                             { return "" }
func (s *fakeSurface) RenderNotification(item NotificationItem) string          { return item.Msg }
func (s *fakeSurface) HandleFrame(context.Context, *RootModel, SurfaceFrameMsg) {}
func (s *fakeSurface) Theme() *theme.Theme                                      { return nil }
func (s *fakeSurface) ResumeSession(_ context.Context, _ string) tea.Cmd        { return nil }
func (s *fakeSurface) DoctorReport() DoctorReport                               { return s.doctorReport }
func (s *fakeSurface) SetDoctorReport(report DoctorReport)                      { s.doctorReport = report }
func (s *fakeSurface) SetDoctorStatus(status string)                            { s.doctorStatus = status }
func (s *fakeSurface) SetSize(int, int)                                         {}
func (s *fakeSurface) SetStore(*SessionStore)                                   {}
func (s *fakeSurface) SetActiveTab(TabID)                                       {}
func (s *fakeSurface) SetFilter(string)                                         {}
func (s *fakeSurface) Refresh()                                                 {}
func (s *fakeSurface) Update(msg tea.Msg) (Region1Surface, tea.Cmd)             { return s, nil }
func (s *fakeSurface) View() string                                             { return "" }
func (s *fakeSurface) HandleInputSubmit(string) tea.Cmd                         { return nil }
func (s *fakeSurface) Cleanup()                                                 {}
func (s *fakeSurface) FocusFilescopes()                                         {}
func (s *fakeSurface) OpenSecurityGuard()                                       {}
func (s *fakeSurface) OpenAIProvider()                                          {}
func (s *fakeSurface) OpenKeybindings()                                         {}
func (s *fakeSurface) OpenDoctor()                                              {}

type hostileInputSurface struct {
	keys []string
}

func (s *hostileInputSurface) SetSize(int, int) {}
func (s *hostileInputSurface) View() string     { return "hostile-input" }
func (s *hostileInputSurface) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	s.keys = append(s.keys, msg.String())
	return nil, true
}

type hostileNavSurface struct {
	keys []string
}

func (s *hostileNavSurface) SetSize(int, int) {}
func (s *hostileNavSurface) View() string     { return "hostile-nav" }
func (s *hostileNavSurface) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	s.keys = append(s.keys, msg.String())
	return nil, true
}

type ownedSurface struct {
	*fakeSurface
	input *hostileInputSurface
	nav   *hostileNavSurface
}

func (s *ownedSurface) NewInput(RuntimeAdapter, *AgentContext, *Session) InputSurface {
	return s.input
}

func (s *ownedSurface) NewNav(RuntimeAdapter, *AgentContext, *Session) NavSurface {
	return s.nav
}

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

func (f *countingFactory) AvailableAgents() []string {
	if f.shared == nil {
		return nil
	}
	name := normalizeSurfaceKey(f.shared.Name())
	if name == "" || name == "none" {
		return nil
	}
	return []string{name}
}

func TestRootModelResizeAllocatesChromeRows(t *testing.T) {
	surface := &fakeSurface{name: "guest", chat: &fakeChatPane{}}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)
	q := &NotificationQueue{}
	m.notifQ = q
	m.notifBar = NewNotificationBar(q)
	q.Push(NotificationItem{ID: "hitl-1", Kind: NotifKindHITL, Msg: "approval required"})

	resized, _ := m.handleResize(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := resized.(RootModel)

	if got, want := rm.layout.Region1Rows(), 37; got != want {
		t.Fatalf("region1 rows = %d, want %d", got, want)
	}
	if got, want := rm.layout.Region1PaneRows(), 36; got != want {
		t.Fatalf("region1 pane rows = %d, want %d", got, want)
	}
	if got, want := rm.layout.Region2Width(), chromeAgentWidth; got != want {
		t.Fatalf("region2 width = %d, want %d", got, want)
	}
	if got, want := rm.layout.Region3Width(), 100; got != want {
		t.Fatalf("region3 width = %d, want %d", got, want)
	}
	if got := surface.chat.(*fakeChatPane).height; got != 36 {
		t.Fatalf("chat height = %d, want %d", got, 36)
	}
	if got := rm.inputBar.width; got != 100 {
		t.Fatalf("input width = %d, want %d", got, 100)
	}
}

func TestReservedChordsBypassSurfaceOwnedInput(t *testing.T) {
	surface := &ownedSurface{
		fakeSurface: &fakeSurface{name: "guest", chat: &fakeChatPane{}},
		input:       &hostileInputSurface{},
		nav:         &hostileNavSurface{},
	}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA, Alt: false})
	rm := updated.(RootModel)

	if rm.agentPicker == nil || !rm.agentPicker.IsOpen() {
		t.Fatal("expected agent picker to open on ctrl+a")
	}
	if len(surface.input.keys) != 0 {
		t.Fatalf("expected owned input surface to be bypassed, got %#v", surface.input.keys)
	}
	if len(surface.nav.keys) != 0 {
		t.Fatalf("expected owned nav surface to be bypassed, got %#v", surface.nav.keys)
	}

	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyF1})
	rm = updated.(RootModel)

	if !rm.showHelp {
		t.Fatal("expected help overlay to toggle on f1")
	}
	if len(surface.input.keys) != 0 {
		t.Fatalf("expected help key to bypass owned input surface, got %#v", surface.input.keys)
	}
	if len(surface.nav.keys) != 0 {
		t.Fatalf("expected help key to bypass owned nav surface, got %#v", surface.nav.keys)
	}
}
