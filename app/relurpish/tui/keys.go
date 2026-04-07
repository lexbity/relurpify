package tui

import "github.com/charmbracelet/bubbles/key"

// globalKeyMap holds all global keybindings for the TUI.
type globalKeyMap struct {
	// Navigation
	Quit    key.Binding
	Help    key.Binding
	Tab1    key.Binding
	Tab2    key.Binding
	Tab3    key.Binding
	Tab4    key.Binding
	TabNext key.Binding
	TabPrev key.Binding

	// Chat operations
	Undo       key.Binding
	Redo       key.Binding
	ScrollUp   key.Binding
	ScrollDown key.Binding
	PageUp     key.Binding
	FilePicker key.Binding
	Compact    key.Binding
	ToggleSidebar key.Binding

	// UI toggles
	ToggleBar key.Binding
	SearchMode key.Binding
}

// GlobalKeys is the application-wide keybinding set.
var GlobalKeys = globalKeyMap{
	// Navigation
	Quit:    key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d"), key.WithHelp("ctrl+c", "quit")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Tab1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "chat")),
	Tab2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "tasks")),
	Tab3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "session")),
	Tab4:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "settings")),
	TabNext: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	TabPrev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

	// Chat operations
	Undo:         key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "undo")),
	Redo:         key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "redo")),
	ScrollUp:     key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "scroll up")),
	ScrollDown:   key.NewBinding(key.WithKeys("pagedown"), key.WithHelp("pagedown", "scroll down")),
	PageUp:       key.NewBinding(key.WithKeys("pageup"), key.WithHelp("pageup", "page up")),
	FilePicker:   key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "file picker")),
	Compact:      key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "compact")),
	ToggleSidebar: key.NewBinding(key.WithKeys("ctrl+]"), key.WithHelp("ctrl+]", "toggle sidebar")),

	// UI toggles
	ToggleBar:  key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "toggle title")),
	SearchMode: key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f", "search")),
}

// ShortHelp returns compact keybinding descriptions.
func (k globalKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Quit,
		k.Help,
		k.Tab1,
		k.Tab2,
		k.Tab3,
		k.Tab4,
		k.Undo,
		k.Redo,
	}
}

// FullHelp returns the full keybinding table organized by category.
func (k globalKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Quit and help
		{k.Quit, k.Help},

		// Tab navigation
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4},
		{k.TabNext, k.TabPrev},

		// Chat operations
		{k.Undo, k.Redo},
		{k.ScrollUp, k.ScrollDown, k.PageUp},
		{k.FilePicker, k.Compact, k.ToggleSidebar},

		// UI toggles and search
		{k.ToggleBar, k.SearchMode},
	}
}
