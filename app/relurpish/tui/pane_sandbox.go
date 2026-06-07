package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type sandboxRuntime interface {
	SessionInfo() SessionInfo
	LoadSandboxManifest() (*config.AgentManifest, error)
	SaveSandboxManifest(*config.AgentManifest) (string, error)
	SandboxBackend() string
	SaveSandboxBackend(string) (string, error)
	ReloadWorkspace(context.Context, string) error
}

type sandboxNodeKind int

const (
	sandboxNodeCategory sandboxNodeKind = iota
	sandboxNodeFileScope
	sandboxNodeCommandDefault
	sandboxNodeCommandPattern
	sandboxNodeNetworkRule
	sandboxNodeProviderPolicy
	sandboxNodeToolPolicy
)

type sandboxVisibleNode struct {
	node  *sandboxNode
	depth int
}

type sandboxNode struct {
	ID         string
	Label      string
	Kind       sandboxNodeKind
	State      agentspec.AgentPermissionLevel
	Summary    string
	Expandable bool
	Expanded   bool
	Selectable bool
	Children   []*sandboxNode

	// File scope fields.
	fileIndex int
	filePerm  permissions.FileSystemPermission

	// Command fields.
	patternIndex int
	pattern      string
	isAllowList  bool
	defaultBash  bool

	// Network fields.
	networkIndex int
	networkPerm  permissions.NetworkPermission

	// Provider policy fields.
	providerID  string
	providerPol agentspec.ProviderPolicy

	// Tool policy fields.
	toolName string
	toolPol  agentspec.ToolPolicy
}

// SandboxPane edits the sandbox and permission-related manifest sections.
type SandboxPane struct {
	runtime sandboxRuntime

	manifest *config.AgentManifest
	root     *sandboxNode
	visible  []sandboxVisibleNode

	selectedID string
	sel        int
	filter     string
	width      int
	height     int
	status     string

	editing    bool
	editBuffer string
	editLabel  string

	confirmBackend bool
	pendingBackend string

	expanded map[string]bool
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewSandboxPane loads the current manifest and builds the tree editor.
func NewSandboxPane(rt sandboxRuntime) *SandboxPane {
	p := &SandboxPane{
		th:       theme.Default(),
		runtime:  rt,
		expanded: make(map[string]bool),
	}
	p.Refresh()
	return p
}

func (p *SandboxPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *SandboxPane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
	p.rebuildVisible()
}

func (p *SandboxPane) Refresh() {
	if p.runtime == nil {
		p.manifest = nil
		p.root = nil
		p.visible = nil
		return
	}
	loaded, err := p.runtime.LoadSandboxManifest()
	if err != nil {
		p.status = fmt.Sprintf("load failed: %v", err)
		p.manifest = nil
		p.root = nil
		p.visible = nil
		return
	}
	p.manifest = loaded
	p.root = p.buildTree()
	p.rebuildVisible()
	if p.selectedID != "" && p.selectByID(p.selectedID) {
		return
	}
	if len(p.visible) > 0 {
		p.sel = clampIndex(p.sel, len(p.visible))
		p.selectedID = p.visible[p.sel].node.ID
	}
}

func (p *SandboxPane) Update(msg tea.Msg) (*SandboxPane, tea.Cmd) {
	switch msg := msg.(type) {
	case sandboxPersistedMsg:
		if msg.Err != nil {
			p.status = fmt.Sprintf("save failed: %v", msg.Err)
			return p, nil
		}
		p.status = "sandbox saved"
		if msg.Backup != "" {
			p.status = fmt.Sprintf("sandbox saved (backup: %s)", msg.Backup)
		}
		p.Refresh()
		return p, nil
	case tea.KeyMsg:
		if p.confirmBackend {
			switch msg.String() {
			case "enter", "y":
				backend := p.pendingBackend
				p.confirmBackend = false
				p.pendingBackend = ""
				return p, p.persistBackendCmd(backend)
			case "esc", "n":
				p.confirmBackend = false
				p.pendingBackend = ""
				p.status = "backend change canceled"
				return p, nil
			}
			return p, nil
		}
		if p.editing {
			switch msg.String() {
			case "enter":
				p.commitEdit()
				return p, nil
			case "esc":
				p.editing = false
				p.editBuffer = ""
				p.editLabel = ""
				return p, nil
			case "backspace", "delete":
				if len(p.editBuffer) > 0 {
					p.editBuffer = p.editBuffer[:len(p.editBuffer)-1]
				}
				return p, nil
			}
			if len(msg.Runes) > 0 {
				p.editBuffer += string(msg.Runes)
			}
			return p, nil
		}
		// Universal list grammar first, then pane-specific keys.
		var gram ListGrammar
		if cmd, handled := gram.HandleKey(p, msg); handled {
			_ = cmd
		}
		switch msg.String() {
		case "left", "h":
			p.collapseSelection()
		case "right", "l":
			p.expandSelection()
		case "e":
			p.beginEdit()
		case "p":
			p.promptBackendToggle()
		case "s":
			return p, p.persistManifestCmd()
		case "r":
			p.Refresh()
		}
	}
	return p, nil
}

func (p *SandboxPane) View() string {
	if p.root == nil {
		if p.status == "" {
			return sectionPanel(p.th, "Sandbox Permissions", p.width, p.th.Dim().Render("No manifest loaded"))
		}
		return sectionPanel(p.th, "Sandbox Permissions", p.width, p.th.Dim().Render(p.status))
	}
	widths := splitWidths(p.width, 11, 9)
	left := sectionPanel(p.th, "Sandbox Permissions", widths[0], p.renderTreeLines())
	right := sectionPanel(p.th, "Permission Editor", widths[1], p.renderDetailLines()...)
	footer := p.th.Dim().Render("↑↓ navigate  h/l collapse-expand  space cycle  e edit  s save  p backend  r reload")
	if p.filter != "" {
		footer = p.th.Dim().Render(fmt.Sprintf("filter: %q", p.filter)) + "\n" + footer
	}
	if p.confirmBackend {
		footer = warningText(p.th, fmt.Sprintf("Switch sandbox backend to %s? press enter to confirm or esc to cancel", p.pendingBackend)) + "\n" + footer
	}
	if p.editing {
		footer = warningText(p.th, fmt.Sprintf("Edit %s: %s", p.editLabel, p.editBuffer)) + "\n" + footer
	}
	if p.status != "" {
		footer = p.th.Dim().Render(p.status) + "\n" + footer
	}
	return strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		footer,
	}, "\n\n")
}

func (p *SandboxPane) buildTree() *sandboxNode {
	if p.manifest == nil {
		return nil
	}
	root := &sandboxNode{
		ID:         "sandbox-root",
		Label:      "Sandbox",
		Kind:       sandboxNodeCategory,
		Expandable: true,
		Expanded:   true,
		Selectable: false,
	}
	root.Children = append(root.Children,
		p.buildFileCategory(),
		p.buildCommandCategory(),
		p.buildNetworkCategory(),
		p.buildProviderCategory(),
		p.buildToolCategory(),
	)
	return root
}

func (p *SandboxPane) buildFileCategory() *sandboxNode {
	cat := &sandboxNode{
		ID:         "files",
		Label:      "File Scopes",
		Kind:       sandboxNodeCategory,
		Expandable: true,
		Expanded:   p.expandedState("files", true),
	}
	for i, perm := range p.manifest.Spec.Permissions.FileSystem {
		state := agentspec.AgentPermissionAllow
		if perm.HITLRequired {
			state = agentspec.AgentPermissionAsk
		}
		cat.Children = append(cat.Children, &sandboxNode{
			ID:         fmt.Sprintf("file:%d", i),
			Label:      perm.Path,
			Kind:       sandboxNodeFileScope,
			State:      state,
			Summary:    string(perm.Action),
			Expandable: false,
			Selectable: true,
			fileIndex:  i,
			filePerm:   perm,
		})
	}
	return cat
}

func (p *SandboxPane) buildCommandCategory() *sandboxNode {
	cat := &sandboxNode{
		ID:         "commands",
		Label:      "Commands",
		Kind:       sandboxNodeCategory,
		Expandable: true,
		Expanded:   p.expandedState("commands", true),
	}
	if p.manifest.Spec.Agent != nil {
		bash := p.manifest.Spec.Agent.Bash
		cat.Children = append(cat.Children, &sandboxNode{
			ID:           "command:default",
			Label:        "Default",
			Kind:         sandboxNodeCommandDefault,
			State:        bash.Default,
			Summary:      "shell default policy",
			Selectable:   true,
			defaultBash:  true,
			toolName:     "",
			pattern:      "",
			isAllowList:  false,
			patternIndex: -1,
		})
		for i, pattern := range bash.AllowPatterns {
			cat.Children = append(cat.Children, &sandboxNode{
				ID:           fmt.Sprintf("command:allow:%d", i),
				Label:        pattern,
				Kind:         sandboxNodeCommandPattern,
				State:        agentspec.AgentPermissionAllow,
				Summary:      "allow pattern",
				Selectable:   true,
				patternIndex: i,
				pattern:      pattern,
				isAllowList:  true,
			})
		}
		for i, pattern := range bash.DenyPatterns {
			cat.Children = append(cat.Children, &sandboxNode{
				ID:           fmt.Sprintf("command:deny:%d", i),
				Label:        pattern,
				Kind:         sandboxNodeCommandPattern,
				State:        agentspec.AgentPermissionDeny,
				Summary:      "deny pattern",
				Selectable:   true,
				patternIndex: i,
				pattern:      pattern,
				isAllowList:  false,
			})
		}
	}
	return cat
}

func (p *SandboxPane) buildNetworkCategory() *sandboxNode {
	cat := &sandboxNode{
		ID:         "network",
		Label:      "Network Rules",
		Kind:       sandboxNodeCategory,
		Expandable: true,
		Expanded:   p.expandedState("network", true),
	}
	for i, perm := range p.manifest.Spec.Permissions.Network {
		state := agentspec.AgentPermissionAllow
		if perm.HITLRequired {
			state = agentspec.AgentPermissionAsk
		}
		label := perm.Host
		if label == "" {
			label = fmt.Sprintf("%s/%s", perm.Direction, perm.Protocol)
		}
		cat.Children = append(cat.Children, &sandboxNode{
			ID:           fmt.Sprintf("network:%d", i),
			Label:        label,
			Kind:         sandboxNodeNetworkRule,
			State:        state,
			Summary:      fmt.Sprintf("%s %s:%d", perm.Direction, perm.Protocol, perm.Port),
			Selectable:   true,
			networkIndex: i,
			networkPerm:  perm,
		})
	}
	return cat
}

func (p *SandboxPane) buildProviderCategory() *sandboxNode {
	cat := &sandboxNode{
		ID:         "providers",
		Label:      "Capability Servers",
		Kind:       sandboxNodeCategory,
		Expandable: true,
		Expanded:   p.expandedState("providers", true),
	}
	if p.manifest.Spec.Agent != nil {
		keys := make([]string, 0, len(p.manifest.Spec.Agent.ProviderPolicies))
		for key := range p.manifest.Spec.Agent.ProviderPolicies {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			pol := p.manifest.Spec.Agent.ProviderPolicies[key]
			state := pol.Activate
			if strings.TrimSpace(string(state)) == "" {
				state = agentspec.AgentPermissionAsk
			}
			cat.Children = append(cat.Children, &sandboxNode{
				ID:          "provider:" + key,
				Label:       key,
				Kind:        sandboxNodeProviderPolicy,
				State:       state,
				Summary:     fmt.Sprintf("trust=%s", pol.DefaultTrust),
				Selectable:  true,
				providerID:  key,
				providerPol: pol,
			})
		}
	}
	return cat
}

func (p *SandboxPane) buildToolCategory() *sandboxNode {
	cat := &sandboxNode{
		ID:         "tools",
		Label:      "Tool Execution",
		Kind:       sandboxNodeCategory,
		Expandable: true,
		Expanded:   p.expandedState("tools", true),
	}
	if p.manifest.Spec.Agent != nil {
		keys := make([]string, 0, len(p.manifest.Spec.Agent.ToolExecutionPolicy))
		for key := range p.manifest.Spec.Agent.ToolExecutionPolicy {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			pol := p.manifest.Spec.Agent.ToolExecutionPolicy[key]
			state := pol.Execute
			if strings.TrimSpace(string(state)) == "" {
				state = agentspec.AgentPermissionAsk
			}
			cat.Children = append(cat.Children, &sandboxNode{
				ID:         "tool:" + key,
				Label:      key,
				Kind:       sandboxNodeToolPolicy,
				State:      state,
				Summary:    "execute policy",
				Selectable: true,
				toolName:   key,
				toolPol:    pol,
			})
		}
	}
	return cat
}

func (p *SandboxPane) rebuildVisible() {
	p.visible = p.visibleNodes()
	if p.sel >= len(p.visible) {
		p.sel = max(0, len(p.visible)-1)
	}
	if len(p.visible) > 0 {
		p.selectedID = p.visible[p.sel].node.ID
	}
}

func (p *SandboxPane) matchesNode(node *sandboxNode) bool {
	if p.filter == "" || node == nil {
		return true
	}
	candidate := strings.ToLower(strings.Join([]string{
		node.ID,
		node.Label,
		node.Summary,
		string(node.State),
		node.providerID,
		node.toolName,
		node.pattern,
	}, " "))
	return strings.Contains(candidate, p.filter)
}

func (p *SandboxPane) expandedState(id string, def bool) bool {
	if p == nil {
		return def
	}
	if value, ok := p.expanded[id]; ok {
		return value
	}
	return def
}

func clampIndex(idx, length int) int {
	if length <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= length {
		return length - 1
	}
	return idx
}

func (p *SandboxPane) moveSelection(delta int) {
	if len(p.visible) == 0 {
		return
	}
	p.sel += delta
	p.sel = clampIndex(p.sel, len(p.visible))
	p.selectedID = p.visible[p.sel].node.ID
}

func (p *SandboxPane) collapseSelection() {
	node := p.selectedNode()
	if node == nil {
		return
	}
	if node.Expandable && node.Expanded {
		node.Expanded = false
		p.expanded[node.ID] = false
		return
	}
	if parent := p.parentOf(node.ID); parent != nil {
		p.selectedID = parent.ID
		p.rebuildVisible()
	}
}

func (p *SandboxPane) expandSelection() {
	node := p.selectedNode()
	if node == nil {
		return
	}
	if node.Expandable {
		node.Expanded = true
		p.expanded[node.ID] = true
		p.rebuildVisible()
		return
	}
	if len(node.Children) > 0 {
		node.Expanded = true
		p.expanded[node.ID] = true
		p.rebuildVisible()
	}
}

func (p *SandboxPane) toggleSelectionState() {
	node := p.selectedNode()
	if node == nil || !node.Selectable {
		if node != nil && node.Expandable {
			node.Expanded = !node.Expanded
			p.expanded[node.ID] = node.Expanded
			p.rebuildVisible()
		}
		return
	}
	switch node.Kind {
	case sandboxNodeFileScope, sandboxNodeNetworkRule, sandboxNodeProviderPolicy, sandboxNodeToolPolicy, sandboxNodeCommandDefault, sandboxNodeCommandPattern:
		node.State = nextPermissionLevel(node.State)
	}
}

func (p *SandboxPane) promptBackendToggle() {
	if p.runtime == nil {
		p.status = "runtime unavailable"
		return
	}
	current := strings.ToLower(strings.TrimSpace(p.runtime.SandboxBackend()))
	next := "gvisor"
	if current != "docker" {
		next = "docker"
	}
	p.confirmBackend = true
	p.pendingBackend = next
	p.status = ""
}

func (p *SandboxPane) beginEdit() {
	node := p.selectedNode()
	if node == nil || !node.Selectable {
		return
	}
	switch node.Kind {
	case sandboxNodeFileScope:
		p.editing = true
		p.editLabel = "file path"
		p.editBuffer = node.Label
	case sandboxNodeNetworkRule:
		p.editing = true
		p.editLabel = "network host"
		p.editBuffer = node.Label
	case sandboxNodeCommandPattern:
		p.editing = true
		p.editLabel = "command pattern"
		p.editBuffer = node.Label
	case sandboxNodeProviderPolicy:
		p.editing = true
		p.editLabel = "provider id"
		p.editBuffer = node.Label
	case sandboxNodeToolPolicy:
		p.editing = true
		p.editLabel = "tool name"
		p.editBuffer = node.Label
	default:
		p.status = "node is not editable"
	}
}

func (p *SandboxPane) FocusFilescopes() {
	if p.root == nil {
		return
	}
	if node := p.findNode("files"); node != nil {
		node.Expanded = true
		p.expanded[node.ID] = true
	}
	p.rebuildVisible()
	if !p.selectByID("files") {
		p.sel = 0
	}
}

func (p *SandboxPane) commitEdit() {
	node := p.selectedNode()
	if node == nil {
		return
	}
	value := strings.TrimSpace(p.editBuffer)
	if value == "" {
		p.editing = false
		p.editBuffer = ""
		p.editLabel = ""
		return
	}
	node.Label = value
	switch node.Kind {
	case sandboxNodeFileScope:
		node.filePerm.Path = value
	case sandboxNodeNetworkRule:
		node.networkPerm.Host = value
	case sandboxNodeCommandPattern:
		node.pattern = value
		node.Label = value
	case sandboxNodeProviderPolicy:
		node.providerID = value
		node.Label = value
	case sandboxNodeToolPolicy:
		node.toolName = value
		node.Label = value
	}
	p.editing = false
	p.editBuffer = ""
	p.editLabel = ""
}

func (p *SandboxPane) findNode(id string) *sandboxNode {
	var walk func(node *sandboxNode) *sandboxNode
	walk = func(node *sandboxNode) *sandboxNode {
		if node == nil {
			return nil
		}
		if node.ID == id {
			return node
		}
		for _, child := range node.Children {
			if found := walk(child); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(p.root)
}

func (p *SandboxPane) persistBackendCmd(backend string) tea.Cmd {
	return func() tea.Msg {
		if p.runtime == nil {
			return sandboxPersistedMsg{Err: fmt.Errorf("runtime unavailable")}
		}
		backup, err := p.runtime.SaveSandboxBackend(backend)
		if err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		workspace := p.runtime.SessionInfo().Workspace
		if err := p.runtime.ReloadWorkspace(context.Background(), workspace); err != nil {
			return sandboxPersistedMsg{Workspace: workspace, Backup: backup, Err: err}
		}
		return sandboxPersistedMsg{Workspace: workspace, Backup: backup}
	}
}

func (p *SandboxPane) persistManifestCmd() tea.Cmd {
	return func() tea.Msg {
		if p.runtime == nil {
			return sandboxPersistedMsg{Err: fmt.Errorf("runtime unavailable")}
		}
		if p.manifest == nil {
			return sandboxPersistedMsg{Err: fmt.Errorf("manifest unavailable")}
		}
		clone, err := p.buildSavedManifest()
		if err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		backup, err := p.runtime.SaveSandboxManifest(clone)
		if err != nil {
			return sandboxPersistedMsg{Err: err}
		}
		workspace := p.runtime.SessionInfo().Workspace
		if err := p.runtime.ReloadWorkspace(context.Background(), workspace); err != nil {
			return sandboxPersistedMsg{Workspace: workspace, Backup: backup, Err: err}
		}
		return sandboxPersistedMsg{Workspace: workspace, Backup: backup}
	}
}

func (p *SandboxPane) buildSavedManifest() (*config.AgentManifest, error) {
	clone, err := config.CloneAgentManifest(p.manifest)
	if err != nil {
		return nil, err
	}
	if clone == nil {
		return nil, fmt.Errorf("manifest unavailable")
	}
	if p.root != nil {
		for _, child := range p.root.Children {
			switch child.ID {
			case "files":
				p.applyFileCategory(clone, child)
			case "commands":
				p.applyCommandCategory(clone, child)
			case "network":
				p.applyNetworkCategory(clone, child)
			case "providers":
				p.applyProviderCategory(clone, child)
			case "tools":
				p.applyToolCategory(clone, child)
			}
		}
	}
	return clone, nil
}

func (p *SandboxPane) applyFileCategory(clone *config.AgentManifest, cat *sandboxNode) {
	perms := make([]permissions.FileSystemPermission, 0, len(cat.Children))
	for _, child := range cat.Children {
		if child.State == agentspec.AgentPermissionDeny {
			continue
		}
		perm := child.filePerm
		perm.Path = child.Label
		perm.HITLRequired = child.State == agentspec.AgentPermissionAsk
		perms = append(perms, perm)
	}
	sort.Slice(perms, func(i, j int) bool { return perms[i].Path < perms[j].Path })
	policy := ensureSandboxPolicy(&clone.Spec)
	policy.Permissions.FileSystem = perms
	clone.Spec.Permissions.FileSystem = perms
}

func (p *SandboxPane) applyCommandCategory(clone *config.AgentManifest, cat *sandboxNode) {
	if clone.Spec.Agent == nil {
		clone.Spec.Agent = &agentspec.AgentRuntimeSpec{}
	}
	bash := clone.Spec.Agent.Bash
	bash.AllowPatterns = bash.AllowPatterns[:0]
	bash.DenyPatterns = bash.DenyPatterns[:0]
	for _, child := range cat.Children {
		switch child.Kind {
		case sandboxNodeCommandDefault:
			bash.Default = child.State
		case sandboxNodeCommandPattern:
			switch child.State {
			case agentspec.AgentPermissionAllow:
				bash.AllowPatterns = append(bash.AllowPatterns, child.Label)
			case agentspec.AgentPermissionDeny:
				bash.DenyPatterns = append(bash.DenyPatterns, child.Label)
			default:
				// Ask falls back to the shell default.
			}
		}
	}
	sort.Strings(bash.AllowPatterns)
	sort.Strings(bash.DenyPatterns)
	clone.Spec.Agent.Bash = bash
}

func (p *SandboxPane) applyNetworkCategory(clone *config.AgentManifest, cat *sandboxNode) {
	perms := make([]permissions.NetworkPermission, 0, len(cat.Children))
	for _, child := range cat.Children {
		if child.State == agentspec.AgentPermissionDeny {
			continue
		}
		perm := child.networkPerm
		perm.Host = child.Label
		perm.HITLRequired = child.State == agentspec.AgentPermissionAsk
		perms = append(perms, perm)
	}
	sort.Slice(perms, func(i, j int) bool {
		if perms[i].Direction == perms[j].Direction {
			if perms[i].Host == perms[j].Host {
				return perms[i].Port < perms[j].Port
			}
			return perms[i].Host < perms[j].Host
		}
		return perms[i].Direction < perms[j].Direction
	})
	policy := ensureSandboxPolicy(&clone.Spec)
	policy.Permissions.Network = perms
	clone.Spec.Permissions.Network = perms
}

func ensureSandboxPolicy(spec *config.ManifestSpec) *config.ManifestPolicySpec {
	if spec == nil {
		return nil
	}
	if spec.Policy == nil {
		policy := config.ManifestPolicySpec{
			Permissions: spec.Permissions,
			Resources:   spec.Resources,
			Security:    spec.Security,
			Audit:       spec.Audit,
			Policies:    spec.Policies,
			Defaults:    spec.Defaults,
		}
		spec.Policy = &policy
	}
	return spec.Policy
}

func (p *SandboxPane) applyProviderCategory(clone *config.AgentManifest, cat *sandboxNode) {
	if clone.Spec.Agent == nil {
		clone.Spec.Agent = &agentspec.AgentRuntimeSpec{}
	}
	if clone.Spec.Agent.ProviderPolicies == nil {
		clone.Spec.Agent.ProviderPolicies = make(map[string]agentspec.ProviderPolicy)
	}
	for _, child := range cat.Children {
		pol := child.providerPol
		if child.State == agentspec.AgentPermissionDeny {
			delete(clone.Spec.Agent.ProviderPolicies, child.Label)
			continue
		}
		pol.Activate = child.State
		clone.Spec.Agent.ProviderPolicies[child.Label] = pol
	}
}

func (p *SandboxPane) applyToolCategory(clone *config.AgentManifest, cat *sandboxNode) {
	if clone.Spec.Agent == nil {
		clone.Spec.Agent = &agentspec.AgentRuntimeSpec{}
	}
	if clone.Spec.Agent.ToolExecutionPolicy == nil {
		clone.Spec.Agent.ToolExecutionPolicy = make(map[string]agentspec.ToolPolicy)
	}
	for _, child := range cat.Children {
		if child.State == agentspec.AgentPermissionDeny {
			delete(clone.Spec.Agent.ToolExecutionPolicy, child.Label)
			continue
		}
		pol := child.toolPol
		pol.Execute = child.State
		clone.Spec.Agent.ToolExecutionPolicy[child.Label] = pol
	}
}

func (p *SandboxPane) renderTreeLines() string {
	nodes := p.visibleNodes()
	if len(nodes) == 0 {
		return p.th.Dim().Render("(no permissions)")
	}
	lines := make([]string, 0, len(nodes))
	for _, item := range nodes {
		line := p.renderNodeLine(item.node, item.depth)
		lines = append(lines, line)
	}
	return sectionList(p.th, lines, p.sel, p.height-8)
}

func (p *SandboxPane) renderNodeLine(node *sandboxNode, depth int) string {
	indent := strings.Repeat("  ", depth)
	prefix := "•"
	if node.Expandable {
		if node.Expanded {
			prefix = "▾"
		} else {
			prefix = "▸"
		}
	}
	label := node.Label
	if node.Selectable {
		label = fmt.Sprintf("%s [%s] %s", prefix, stateLabel(node.State), node.Label)
	} else {
		label = fmt.Sprintf("%s %s", prefix, node.Label)
	}
	if node.Summary != "" && node.Selectable {
		label += "  " + p.th.Dim().Render(node.Summary)
	}
	return indent + label
}

func (p *SandboxPane) renderDetailLines() []string {
	node := p.selectedNode()
	if node == nil {
		return []string{p.th.Dim().Render("No selection")}
	}
	lines := []string{
		p.th.Header().Render(node.Label),
		p.th.Dim().Render("kind: " + nodeKindLabel(node.Kind)),
		p.th.Dim().Render("state: " + stateLabel(node.State)),
	}
	switch node.Kind {
	case sandboxNodeFileScope:
		lines = append(lines,
			"action: "+string(node.filePerm.Action),
			"path: "+node.Label,
			"hitl: "+fmt.Sprint(node.State == agentspec.AgentPermissionAsk),
		)
	case sandboxNodeCommandDefault:
		lines = append(lines, "default shell policy")
	case sandboxNodeCommandPattern:
		lines = append(lines, "pattern: "+node.Label)
	case sandboxNodeNetworkRule:
		lines = append(lines,
			fmt.Sprintf("direction: %s", node.networkPerm.Direction),
			fmt.Sprintf("protocol: %s", node.networkPerm.Protocol),
			fmt.Sprintf("host: %s", node.Label),
			fmt.Sprintf("port: %d", node.networkPerm.Port),
		)
	case sandboxNodeProviderPolicy:
		lines = append(lines,
			"provider: "+node.Label,
			"trust: "+string(node.providerPol.DefaultTrust),
			"credential sharing: "+fmt.Sprint(node.providerPol.AllowCredentialSharing),
		)
	case sandboxNodeToolPolicy:
		lines = append(lines, "tool: "+node.Label)
	}
	lines = append(lines, "")
	lines = append(lines, p.th.Dim().Render("space cycles allow -> ask -> deny"))
	lines = append(lines, p.th.Dim().Render("e edits the primary label field"))
	lines = append(lines, p.th.Dim().Render("s saves and reloads the runtime"))
	lines = append(lines, p.th.Dim().Render("p switches sandbox backend"))
	return lines
}

func (p *SandboxPane) selectedNode() *sandboxNode {
	if len(p.visible) == 0 {
		return nil
	}
	p.sel = clampIndex(p.sel, len(p.visible))
	return p.visible[p.sel].node
}

func (p *SandboxPane) selectByID(id string) bool {
	for i, item := range p.visible {
		if item.node.ID == id {
			p.sel = i
			p.selectedID = id
			return true
		}
	}
	return false
}

func (p *SandboxPane) parentOf(id string) *sandboxNode {
	var walk func(node *sandboxNode) *sandboxNode
	walk = func(node *sandboxNode) *sandboxNode {
		if node == nil {
			return nil
		}
		for _, child := range node.Children {
			if child.ID == id {
				return node
			}
			if parent := walk(child); parent != nil {
				return parent
			}
		}
		return nil
	}
	if p.root == nil {
		return nil
	}
	if p.root.ID == id {
		return nil
	}
	return walk(p.root)
}

func (p *SandboxPane) visibleNodes() []sandboxVisibleNode {
	if p.root == nil {
		return nil
	}
	var out []sandboxVisibleNode
	var subtreeMatches func(node *sandboxNode) bool
	subtreeMatches = func(node *sandboxNode) bool {
		if node == nil {
			return false
		}
		if p.matchesNode(node) {
			return true
		}
		for _, child := range node.Children {
			if subtreeMatches(child) {
				return true
			}
		}
		return false
	}
	var walk func(node *sandboxNode, depth int)
	walk = func(node *sandboxNode, depth int) {
		if node == nil {
			return
		}
		include := depth == 0 || node.Selectable || node.Expandable
		if p.filter == "" {
			if include {
				out = append(out, sandboxVisibleNode{node: node, depth: depth})
			}
			if node.Expandable && !node.Expanded {
				return
			}
			for _, child := range node.Children {
				walk(child, depth+1)
			}
			return
		}
		if include && subtreeMatches(node) {
			out = append(out, sandboxVisibleNode{node: node, depth: depth})
		}
		for _, child := range node.Children {
			if subtreeMatches(child) {
				walk(child, depth+1)
			}
		}
	}
	for _, child := range p.root.Children {
		if p.filter == "" {
			walk(child, 0)
			continue
		}
		if subtreeMatches(child) {
			walk(child, 0)
		}
	}
	if len(out) == 0 {
		return nil
	}
	if p.sel >= len(out) {
		p.sel = len(out) - 1
	}
	return out
}

func nextPermissionLevel(current agentspec.AgentPermissionLevel) agentspec.AgentPermissionLevel {
	switch current {
	case agentspec.AgentPermissionAllow:
		return agentspec.AgentPermissionAsk
	case agentspec.AgentPermissionAsk:
		return agentspec.AgentPermissionDeny
	default:
		return agentspec.AgentPermissionAllow
	}
}

func stateLabel(level agentspec.AgentPermissionLevel) string {
	switch level {
	case agentspec.AgentPermissionAllow:
		return "allow"
	case agentspec.AgentPermissionAsk:
		return "ask"
	case agentspec.AgentPermissionDeny:
		return "deny"
	default:
		return "unset"
	}
}

func nodeKindLabel(kind sandboxNodeKind) string {
	switch kind {
	case sandboxNodeCategory:
		return "category"
	case sandboxNodeFileScope:
		return "file scope"
	case sandboxNodeCommandDefault:
		return "command default"
	case sandboxNodeCommandPattern:
		return "command pattern"
	case sandboxNodeNetworkRule:
		return "network rule"
	case sandboxNodeProviderPolicy:
		return "provider policy"
	case sandboxNodeToolPolicy:
		return "tool policy"
	default:
		return "unknown"
	}
}

func warningText(th *theme.Theme, value string) string {
	return th.Warning().Bold(true).Render(value)
}

// ── ListEditor implementation ──────────────────────────────────────────────

func (p *SandboxPane) Actions() []Action {
	if p == nil {
		return nil
	}
	return []Action{
		{Label: "navigate", Key: "↑↓"},
		{Label: "toggle", Key: "space"},
		{Label: "edit", Key: "e"},
		{Label: "save", Key: "s"},
		{Label: "reload", Key: "r"},
	}
}
func (p *SandboxPane) ItemCount() int {
	if p == nil || p.root == nil {
		return 0
	}
	return len(p.visibleNodes())
}
func (p *SandboxPane) Selected() int { return p.sel }
func (p *SandboxPane) Move(delta int) int {
	p.moveSelection(delta)
	return p.sel
}
func (p *SandboxPane) OnActivate() tea.Cmd {
	if p == nil {
		return nil
	}
	return p.persistManifestCmd()
}
func (p *SandboxPane) OnToggle() tea.Cmd {
	if p == nil {
		return nil
	}
	p.toggleSelectionState()
	return nil
}
func (p *SandboxPane) OnNew() tea.Cmd    { return nil }
func (p *SandboxPane) OnDelete() tea.Cmd { return nil }
func (p *SandboxPane) OnFilter(query string) {
	if p != nil {
		p.SetFilter(query)
	}
}

// ── Theme setter ───────────────────────────────────────────────────────────

// SetTheme sets the active semantic style source.
func (p *SandboxPane) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
