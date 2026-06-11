package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

func TestDropdownInitialState(t *testing.T) {
	d := NewDropdown("Agent", []DropdownItem{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	if d.IsOpen() {
		t.Error("dropdown should start closed")
	}
	// Initially the first item is selected (sel=0).
	if d.SelectedID() != "a" {
		t.Errorf("first item = %q, want a", d.SelectedID())
	}
}

func TestDropdownOpenCloseKeyboard(t *testing.T) {
	d := NewDropdown("Agent", []DropdownItem{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	d.Focus()

	// Enter opens.
	cmd, handled := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled")
	}
	if cmd != nil {
		t.Fatal("open should not produce a command")
	}
	if !d.IsOpen() {
		t.Error("dropdown should be open after enter")
	}

	// Esc closes.
	_, handled = d.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Fatal("esc should be handled")
	}
	if d.IsOpen() {
		t.Error("dropdown should be closed after esc")
	}
}

func TestDropdownNavigate(t *testing.T) {
	d := NewDropdown("Test", []DropdownItem{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B"},
		{ID: "c", Label: "C"},
	})
	d.Focus()
	d.Open()

	// Down moves.
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.sel != 1 {
		t.Errorf("after down, sel = %d, want 1", d.sel)
	}
	// Up moves.
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.sel != 0 {
		t.Errorf("after up, sel = %d, want 0", d.sel)
	}
}

func TestDropdownSelect(t *testing.T) {
	d := NewDropdown("Test", []DropdownItem{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B"},
	})
	d.Focus()
	d.Open()
	d.Update(tea.KeyMsg{Type: tea.KeyDown})

	cmd, handled := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter on open dropdown should be handled")
	}
	if cmd == nil {
		t.Fatal("select should produce a command")
	}
	msg := cmd()
	sel, ok := msg.(DropdownSelectedMsg)
	if !ok {
		t.Fatalf("command produced %T, want DropdownSelectedMsg", msg)
	}
	if sel.ID != "b" || sel.Label != "B" {
		t.Errorf("selected = %+v, want {b B}", sel)
	}
	if d.IsOpen() {
		t.Error("dropdown should close after selection")
	}
}

func TestDropdownMouseClick(t *testing.T) {
	d := NewDropdown("Agent", []DropdownItem{
		{ID: "x", Label: "X"},
	})
	d.SetRenderPos(10, 5)
	d.SetWidth(20)

	// Click on closed dropdown.
	cmd, handled := d.HandleClick(12, 5)
	if !handled {
		t.Fatal("click on closed dropdown should be handled")
	}
	if cmd != nil {
		t.Fatal("open should not produce command")
	}
	if !d.IsOpen() {
		t.Error("click should open dropdown")
	}

	// Click on an item.
	cmd, handled = d.HandleClick(12, 6)
	if !handled {
		t.Fatal("click on item should be handled")
	}
	if cmd == nil {
		t.Fatal("item click should produce command")
	}
	msg := cmd()
	sel, ok := msg.(DropdownSelectedMsg)
	if !ok || sel.ID != "x" {
		t.Errorf("item click produced %+v, want {x X}", sel)
	}
}

func TestDropdownClickOutside(t *testing.T) {
	d := NewDropdown("Test", []DropdownItem{{ID: "a", Label: "A"}})
	d.Open()
	d.SetRenderPos(10, 5)
	d.SetWidth(20)

	// Click far away.
	cmd, handled := d.HandleClick(1, 1)
	if !handled {
		t.Fatal("click outside open dropdown should close it")
	}
	if cmd != nil {
		t.Fatal("close should not produce command")
	}
	if d.IsOpen() {
		t.Error("click outside should close dropdown")
	}
}

func TestDropdownNotFocusedIgnoreKeys(t *testing.T) {
	d := NewDropdown("Test", []DropdownItem{{ID: "a", Label: "A"}})
	_, handled := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if handled {
		t.Error("unfocused dropdown should not handle keys")
	}
}

func TestDropdownRender(t *testing.T) {
	d := NewDropdown("Agent", []DropdownItem{
		{ID: "a", Label: "Alpha"},
		{ID: "b", Label: "Beta"},
	})
	d.SetTheme(theme.Default())
	view := d.View()
	if !strings.Contains(view, "Agent") {
		t.Errorf("view missing label: %s", view)
	}
	if !strings.Contains(view, "▾") {
		t.Errorf("closed dropdown should show ▾: %s", view)
	}

	d.Open()
	view = d.View()
	if !strings.Contains(view, "Alpha") {
		t.Errorf("open dropdown missing items: %s", view)
	}
}
