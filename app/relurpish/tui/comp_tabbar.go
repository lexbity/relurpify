package tui

import (
	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

import (
	"fmt"
	"strings"
)

// TabBar renders the bottom tab strip from the registered tab set.
type TabBar struct {
	active   TabID
	registry *TabRegistry
	width    int
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewTabBar creates a TabBar with the given active tab.
func NewTabBar(active TabID) TabBar {
	return TabBar{active: active, th: theme.Default()}
}

// SetActive updates the active tab.
func (tb *TabBar) SetActive(id TabID) { tb.active = id }

// SetRegistry wires the tab bar to a registry for rendering.
func (tb *TabBar) SetRegistry(r *TabRegistry) { tb.registry = r }

// SetWidth propagates terminal width.
func (tb *TabBar) SetWidth(w int) { tb.width = w }

// View renders the tab bar.
func (tb TabBar) View() string {
	if tb.registry == nil {
		return tb.th.Bar().Width(tb.width).Render("")
	}
	tabs := tb.registry.All()
	if len(tabs) == 0 {
		return tb.th.Bar().Width(tb.width).Render("")
	}
	available := tb.width - (len(tabs)-1)*2
	if available < len(tabs) {
		available = len(tabs)
	}
	cellWidth := available / len(tabs)
	if cellWidth < 1 {
		cellWidth = 1
	}
	var parts []string
	for i, t := range tabs {
		label := fmt.Sprintf("[%d] %s", i+1, t.Label)
		label = clipText(label, cellWidth)
		if t.ID == tb.active {
			parts = append(parts, tb.th.Active().Width(cellWidth).Render(label))
		} else {
			parts = append(parts, tb.th.Dim().Width(cellWidth).Render(label))
		}
	}
	content := strings.Join(parts, "  ")
	return tb.th.Bar().Width(tb.width).Render(content)
}

// SetTheme sets the active semantic style source.
func (tb *TabBar) SetTheme(th *theme.Theme) {
	if th != nil {
		tb.th = th
	}
}
