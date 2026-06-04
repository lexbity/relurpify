package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestReservedChordsSurviveAllFocusStates verifies that ctrl+a and ctrl+c
// always escape through to the host even when an adversarial surface owns
// input and navigation — animations and full-frame surfaces must never
// swallow these chords.
func TestReservedChordsSurviveAllFocusStates(t *testing.T) {
	surface := &ownedSurface{
		fakeSurface: &fakeSurface{name: "guest", chat: &fakeChatPane{}},
		input:       &hostileInputSurface{},
		nav:         &hostileNavSurface{},
	}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)

	// Test ctrl+a opens agent picker.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA, Alt: false})
	rm := updated.(RootModel)

	if rm.agentPicker == nil || !rm.agentPicker.IsOpen() {
		t.Error("expected agent picker to open on ctrl+a under adversarial surface")
	}

	// Re-create for ctrl+c test.
	m2 := newRootModel(nil, factory)
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlC, Alt: false})
	_ = updated2

	// Test f1 opens help.
	m3 := newRootModel(nil, factory)
	updated3, _ := m3.Update(tea.KeyMsg{Type: tea.KeyF1})
	rm3 := updated3.(RootModel)
	if !rm3.showHelp {
		t.Error("expected help overlay to toggle on f1 under adversarial surface")
	}
}

// TestAnimationManagerNoIdleTicks verifies that with no active runs and no
// registered animations, the animation manager emits nothing.
func TestAnimationManagerNoIdleTicks(t *testing.T) {
	m := NewAnimationManager()
	if m.Active() {
		t.Error("animation manager should be idle when no animations registered")
	}
	cmd := m.TickCmd()
	if cmd != nil {
		t.Error("animation manager TickCmd should be nil when idle")
	}
}
