package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// renderAgentCell renders the active-agent chrome cell for Region 2.
func renderAgentCell(agent string, width int) string {
	if width < chromeMinCellWidth {
		width = chromeMinCellWidth
	}
	label := strings.TrimSpace(agent)
	if label == "" {
		label = "none"
	}
	label = clipText(label, width)
	return agentStripActiveStyle.Width(width).Render(label)
}

func clipText(value string, width int) string {
	if width < chromeMinCellWidth {
		return ""
	}
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

// SubTabBar renders the subtab row for the currently active main tab.
type SubTabBar struct {
	active  SubTabID
	subtabs []SubTabDefinition
	width   int
}

// NewSubTabBar creates a SubTabBar from a tab definition.
func NewSubTabBar(def TabDefinition) SubTabBar {
	active := ""
	if len(def.SubTabs) > 0 {
		active = def.SubTabs[0].ID
	}
	return SubTabBar{active: active, subtabs: def.SubTabs}
}

// SetActive updates the active subtab.
func (s *SubTabBar) SetActive(id SubTabID) { s.active = id }

// SetWidth propagates terminal width.
func (s *SubTabBar) SetWidth(w int) { s.width = w }

// SetSubTabs updates the rendered subtab list (called on tab switch).
func (s *SubTabBar) SetSubTabs(def TabDefinition) {
	s.subtabs = def.SubTabs
	// Preserve active if still present; otherwise reset to first.
	for _, st := range def.SubTabs {
		if st.ID == s.active {
			return
		}
	}
	if len(def.SubTabs) > 0 {
		s.active = def.SubTabs[0].ID
	} else {
		s.active = ""
	}
}

// View renders the subtab bar. Returns empty string when there are no subtabs.
func (s SubTabBar) View() string {
	if len(s.subtabs) == 0 {
		return subtabBarEmptyStyle.Width(s.width).Render("")
	}
	parts := make([]string, 0, len(s.subtabs))
	available := s.width - (len(s.subtabs)-1)*2
	if available < len(s.subtabs) {
		available = len(s.subtabs)
	}
	cellWidth := available / len(s.subtabs)
	if cellWidth < chromeMinCellWidth {
		cellWidth = chromeMinCellWidth
	}
	for i, st := range s.subtabs {
		label := fmt.Sprintf("[%d] %s", i+1, st.Label)
		label = clipText(label, cellWidth)
		if st.ID == s.active {
			parts = append(parts, subtabActiveStyle.Width(cellWidth).Render(label))
		} else {
			parts = append(parts, subtabInactiveStyle.Width(cellWidth).Render(label))
		}
	}
	content := strings.Join(parts, "  ")
	return subtabBarStyle.Width(s.width).Render(content)
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

	m.subTabBar.SetWidth(msg.Width)
	m.tabBar.SetWidth(msg.Width)
	if m.notifBar != nil {
		m.notifBar.SetWidth(msg.Width)
	}
	if m.inputBar != nil {
		m.inputBar.SetWidth(m.layout.Region3Width())
	}
	if m.activeInput != nil {
		m.activeInput.SetSize(m.layout.Region3Width(), 1)
	}
	m.help.SetSize(msg.Width, msg.Height)

	paneH := m.layout.Region1PaneRows()
	if m.chat != nil {
		m.chat.SetSize(msg.Width, paneH)
	}
	m.session.SetSize(msg.Width, paneH)
	if m.baseSurface != nil {
		m.baseSurface.SetSize(msg.Width, paneH)
	}
	if m.activeNav != nil {
		m.activeNav.SetSize(msg.Width, 1)
	}
	if m.hitlRow != nil {
		m.hitlRow.SetWidth(msg.Width)
	}

	return m, nil
}
