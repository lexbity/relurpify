package tui

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	tea "github.com/charmbracelet/bubbletea"
)

// ButtonClickedMsg is emitted when a Button is activated.
type ButtonClickedMsg struct {
	Label string
	Data  any
}

// Button renders a labelled, clickable pill. Both keyboard (enter) and mouse
// (click) activation are supported.
type Button struct {
	label   string
	data    any
	focused bool
	x, y    int
	width   int
	th      *theme.Theme
}

// NewButton creates a button with the given label.
func NewButton(label string) *Button {
	return &Button{label: label}
}

// SetData attaches arbitrary data carried in the click message.
func (b *Button) SetData(data any) { b.data = data }

// SetTheme sets the theme for styled rendering.
func (b *Button) SetTheme(th *theme.Theme) {
	if th != nil {
		b.th = th
	}
}

// SetWidth sets the rendered width.
func (b *Button) SetWidth(w int) { b.width = w }

// Focus sets keyboard focus.
func (b *Button) Focus()          { b.focused = true }
func (b *Button) Blur()           { b.focused = false }
func (b *Button) IsFocused() bool { return b.focused }

// Label returns the button text.
func (b *Button) Label() string { return b.label }

// Update handles keyboard activation.
func (b *Button) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !b.focused {
		return nil, false
	}
	if msg.String() == "enter" || msg.String() == " " {
		return b.clickCmd(), true
	}
	return nil, false
}

// HandleClick checks if the given mouse position hits the button.
func (b *Button) HandleClick(x, y int) (tea.Cmd, bool) {
	if y == b.y && x >= b.x && x <= b.x+b.width {
		return b.clickCmd(), true
	}
	return nil, false
}

// SetRenderPos records where the button was last rendered.
func (b *Button) SetRenderPos(x, y int) {
	b.x = x
	b.y = y
}

// RenderWidth returns the rendered width of the button.
func (b *Button) RenderWidth() int {
	if b.th == nil {
		b.th = theme.Default()
	}
	return lipglossWidth(b.th.Button(b.focused).Render(b.label))
}

func (b *Button) clickCmd() tea.Cmd {
	return func() tea.Msg {
		return ButtonClickedMsg{Label: b.label, Data: b.data}
	}
}

// View renders the button.
func (b *Button) View() string {
	if b.th == nil {
		b.th = theme.Default()
	}
	return b.th.Button(b.focused).Render(fmt.Sprintf("(%s)", b.label))
}

// lipglossWidth returns the rendered width of a string using lipgloss.
func lipglossWidth(s string) int {
	w := 0
	for _, r := range s {
		// Rough CJK width detection.
		if r >= 0x4E00 && r <= 0x9FFF {
			w += 2
		} else {
			w++
		}
	}
	return w
}
