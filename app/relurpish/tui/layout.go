package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	chromeAgentWidth   = 20
	chromeBottomRows   = 2
	chromeSubtabRows   = 1
	chromeMinCellWidth = 1
)

// ChromeLayout tracks terminal dimensions and derives component heights for
// the host chrome structure.
type ChromeLayout struct {
	Width             int
	Height            int
	HitlHeight        int
	AgentWidth        int
	Region1Height     int
	Region1PaneHeight int
	InputWidth        int
}

// Recalculate updates all dimensions on WindowSizeMsg.
func (c *ChromeLayout) Recalculate(width, height int, hitlVisible bool) {
	if c == nil {
		return
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	c.Width = width
	c.Height = height
	c.HitlHeight = 0
	if hitlVisible {
		c.HitlHeight = 1
	}
	c.AgentWidth = chromeAgentWidth
	if width > 0 && c.AgentWidth >= width {
		c.AgentWidth = width - chromeMinCellWidth
		if c.AgentWidth < chromeMinCellWidth {
			c.AgentWidth = chromeMinCellWidth
		}
	}
	c.Region1Height = height - chromeBottomRows - c.HitlHeight
	if c.Region1Height < chromeSubtabRows+chromeMinCellWidth {
		c.Region1Height = chromeSubtabRows + chromeMinCellWidth
	}
	c.Region1PaneHeight = c.Region1Height - chromeSubtabRows
	if c.Region1PaneHeight < chromeMinCellWidth {
		c.Region1PaneHeight = chromeMinCellWidth
	}
	c.InputWidth = width - c.AgentWidth
	if c.InputWidth < chromeMinCellWidth {
		c.InputWidth = chromeMinCellWidth
	}
}

// Region3Width returns the width available to the universal input bar.
func (c ChromeLayout) Region3Width() int {
	if c.InputWidth > 0 {
		return c.InputWidth
	}
	if c.Width > 0 {
		return c.Width
	}
	return chromeMinCellWidth
}

// Region2Width returns the width available to the active-agent chrome cell.
func (c ChromeLayout) Region2Width() int {
	if c.AgentWidth > 0 {
		return c.AgentWidth
	}
	if c.Width > 0 {
		if c.Width < chromeAgentWidth {
			return c.Width
		}
		return chromeAgentWidth
	}
	return chromeMinCellWidth
}

// Region1PaneRows returns the rows available to the active pane within the
// first region after the subtab strip is accounted for.
func (c ChromeLayout) Region1PaneRows() int {
	if c.Region1PaneHeight > 0 {
		return c.Region1PaneHeight
	}
	return chromeMinCellWidth
}

// Region1Rows returns the total rows available to the first region.
func (c ChromeLayout) Region1Rows() int {
	if c.Region1Height > 0 {
		return c.Region1Height
	}
	return chromeSubtabRows + chromeMinCellWidth
}

func clipText(value string, width int) string {
	if width < 1 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

// notificationRowVisible reports whether the host should reserve a row for
// notification or HITL presentation.
func (m RootModel) notificationRowVisible() bool {
	if m.notifBar != nil && m.notifBar.Active() {
		return true
	}
	return m.hitlRow != nil && m.hitlRow.Active()
}

// handleResize distributes new terminal dimensions to all host-owned
// components.
func (m RootModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true

	m.layout.Recalculate(msg.Width, msg.Height, m.notificationRowVisible())

	if m.notifBar != nil {
		m.notifBar.SetWidth(msg.Width)
	}
	if m.inputBar != nil {
		m.inputBar.SetWidth(msg.Width)
	}
	paneH := m.layout.Region1PaneRows()
	if m.doctor != nil {
		m.doctor.SetSize(msg.Width, max(1, msg.Height-5))
	}
	if m.workshop != nil {
		m.workshop.SetSize(msg.Width, max(1, msg.Height-5))
	}
	if m.keybindings != nil {
		m.keybindings.SetSize(msg.Width, max(1, msg.Height-5))
	}
	if m.guard != nil {
		m.guard.SetSize(msg.Width, max(1, msg.Height-5))
	}
	if m.provider != nil {
		m.provider.resize(msg.Width, max(1, msg.Height-5))
	}
	if m.chat != nil {
		m.chat.SetSize(msg.Width, paneH)
	}
	m.session.SetSize(msg.Width, paneH)
	if m.baseSurface != nil {
		m.baseSurface.SetSize(msg.Width, paneH)
	}
	if m.hitlRow != nil {
		m.hitlRow.SetWidth(msg.Width)
	}

	return m, nil
}

