package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// circuitDiagram renders a source-ordered row when every edge is representable
// in that row. Other topologies use the dependency list; no inferred edge is
// drawn to make a graph look connected. Activity never changes coordinates.
func circuitDiagram(steps []CircuitStep, selected string, width int, draft bool) (string, bool) {
	if len(steps) == 0 || len(steps) > 5 {
		return "", false
	}
	const gap = 5
	boxWidth := min(22, (width-gap*(len(steps)-1))/len(steps))
	if boxWidth < 14 {
		return "", false
	}
	previous := ""
	for _, step := range steps {
		if len(step.Dependencies) > 1 {
			return "", false
		}
		if len(step.Dependencies) == 1 && (previous == "" || step.Dependencies[0] != previous) {
			return "", false
		}
		previous = step.ID
	}
	rows := make([]string, 4)
	for i, step := range steps {
		if i > 0 {
			for row := range rows {
				link := strings.Repeat(" ", gap)
				if row == 1 && len(step.Dependencies) == 1 {
					link = " --> "
				}
				rows[row] += link
			}
		}
		label := "  " + terminalText(step.ID)
		if step.ID == selected {
			label = "> " + terminalText(step.ID)
		}
		marker, status := circuitStatus(step.Status)
		state := marker + " " + status
		if draft {
			state = terminalText(step.Strategy)
		}
		border := "+" + strings.Repeat("-", boxWidth-2) + "+"
		rows[0] += border
		rows[1] += "|" + circuitCell(label, boxWidth-2) + "|"
		rows[2] += "|" + circuitCell(state, boxWidth-2) + "|"
		rows[3] += border
	}
	padding := strings.Repeat(" ", max(0, (width-ansi.StringWidth(rows[0]))/2))
	for i := range rows {
		rows[i] = padding + rows[i]
	}
	return strings.Join(rows, "\n"), true
}

func circuitCell(value string, width int) string {
	value = ansi.Truncate(value, width, "…")
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}
