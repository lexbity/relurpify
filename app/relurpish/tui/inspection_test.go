package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestV2InspectorRetainsReadingSelectionAcrossUpdates(t *testing.T) {
	p := documentInspector{}
	document := InspectionDocument{Title: "Configuration", Entries: []InspectionEntry{{ID: "a", Label: "First"}, {ID: "b", Label: "Second", Body: strings.Repeat("line\n", 30)}}}
	p.apply(document)
	p.key(tea.KeyMsg{Type: tea.KeyDown})
	p.key(tea.KeyMsg{Type: tea.KeyEnter})
	p.key(tea.KeyMsg{Type: tea.KeyDown})
	p.apply(document)
	if p.selected != "b" || !p.detail || p.offset != 1 {
		t.Fatal("refresh stole document selection or reading position")
	}
	document.Entries = document.Entries[:1]
	p.apply(document)
	if p.detail || p.notice == "" {
		t.Fatal("removed entry silently became another selected detail")
	}
}

func TestV2CommandBarBorrowRestoresExactDraft(t *testing.T) {
	m, _ := newFocusTestModel()
	const draft = "  repair 界\nthen verify  "
	m.inputBar.SetValue(draft)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(RootModel)
	if m.inputBar.Value() != "/" {
		t.Fatal("command shortcut did not borrow the universal bar")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	if m.inputBar.Value() != draft {
		t.Fatal("cancel changed request bytes")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(RootModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("settings")})
	m = next.(RootModel)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(RootModel)
	if cmd == nil {
		t.Fatal("command text did not produce an invocation")
	}
	next, _ = m.Update(cmd())
	m = next.(RootModel)
	if m.activeTab != TabSettings || m.inputBar.Value() != draft {
		t.Fatal("accepted navigation failed to restore the exact unsent request")
	}
}
