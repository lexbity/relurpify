package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type IdentityPane struct {
	subtabPaneState
}

func NewIdentityPane() *IdentityPane {
	p := &IdentityPane{}
	p.initTabs([]SubTab{{Label: "Subjects"}, {Label: "Externals"}, {Label: "Tokens"}})
	return p
}

func (p *IdentityPane) HandleCommand(cmd string, args []string) tea.Cmd {
	switch cmd {
	case "issue":
		if p.activeSubtab() != 2 || len(args) == 0 {
			return nil
		}
		scope := ""
		if len(args) > 1 {
			scope = strings.Join(args[1:], " ")
		}
		subjectID := strings.TrimSpace(args[0])
		return func() tea.Msg {
			return tokenActionRequestMsg{action: "issue", subjectID: subjectID, scope: scope}
		}
	case "revoke":
		if p.activeSubtab() != 2 || len(args) == 0 {
			return nil
		}
		return func() tea.Msg {
			return tokenActionRequestMsg{action: "revoke", tokenID: strings.TrimSpace(args[0])}
		}
	}
	return nil
}

func (p *IdentityPane) View() string {
	var lines []string
	switch p.activeSubtab() {
	case 1:
		lines = p.externalLines()
	case 2:
		lines = p.tokenLines()
	default:
		lines = p.subjectLines()
	}
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("No matching identity items.")}
	}
	title := "Identity"
	switch p.activeSubtab() {
	case 1:
		title = "External Identities"
	case 2:
		title = "Tokens"
	default:
		title = "Subjects"
	}
	return panelStyle.Width(p.panelWidth()).Render(sectionTitle(title) + "\n" + strings.Join(lines, "\n"))
}

func (p *IdentityPane) subjectLines() []string {
	lines := make([]string, 0, len(p.state.Subjects))
	for _, subject := range p.state.Subjects {
		line := fmt.Sprintf("%-12s %-18s %s", subject.Kind, emptyFallback(subject.TenantID, "default"), subject.ID)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 && len(p.state.Subjects) == 0 {
		return []string{mutedStyle.Render("No enrolled subjects.")}
	}
	return lines
}

func (p *IdentityPane) externalLines() []string {
	lines := make([]string, 0, len(p.state.ExternalIDs))
	for _, identity := range p.state.ExternalIDs {
		name := identity.DisplayName
		if name == "" {
			name = identity.ExternalID
		}
		line := fmt.Sprintf("%-10s %-14s %-18s %s", identity.Provider, emptyFallback(identity.AccountID, "-"), name, identity.SubjectID)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 && len(p.state.ExternalIDs) == 0 {
		return []string{mutedStyle.Render("No external identities recorded.")}
	}
	return lines
}

func (p *IdentityPane) tokenLines() []string {
	lines := make([]string, 0, len(p.state.Tokens))
	for _, token := range p.state.Tokens {
		status := "active"
		if token.RevokedAt != nil {
			status = "revoked"
		}
		line := fmt.Sprintf("%-18s %-10s %-16s %s", token.Name, status, emptyFallback(strings.Join(token.Scope, ","), "-"), token.ID)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 && len(p.state.Tokens) == 0 {
		return []string{mutedStyle.Render("No issued admin tokens.")}
	}
	return lines
}
