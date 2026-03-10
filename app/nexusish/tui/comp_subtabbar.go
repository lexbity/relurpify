package tui

import "strings"

type SubTab struct {
	Label  string
	Badge  string
	Urgent bool
}

type SubTabBar struct {
	tabs   []SubTab
	active int
	width  int
}

func (b *SubTabBar) SetTabs(tabs []SubTab) {
	b.tabs = tabs
}

func (b *SubTabBar) SetWidth(width int) {
	b.width = width
}

func (b *SubTabBar) SetActive(active int) {
	if active >= 0 && active < len(b.tabs) {
		b.active = active
	}
}

func (b SubTabBar) Active() int {
	return b.active
}

func (b SubTabBar) View() string {
	parts := make([]string, 0, len(b.tabs))
	for i, tab := range b.tabs {
		label := tab.Label
		if tab.Badge != "" {
			label += " " + badgeStyle.Render(tab.Badge)
		}
		style := subTabInactiveStyle
		if i == b.active {
			style = subTabActiveStyle
		}
		if tab.Urgent {
			style = style.Copy().BorderForeground(warningColor)
		}
		parts = append(parts, style.Render(label))
	}
	return subTabBarStyle.Width(b.width).Render(strings.Join(parts, " "))
}
