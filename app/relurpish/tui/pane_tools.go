package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lexcodex/relurpify/framework/core"
)

// knownClassOrder defines the display order for capability risk groups.
var knownClassOrder = []string{"read-only", "execute", "destructive", "network"}

type toolsRowKind int

const (
	toolsRowKindHeader toolsRowKind = iota
	toolsRowKindTool
)

type toolsRow struct {
	kind toolsRowKind
	tag  string // section class label (both header and tool rows)
	name string // tool name (tool rows only)
}

// ToolsPane shows registered local tools grouped by capability class with live policy editing.
type ToolsPane struct {
	tools       []ToolInfo
	tagPolicies map[string]core.AgentPermissionLevel
	rows        []toolsRow
	sel         int
	runtime     RuntimeAdapter
	width       int
	height      int
}

// NewToolsPane creates a ToolsPane and loads the initial tool list.
func NewToolsPane(rt RuntimeAdapter) *ToolsPane {
	p := &ToolsPane{runtime: rt}
	p.load()
	return p
}

func (p *ToolsPane) load() {
	p.tagPolicies = make(map[string]core.AgentPermissionLevel)
	if p.runtime == nil {
		p.rows = nil
		return
	}
	p.tools = p.runtime.ListToolsInfo()
	if tp := p.runtime.GetClassPolicies(); tp != nil {
		p.tagPolicies = tp
	}
	p.buildRows()
}

func (p *ToolsPane) buildRows() {
	groups := make(map[string][]ToolInfo, len(knownClassOrder))
	var other []ToolInfo

	for _, t := range p.tools {
		placed := false
		for _, kt := range knownClassOrder {
			for _, label := range t.Labels {
				if label == kt {
					groups[kt] = append(groups[kt], t)
					placed = true
					break
				}
			}
			if placed {
				break
			}
		}
		if !placed {
			other = append(other, t)
		}
	}

	p.rows = nil
	for _, kt := range knownClassOrder {
		tools := groups[kt]
		if len(tools) == 0 {
			continue
		}
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		p.rows = append(p.rows, toolsRow{kind: toolsRowKindHeader, tag: kt})
		for _, t := range tools {
			p.rows = append(p.rows, toolsRow{kind: toolsRowKindTool, tag: kt, name: t.Name})
		}
	}
	if len(other) > 0 {
		sort.Slice(other, func(i, j int) bool { return other[i].Name < other[j].Name })
		p.rows = append(p.rows, toolsRow{kind: toolsRowKindHeader, tag: "other"})
		for _, t := range other {
			p.rows = append(p.rows, toolsRow{kind: toolsRowKindTool, tag: "other", name: t.Name})
		}
	}
}

// SetSize resizes the pane.
func (p *ToolsPane) SetSize(w, h int) { p.width = w; p.height = h }

func (p *ToolsPane) toolByName(name string) (ToolInfo, bool) {
	for _, t := range p.tools {
		if t.Name == name {
			return t, true
		}
	}
	return ToolInfo{}, false
}

func cyclePermissionLevel(level core.AgentPermissionLevel) core.AgentPermissionLevel {
	switch level {
	case "":
		return core.AgentPermissionAllow
	case core.AgentPermissionAllow:
		return core.AgentPermissionAsk
	case core.AgentPermissionAsk:
		return core.AgentPermissionDeny
	default: // deny → reset to inherited
		return ""
	}
}

// Update handles keyboard navigation and policy editing.
func (p *ToolsPane) Update(msg tea.Msg) (*ToolsPane, tea.Cmd) {
	kMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return p, nil
	}
	switch kMsg.String() {
	case "up":
		if p.sel > 0 {
			p.sel--
		}
	case "down":
		if p.sel < len(p.rows)-1 {
			p.sel++
		}
	case " ", "enter":
		if p.sel < len(p.rows) {
			p.cycleSelected()
		}
	case "r":
		if p.sel < len(p.rows) {
			p.resetSelected()
		}
	case "s":
		if cmd := p.saveSelected(); cmd != nil {
			return p, cmd
		}
	}
	return p, nil
}

func (p *ToolsPane) cycleSelected() {
	row := p.rows[p.sel]
	switch row.kind {
	case toolsRowKindHeader:
		cur := p.tagPolicies[row.tag]
		next := cyclePermissionLevel(cur)
		if next == "" {
			delete(p.tagPolicies, row.tag)
		} else {
			p.tagPolicies[row.tag] = next
		}
		if p.runtime != nil {
			p.runtime.SetClassPolicyLive(row.tag, next)
		}
	case toolsRowKindTool:
		t, ok := p.toolByName(row.name)
		if !ok {
			return
		}
		next := cyclePermissionLevel(t.Policy)
		for i := range p.tools {
			if p.tools[i].Name == row.name {
				p.tools[i].Policy = next
				p.tools[i].HasPolicy = next != ""
				break
			}
		}
		if p.runtime != nil {
			p.runtime.SetToolPolicyLive(row.name, next)
		}
	}
}

func (p *ToolsPane) resetSelected() {
	row := p.rows[p.sel]
	switch row.kind {
	case toolsRowKindHeader:
		delete(p.tagPolicies, row.tag)
		if p.runtime != nil {
			p.runtime.SetClassPolicyLive(row.tag, "")
		}
	case toolsRowKindTool:
		for i := range p.tools {
			if p.tools[i].Name == row.name {
				p.tools[i].Policy = ""
				p.tools[i].HasPolicy = false
				break
			}
		}
		if p.runtime != nil {
			p.runtime.SetToolPolicyLive(row.name, "")
		}
	}
}

func (p *ToolsPane) saveSelected() tea.Cmd {
	if p.sel >= len(p.rows) {
		return nil
	}
	row := p.rows[p.sel]
	if row.kind != toolsRowKindTool {
		return nil
	}
	t, ok := p.toolByName(row.name)
	if !ok || !t.HasPolicy || t.Policy == "" || p.runtime == nil {
		return nil
	}
	name, level, rt := t.Name, t.Policy, p.runtime
	return func() tea.Msg {
		if err := rt.SaveToolPolicy(name, level); err != nil {
			return chatSystemMsg{text: fmt.Sprintf("Save failed: %v", err)}
		}
		return chatSystemMsg{text: fmt.Sprintf("Policy for '%s' saved to manifest", name)}
	}
}

// effectivePolicy returns the tool's effective permission and whether it's a per-tool override.
func (p *ToolsPane) effectivePolicy(t ToolInfo) (level core.AgentPermissionLevel, isOverride bool) {
	if t.HasPolicy && t.Policy != "" {
		return t.Policy, true
	}
	best := core.AgentPermissionLevel("")
	for _, label := range t.Labels {
		pol, ok := p.tagPolicies[label]
		if !ok {
			continue
		}
		switch {
		case pol == core.AgentPermissionDeny:
			return pol, false
		case pol == core.AgentPermissionAsk && best != core.AgentPermissionDeny:
			best = pol
		case pol == core.AgentPermissionAllow && best == "":
			best = pol
		}
	}
	return best, false
}

func toolsPolicyStyle(level core.AgentPermissionLevel) lipgloss.Style {
	switch level {
	case core.AgentPermissionAllow:
		return completedStyle
	case core.AgentPermissionAsk:
		return inProgressStyle
	case core.AgentPermissionDeny:
		return lipgloss.NewStyle().Foreground(colorError)
	default:
		return dimStyle
	}
}

// View renders the tools pane.
func (p *ToolsPane) View() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Local Tools & Permissions"))
	b.WriteString("\n\n")

	if len(p.rows) == 0 {
		b.WriteString(dimStyle.Render("No local tools registered."))
		return b.String()
	}

	maxVisible := p.height - 5
	if maxVisible < 1 {
		maxVisible = 1
	}

	start := 0
	if p.sel >= start+maxVisible {
		start = p.sel - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(p.rows) {
		end = len(p.rows)
	}

	for i := start; i < end; i++ {
		row := p.rows[i]
		isSelected := i == p.sel
		var line string

		switch row.kind {
		case toolsRowKindHeader:
			pol := p.tagPolicies[row.tag]
			polStr := string(pol)
			if polStr == "" {
				polStr = "default"
			}
			content := fmt.Sprintf("── %s [%s]", row.tag, polStr)
			if isSelected {
				line = panelItemActiveStyle.Render(content)
			} else {
				line = sectionHeaderStyle.Render(content)
			}

		case toolsRowKindTool:
			t, ok := p.toolByName(row.name)
			if !ok {
				continue
			}
			eff, isOverride := p.effectivePolicy(t)
			polStr := string(eff)
			if polStr == "" {
				polStr = "default"
			}
			prefix := "  "
			if isSelected {
				prefix = panelItemActiveStyle.Render(">") + " "
			}
			nameW := fmt.Sprintf("%-22s", t.Name)
			if isOverride {
				nameW = filePathStyle.Render(fmt.Sprintf("%-22s", t.Name))
			}
			polW := toolsPolicyStyle(eff).Render(fmt.Sprintf("%-5s", polStr))
			label := ""
			if isOverride {
				label = " " + dimStyle.Render("(custom)")
			} else if eff != "" {
				label = " " + dimStyle.Render("(tag)")
			}
			runtimeLabel := dimStyle.Render(fmt.Sprintf("[%s]", toolRuntimeLabel(t)))
			line = prefix + nameW + " " + polW + " " + runtimeLabel + label
		}

		b.WriteString(line + "\n")
	}

	hint := "↑↓ navigate  space/enter cycle  r reset"
	if p.sel < len(p.rows) && p.rows[p.sel].kind == toolsRowKindTool {
		if t, ok := p.toolByName(p.rows[p.sel].name); ok && t.HasPolicy && t.Policy != "" {
			hint += "  s save to manifest"
		}
	}
	b.WriteString("\n" + dimStyle.Render(hint))
	return b.String()
}

func toolRuntimeLabel(t ToolInfo) string {
	family := strings.TrimSpace(t.RuntimeFamily)
	if family == "" {
		family = "local-tool"
	}
	scope := strings.TrimSpace(t.Scope)
	if scope == "" {
		return family
	}
	return family + "/" + scope
}
