package tui

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// configSection identifies which part of the config pane is visible.
type configSection int

const (
	configSectionPolicies configSection = iota
	configSectionCapabilities
	configSectionPrompts
	configSectionTools
	configSectionContract
)

// ConfigPane displays capability class policies, tool policies, and the
// resolved agent contract. It has no subtabs.
type ConfigPane struct {
	section configSection
	sel     int

	// Loaded state
	classPolicies map[string]core.AgentPermissionLevel
	capabilities  []CapabilityInfo
	capability    *CapabilityDetail
	prompts       []PromptInfo
	prompt        *PromptDetail
	tools         []ToolInfo
	contract      *ContractSummary

	runtime       RuntimeAdapter
	width, height int
}

// NewConfigPane creates a ConfigPane and loads initial state from the runtime.
func NewConfigPane(rt RuntimeAdapter) *ConfigPane {
	p := &ConfigPane{runtime: rt}
	if rt != nil {
		p.classPolicies = rt.GetClassPolicies()
		p.capabilities = rt.ListCapabilities()
		p.prompts = rt.ListPrompts()
		p.tools = rt.ListToolsInfo()
		p.contract = rt.ContractSummary()
		p.refreshDetail()
	}
	return p
}

// SetSize resizes the pane.
func (p *ConfigPane) SetSize(w, h int) { p.width = w; p.height = h }

// Refresh reloads live state from the runtime.
func (p *ConfigPane) Refresh() {
	if p.runtime == nil {
		return
	}
	p.classPolicies = p.runtime.GetClassPolicies()
	p.capabilities = p.runtime.ListCapabilities()
	p.prompts = p.runtime.ListPrompts()
	p.tools = p.runtime.ListToolsInfo()
	p.contract = p.runtime.ContractSummary()
	p.refreshDetail()
}

// Update handles key navigation and data refresh messages.
func (p *ConfigPane) Update(msg tea.Msg) (*ConfigPane, tea.Cmd) {
	switch msg := msg.(type) {
	case configRefreshMsg:
		p.Refresh()
		return p, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			p.section = configSection((int(p.section) + 1) % 5)
			p.sel = 0
			p.refreshDetail()
		case "shift+tab":
			p.section = configSection((int(p.section) + 4) % 5)
			p.sel = 0
			p.refreshDetail()
		case "down":
			maxSel := p.maxSel()
			if p.sel < maxSel-1 {
				p.sel++
				p.refreshDetail()
			}
		case "up":
			if p.sel > 0 {
				p.sel--
				p.refreshDetail()
			}
		case "r":
			return p, func() tea.Msg { return configRefreshMsg{} }
		case "a":
			return p, p.togglePolicy(core.AgentPermissionAllow)
		case "d":
			return p, p.togglePolicy(core.AgentPermissionDeny)
		case "c":
			return p, p.togglePolicy("")
		}
	}
	return p, nil
}

func (p *ConfigPane) maxSel() int {
	switch p.section {
	case configSectionPolicies:
		return len(p.classPolicyRows())
	case configSectionCapabilities:
		return len(p.capabilities)
	case configSectionPrompts:
		return len(p.prompts)
	case configSectionTools:
		return len(p.tools)
	default:
		return 0
	}
}

func (p *ConfigPane) togglePolicy(level core.AgentPermissionLevel) tea.Cmd {
	if p.runtime == nil {
		return nil
	}
	switch p.section {
	case configSectionPolicies:
		rows := p.classPolicyRows()
		if p.sel >= len(rows) {
			return nil
		}
		class := rows[p.sel].class
		p.runtime.SetClassPolicyLive(class, level)
		p.classPolicies = p.runtime.GetClassPolicies()
		action := string(level)
		if action == "" {
			action = "cleared"
		}
		return func() tea.Msg {
			return chatSystemMsg{Text: fmt.Sprintf("class policy %q → %s", class, action)}
		}
	case configSectionTools:
		if p.sel >= len(p.tools) {
			return nil
		}
		tool := p.tools[p.sel]
		p.runtime.SetToolPolicyLive(tool.Name, level)
		p.tools = p.runtime.ListToolsInfo()
		action := string(level)
		if action == "" {
			action = "cleared"
		}
		return func() tea.Msg {
			return chatSystemMsg{Text: fmt.Sprintf("tool policy %q → %s", tool.Name, action)}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row helpers
// ---------------------------------------------------------------------------

type classPolicyRow struct {
	class  string
	policy core.AgentPermissionLevel
}

func (p *ConfigPane) classPolicyRows() []classPolicyRow {
	var rows []classPolicyRow
	for class, level := range p.classPolicies {
		rows = append(rows, classPolicyRow{class: class, policy: level})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].class < rows[j].class })
	return rows
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (p *ConfigPane) View() string {
	switch p.section {
	case configSectionPolicies:
		return p.viewPolicies()
	case configSectionCapabilities:
		return p.viewCapabilities()
	case configSectionPrompts:
		return p.viewPrompts()
	case configSectionTools:
		return p.viewTools()
	case configSectionContract:
		return p.viewContract()
	default:
		return p.viewPolicies()
	}
}

func (p *ConfigPane) sectionTabs() string {
	labels := []struct {
		s     configSection
		label string
	}{
		{configSectionPolicies, "policies"},
		{configSectionCapabilities, "capabilities"},
		{configSectionPrompts, "prompts"},
		{configSectionTools, "tools"},
		{configSectionContract, "contract"},
	}
	var parts []string
	for _, l := range labels {
		if l.s == p.section {
			parts = append(parts, subtabActiveStyle.Render(l.label))
		} else {
			parts = append(parts, subtabInactiveStyle.Render(l.label))
		}
	}
	return strings.Join(parts, "  ")
}

func (p *ConfigPane) refreshDetail() {
	if p.runtime == nil {
		return
	}
	switch p.section {
	case configSectionCapabilities:
		p.capability = nil
		if p.sel >= 0 && p.sel < len(p.capabilities) {
			detail, err := p.runtime.GetCapabilityDetail(p.capabilities[p.sel].ID)
			if err == nil {
				p.capability = detail
			}
		}
	case configSectionPrompts:
		p.prompt = nil
		if p.sel >= 0 && p.sel < len(p.prompts) {
			detail, err := p.runtime.GetPromptDetail(p.prompts[p.sel].PromptID)
			if err == nil {
				p.prompt = detail
			}
		}
	}
}

func (p *ConfigPane) viewPolicies() string {
	var b strings.Builder
	b.WriteString(p.sectionTabs() + "\n\n")
	b.WriteString(sectionHeaderStyle.Render("Capability Class Policies") + "\n")

	rows := p.classPolicyRows()
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("  No class policies configured.") + "\n")
	} else {
		maxVisible := p.height - 7
		if maxVisible < 1 {
			maxVisible = 8
		}
		start := 0
		if p.sel >= maxVisible {
			start = p.sel - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(rows) {
			end = len(rows)
		}
		for i := start; i < end; i++ {
			row := rows[i]
			levelStyle := dimStyle
			levelLabel := string(row.policy)
			switch row.policy {
			case core.AgentPermissionAllow:
				levelStyle = completedStyle
			case core.AgentPermissionDeny:
				levelStyle = diffRemoveStyle
			default:
				levelLabel = "inherit"
			}
			line := fmt.Sprintf("  %-30s  %s", row.class, levelStyle.Render(levelLabel))
			if i == p.sel {
				line = panelItemActiveStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[a] allow  [d] deny  [c] clear  ↑↓ navigate  tab section  [r] refresh"))
	return b.String()
}

func (p *ConfigPane) viewCapabilities() string {
	widths := (&PlannerPane{width: p.width}).splitWidths(5, 7)
	lines := make([]string, 0, len(p.capabilities))
	for _, capability := range p.capabilities {
		lines = append(lines, fmt.Sprintf("%s  %s  %s", capability.Name, capability.Kind, capability.RuntimeFamily))
	}
	detail := []string{dimStyle.Render("No capability selected.")}
	if p.capability != nil {
		detail = []string{
			p.capability.Meta.Title,
			"",
			dimStyle.Render("ID") + "  " + p.capability.Meta.ID,
			dimStyle.Render("Kind") + "  " + p.capability.Meta.Kind,
			dimStyle.Render("Runtime") + "  " + p.capability.Meta.RuntimeFamily,
			dimStyle.Render("Trust") + "  " + p.capability.Meta.TrustClass,
			dimStyle.Render("Scope") + "  " + fallback(p.capability.Meta.Scope, "n/a"),
			dimStyle.Render("Exposure") + "  " + fallback(p.capability.Exposure, "n/a"),
			dimStyle.Render("Provider") + "  " + fallback(p.capability.ProviderID, "n/a"),
			dimStyle.Render("Risk") + "  " + joinOrNA(p.capability.RiskClasses),
			dimStyle.Render("Effects") + "  " + joinOrNA(p.capability.EffectClasses),
		}
		if p.capability.Description != "" {
			detail = append(detail, "", p.capability.Description)
		}
	}
	return strings.Join([]string{
		p.sectionTabs(),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Capabilities", widths[0], plannerList(lines, p.sel, p.height-10)),
			plannerPanel("Detail", widths[1], detail...),
		),
		dimStyle.Render("↑↓ navigate  tab section  [r] refresh"),
	}, "\n\n")
}

func (p *ConfigPane) viewPrompts() string {
	widths := (&PlannerPane{width: p.width}).splitWidths(5, 7)
	lines := make([]string, 0, len(p.prompts))
	for _, prompt := range p.prompts {
		lines = append(lines, fmt.Sprintf("%s  %s  %s", prompt.Meta.Title, prompt.Meta.RuntimeFamily, fallback(prompt.ProviderID, "local")))
	}
	detail := []string{dimStyle.Render("No prompt selected.")}
	if p.prompt != nil {
		detail = []string{
			p.prompt.Meta.Title,
			"",
			dimStyle.Render("ID") + "  " + p.prompt.PromptID,
			dimStyle.Render("Runtime") + "  " + fallback(p.prompt.Meta.RuntimeFamily, "n/a"),
			dimStyle.Render("Trust") + "  " + fallback(p.prompt.Meta.TrustClass, "n/a"),
			dimStyle.Render("Provider") + "  " + fallback(p.prompt.ProviderID, "n/a"),
			dimStyle.Render("Metadata") + "  " + joinOrNA(p.prompt.Metadata),
		}
		if p.prompt.Description != "" {
			detail = append(detail, "", p.prompt.Description)
		}
		for i, message := range p.prompt.Messages {
			detail = append(detail, "", dimStyle.Render(fmt.Sprintf("Message %d (%s)", i+1, message.Role)))
			for _, block := range message.Content {
				detail = append(detail, renderStructuredContentPreview(block)...)
			}
		}
	}
	return strings.Join([]string{
		p.sectionTabs(),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Prompts", widths[0], plannerList(lines, p.sel, p.height-10)),
			plannerPanel("Detail", widths[1], detail...),
		),
		dimStyle.Render("↑↓ navigate  tab section  [r] refresh"),
	}, "\n\n")
}

func (p *ConfigPane) viewTools() string {
	var b strings.Builder
	b.WriteString(p.sectionTabs() + "\n\n")
	b.WriteString(sectionHeaderStyle.Render("Tool Policies") + "\n")

	if len(p.tools) == 0 {
		b.WriteString(dimStyle.Render("  No tools registered.") + "\n")
	} else {
		maxVisible := p.height - 7
		if maxVisible < 1 {
			maxVisible = 8
		}
		start := 0
		if p.sel >= maxVisible {
			start = p.sel - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(p.tools) {
			end = len(p.tools)
		}
		for i := start; i < end; i++ {
			tool := p.tools[i]
			levelStyle := dimStyle
			levelLabel := string(tool.Policy)
			switch tool.Policy {
			case core.AgentPermissionAllow:
				levelStyle = completedStyle
			case core.AgentPermissionDeny:
				levelStyle = diffRemoveStyle
			default:
				levelLabel = "default"
			}
			line := fmt.Sprintf("  %-28s  %s", tool.Name, levelStyle.Render(levelLabel))
			if i == p.sel {
				line = panelItemActiveStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
		if len(p.tools) > maxVisible {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  (%d/%d)", p.sel+1, len(p.tools))) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[a] allow  [d] deny  [c] clear  ↑↓ navigate  tab section  [r] refresh"))
	return b.String()
}

func (p *ConfigPane) viewContract() string {
	var b strings.Builder
	b.WriteString(p.sectionTabs() + "\n\n")
	b.WriteString(sectionHeaderStyle.Render("Agent Contract") + "\n")

	c := p.contract
	if c == nil {
		b.WriteString(dimStyle.Render("  Contract not available.") + "\n")
	} else {
		rows := []struct{ k, v string }{
			{"agent", c.AgentID},
			{"manifest", c.ManifestName + " " + dimStyle.Render(c.ManifestVersion)},
			{"workspace", c.Workspace},
			{"capabilities", fmt.Sprintf("%d  admitted: %d  rejected: %d", c.CapabilityCount, c.AdmissionCount, c.RejectedCount)},
			{"policy rules", fmt.Sprintf("%d", c.PolicyRuleCount)},
		}
		for _, r := range rows {
			if r.v == "" {
				continue
			}
			b.WriteString(dimStyle.Render(fmt.Sprintf("%-14s", r.k)) + "  " + r.v + "\n")
		}
		if len(c.AppliedSkills) > 0 {
			b.WriteString("\n" + dimStyle.Render("skills\n"))
			for _, s := range c.AppliedSkills {
				b.WriteString(completedStyle.Render("  ✓ ") + s + "\n")
			}
		}
		if len(c.FailedSkills) > 0 {
			for _, s := range c.FailedSkills {
				b.WriteString(diffRemoveStyle.Render("  ✗ ") + s + "\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("tab section  [r] refresh"))
	return b.String()
}
