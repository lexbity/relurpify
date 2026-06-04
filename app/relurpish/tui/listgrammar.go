package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Action describes a single user-facing verb shown in the action footer.
type Action struct {
	Label       string // e.g. "up/down navigate"
	Key         string // e.g. "↑↓"
	Description string // optional longer text
}

// ListEditor is the interface that a list-based pane implements so the
// universal ListGrammar can dispatch keyboard input to it.
type ListEditor interface {
	// Actions returns the list of currently valid actions for the footer.
	Actions() []Action

	// ItemCount returns the number of items in the list.
	ItemCount() int

	// Selected returns the index of the currently selected item.
	Selected() int

	// Move moves the selection by delta (+1 down, -1 up). Returns the
	// new index (clamped).
	Move(delta int) int

	// OnActivate is called when enter is pressed on the selected item.
	OnActivate() tea.Cmd

	// OnToggle is called when space is pressed. This is the universal
	// cycle / toggle action.
	OnToggle() tea.Cmd

	// OnNew is called when n is pressed.
	OnNew() tea.Cmd

	// OnDelete is called when d is pressed.
	OnDelete() tea.Cmd

	// OnFilter sets a text filter on the list.
	OnFilter(query string)
}

// ListGrammar dispatches the universal list-pane keyboard grammar to a
// ListEditor.  It handles navigation (↑↓/jk), activation (enter), toggle
// (space), create (n), delete (d), filter (/), and cancel (esc).
type ListGrammar struct{}

// HandleKey routes a key press to the editor.  Returns the resulting command
// and whether the key was consumed.
func (ListGrammar) HandleKey(editor ListEditor, msg tea.KeyMsg) (tea.Cmd, bool) {
	if editor == nil {
		return nil, false
	}
	switch msg.String() {
	case "up", "k":
		editor.Move(-1)
		return nil, true
	case "down", "j":
		editor.Move(1)
		return nil, true
	case "pgup":
		editor.Move(-5)
		return nil, true
	case "pgdown":
		editor.Move(5)
		return nil, true
	case "home":
		for editor.Selected() > 0 {
			editor.Move(-1)
		}
		return nil, true
	case "end":
		for editor.Selected() < editor.ItemCount()-1 {
			editor.Move(1)
		}
		return nil, true
	case "enter":
		return editor.OnActivate(), true
	case " ":
		return editor.OnToggle(), true
	case "n":
		return editor.OnNew(), true
	case "d":
		return editor.OnDelete(), true
	case "esc":
		editor.OnFilter("")
		return nil, true
	case "/", ":":
		// Filter mode is entered via the input bar — this just prevents
		// the key from propagating further in list contexts.
		return nil, true
	}
	return nil, false
}

// DefaultActions returns the universal set of list-pane footer actions.
func (ListGrammar) DefaultActions() []Action {
	return []Action{
		{Label: "up/down navigate", Key: "↑↓"},
		{Label: "activate", Key: "enter"},
		{Label: "toggle/cycle", Key: "space"},
		{Label: "new", Key: "n"},
		{Label: "delete", Key: "d"},
		{Label: "filter", Key: "/"},
		{Label: "cancel", Key: "esc"},
	}
}
