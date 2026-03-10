package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type DashboardPane struct {
	state  RuntimeState
	width  int
	height int
}

func NewDashboardPane() *DashboardPane {
	return &DashboardPane{}
}

func (p *DashboardPane) SetData(state RuntimeState) { p.state = state }

func (p *DashboardPane) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DashboardPane) View() string {
	panelWidth := paneWidthFor(p.width)
	left := panelStyle.Width(panelWidth).Render(
		sectionTitle("Gateway") + "\n" +
			kv("Bind", p.state.BindAddr) + "\n" +
			kv("PID", fmt.Sprintf("%d", p.state.PID)) + "\n" +
			kv("Seq", fmt.Sprintf("%d", p.state.LastSeq)) + "\n" +
			kv("Tenant", emptyFallback(p.state.TenantID, "default")),
	)
	right := panelStyle.Width(panelWidth).Render(
		sectionTitle("Surface") + "\n" +
			kv("Paired Nodes", fmt.Sprintf("%d", len(p.state.PairedNodes))) + "\n" +
			kv("Pending Pairings", fmt.Sprintf("%d", len(p.state.PendingPairings))) + "\n" +
			kv("Channels", fmt.Sprintf("%d", len(p.state.Channels))) + "\n" +
			kv("Sessions", fmt.Sprintf("%d", len(p.state.ActiveSessions))),
	)
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right), "", panelStyle.Width(max(p.width-2, 20)).Render(sectionTitle("Top Events")+"\n"+strings.Join(p.topEventLines(8), "\n")))
}

func (p *DashboardPane) topEventLines(limit int) []string {
	if len(p.state.EventCounts) == 0 {
		return []string{mutedStyle.Render("No events recorded.")}
	}
	type entry struct {
		name  string
		count uint64
	}
	entries := make([]entry, 0, len(p.state.EventCounts))
	for name, count := range p.state.EventCounts {
		entries = append(entries, entry{name: name, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].name < entries[j].name
		}
		return entries[i].count > entries[j].count
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	lines := make([]string, 0, len(entries))
	for _, item := range entries {
		lines = append(lines, fmt.Sprintf("%-32s %d", item.name, item.count))
	}
	return lines
}
