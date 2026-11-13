package tui

import (
	"sort"
	"strings"
)

// helpLines uses the same catalog/context as completion and command dispatch.
// Help replaces the canvas, so its bounded viewport cannot hide the composer.
func (m RootModel) helpLines() []string {
	lines := []string{"Commands · Escape closes help", "F6 / Tab: focus canvas · Ctrl+G: open attention", "Alt+Enter: newline · PageUp/PageDown: scroll · Ctrl+Q: quit", ""}
	if m.cmdReg == nil {
		return append(lines, "Command catalog unavailable")
	}
	if m.helpCommand != "" {
		if command, ok := m.cmdReg.Lookup(m.helpCommand); ok {
			return append(lines, command.Usage, "", command.Description)
		}
	}
	commands := m.cmdReg.Match("", m.activeTab, m.tabs.ActiveSubTab())
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	for _, command := range commands {
		lines = append(lines, command.Usage+"  "+command.Description)
	}
	return lines
}

func (m RootModel) helpView() string {
	lines := m.helpLines()
	start := min(m.helpOffset, max(0, len(lines)-1))
	return strings.Join(lines[start:], "\n")
}
