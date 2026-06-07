package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

// DropdownSelectedMsg is emitted when a dropdown item is selected by mouse
// or keyboard.
type DropdownSelectedMsg struct {
	ID    string
	Label string
}

// Dropdown renders a labelled closed control that opens to show a list of
// items. Both keyboard (↑↓ enter esc) and mouse (click to open, click item)
// are supported.
type Dropdown struct {
	label   string
	items   []DropdownItem
	open    bool
	sel     int
	focused bool
	x, y    int // last known render position for mouse hit-testing
	width   int
	th      *theme.Theme
}

// DropdownItem is a single option in a dropdown list.
type DropdownItem struct {
	ID    string
	Label string
}

// NewDropdown creates a closed dropdown with the given label.
func NewDropdown(label string, items []DropdownItem) *Dropdown {
	return &Dropdown{
		label: label,
		items: items,
	}
}

// SetTheme sets the theme for styled rendering.
func (d *Dropdown) SetTheme(th *theme.Theme) {
	if th != nil {
		d.th = th
	}
}

// SetWidth sets the rendered width.
func (d *Dropdown) SetWidth(w int) {
	d.width = w
}

// Focus sets keyboard focus.
func (d *Dropdown) Focus()       { d.focused = true }
func (d *Dropdown) Blur()        { d.focused = false }
func (d *Dropdown) IsOpen() bool { return d.open }

// Selected returns the currently selected item (or zero value).
func (d *Dropdown) Selected() DropdownItem {
	if d.sel < 0 || d.sel >= len(d.items) {
		return DropdownItem{}
	}
	return d.items[d.sel]
}

// SelectedID returns the ID of the selected item.
func (d *Dropdown) SelectedID() string {
	if d.sel < 0 || d.sel >= len(d.items) {
		return ""
	}
	return d.items[d.sel].ID
}

// Open opens the dropdown list.
func (d *Dropdown) Open()  { d.open = true }
func (d *Dropdown) Close() { d.open = false; d.sel = 0 }

// Update handles keyboard input. Returns a command when enter is pressed on
// an item.
func (d *Dropdown) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !d.focused {
		return nil, false
	}
	if !d.open {
		if msg.String() == "enter" || msg.String() == " " {
			d.open = true
			return nil, true
		}
		return nil, false
	}
	switch msg.String() {
	case "up", "k":
		if d.sel > 0 {
			d.sel--
		}
		return nil, true
	case "down", "j":
		if d.sel < len(d.items)-1 {
			d.sel++
		}
		return nil, true
	case "enter", " ":
		d.open = false
		if d.sel >= 0 && d.sel < len(d.items) {
			return func() tea.Msg {
				return DropdownSelectedMsg{ID: d.items[d.sel].ID, Label: d.items[d.sel].Label}
			}, true
		}
		return nil, true
	case "esc":
		d.open = false
		return nil, true
	}
	return nil, false
}

// HandleClick checks if the given mouse position hits the dropdown and
// returns true if consumed.
func (d *Dropdown) HandleClick(x, y int) (tea.Cmd, bool) {
	if d.open {
		// Check item list hit area.
		listTop := d.y + 1
		idx := y - listTop
		if idx >= 0 && idx < len(d.items) && x >= d.x && x <= d.x+d.width {
			d.sel = idx
			d.open = false
			return func() tea.Msg {
				return DropdownSelectedMsg{ID: d.items[idx].ID, Label: d.items[idx].Label}
			}, true
		}
		d.open = false
		return nil, true
	}
	// Check closed control hit area.
	if y == d.y && x >= d.x && x <= d.x+d.width {
		d.open = true
		return nil, true
	}
	return nil, false
}

// SetRenderPos records where the dropdown was last rendered, for mouse
// hit-testing.
func (d *Dropdown) SetRenderPos(x, y int) {
	d.x = x
	d.y = y
}

// View renders the dropdown.
func (d *Dropdown) View() string {
	if d.th == nil {
		d.th = theme.Default()
	}
	prefix := fmt.Sprintf("%s ", d.label)
	if d.open {
		itemLines := make([]string, 0, len(d.items)+1)
		for i, item := range d.items {
			marker := " "
			if i == d.sel {
				marker = "▸"
			}
			line := fmt.Sprintf(" %s %s", marker, item.Label)
			if i == d.sel && d.focused {
				line = d.th.Active().Render(line)
			}
			itemLines = append(itemLines, line)
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			d.th.Button(d.focused).Render(prefix+"▴"),
			strings.Join(itemLines, "\n"),
		)
	}
	return d.th.Button(d.focused).Render(prefix + "▾")
}
