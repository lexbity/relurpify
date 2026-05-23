package euclotui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type GraphPane struct {
	router   *EucloEventRouter
	width    int
	height   int
	selected int
}

func NewGraphPane(router *EucloEventRouter) *GraphPane {
	return &GraphPane{router: router}
}

func (p *GraphPane) SetRouter(router *EucloEventRouter) { p.router = router }

func (p *GraphPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *GraphPane) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		order := p.nodeOrder()
		if len(order) == 0 {
			return nil
		}
		switch msg.String() {
		case "up", "k":
			p.moveSelection(-1, len(order))
		case "down", "j":
			p.moveSelection(1, len(order))
		case "pgup":
			p.moveSelection(-5, len(order))
		case "pgdown":
			p.moveSelection(5, len(order))
		case "home":
			p.selected = 0
		case "end":
			p.selected = len(order) - 1
		}
	}
	return nil
}

func (p *GraphPane) View() string {
	if p.width < 60 {
		return dimStyle.Render("Terminal too narrow. Minimum 60 columns required.")
	}
	snap := p.snapshot()
	order := p.nodeOrderFrom(snap)
	if len(order) == 0 {
		return eucloFrameStyle.Render(sectionHeaderStyle.Render("Graph Surface") + "\n" + dimStyle.Render("No execution nodes yet."))
	}
	if p.selected < 0 || p.selected >= len(order) {
		p.selected = 0
	}
	leftW, rightW := graphSplitWidths(p.width)
	left := p.renderTimeline(snap, order, leftW)
	right := p.renderDetail(snap, order[p.selected], rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (p *GraphPane) snapshot() EucloProjectionSnapshot {
	if p == nil || p.router == nil {
		return EucloProjectionSnapshot{}
	}
	return p.router.Snapshot()
}

func (p *GraphPane) nodeOrder() []string {
	return p.nodeOrderFrom(p.snapshot())
}

func (p *GraphPane) nodeOrderFrom(snap EucloProjectionSnapshot) []string {
	if len(snap.Graph.Order) == 0 {
		return nil
	}
	order := make([]string, 0, len(snap.Graph.Order))
	for _, id := range snap.Graph.Order {
		if node := snap.Graph.Nodes[id]; node != nil {
			order = append(order, id)
		}
	}
	return order
}

func (p *GraphPane) moveSelection(delta, total int) {
	if total <= 0 {
		p.selected = 0
		return
	}
	p.selected += delta
	if p.selected < 0 {
		p.selected = 0
	}
	if p.selected >= total {
		p.selected = total - 1
	}
}

func (p *GraphPane) renderTimeline(snap EucloProjectionSnapshot, order []string, width int) string {
	if width < 24 {
		width = 24
	}
	var lines []string
	lines = append(lines, sectionHeaderStyle.Render("Execution DAG"))
	for i, id := range order {
		node := snap.Graph.Nodes[id]
		if node == nil {
			continue
		}
		icon := graphStatusIcon(node.Status)
		label := node.Label
		if label == "" {
			label = node.ID
		}
		line := fmt.Sprintf("  %s %s", icon, label)
		if i == p.selected {
			line = panelItemActiveStyle.Render(line)
		} else if node.Status == "skipped" {
			line = dimStyle.Render(line)
		} else {
			line = panelItemStyle.Render(line)
		}
		lines = append(lines, line)
		if i < len(order)-1 {
			lines = append(lines, "  "+dimStyle.Render("│"))
		}
	}
	return graphPanelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (p *GraphPane) renderDetail(snap EucloProjectionSnapshot, nodeID string, width int) string {
	if width < 24 {
		width = 24
	}
	node := snap.Graph.Nodes[nodeID]
	if node == nil {
		return graphPanelStyle.Width(width).Render(sectionHeaderStyle.Render("Node Detail") + "\n" + dimStyle.Render("No node selected."))
	}
	var lines []string
	lines = append(lines, sectionHeaderStyle.Render("Node Detail"))
	lines = append(lines, fmt.Sprintf("%s %s", headerStyle.Render(node.ID), dimStyle.Render("("+node.Status+")")))
	if node.RecipeID != "" {
		lines = append(lines, dimStyle.Render("recipe: ")+node.RecipeID)
	}
	if node.StepID != "" {
		lines = append(lines, dimStyle.Render("step: ")+node.StepID)
	}
	if node.LastEvent != "" {
		lines = append(lines, dimStyle.Render("event: ")+string(node.LastEvent))
	}
	if node.Label != "" {
		lines = append(lines, "")
		lines = append(lines, node.Label)
	}
	if len(node.RouteScores) > 0 {
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Route Scores"))
		for _, key := range sortedScoreKeys(node.RouteScores) {
			lines = append(lines, fmt.Sprintf("  %s %0.2f", dimStyle.Render(key+":"), node.RouteScores[key]))
		}
	}
	return graphPanelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func graphStatusIcon(status string) string {
	switch status {
	case "completed":
		return "●"
	case "running":
		return "◐"
	case "failed":
		return "✗"
	case "skipped":
		return dimStyle.Render("○")
	default:
		return "○"
	}
}

func graphSplitWidths(total int) (int, int) {
	if total < 0 {
		total = 0
	}
	left := (total * 4) / 10
	if left < 24 {
		left = 24
	}
	right := total - left
	if right < 24 {
		right = 24
	}
	if left+right > total && total > 0 {
		right = total - left
		if right < 24 {
			right = 24
		}
	}
	return left, right
}

var graphPanelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorSecondary).
	Padding(0, 1)
