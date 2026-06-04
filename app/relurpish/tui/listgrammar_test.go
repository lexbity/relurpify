package tui

import (
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	tea "github.com/charmbracelet/bubbletea"
)

type testListEditor struct {
	items    []string
	sel      int
	activate bool
	toggled  bool
	created  bool
	deleted  bool
	filter   string
}

func (e *testListEditor) Actions() []Action {
	actions := (ListGrammar{}).DefaultActions()
	if e.items == nil {
		actions = append(actions, Action{Label: "custom", Key: "c"})
	}
	return actions
}
func (e *testListEditor) ItemCount() int { return len(e.items) }
func (e *testListEditor) Selected() int  { return e.sel }
func (e *testListEditor) Move(delta int) int {
	e.sel += delta
	if e.sel < 0 {
		e.sel = 0
	}
	if e.sel >= len(e.items) {
		e.sel = len(e.items) - 1
	}
	return e.sel
}
func (e *testListEditor) OnActivate() tea.Cmd { e.activate = true; return nil }
func (e *testListEditor) OnToggle() tea.Cmd   { e.toggled = true; return nil }
func (e *testListEditor) OnNew() tea.Cmd      { e.created = true; return nil }
func (e *testListEditor) OnDelete() tea.Cmd   { e.deleted = true; return nil }
func (e *testListEditor) OnFilter(q string)   { e.filter = q }

func TestListGrammarNavigateUpDown(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a", "b", "c"}}

	g.HandleKey(e, tea.KeyMsg{Type: tea.KeyDown})
	if e.sel != 1 {
		t.Errorf("after down: sel=%d, want 1", e.sel)
	}
	g.HandleKey(e, tea.KeyMsg{Type: tea.KeyUp})
	if e.sel != 0 {
		t.Errorf("after up: sel=%d, want 0", e.sel)
	}
}

func TestListGrammarNavigateJK(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a", "b", "c"}}

	g.HandleKey(e, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if e.sel != 1 {
		t.Errorf("after j: sel=%d, want 1", e.sel)
	}
	g.HandleKey(e, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if e.sel != 0 {
		t.Errorf("after k: sel=%d, want 0", e.sel)
	}
}

func TestListGrammarEnter(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a"}}

	cmd, handled := g.HandleKey(e, tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled")
	}
	if !e.activate {
		t.Error("OnActivate should be called")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestListGrammarSpace(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a"}}

	_, handled := g.HandleKey(e, tea.KeyMsg{Type: tea.KeySpace})
	if !handled {
		t.Fatal("space should be handled")
	}
	if !e.toggled {
		t.Error("OnToggle should be called")
	}
}

func TestListGrammarNew(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a"}}

	_, handled := g.HandleKey(e, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !handled {
		t.Fatal("n should be handled")
	}
	if !e.created {
		t.Error("OnNew should be called")
	}
}

func TestListGrammarDelete(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a"}}

	_, handled := g.HandleKey(e, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !handled {
		t.Fatal("d should be handled")
	}
	if !e.deleted {
		t.Error("OnDelete should be called")
	}
}

func TestListGrammarEscClearsFilter(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a"}}

	_, handled := g.HandleKey(e, tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Fatal("esc should be handled")
	}
}

func TestListGrammarHomeEnd(t *testing.T) {
	var g ListGrammar
	e := &testListEditor{items: []string{"a", "b", "c", "d", "e"}}
	e.sel = 2

	g.HandleKey(e, tea.KeyMsg{Type: tea.KeyHome})
	if e.sel != 0 {
		t.Errorf("after home: sel=%d, want 0", e.sel)
	}

	g.HandleKey(e, tea.KeyMsg{Type: tea.KeyEnd})
	if e.sel != 4 {
		t.Errorf("after end: sel=%d, want 4", e.sel)
	}
}

func TestActionFooterDefaultVisible(t *testing.T) {
	f := NewActionFooter()
	if !f.Visible() {
		t.Error("footer should be visible by default")
	}
}

func TestActionFooterToggle(t *testing.T) {
	f := NewActionFooter()
	f.ToggleVisibility()
	if f.Visible() {
		t.Error("footer should be hidden after toggle")
	}
	f.ToggleVisibility()
	if !f.Visible() {
		t.Error("footer should be visible after second toggle")
	}
}

func TestActionFooterRendersActions(t *testing.T) {
	f := NewActionFooter()
	f.SetTheme(theme.Default())
	f.SetActions([]Action{
		{Label: "navigate", Key: "↑↓"},
		{Label: "activate", Key: "enter"},
	})
	view := f.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestActionFooterHidden(t *testing.T) {
	f := NewActionFooter()
	f.ToggleVisibility()
	view := f.View()
	if view != "" {
		t.Error("hidden footer should render empty")
	}
}

func TestActionFooterNilSafe(t *testing.T) {
	var f *ActionFooter
	if f.Visible() {
		t.Error("nil footer should not be visible")
	}
	if f.View() != "" {
		t.Error("nil footer view should be empty")
	}
	f.ToggleVisibility()
	f.SetActions(nil)
}
