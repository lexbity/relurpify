package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const commandPaletteMaxRows = 6

type commandItem struct {
	Name        string
	Usage       string
	Description string
	Score       int
}

type commandPaletteState struct {
	items    []commandItem
	selected int
}

func (m Model) updateCommandPalette(query string) Model {
	query = strings.TrimSpace(strings.TrimPrefix(query, "/"))
	if fields := strings.Fields(query); len(fields) > 0 {
		query = fields[0]
	}
	items := make([]commandItem, 0, len(commandRegistry))
	for _, cmd := range listCommandsSorted() {
		name := cmd.Name
		usage := cmd.Usage
		desc := cmd.Description
		score := 0
		if query != "" {
			if ok, s := fuzzyMatchScore(query, name); ok {
				score = s
			} else if ok, s := fuzzyMatchScore(query, usage); ok {
				score = s - 2
			} else {
				continue
			}
		}
		items = append(items, commandItem{Name: name, Usage: usage, Description: desc, Score: score})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Name < items[j].Name
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > commandPaletteMaxRows {
		items = items[:commandPaletteMaxRows]
	}
	m.commandPalette.items = items
	if m.commandPalette.selected >= len(items) {
		m.commandPalette.selected = 0
	}
	return m
}

func listCommandsSorted() []Command {
	commands := make([]Command, 0, len(commandRegistry))
	for _, cmd := range commandRegistry {
		commands = append(commands, cmd)
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	return commands
}

func (m Model) commandPaletteSelection() (commandItem, bool) {
	if len(m.commandPalette.items) == 0 {
		return commandItem{}, false
	}
	if m.commandPalette.selected < 0 || m.commandPalette.selected >= len(m.commandPalette.items) {
		return commandItem{}, false
	}
	return m.commandPalette.items[m.commandPalette.selected], true
}

func (m Model) applyCommandAutocomplete() Model {
	item, ok := m.commandPaletteSelection()
	if !ok {
		return m
	}
	value := "/" + item.Name
	if strings.Contains(item.Usage, " ") {
		value += " "
	}
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m
}

func renderCommandItem(item commandItem) string {
	usage := item.Usage
	if usage == "" {
		usage = "/" + item.Name
	}
	if item.Description == "" {
		return usage
	}
	return usage + " - " + item.Description
}

type paletteNoopMsg struct{}

func paletteNoopCmd() tea.Cmd {
	return func() tea.Msg {
		return paletteNoopMsg{}
	}
}
