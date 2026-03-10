package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type InputMode int

const (
	InputModeNormal InputMode = iota
	InputModeFilter
	InputModeCommand
	InputModeHelp
)

type InputBar struct {
	width int
	mode  InputMode
	value string
}

func NewInputBar() *InputBar {
	return &InputBar{}
}

func (b *InputBar) SetWidth(width int) {
	b.width = width
}

func (b *InputBar) Update(msg tea.KeyMsg) (bool, tea.Cmd) {
	if b == nil {
		return false, nil
	}
	switch b.mode {
	case InputModeNormal:
		switch msg.String() {
		case "/":
			b.mode = InputModeFilter
			b.value = ""
			return true, nil
		case ":":
			b.mode = InputModeCommand
			b.value = ""
			return true, nil
		case "?":
			return true, func() tea.Msg { return inputSubmittedMsg{mode: InputModeHelp, value: ""} }
		default:
			return false, nil
		}
	default:
		switch msg.String() {
		case "esc":
			b.mode = InputModeNormal
			b.value = ""
			return true, nil
		case "backspace":
			if len(b.value) > 0 {
				b.value = b.value[:len(b.value)-1]
			}
			if b.mode == InputModeFilter {
				value := b.value
				return true, func() tea.Msg { return inputSubmittedMsg{mode: InputModeFilter, value: value} }
			}
			return true, nil
		case "enter":
			mode := b.mode
			value := strings.TrimSpace(b.value)
			b.mode = InputModeNormal
			b.value = ""
			return true, func() tea.Msg { return inputSubmittedMsg{mode: mode, value: value} }
		default:
			if len(msg.Runes) == 1 && msg.Alt == false && msg.Type == tea.KeyRunes {
				b.value += string(msg.Runes)
				if b.mode == InputModeFilter {
					value := b.value
					return true, func() tea.Msg { return inputSubmittedMsg{mode: InputModeFilter, value: value} }
				}
				return true, nil
			}
			return true, nil
		}
	}
}

func (b InputBar) View(active tabID) string {
	prefix := ">"
	hint := "filter /  :command  ?help"
	switch b.mode {
	case InputModeFilter:
		prefix = "/"
		hint = "filter active pane"
	case InputModeCommand:
		prefix = ":"
		hint = "command palette"
	}
	content := prefix + b.value
	if strings.TrimSpace(b.value) == "" {
		content += " " + mutedStyle.Render(hint)
	}
	return inputBarStyle.Width(b.width).Render(content)
}
