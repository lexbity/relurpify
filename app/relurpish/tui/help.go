package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// helpCommands reads the same scoped catalog as completion and dispatch.
func (m RootModel) helpCommands() []Command {
	if m.cmdReg == nil {
		return nil
	}
	commands := m.cmdReg.Match("", m.activeTab, m.tabs.ActiveSubTab())
	sort.Slice(commands, func(i, j int) bool {
		iLocal := commands[i].Name == string(m.activeTab) || len(commands[i].TabFilter) > 0
		jLocal := commands[j].Name == string(m.activeTab) || len(commands[j].TabFilter) > 0
		if iLocal != jLocal {
			return iLocal
		}
		return commands[i].Name < commands[j].Name
	})
	return commands
}

// helpLines provides selected-command detail without an exhaustive overlay.
func (m RootModel) helpLines() []string {
	lines := []string{"Commands / " + terminalText(string(m.activeTab)), "Plain text always requests Euclo.", strings.Join(m.keys.FocusToggle.Keys(), " / ") + ": change focus · " + strings.Join(m.keys.Attention.Keys(), " / ") + ": attention", "Alt+Enter: newline · Alt+F: complete @ reference · Ctrl+Q: quit review", ""}
	if m.cmdReg == nil {
		return append(lines, "Command catalog unavailable")
	}
	if m.helpCommand != "" {
		if command, ok := m.cmdReg.Lookup(m.helpCommand); ok {
			return append(lines, command.Usage, "", command.Description, "", "Escape returns to command help · ↑/↓ scroll")
		}
	}
	commands := m.helpCommands()
	budget := max(1, m.height-16)
	selected := min(m.helpSelection, max(0, len(commands)-1))
	start := max(0, selected-budget+1)
	for i := start; i < min(len(commands), start+budget); i++ {
		marker := "  "
		if i == selected {
			marker = "> "
		}
		lines = append(lines, marker+commands[i].Usage)
	}
	if len(commands) > 0 {
		lines = append(lines, "", commands[selected].Description)
	}
	return append(lines, "", "↑/↓ select · Enter details · Escape closes help")
}

func (m RootModel) helpView() string {
	lines := m.helpLines()
	// Wrap each selected detail instead of losing the ends of long descriptions.
	width := m.width
	if width < 1 {
		width = 80
	}
	wrapped := strings.Split(ansi.Hardwrap(terminalText(strings.Join(lines, "\n")), width, true), "\n")
	start := 0
	if m.helpCommand != "" {
		start = min(m.helpOffset, max(0, len(wrapped)-max(1, m.height-5)))
	}
	return strings.Join(wrapped[start:], "\n")
}
