package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Messages emitted by InputBar.
type InputSubmittedMsg struct{ Value string }
type CommandInvokedMsg struct {
	Name string
	Args []string
}
type GlobalKeyMsg struct{ Key string }

const commandPaletteRows = 6

// commandItem is a palette entry for an autocomplete dropdown.
type commandItem struct {
	Name        string
	Usage       string
	Description string
	Score       int
}

// InputBar wraps textinput with prefix-by-tab, ↑↓ history, and inline /command palette.
type InputBar struct {
	input   textinput.Model
	history InputHistory
	palette []commandItem
	palSel  int
	palOpen bool
	width   int
	// searchMode suppresses global keys and shows a search placeholder.
	searchMode bool
}

// NewInputBar creates a focused InputBar.
func NewInputBar() *InputBar {
	ti := textinput.New()
	ti.Placeholder = "Type a message or /help for commands"
	ti.Focus()
	return &InputBar{input: ti}
}

// SetWidth sets the input width.
func (b *InputBar) SetWidth(w int) {
	b.width = w
	b.input.Width = max(10, w-4)
}

// Value returns the current text value.
func (b *InputBar) Value() string { return b.input.Value() }

// SetValue sets the input text.
func (b *InputBar) SetValue(v string) { b.input.SetValue(v) }

// SetSearchMode enters or exits search mode.
func (b *InputBar) SetSearchMode(on bool) {
	b.searchMode = on
	if on {
		b.input.Placeholder = "search messages..."
		b.input.SetValue("")
		b.input.Focus()
	} else {
		b.input.Placeholder = "Type a message or /help for commands"
	}
}

// SetFilePickerMode enters or exits file picker mode.
func (b *InputBar) SetFilePickerMode(on bool) {
	if on {
		b.input.Placeholder = "@ - select files or type path"
		b.input.SetValue("@")
		b.input.Focus()
	} else {
		b.input.Placeholder = "Type a message or /help for commands"
		b.input.SetValue("")
	}
}

// prefix returns the prompt prefix for the current context.
func (b *InputBar) prefix(tab TabID) string {
	if b.searchMode {
		return "🔍 "
	}
	switch tab {
	case TabSession:
		return "@ "
	case TabTasks:
		return "+ "
	case TabSettings:
		return "? "
	default:
		return "> "
	}
}

// Update processes key input and emits typed messages.
func (b *InputBar) Update(msg tea.Msg, activeTab TabID) (*InputBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Intercept global navigation keys only when input is empty.
		if b.input.Value() == "" && !b.palOpen && !b.searchMode {
			switch msg.String() {
			case "1", "2", "3", "4", "?", "ctrl+t", "ctrl+f":
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			case "tab", "shift+tab":
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			}
		}

		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }

		case "enter":
			raw := strings.TrimSpace(b.input.Value())
			b.input.SetValue("")
			b.palOpen = false
			b.palette = nil
			b.palSel = 0
			if raw == "" {
				return b, nil
			}
			if strings.HasPrefix(raw, "/") {
				name, args := parseSlashCommand(raw)
				if name != "" {
					b.history.Push(raw)
					return b, func() tea.Msg { return CommandInvokedMsg{Name: name, Args: args} }
				}
			}
			b.history.Push(raw)
			return b, func() tea.Msg { return InputSubmittedMsg{Value: raw} }

		case "esc":
			if b.palOpen {
				b.palOpen = false
				b.palette = nil
				b.input.SetValue("")
				return b, nil
			}
			if b.searchMode {
				b.SetSearchMode(false)
				return b, func() tea.Msg { return GlobalKeyMsg{Key: "esc"} }
			}

		case "up":
			if b.palOpen && b.palSel > 0 {
				b.palSel--
				return b, nil
			}
			prev := b.history.Prev()
			if prev != "" {
				b.input.SetValue(prev)
				b.input.CursorEnd()
			}
			return b, nil

		case "down":
			if b.palOpen && b.palSel < len(b.palette)-1 {
				b.palSel++
				return b, nil
			}
			next := b.history.Next()
			b.input.SetValue(next)
			b.input.CursorEnd()
			return b, nil

		case "tab":
			if b.palOpen && len(b.palette) > 0 {
				b.autocomplete()
				b.updatePalette()
				return b, nil
			}
		}

		// Pass through to textinput.
		var cmd tea.Cmd
		b.input, cmd = b.input.Update(msg)

		// Check if we should open the command palette.
		val := b.input.Value()
		if strings.HasPrefix(val, "/") {
			b.palOpen = true
			b.updatePalette()
		} else {
			b.palOpen = false
			b.palette = nil
		}
		return b, cmd
	}

	// Pass through blink tick etc.
	var cmd tea.Cmd
	b.input, cmd = b.input.Update(msg)
	return b, cmd
}

func (b *InputBar) autocomplete() {
	if len(b.palette) == 0 {
		return
	}
	if b.palSel < 0 || b.palSel >= len(b.palette) {
		b.palSel = 0
	}
	item := b.palette[b.palSel]
	v := "/" + item.Name
	if strings.Contains(item.Usage, " ") {
		v += " "
	}
	b.input.SetValue(v)
	b.input.CursorEnd()
}

func (b *InputBar) updatePalette() {
	query := strings.TrimPrefix(b.input.Value(), "/")
	if f := strings.Fields(query); len(f) > 0 {
		query = f[0]
	}
	var items []commandItem
	for _, cmd := range listCommandsSorted() {
		score := 0
		if query != "" {
			ok, s := fuzzyMatchScore(query, cmd.Name)
			if ok {
				score = s
			} else if ok2, s2 := fuzzyMatchScore(query, cmd.Usage); ok2 {
				score = s2 - 2
			} else {
				continue
			}
		}
		items = append(items, commandItem{Name: cmd.Name, Usage: cmd.Usage, Description: cmd.Description, Score: score})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Name < items[j].Name
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > commandPaletteRows {
		items = items[:commandPaletteRows]
	}
	b.palette = items
	if b.palSel >= len(items) {
		b.palSel = 0
	}
}

// View renders the input bar (prefix + textinput + hint).
func (b *InputBar) View(activeTab TabID, streaming bool) string {
	prefix := inputPrefixStyle.Render(b.prefix(activeTab))

	var hint string
	if streaming {
		hint = dimStyle.Render(" streaming…  pgup/down scroll | ctrl+c quit")
	} else if b.palOpen && len(b.palette) > 0 {
		hint = dimStyle.Render(" enter run | esc cancel | tab complete | ↑↓ select")
	} else if b.searchMode {
		hint = dimStyle.Render(" esc exit search")
	} else {
		hint = dimStyle.Render(" / commands | @ context | ctrl+f search | ? help")
	}

	content := prefix + b.input.View()
	if hint != "" {
		content += " " + hint
	}
	bar := inputBarNewStyle.Width(b.width).Render(content)

	if b.palOpen && len(b.palette) > 0 {
		return b.renderPalette() + "\n" + bar
	}
	return bar
}

func (b *InputBar) renderPalette() string {
	lines := []string{panelHeaderStyle.Render("Commands")}
	for i, item := range b.palette {
		label := fmt.Sprintf("%s  %s", item.Usage, dimStyle.Render(item.Description))
		if i == b.palSel {
			label = panelItemActiveStyle.Render(label)
		} else {
			label = panelItemStyle.Render(label)
		}
		lines = append(lines, label)
	}
	return panelStyle.Width(b.width).Render(strings.Join(lines, "\n"))
}

func parseSlashCommand(input string) (string, []string) {
	parts := strings.Fields(input)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return "", nil
	}
	name := strings.TrimPrefix(parts[0], "/")
	return name, parts[1:]
}
