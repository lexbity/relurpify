package tui

import (
	"fmt"
	"strings"
)

// TabBar renders the bottom tab strip.
type TabBar struct {
	active TabID
	width  int
}

// NewTabBar creates a TabBar with the given active tab.
func NewTabBar(active TabID) TabBar {
	return TabBar{active: active}
}

// SetActive updates the active tab.
func (tb *TabBar) SetActive(id TabID) { tb.active = id }

// SetWidth propagates terminal width.
func (tb *TabBar) SetWidth(w int) { tb.width = w }

// View renders the tab bar.
func (tb TabBar) View() string {
	tabs := []struct {
		id    TabID
		label string
	}{
		{TabChat, "1 Chat"},
		{TabTasks, "2 Tasks"},
		{TabSession, "3 Session"},
		{TabSettings, "4 Settings"},
		{TabTools, "5 Tools"},
	}
	parts := make([]string, 0, len(tabs))
	for _, t := range tabs {
		label := fmt.Sprintf("[%s]", t.label)
		if t.id == tb.active {
			parts = append(parts, tabActiveStyle.Render(label))
		} else {
			parts = append(parts, tabInactiveStyle.Render(label))
		}
	}
	content := strings.Join(parts, "  ")
	return tabBarStyle.Width(tb.width).Render(content)
}
