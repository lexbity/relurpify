package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type NodesPane struct {
	subtabs  SubTabBar
	pending  PendingPairingsView
	state    RuntimeState
	filter   string
	selected int
	width    int
	height   int
	active   bool
}

func NewNodesPane() *NodesPane {
	p := &NodesPane{pending: NewPendingPairingsView()}
	p.refreshTabs()
	return p
}

func (p *NodesPane) SetData(state RuntimeState) {
	p.state = state
	p.refreshTabs()
	if len(p.state.PendingPairings) == 0 {
		p.selected = 0
		return
	}
	if p.selected >= len(p.state.PendingPairings) {
		p.selected = len(p.state.PendingPairings) - 1
	}
}

func (p *NodesPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.subtabs.SetWidth(width)
}

func (p *NodesPane) SetActive(active bool) {
	p.active = active
}

func (p *NodesPane) SetFilter(query string) {
	p.filter = strings.TrimSpace(strings.ToLower(query))
}

func (p *NodesPane) HasSubTabs() bool { return true }

func (p *NodesPane) SubTabBar() string { return p.subtabs.View() }

func (p *NodesPane) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "[":
		p.subtabs.SetActive(max(0, p.subtabs.Active()-1))
	case "]":
		p.subtabs.SetActive(min(2, p.subtabs.Active()+1))
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
	case "down", "j":
		if p.selected < len(p.state.PendingPairings)-1 {
			p.selected++
		}
	case "a":
		if p.subtabs.Active() == 1 && len(p.state.PendingPairings) > 0 {
			return requestPairingAction("approve", p.state.PendingPairings[p.selected].Code)
		}
	case "x":
		if p.subtabs.Active() == 1 && len(p.state.PendingPairings) > 0 {
			return requestPairingAction("reject", p.state.PendingPairings[p.selected].Code)
		}
	}
	return nil
}

func (p *NodesPane) View() string {
	switch p.subtabs.Active() {
	case 1:
		return panelStyle.Width(max(p.width-2, 20)).Render(sectionTitle("Pending Pairings") + "\n" + p.pending.View(p.filteredPairings(), p.selected, p.active))
	case 2:
		return panelStyle.Width(max(p.width-2, 20)).Render(sectionTitle("Capabilities") + "\n" + mutedStyle.Render("Live node capability inventory will come from the MCP-backed runtime adapter."))
	default:
		lines := p.filteredNodeLines()
		if len(lines) == 0 {
			lines = []string{mutedStyle.Render("No paired nodes.")}
		}
		return panelStyle.Width(max(p.width-2, 20)).Render(sectionTitle("Paired Nodes") + "\n" + strings.Join(lines, "\n"))
	}
}

func (p *NodesPane) refreshTabs() {
	p.subtabs.SetTabs([]SubTab{
		{Label: "Paired", Badge: fmt.Sprintf("%d", len(p.state.PairedNodes))},
		{Label: "Pending", Badge: pairingBadge(len(p.state.PendingPairings)), Urgent: len(p.state.PendingPairings) > 0},
		{Label: "Capabilities"},
	})
}

func (p *NodesPane) filteredNodeLines() []string {
	lines := make([]string, 0, len(p.state.PairedNodes))
	for _, node := range p.state.PairedNodes {
		line := fmt.Sprintf("%-18s %-10s %-12s %s", node.Name, node.Platform, node.TrustClass, node.ID)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
	}
	return lines
}

func (p *NodesPane) filteredPairings() []PendingPairingInfo {
	if p.filter == "" {
		return p.state.PendingPairings
	}
	out := make([]PendingPairingInfo, 0, len(p.state.PendingPairings))
	for _, pairing := range p.state.PendingPairings {
		line := pairing.Code + " " + pairing.DeviceID
		if matchesFilter(p.filter, line) {
			out = append(out, pairing)
		}
	}
	return out
}
