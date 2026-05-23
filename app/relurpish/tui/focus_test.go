package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type recordingChatPane struct {
	keys []string
}

func (p *recordingChatPane) Init() tea.Cmd { return nil }
func (p *recordingChatPane) Update(msg tea.Msg) (ChatPaner, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		p.keys = append(p.keys, k.String())
	}
	return p, nil
}
func (p *recordingChatPane) View() string                      { return "" }
func (p *recordingChatPane) SetSize(int, int)                  {}
func (p *recordingChatPane) SetSubTab(SubTabID)                {}
func (p *recordingChatPane) ActiveSubTab() SubTabID            { return "" }
func (p *recordingChatPane) HandleInputSubmit(string) tea.Cmd  { return nil }
func (p *recordingChatPane) HasActiveRuns() bool               { return false }
func (p *recordingChatPane) StartRun(string) (tea.Cmd, string) { return nil, "" }
func (p *recordingChatPane) Undo() bool                        { return false }
func (p *recordingChatPane) Redo() bool                        { return false }
func (p *recordingChatPane) ToggleCompact()                    {}
func (p *recordingChatPane) Cleanup()                          {}
func (p *recordingChatPane) AppendMessage(Message)             {}
func (p *recordingChatPane) ClearMessages()                    {}
func (p *recordingChatPane) Messages() []Message               { return nil }
func (p *recordingChatPane) SetSearchFilter(string)            {}
func (p *recordingChatPane) ScrollUp()                         {}
func (p *recordingChatPane) PageDown()                         {}
func (p *recordingChatPane) PageUp()                           {}
func (p *recordingChatPane) AddSystemMessage(string)           {}
func (p *recordingChatPane) RollbackLastUndo()                 {}
func (p *recordingChatPane) PushUndoSnapshot([]Message)        {}
func (p *recordingChatPane) HITLService() HITLServiceIface     { return nil }
func (p *recordingChatPane) StartRunWithMetadata(string, map[string]any) (tea.Cmd, string) {
	return nil, ""
}
func (p *recordingChatPane) StartRunSilent(string) (tea.Cmd, string) { return nil, "" }
func (p *recordingChatPane) SetCompactRunID(string, int)             {}
func (p *recordingChatPane) AllowParallel() bool                     { return false }
func (p *recordingChatPane) SetAllowParallel(bool)                   {}
func (p *recordingChatPane) LastPrompt() string                      { return "" }
func (p *recordingChatPane) StopLatestRun() tea.Cmd                  { return nil }
func (p *recordingChatPane) RetryLastRun() tea.Cmd                   { return nil }
func (p *recordingChatPane) ApplyPendingChanges(ChangeStatus) int    { return 0 }
func (p *recordingChatPane) MutateMessages(func([]Message))          {}
func (p *recordingChatPane) AddFile(string) tea.Cmd                  { return nil }

func newFocusTestModel() (RootModel, *recordingChatPane) {
	chat := &recordingChatPane{}
	surface := &fakeSurface{name: "euclo", chat: chat}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)
	return m, chat
}

func TestFocusMovesIntoRegion1FromInput(t *testing.T) {
	m, _ := newFocusTestModel()
	if !m.focus.State().InInput() {
		t.Fatalf("expected input focus at startup")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := updated.(RootModel)
	if !rm.focus.State().InRegion1() {
		t.Fatalf("expected focus to move into region 1 on tab")
	}
	if rm.inputBar.Focused() {
		t.Fatalf("expected input bar to blur when region 1 owns focus")
	}
}

func TestCtrlDownMovesIntoRegion1(t *testing.T) {
	m, _ := newFocusTestModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	rm := updated.(RootModel)
	if !rm.focus.State().InRegion1() {
		t.Fatalf("expected ctrl+down to move focus into region 1")
	}
}

func TestPrintableCharacterReturnsToInputWithoutDroppingRune(t *testing.T) {
	m, chat := newFocusTestModel()
	m.setFocus(FocusRegionRegion1)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	rm := updated.(RootModel)

	if !rm.focus.State().InInput() {
		t.Fatalf("expected printable character to return focus to input")
	}
	if got := rm.inputBar.Value(); got != "a" {
		t.Fatalf("input value = %q, want %q", got, "a")
	}
	if len(chat.keys) != 0 {
		t.Fatalf("expected printable char to bypass region 1 routing, got %#v", chat.keys)
	}
}

func TestEscReturnsFocusToInput(t *testing.T) {
	m, _ := newFocusTestModel()
	m.setFocus(FocusRegionRegion1)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm := updated.(RootModel)
	if !rm.focus.State().InInput() {
		t.Fatalf("expected esc to return focus to input")
	}
}

func TestArrowKeysRouteToRegion1WhenFocused(t *testing.T) {
	m, chat := newFocusTestModel()
	m.setFocus(FocusRegionRegion1)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	rm := updated.(RootModel)

	if !rm.focus.State().InRegion1() {
		t.Fatalf("expected arrow key to keep focus in region 1")
	}
	if len(chat.keys) != 1 || chat.keys[0] != "up" {
		t.Fatalf("expected arrow key to route to region 1, got %#v", chat.keys)
	}
}

func TestFocusRouterKeepsRegion1TabAvailableToSurface(t *testing.T) {
	m, chat := newFocusTestModel()
	m.setFocus(FocusRegionRegion1)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := updated.(RootModel)

	if !rm.focus.State().InRegion1() {
		t.Fatalf("expected tab to keep region 1 focused when already inside it")
	}
	if len(chat.keys) != 1 || chat.keys[0] != "tab" {
		t.Fatalf("expected tab to route to the active surface, got %#v", chat.keys)
	}
}

func TestFocusModelInitializesWithoutRuntime(t *testing.T) {
	m, _ := newFocusTestModel()
	if m.inputBar == nil {
		t.Fatal("expected input bar to exist")
	}
	if !m.inputBar.Focused() {
		t.Fatal("expected input bar to own focus at startup")
	}
}
