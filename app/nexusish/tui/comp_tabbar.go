package tui

import (
	"fmt"
	"strings"
)

type TabBar struct {
	active tabID
	width  int
	tabs   []tabSpec
	meta   map[tabID]tabBarMeta
}

type tabBarMeta struct {
	badge  string
	urgent bool
}

func NewTabBar(active tabID, tabs []tabSpec) TabBar {
	return TabBar{active: active, tabs: tabs, meta: make(map[tabID]tabBarMeta, len(tabs))}
}

func (tb *TabBar) SetActive(active tabID) {
	tb.active = active
}

func (tb *TabBar) SetWidth(width int) {
	tb.width = width
}

func (tb *TabBar) SetBadge(id tabID, badge string, urgent bool) {
	if tb.meta == nil {
		tb.meta = map[tabID]tabBarMeta{}
	}
	tb.meta[id] = tabBarMeta{badge: badge, urgent: urgent}
}

func (tb TabBar) View() string {
	parts := make([]string, 0, len(tb.tabs))
	for _, tab := range tb.tabs {
		labelText := tab.label
		if meta, ok := tb.meta[tab.id]; ok && meta.badge != "" {
			labelText += " " + meta.badge
		}
		label := fmt.Sprintf("[%s]", labelText)
		if tab.id == tb.active {
			parts = append(parts, tabActiveStyle.Render(label))
		} else {
			style := tabInactiveStyle
			if meta, ok := tb.meta[tab.id]; ok && meta.urgent {
				style = style.Copy().Foreground(warningColor).Bold(true)
			}
			parts = append(parts, style.Render(label))
		}
	}
	return tabBarStyle.Width(tb.width).Render(strings.Join(parts, "  "))
}
