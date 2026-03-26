package tui

import (
	"fmt"
	"strings"
)

// TabBar renders the bottom tab strip from the registered tab set.
type TabBar struct {
	active   TabID
	registry *TabRegistry
	width    int
}

// NewTabBar creates a TabBar with the given active tab.
func NewTabBar(active TabID) TabBar {
	return TabBar{active: active}
}

// SetActive updates the active tab.
func (tb *TabBar) SetActive(id TabID) { tb.active = id }

// SetRegistry wires the tab bar to a registry for rendering.
func (tb *TabBar) SetRegistry(r *TabRegistry) { tb.registry = r }

// SetWidth propagates terminal width.
func (tb *TabBar) SetWidth(w int) { tb.width = w }

// View renders the tab bar.
func (tb TabBar) View() string {
	var parts []string
	for i, t := range tb.registry.All() {
		label := fmt.Sprintf("[%d %s]", i+1, t.Label)
		if t.ID == tb.active {
			parts = append(parts, tabActiveStyle.Render(label))
		} else {
			parts = append(parts, tabInactiveStyle.Render(label))
		}
	}
	content := strings.Join(parts, "  ")
	return tabBarStyle.Width(tb.width).Render(content)
}
