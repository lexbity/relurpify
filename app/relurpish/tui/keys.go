package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// globalKeyMap holds all global keybindings for the TUI.
type globalKeyMap struct {
	// Navigation
	Quit         key.Binding
	Help         key.Binding
	AgentPicker  key.Binding
	Tab1         key.Binding
	Tab2         key.Binding
	Tab3         key.Binding
	Tab4         key.Binding
	Tab5         key.Binding
	Tab6         key.Binding
	FocusRegion1 key.Binding

	// Chat operations
	Undo          key.Binding
	Redo          key.Binding
	ScrollUp      key.Binding
	ScrollDown    key.Binding
	PageUp        key.Binding
	FilePicker    key.Binding
	Compact       key.Binding
	ToggleSidebar key.Binding

	// UI toggles
	SearchMode key.Binding
	// Sidebar operations (chat context sidebar)
	SidebarToggle key.Binding

	// Service operations (session services subtab)
	ServiceStop       key.Binding
	ServiceRestart    key.Binding
	ServiceRestartAll key.Binding
}

// GlobalKeys is the application-wide keybinding set.
var GlobalKeys = globalKeyMap{
	// Navigation
	Quit:         key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d"), key.WithHelp("ctrl+c", "quit")),
	Help:         key.NewBinding(key.WithKeys("f1"), key.WithHelp("f1", "help")),
	AgentPicker:  key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "agent picker")),
	Tab1:         key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "welcome")),
	Tab2:         key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "sandbox")),
	Tab3:         key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "security")),
	Tab4:         key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "ai provider")),
	Tab5:         key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "keybindings")),
	Tab6:         key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "doctor")),
	FocusRegion1: key.NewBinding(key.WithKeys("tab", "ctrl+down"), key.WithHelp("tab/ctrl+down", "focus region 1")),

	// Chat operations
	Undo:          key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "undo")),
	Redo:          key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "redo")),
	ScrollUp:      key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "scroll up")),
	ScrollDown:    key.NewBinding(key.WithKeys("pagedown"), key.WithHelp("pagedown", "scroll down")),
	PageUp:        key.NewBinding(key.WithKeys("pageup"), key.WithHelp("pageup", "page up")),
	FilePicker:    key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "file picker")),
	Compact:       key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "compact")),
	ToggleSidebar: key.NewBinding(key.WithKeys("ctrl+]"), key.WithHelp("ctrl+]", "toggle sidebar")),

	// UI toggles
	SearchMode: key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f", "search")),

	// Sidebar operations
	SidebarToggle: key.NewBinding(key.WithKeys("ctrl+]"), key.WithHelp("ctrl+]", "toggle sidebar")),

	// Service operations
	ServiceStop:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stop service")),
	ServiceRestart:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart service")),
	ServiceRestartAll: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restart all services")),
}

// ReservedChords are consumed by the host before any surface or region can
// handle the key event.
var ReservedChords = []key.Binding{
	GlobalKeys.Quit,
	GlobalKeys.Help,
	GlobalKeys.AgentPicker,
}

func keyMatchesBinding(binding key.Binding, keyStr string) bool {
	keyStr = strings.ToLower(strings.TrimSpace(keyStr))
	if keyStr == "" {
		return false
	}
	for _, candidate := range binding.Keys() {
		if strings.ToLower(strings.TrimSpace(candidate)) == keyStr {
			return true
		}
	}
	return false
}

func keyMsgMatchesBinding(msg tea.KeyMsg, binding key.Binding) bool {
	return keyMatchesBinding(binding, msg.String())
}

func isReservedChord(msg tea.KeyMsg) bool {
	for _, binding := range ReservedChords {
		if keyMsgMatchesBinding(msg, binding) {
			return true
		}
	}
	return false
}

// ShortHelp returns compact keybinding descriptions.
func (k globalKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Quit,
		k.Help,
		k.AgentPicker,
		k.Tab1,
		k.Tab2,
		k.Tab3,
		k.Tab4,
		k.Tab5,
		k.Tab6,
		k.FocusRegion1,
		k.Undo,
		k.Redo,
	}
}

// FullHelp returns the full keybinding table organized by category.
func (k globalKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Quit and help
		{k.Quit, k.Help},
		{k.AgentPicker},

		// Tab navigation
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Tab5, k.Tab6},
		{k.FocusRegion1},

		// Chat operations
		{k.Undo, k.Redo},
		{k.ScrollUp, k.ScrollDown, k.PageUp},
		{k.FilePicker, k.Compact, k.ToggleSidebar},

		// UI toggles and search
		{k.SearchMode},

		// Sidebar operations (chat context)
		{k.SidebarToggle},

		// Service operations (session services subtab)
		{k.ServiceStop, k.ServiceRestart, k.ServiceRestartAll},
	}
}
