package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keybindingRuntime interface {
	SessionInfo() SessionInfo
}

type keybindingTarget struct {
	Action      string
	Scope       string
	Source      string
	Description string
	DefaultKeys []string
	Binding     *key.Binding
}

type keybindingConflict struct {
	Target keybindingTarget
	Other  keybindingTarget
	Key    string
}

type KeybindingPane struct {
	runtime keybindingRuntime

	targets []keybindingTarget
	rows    []keybindingTarget
	sel     int
	filter  string

	waitingForKey bool
	editLabel     string
	editBuffer    string
	confirm       *keybindingConflict
	status        string

	width  int
	height int
}

func NewKeybindingPane(rt keybindingRuntime) *KeybindingPane {
	p := &KeybindingPane{runtime: rt}
	p.loadPersistedBindings()
	p.Refresh()
	return p
}

func (p *KeybindingPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *KeybindingPane) Refresh() {
	p.targets = buildKeybindingTargets()
	p.applyTargetState()
	p.rebuildRows()
}

func (p *KeybindingPane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
	p.rebuildRows()
}

func (p *KeybindingPane) Update(msg tea.Msg) (*KeybindingPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.confirm != nil {
			switch msg.String() {
			case "y", "enter":
				p.applyPendingConflict()
				return p, p.persistCmd()
			case "n", "esc":
				p.confirm = nil
				p.status = "reassignment canceled"
				return p, nil
			}
			return p, nil
		}
		if p.waitingForKey {
			switch msg.String() {
			case "esc":
				p.waitingForKey = false
				p.editLabel = ""
				p.editBuffer = ""
				return p, nil
			default:
				keyStr := strings.TrimSpace(msg.String())
				if keyStr == "" {
					return p, nil
				}
				if conflict, ok := p.detectConflict(keyStr); ok {
					p.confirm = &keybindingConflict{
						Target: p.currentTarget(),
						Other:  conflict,
						Key:    keyStr,
					}
					p.waitingForKey = false
					return p, nil
				}
				p.applyBinding(keyStr)
				return p, p.persistCmd()
			}
		}
		switch msg.String() {
		case "up", "k":
			if p.sel > 0 {
				p.sel--
			}
		case "down", "j":
			if p.sel < len(p.rows)-1 {
				p.sel++
			}
		case "e":
			p.waitingForKey = true
			p.editLabel = p.currentTarget().Action
			p.editBuffer = ""
			p.status = "press the new keybinding"
		case "r":
			p.resetSelected()
			return p, p.persistCmd()
		case "R":
			p.resetAll()
			return p, p.persistCmd()
		}
	}
	return p, nil
}

func (p *KeybindingPane) View() string {
	rows := p.filteredRows()
	if len(rows) == 0 {
		rows = []keybindingTarget{{Action: "(none)", Description: "No bindings match the current filter"}}
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		keyStr := strings.Join(row.bindingKeys(), ", ")
		if keyStr == "" {
			keyStr = "(unbound)"
		}
		line := fmt.Sprintf("%-24s %-18s %-10s %s", row.Action, keyStr, row.Scope, row.Source)
		if p.rowMatchesSelected(row) {
			line = panelItemActiveStyle.Render(line)
		}
		lines = append(lines, line)
	}
	footer := dimStyle.Render("up/down navigate  e rebind  r reset  R reset all")
	if p.filter != "" {
		footer = dimStyle.Render(fmt.Sprintf("filter: %q", p.filter)) + "\n" + footer
	}
	if p.waitingForKey {
		footer = warningText(fmt.Sprintf("Rebind %s: press the new key", p.editLabel)) + "\n" + footer
	}
	if p.confirm != nil {
		footer = warningText(fmt.Sprintf("Key %s is bound to %q. Reassign and clear old binding? [y/n]", p.confirm.Key, p.confirm.Other.Action)) + "\n" + footer
	}
	if p.status != "" {
		footer = dimStyle.Render(p.status) + "\n" + footer
	}
	return strings.Join([]string{
		sectionPanel("Keybindings", p.width, sectionList(lines, p.selectedRowIndex(), p.height-8)),
		footer,
	}, "\n\n")
}

func (p *KeybindingPane) loadPersistedBindings() {
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	path := filepath.Join(manifest.New(workspace).ConfigRoot(), "keybindings.yaml")
	cfg, err := cfgload.LoadRuntimeKeybindingConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		p.status = fmt.Sprintf("parse failed: %v", err)
		return
	}
	lookup := buildKeybindingLookup()
	for _, binding := range cfg.Bindings {
		target, ok := lookup[binding.Action]
		if !ok || target.Binding == nil || len(binding.Keys) == 0 {
			continue
		}
		target.Binding.SetKeys(binding.Keys...)
	}
}

func (p *KeybindingPane) applyTargetState() {
	// The live targets are the source of truth. This method only rebuilds the
	// in-memory list after a reload or persistence action.
	p.targets = buildKeybindingTargets()
}

func (p *KeybindingPane) rebuildRows() {
	p.rows = make([]keybindingTarget, 0, len(p.targets))
	for _, target := range p.targets {
		if p.filter == "" {
			p.rows = append(p.rows, target)
			continue
		}
		if strings.Contains(strings.ToLower(target.Action), p.filter) ||
			strings.Contains(strings.ToLower(target.Description), p.filter) ||
			strings.Contains(strings.ToLower(target.Scope), p.filter) ||
			strings.Contains(strings.ToLower(strings.Join(target.bindingKeys(), ", ")), p.filter) {
			p.rows = append(p.rows, target)
		}
	}
	if len(p.rows) == 0 {
		p.sel = 0
		return
	}
	p.sel = clampIndex(p.sel, len(p.rows))
}

func (p *KeybindingPane) filteredRows() []keybindingTarget {
	if len(p.rows) == 0 {
		p.rebuildRows()
	}
	return append([]keybindingTarget(nil), p.rows...)
}

func (p *KeybindingPane) selectedRowIndex() int {
	if len(p.rows) == 0 {
		return 0
	}
	return clampIndex(p.sel, len(p.rows))
}

func (p *KeybindingPane) currentTarget() keybindingTarget {
	if len(p.rows) == 0 {
		return keybindingTarget{}
	}
	return p.rows[p.selectedRowIndex()]
}

func (p *KeybindingPane) rowMatchesSelected(row keybindingTarget) bool {
	if len(p.rows) == 0 {
		return false
	}
	return p.rows[p.selectedRowIndex()].Action == row.Action
}

func (p *KeybindingPane) detectConflict(keyStr string) (keybindingTarget, bool) {
	for _, target := range p.targets {
		if target.Action == p.currentTarget().Action {
			continue
		}
		for _, candidate := range target.bindingKeys() {
			if strings.EqualFold(candidate, keyStr) {
				return target, true
			}
		}
	}
	return keybindingTarget{}, false
}

func (p *KeybindingPane) applyBinding(keyStr string) {
	target := p.currentTarget()
	if target.Binding == nil {
		return
	}
	target.Binding.SetKeys(keyStr)
	p.status = fmt.Sprintf("%s → %s", target.Action, keyStr)
	p.waitingForKey = false
	p.editLabel = ""
	p.editBuffer = ""
	p.Refresh()
}

func (p *KeybindingPane) applyPendingConflict() {
	if p.confirm == nil {
		return
	}
	if p.confirm.Other.Binding != nil {
		p.confirm.Other.Binding.Unbind()
	}
	if p.confirm.Target.Binding != nil {
		p.confirm.Target.Binding.SetKeys(p.confirm.Key)
	}
	p.status = fmt.Sprintf("reassigned %s to %s", p.confirm.Target.Action, p.confirm.Key)
	p.confirm = nil
	p.Refresh()
}

func (p *KeybindingPane) resetSelected() {
	target := p.currentTarget()
	if target.Binding == nil {
		return
	}
	target.Binding.SetKeys(target.DefaultKeys...)
	p.status = fmt.Sprintf("reset %s", target.Action)
	p.Refresh()
}

func (p *KeybindingPane) resetAll() {
	for _, target := range p.targets {
		if target.Binding == nil {
			continue
		}
		target.Binding.SetKeys(target.DefaultKeys...)
	}
	p.status = "reset all keybindings"
	p.Refresh()
}

func (p *KeybindingPane) persistCmd() tea.Cmd {
	cfg := cfgload.RuntimeKeybindingConfig{Bindings: make([]cfgload.RuntimeKeybindingEntry, 0, len(p.targets))}
	workspace := ""
	if p.runtime != nil {
		workspace = p.runtime.SessionInfo().Workspace
	}
	for _, target := range p.targets {
		cfg.Bindings = append(cfg.Bindings, cfgload.RuntimeKeybindingEntry{
			Action:      target.Action,
			Keys:        append([]string(nil), target.bindingKeys()...),
			Scope:       target.Scope,
			Source:      target.Source,
			Description: target.Description,
			DefaultKeys: append([]string(nil), target.DefaultKeys...),
		})
	}
	return func() tea.Msg {
		if workspace == "" {
			return chatSystemMsg{Text: "keybindings save failed: workspace unavailable"}
		}
		path := filepath.Join(manifest.New(workspace).ConfigRoot(), "keybindings.yaml")
		backup, err := cfgload.SaveRuntimeKeybindingConfigWithBackup(path, cfg)
		if err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("keybindings save failed: %v", err)}
		}
		return chatSystemMsg{Text: fmt.Sprintf("keybindings saved (backup: %s)", backup)}
	}
}

func buildKeybindingTargets() []keybindingTarget {
	return []keybindingTarget{
		{Action: "switch surface 1", Scope: "shell", Source: "built-in", Description: "Switch surface 1", DefaultKeys: []string{"1"}, Binding: &GlobalKeys.Tab1},
		{Action: "switch surface 2", Scope: "shell", Source: "built-in", Description: "Switch surface 2", DefaultKeys: []string{"2"}, Binding: &GlobalKeys.Tab2},
		{Action: "switch surface 3", Scope: "shell", Source: "built-in", Description: "Switch surface 3", DefaultKeys: []string{"3"}, Binding: &GlobalKeys.Tab3},
		{Action: "switch surface 4", Scope: "shell", Source: "built-in", Description: "Switch surface 4", DefaultKeys: []string{"4"}, Binding: &GlobalKeys.Tab4},
		{Action: "switch surface 5", Scope: "shell", Source: "built-in", Description: "Switch surface 5", DefaultKeys: []string{"5"}, Binding: &GlobalKeys.Tab5},
		{Action: "switch surface 6", Scope: "shell", Source: "built-in", Description: "Switch surface 6", DefaultKeys: []string{"6"}, Binding: &GlobalKeys.Tab6},
		{Action: "open agent picker", Scope: "shell", Source: "built-in", Description: "Open agent picker", DefaultKeys: []string{"ctrl+a"}, Binding: &GlobalKeys.AgentPicker},
		{Action: "open help", Scope: "shell", Source: "built-in", Description: "Open help overlay", DefaultKeys: []string{"f1"}, Binding: &GlobalKeys.Help},
		{Action: "move focus to region 1", Scope: "shell", Source: "built-in", Description: "Focus region 1", DefaultKeys: []string{"tab", "ctrl+down"}, Binding: &GlobalKeys.FocusRegion1},
		{Action: "quit", Scope: "shell", Source: "built-in", Description: "Quit application", DefaultKeys: []string{"ctrl+c", "ctrl+d"}, Binding: &GlobalKeys.Quit},
		{Action: "search mode", Scope: "shell", Source: "built-in", Description: "Toggle search mode", DefaultKeys: []string{"ctrl+f"}, Binding: &GlobalKeys.SearchMode},
		{Action: "toggle sidebar", Scope: "shell", Source: "built-in", Description: "Toggle context sidebar", DefaultKeys: []string{"ctrl+]"}, Binding: &GlobalKeys.SidebarToggle},
		{Action: "undo", Scope: "shell", Source: "built-in", Description: "Undo last change", DefaultKeys: []string{"ctrl+z"}, Binding: &GlobalKeys.Undo},
		{Action: "redo", Scope: "shell", Source: "built-in", Description: "Redo last change", DefaultKeys: []string{"ctrl+y"}, Binding: &GlobalKeys.Redo},
		{Action: "scroll up", Scope: "shell", Source: "built-in", Description: "Scroll chat upward", DefaultKeys: []string{"ctrl+u"}, Binding: &GlobalKeys.ScrollUp},
		{Action: "scroll down", Scope: "shell", Source: "built-in", Description: "Scroll chat downward", DefaultKeys: []string{"pagedown"}, Binding: &GlobalKeys.ScrollDown},
		{Action: "page up", Scope: "shell", Source: "built-in", Description: "Page up", DefaultKeys: []string{"pageup"}, Binding: &GlobalKeys.PageUp},
		{Action: "file picker", Scope: "shell", Source: "built-in", Description: "Open file picker", DefaultKeys: []string{"@"}, Binding: &GlobalKeys.FilePicker},
		{Action: "compact", Scope: "shell", Source: "built-in", Description: "Compact chat history", DefaultKeys: []string{"ctrl+k"}, Binding: &GlobalKeys.Compact},
		{Action: "stop service", Scope: "shell", Source: "built-in", Description: "Stop focused service", DefaultKeys: []string{"s"}, Binding: &GlobalKeys.ServiceStop},
		{Action: "restart service", Scope: "shell", Source: "built-in", Description: "Restart focused service", DefaultKeys: []string{"r"}, Binding: &GlobalKeys.ServiceRestart},
		{Action: "restart all services", Scope: "shell", Source: "built-in", Description: "Restart all services", DefaultKeys: []string{"R"}, Binding: &GlobalKeys.ServiceRestartAll},
	}
}

func buildKeybindingLookup() map[string]*keybindingTarget {
	targets := buildKeybindingTargets()
	lookup := make(map[string]*keybindingTarget, len(targets))
	for i := range targets {
		lookup[targets[i].Action] = &targets[i]
	}
	return lookup
}

func (t keybindingTarget) bindingKeys() []string {
	if t.Binding == nil {
		return nil
	}
	return t.Binding.Keys()
}
