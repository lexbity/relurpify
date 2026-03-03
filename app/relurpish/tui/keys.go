package tui

import "github.com/charmbracelet/bubbles/key"

// globalKeyMap holds all global keybindings for the TUI.
type globalKeyMap struct {
	Quit      key.Binding
	Help      key.Binding
	Tab1      key.Binding
	Tab2      key.Binding
	Tab3      key.Binding
	Tab4      key.Binding
	TabNext   key.Binding
	TabPrev   key.Binding
	ToggleBar key.Binding
	Search    key.Binding
}

// GlobalKeys is the application-wide keybinding set.
var GlobalKeys = globalKeyMap{
	Quit:      key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d"), key.WithHelp("ctrl+c", "quit")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Tab1:      key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "chat")),
	Tab2:      key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "tasks")),
	Tab3:      key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "session")),
	Tab4:      key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "settings")),
	TabNext:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	TabPrev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	ToggleBar: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "toggle title")),
	Search:    key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f", "search")),
}

// ShortHelp returns compact keybinding descriptions.
func (k globalKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Help, k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.ToggleBar}
}

// FullHelp returns the full keybinding table.
func (k globalKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.Help},
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4},
		{k.TabNext, k.TabPrev},
		{k.ToggleBar, k.Search},
	}
}
