package tui

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
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
	filter  string

	// Loaded state
	classPolicies map[string]agentspec.AgentPermissionLevel
	capabilities  []CapabilityInfo
	capability    *CapabilityDetail
	prompts       []PromptInfo
	prompt        *PromptDetail
	tools         []ToolInfo
	contract      *ContractSummary

	runtime       RuntimeAdapter
	width, height int
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewConfigPane creates a ConfigPane and loads initial state from the runtime.
func NewConfigPane(rt RuntimeAdapter) *ConfigPane {
	p := &ConfigPane{th: theme.Default(), runtime: rt}
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

// SetSection switches the visible config section.
func (p *ConfigPane) SetSection(section configSection) {
	p.section = section
	p.sel = 0
	p.refreshDetail()
}

// SetFilter updates the live filter string for the current base tab.
func (p *ConfigPane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
	p.sel = 0
	p.refreshDetail()
}

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
			return p, p.togglePolicy(agentspec.AgentPermissionAllow)
		case "d":
			return p, p.togglePolicy(agentspec.AgentPermissionDeny)
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

func (p *ConfigPane) togglePolicy(level agentspec.AgentPermissionLevel) tea.Cmd {
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
	policy agentspec.AgentPermissionLevel
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
			parts = append(parts, p.th.Subhead().Render(l.label))
		} else {
			parts = append(parts, p.th.Dim().Render(l.label))
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
	b.WriteString(p.th.Subhead().Render("Capability Class Policies") + "\n")

	rows := p.classPolicyRows()
	if p.filter != "" {
		filtered := rows[:0]
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.class+" "+string(row.policy)), p.filter) {
				filtered = append(filtered, row)
			}
		}
		rows = append([]classPolicyRow(nil), filtered...)
	}
	if len(rows) == 0 {
		b.WriteString(p.th.Dim().Render("  No class policies configured.") + "\n")
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
			levelStyle := p.th.Dim()
			levelLabel := string(row.policy)
			switch row.policy {
			case agentspec.AgentPermissionAllow:
				levelStyle = p.th.Success()
			case agentspec.AgentPermissionDeny:
				levelStyle = p.th.Error()
			default:
				levelLabel = "inherit"
			}
			line := fmt.Sprintf("  %-30s  %s", row.class, levelStyle.Render(levelLabel))
			if i == p.sel {
				line = p.th.Active().Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(p.th.Dim().Render("[a] allow  [d] deny  [c] clear  ↑↓ navigate  tab section  [r] refresh"))
	return b.String()
}

func (p *ConfigPane) viewCapabilities() string {
	widths := splitWidths(p.width, 5, 7)
	lines := make([]string, 0, len(p.capabilities))
	for _, capability := range p.capabilities {
		line := fmt.Sprintf("%s  %s  %s", capability.Name, capability.Kind, capability.RuntimeFamily)
		if p.filter != "" && !strings.Contains(strings.ToLower(line), p.filter) {
			continue
		}
		lines = append(lines, line)
	}
	detail := []string{p.th.Dim().Render("No capability selected.")}
	if p.capability != nil {
		detail = []string{
			p.capability.Meta.Title,
			"",
			p.th.Dim().Render("ID") + "  " + p.capability.Meta.ID,
			p.th.Dim().Render("Kind") + "  " + p.capability.Meta.Kind,
			p.th.Dim().Render("Runtime") + "  " + p.capability.Meta.RuntimeFamily,
			p.th.Dim().Render("Trust") + "  " + p.capability.Meta.TrustClass,
			p.th.Dim().Render("Scope") + "  " + fallback(p.capability.Meta.Scope, "n/a"),
			p.th.Dim().Render("Exposure") + "  " + fallback(p.capability.Exposure, "n/a"),
			p.th.Dim().Render("Provider") + "  " + fallback(p.capability.ProviderID, "n/a"),
			p.th.Dim().Render("Risk") + "  " + joinOrNA(p.capability.RiskClasses),
			p.th.Dim().Render("Effects") + "  " + joinOrNA(p.capability.EffectClasses),
		}
		if p.capability.Description != "" {
			detail = append(detail, "", p.capability.Description)
		}
	}
	return strings.Join([]string{
		p.sectionTabs(),
		lipgloss.JoinHorizontal(lipgloss.Top,
			sectionPanel(p.th, "Capabilities", widths[0], sectionList(p.th, lines, p.sel, p.height-10)),
			sectionPanel(p.th, "Detail", widths[1], detail...),
		),
		p.th.Dim().Render("↑↓ navigate  tab section  [r] refresh"),
	}, "\n\n")
}

func (p *ConfigPane) viewPrompts() string {
	widths := splitWidths(p.width, 5, 7)
	lines := make([]string, 0, len(p.prompts))
	for _, prompt := range p.prompts {
		lines = append(lines, fmt.Sprintf("%s  %s  %s", prompt.Meta.Title, prompt.Meta.RuntimeFamily, fallback(prompt.ProviderID, "local")))
	}
	detail := []string{p.th.Dim().Render("No prompt selected.")}
	if p.prompt != nil {
		detail = []string{
			p.prompt.Meta.Title,
			"",
			p.th.Dim().Render("ID") + "  " + p.prompt.PromptID,
			p.th.Dim().Render("Runtime") + "  " + fallback(p.prompt.Meta.RuntimeFamily, "n/a"),
			p.th.Dim().Render("Trust") + "  " + fallback(p.prompt.Meta.TrustClass, "n/a"),
			p.th.Dim().Render("Provider") + "  " + fallback(p.prompt.ProviderID, "n/a"),
			p.th.Dim().Render("Metadata") + "  " + joinOrNA(p.prompt.Metadata),
		}
		if p.prompt.Description != "" {
			detail = append(detail, "", p.prompt.Description)
		}
		for i, message := range p.prompt.Messages {
			detail = append(detail, "", p.th.Dim().Render(fmt.Sprintf("Message %d (%s)", i+1, message.Role)))
			for _, block := range message.Content {
				detail = append(detail, renderStructuredContentPreview(block)...)
			}
		}
	}
	return strings.Join([]string{
		p.sectionTabs(),
		lipgloss.JoinHorizontal(lipgloss.Top,
			sectionPanel(p.th, "Prompts", widths[0], sectionList(p.th, lines, p.sel, p.height-10)),
			sectionPanel(p.th, "Detail", widths[1], detail...),
		),
		p.th.Dim().Render("↑↓ navigate  tab section  [r] refresh"),
	}, "\n\n")
}

func (p *ConfigPane) viewTools() string {
	var b strings.Builder
	b.WriteString(p.sectionTabs() + "\n\n")
	b.WriteString(p.th.Subhead().Render("Tool Policies") + "\n")

	if len(p.tools) == 0 {
		b.WriteString(p.th.Dim().Render("  No tools registered.") + "\n")
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
			levelStyle := p.th.Dim()
			levelLabel := string(tool.Policy)
			switch tool.Policy {
			case agentspec.AgentPermissionAllow:
				levelStyle = p.th.Success()
			case agentspec.AgentPermissionDeny:
				levelStyle = p.th.Error()
			default:
				levelLabel = "default"
			}
			line := fmt.Sprintf("  %-28s  %s", tool.Name, levelStyle.Render(levelLabel))
			if i == p.sel {
				line = p.th.Active().Render(line)
			}
			b.WriteString(line + "\n")
		}
		if len(p.tools) > maxVisible {
			b.WriteString(p.th.Dim().Render(fmt.Sprintf("  (%d/%d)", p.sel+1, len(p.tools))) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(p.th.Dim().Render("[a] allow  [d] deny  [c] clear  ↑↓ navigate  tab section  [r] refresh"))
	return b.String()
}

func (p *ConfigPane) viewContract() string {
	var b strings.Builder
	b.WriteString(p.sectionTabs() + "\n\n")
	b.WriteString(p.th.Subhead().Render("Agent Contract") + "\n")

	c := p.contract
	if c == nil {
		b.WriteString(p.th.Dim().Render("  Contract not available.") + "\n")
	} else {
		rows := []struct{ k, v string }{
			{"agent", c.AgentID},
			{"manifest", c.ManifestName + " " + p.th.Dim().Render(c.ManifestVersion)},
			{"workspace", c.Workspace},
			{"capabilities", fmt.Sprintf("%d  admitted: %d  rejected: %d", c.CapabilityCount, c.AdmissionCount, c.RejectedCount)},
			{"policy rules", fmt.Sprintf("%d", c.PolicyRuleCount)},
		}
		for _, r := range rows {
			if r.v == "" {
				continue
			}
			b.WriteString(p.th.Dim().Render(fmt.Sprintf("%-14s", r.k)) + "  " + r.v + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(p.th.Dim().Render("tab section  [r] refresh"))
	return b.String()
}

// SetTheme sets the active semantic style source.
func (p *ConfigPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
