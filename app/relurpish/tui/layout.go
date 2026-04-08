package tui

import (
	"fmt"
	"strings"
)

// ChromeLayout tracks terminal dimensions and derives component heights for the
// mode-driven chrome structure:
//
//	title bar      1 row  agent name + workspace path
//	subtab bar     1 row  subtabs of the active main tab
//	main pane      N rows content (variable)
//	input bar      1 row  universal input / command palette
//	main tab bar   1 row  chat · planner · debug · config · session
//	status bar     1 row  context-sensitive keybindings
type ChromeLayout struct {
	Width        int
	Height       int
	TitleHeight  int // always 1
	SubtabHeight int // always 1
	InputHeight  int // 1 normally, 2 during multiline
	TabBarHeight int // always 1
	StatusHeight int // always 1
}

// MainPaneHeight returns available rows for the main pane (minus optional HITL
// overlay rows). Guarantees at least 1.
func (c ChromeLayout) MainPaneHeight(hitlRows int) int {
	reserved := c.TitleHeight + c.SubtabHeight + c.InputHeight + c.TabBarHeight + c.StatusHeight + hitlRows
	h := c.Height - reserved
	if h < 1 {
		return 1
	}
	return h
}

// Recalculate updates all dimensions on WindowSizeMsg. Fixed-height rows are
// always 1; InputHeight defaults to 1 (caller may set to 2 for multiline).
func (c *ChromeLayout) Recalculate(width, height int) {
	c.Width = width
	c.Height = height
	c.TitleHeight = 1
	c.SubtabHeight = 1
	if c.InputHeight == 0 {
		c.InputHeight = 1
	}
	c.TabBarHeight = 1
	c.StatusHeight = 1
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
	for i, st := range s.subtabs {
		label := fmt.Sprintf("%d:%s", i+1, st.Label)
		if st.ID == s.active {
			parts = append(parts, subtabActiveStyle.Render(label))
		} else {
			parts = append(parts, subtabInactiveStyle.Render(label))
		}
	}
	content := strings.Join(parts, "  ")
	return subtabBarStyle.Width(s.width).Render(content)
}
