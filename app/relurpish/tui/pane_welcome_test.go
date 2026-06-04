package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWelcomePaneStartEmitsStartSessionMsg(t *testing.T) {
	pane := NewWelcomePane(&Session{}, nil, &welcomeFactory{agents: []string{"none", "euclo"}})
	pane.SetSize(80, 24)

	// Focus the Start button by navigating to index 1.
	// Focus ring: 0=agent drop, 1=Start, 2=resume drop, 3=Resume, 4=Doctor, 5=Help
	for pane.focusIdx != 1 {
		pane.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	}

	// Enter on Start.
	_, cmd := pane.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected Start to emit a command")
	}
	msg := cmd()
	startMsg, ok := msg.(StartSessionMsg)
	if !ok {
		t.Fatalf("command produced %T, want StartSessionMsg", msg)
	}
	if startMsg.Agent == "" {
		t.Error("StartSessionMsg should have non-empty Agent")
	}
}

func TestWelcomePaneAgentDropdownListsAgents(t *testing.T) {
	factory := &welcomeFactory{agents: []string{"none", "euclo"}}
	pane := NewWelcomePane(&Session{}, nil, factory)
	pane.SetSize(80, 24)

	// Open the dropdown to verify items are rendered.
	pane.agentDrop.Open()
	view := pane.agentDrop.View()
	if !strings.Contains(view, "euclo") {
		t.Errorf("agent dropdown view missing euclo: %s", view)
	}
}

func TestWelcomePaneResumeDropdownPopulated(t *testing.T) {
	store := NewSessionStore(t.TempDir())
	rec := SessionRecord{SessionMeta: SessionMeta{ID: "s1", Agent: "euclo"}}
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}
	pane := NewWelcomePane(&Session{}, store, nil)
	pane.SetSize(80, 24)

	// Verify session shows in resume dropdown.
	pane.resumeDrop.Open()
	view := pane.resumeDrop.View()
	if !strings.Contains(view, "euclo") {
		t.Errorf("resume dropdown view missing agent label: %s", view)
	}
}

func TestWelcomePaneFocusRingOrder(t *testing.T) {
	pane := NewWelcomePane(&Session{}, nil, nil)
	pane.SetSize(80, 24)

	// Tab cycles through focus ring.
	states := []int{0, 1, 2, 3, 4, 5, 0}
	for i, want := range states {
		if pane.focusIdx != want {
			t.Fatalf("step %d: focusIdx = %d, want %d", i, pane.focusIdx, want)
		}
		pane.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	}
}

func TestWelcomePaneRenders(t *testing.T) {
	pane := NewWelcomePane(&Session{}, nil, nil)
	pane.SetSize(80, 24)
	view := pane.View()
	if !strings.Contains(view, "New Session") {
		t.Errorf("view missing 'New Session': %s", view)
	}
	if !strings.Contains(view, "Start") {
		t.Errorf("view missing 'Start': %s", view)
	}
	if !strings.Contains(view, "Doctor") {
		t.Errorf("view missing 'Doctor': %s", view)
	}
}

type welcomeFactory struct {
	agents []string
}

func (f *welcomeFactory) Resolve(string) AgentSurface { return nil }
func (f *welcomeFactory) AvailableAgents() []string {
	if f == nil {
		return nil
	}
	return f.agents
}
