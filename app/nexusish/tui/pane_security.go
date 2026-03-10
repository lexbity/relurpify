package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type SecurityPane struct {
	subtabPaneState
}

func NewSecurityPane() *SecurityPane {
	p := &SecurityPane{}
	p.initTabs([]SubTab{{Label: "Policies"}, {Label: "Audit"}})
	return p
}

func (p *SecurityPane) HandleCommand(cmd string, args []string) tea.Cmd {
	if p.activeSubtab() != 0 || len(args) == 0 {
		return nil
	}
	ruleID := strings.TrimSpace(args[0])
	switch cmd {
	case "enable":
		return func() tea.Msg { return policyActionRequestMsg{ruleID: ruleID, enabled: true} }
	case "disable":
		return func() tea.Msg { return policyActionRequestMsg{ruleID: ruleID, enabled: false} }
	default:
		return nil
	}
}

func (p *SecurityPane) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "[":
		p.stepSubtab(-1, 1)
	case "]":
		p.stepSubtab(1, 1)
	}
	return nil
}

func (p *SecurityPane) View() string {
	if p.activeSubtab() == 1 {
		lines := p.auditLines()
		if len(lines) == 0 {
			lines = []string{mutedStyle.Render("No audit summary available.")}
		}
		return panelStyle.Width(p.panelWidth()).Render(sectionTitle("Audit") + "\n" + strings.Join(lines, "\n"))
	}
	lines := p.policyLines()
	if len(lines) == 0 {
		lines = []string{okStyle.Render("No current security warnings.")}
	}
	return panelStyle.Width(p.panelWidth()).Render(sectionTitle("Policies") + "\n" + strings.Join(lines, "\n"))
}

func (p *SecurityPane) policyLines() []string {
	lines := make([]string, 0, len(p.state.PolicyRules)+len(p.state.SecurityWarnings)+1)
	if len(p.state.PolicyRules) == 0 {
		lines = append(lines, mutedStyle.Render("No persisted policy rules. Showing gateway security warnings instead."))
	}
	for _, rule := range p.state.PolicyRules {
		state := "disabled"
		if rule.Enabled {
			state = "enabled"
		}
		line := fmt.Sprintf("%-20s %-8s %-14s p=%-4d %s", rule.Name, state, emptyFallback(rule.Action, "-"), rule.Priority, emptyFallback(rule.ID, "-"))
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
		if rule.Reason != "" && matchesFilter(p.filter, rule.Reason) {
			lines = append(lines, mutedStyle.Render("  "+rule.Reason))
		}
	}
	for _, warning := range p.state.SecurityWarnings {
		if matchesFilter(p.filter, warning) {
			lines = append(lines, warningStyle.Render("! ")+warning)
		}
	}
	return lines
}

func (p *SecurityPane) auditLines() []string {
	type item struct {
		name  string
		count uint64
	}
	items := make([]item, 0, len(p.state.EventCounts))
	for name, count := range p.state.EventCounts {
		items = append(items, item{name: name, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].name < items[j].name
		}
		return items[i].count > items[j].count
	})
	lines := make([]string, 0, len(items))
	for _, entry := range items {
		line := fmt.Sprintf("%-36s %d", entry.name, entry.count)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
	}
	return lines
}
