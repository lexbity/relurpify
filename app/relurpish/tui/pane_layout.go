package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// splitWidths divides a total width across weighted columns.
func splitWidths(total int, weights ...int) []int {
	if len(weights) == 0 {
		return nil
	}
	if total < len(weights) {
		total = len(weights)
	}
	sum := 0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum == 0 {
		sum = len(weights)
	}
	widths := make([]int, len(weights))
	remaining := total
	for i, w := range weights {
		width := total / len(weights)
		if w > 0 {
			width = total * w / sum
		}
		if i == len(weights)-1 {
			width = remaining
		}
		if width < 1 {
			width = 1
		}
		if width > remaining {
			width = remaining
		}
		widths[i] = width
		remaining -= width
	}
	return widths
}

// sectionPanel renders a simple framed panel used by the config and session panes.
func sectionPanel(title string, width int, lines ...string) string {
	if width < 1 {
		width = 1
	}
	content := []string{panelHeaderStyle.Render(title)}
	content = append(content, lines...)
	return panelStyle.Width(max(24, width)).Render(strings.Join(content, "\n"))
}

// sectionList renders a selectable list with a clipped visible window.
func sectionList(lines []string, selected int, maxVisible int) string {
	if len(lines) == 0 {
		return dimStyle.Render("(empty)")
	}
	if maxVisible < 1 || maxVisible > len(lines) {
		maxVisible = len(lines)
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(lines) {
		selected = len(lines) - 1
	}
	start := 0
	if selected >= maxVisible {
		start = selected - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]
	rendered := make([]string, 0, len(visible))
	for i, line := range visible {
		if start+i == selected {
			rendered = append(rendered, panelItemActiveStyle.Render(line))
		} else {
			rendered = append(rendered, panelItemStyle.Render(line))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rendered...)
}
