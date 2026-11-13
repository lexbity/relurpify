package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// KeyMap belongs to one host. No workspace or demo can alter another host's
// shortcuts. Escape routing and Ctrl+Q remain immutable host mechanics.
type KeyMap struct {
	Quit, Help, AgentPicker, FocusToggle, FocusRegion1, Attention, Commands key.Binding
	Undo, Redo, PageUp, ScrollDown                                          key.Binding
}

func NewKeyMap() *KeyMap {
	return &KeyMap{
		Quit:         key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d")),
		Help:         key.NewBinding(key.WithKeys("f1")),
		AgentPicker:  key.NewBinding(key.WithKeys("ctrl+a")),
		FocusToggle:  key.NewBinding(key.WithKeys("f6")),
		FocusRegion1: key.NewBinding(key.WithKeys("tab", "ctrl+down")),
		Attention:    key.NewBinding(key.WithKeys("ctrl+g")),
		Commands:     key.NewBinding(key.WithKeys("ctrl+p")),
		Undo:         key.NewBinding(key.WithKeys("ctrl+z")),
		Redo:         key.NewBinding(key.WithKeys("ctrl+y")),
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		ScrollDown:   key.NewBinding(key.WithKeys("pgdown")),
	}
}

func (keys *KeyMap) clone() *KeyMap {
	copy := NewKeyMap()
	source, destination := buildKeybindingTargets(keys), buildKeybindingTargets(copy)
	for i := range source {
		destination[i].Binding.SetKeys(append([]string(nil), source[i].Binding.Keys()...)...)
	}
	return copy
}

func keyMatchesBinding(binding key.Binding, value string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range binding.Keys() {
		if candidate == value {
			return true
		}
	}
	return false
}
func keyMsgMatchesBinding(message tea.KeyMsg, binding key.Binding) bool {
	return keyMatchesBinding(binding, message.String())
}
func isReservedChord(message tea.KeyMsg, keys *KeyMap) bool {
	for _, binding := range []key.Binding{keys.Quit, keys.Help, keys.AgentPicker} {
		if keyMsgMatchesBinding(message, binding) {
			return true
		}
	}
	return false
}

// validHostChord excludes printable global shortcuts, which would reinterpret
// ordinary Euclo requests. Local forms keep their own explicit editing keys.
func validHostChord(chord string) bool {
	if chord == "esc" || chord == "ctrl+q" || chord == "" || strings.ContainsAny(chord, "\x1b\x00\n\r ") {
		return false
	}
	// Validate against the same normalized events Bubble Tea actually emits.
	// Arbitrary modifier strings such as ctrl+potato are not terminal chords.
	for kind := tea.KeyF20; kind <= tea.KeyUp; kind++ {
		for _, alt := range []bool{false, true} {
			event := tea.KeyMsg{Type: kind, Alt: alt}
			if event.String() == chord && (kind != tea.KeySpace || alt) {
				return true
			}
		}
	}
	for kind := tea.KeyType(0); kind <= tea.KeyBackspace; kind++ {
		for _, alt := range []bool{false, true} {
			if (tea.KeyMsg{Type: kind, Alt: alt}).String() == chord {
				return true
			}
		}
	}
	if strings.HasPrefix(chord, "alt+") {
		value := strings.TrimPrefix(chord, "alt+")
		r, size := utf8.DecodeRuneInString(value)
		return value != "" && size == len(value) && unicode.IsGraphic(r) && !unicode.IsSpace(r)
	}
	return false
}

type keybindingTarget struct {
	Action, Scope, Description string
	DefaultKeys                []string
	Binding                    *key.Binding
}

func (target keybindingTarget) bindingKeys() []string { return target.Binding.Keys() }

func buildKeybindingTargets(keys *KeyMap) []keybindingTarget {
	return []keybindingTarget{
		{Action: "open_help", Scope: "global", Description: "Open contextual help", DefaultKeys: []string{"f1"}, Binding: &keys.Help},
		{Action: "agent_picker", Scope: "global", Description: "Choose an agent", DefaultKeys: []string{"ctrl+a"}, Binding: &keys.AgentPicker},
		{Action: "toggle_focus", Scope: "global", Description: "Move between canvas and composer", DefaultKeys: []string{"f6"}, Binding: &keys.FocusToggle},
		{Action: "focus_canvas", Scope: "composer", Description: "Focus the current canvas", DefaultKeys: []string{"tab", "ctrl+down"}, Binding: &keys.FocusRegion1},
		{Action: "open_attention", Scope: "global", Description: "Deliberately open a pending decision", DefaultKeys: []string{"ctrl+g"}, Binding: &keys.Attention},
		{Action: "open_commands", Scope: "global", Description: "Open commands while retaining the request draft", DefaultKeys: []string{"ctrl+p"}, Binding: &keys.Commands},
		{Action: "page_up", Scope: "reading", Description: "Scroll the conversation up", DefaultKeys: []string{"pgup"}, Binding: &keys.PageUp},
		{Action: "page_down", Scope: "reading", Description: "Scroll the conversation down", DefaultKeys: []string{"pgdown"}, Binding: &keys.ScrollDown},
		{Action: "undo", Scope: "canvas", Description: "Undo a local recipe/chat edit", DefaultKeys: []string{"ctrl+z"}, Binding: &keys.Undo},
		{Action: "redo", Scope: "canvas", Description: "Redo a local recipe/chat edit", DefaultKeys: []string{"ctrl+y"}, Binding: &keys.Redo},
		{Action: "quit", Scope: "global", Description: "Quit (Ctrl+Q always remains available)", DefaultKeys: []string{"ctrl+c", "ctrl+d"}, Binding: &keys.Quit},
	}
}
