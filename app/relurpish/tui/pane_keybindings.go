package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	tea "github.com/charmbracelet/bubbletea"
)

type bindingProposal struct {
	keys        *KeyMap
	description string
	conflicts   []string
}
type keybindingsSavedMsg struct {
	Keys   *KeyMap
	Status string
	Err    error
}

// KeybindingPane stages a host-local map and activates it only after a
// successful save (or an explicitly labeled session/demo-only change).
type KeybindingPane struct {
	keys          *KeyMap
	workspace     string
	demo          bool
	rows          []keybindingTarget
	sel           int
	filter        string
	waitingForKey bool
	proposal      *bindingProposal
	working       bool
	status        string
	width, height int
}

func NewKeybindingPane(keys *KeyMap, workspace string, demo bool) *KeybindingPane {
	p := &KeybindingPane{keys: keys, workspace: workspace, demo: demo}
	p.load()
	p.Refresh()
	return p
}
func (p *KeybindingPane) SetSize(width, height int) { p.width, p.height = width, height }
func (p *KeybindingPane) OwnsEscape() bool          { return p.waitingForKey || p.proposal != nil || p.working }
func (p *KeybindingPane) SetFilter(filter string)   { p.filter = strings.ToLower(filter); p.Refresh() }
func (p *KeybindingPane) Refresh() {
	p.rows = nil
	for _, target := range buildKeybindingTargets(p.keys) {
		if p.filter == "" || strings.Contains(strings.ToLower(target.Action+" "+target.Description), p.filter) {
			p.rows = append(p.rows, target)
		}
	}
	p.sel = min(p.sel, max(0, len(p.rows)-1))
}
func (p *KeybindingPane) currentTarget() keybindingTarget {
	if len(p.rows) == 0 {
		return keybindingTarget{}
	}
	return p.rows[p.sel]
}

func (p *KeybindingPane) Update(message tea.Msg) (*KeybindingPane, tea.Cmd) {
	switch msg := message.(type) {
	case keybindingsSavedMsg:
		p.working = false
		if msg.Err != nil {
			p.status = "Save failed; previous keys remain active: " + msg.Err.Error()
			return p, nil
		}
		*p.keys = *msg.Keys
		p.proposal = nil
		p.waitingForKey = false
		p.status = msg.Status
		p.Refresh()
		return p, nil
	case tea.KeyMsg:
		if p.working {
			return p, nil
		}
		if p.proposal != nil {
			switch msg.String() {
			case "esc", "n":
				p.proposal = nil
				p.status = "Rebinding canceled"
			case "enter", "y":
				return p, p.persist()
			}
			return p, nil
		}
		if p.waitingForKey {
			if msg.String() == "esc" {
				p.waitingForKey = false
				return p, nil
			}
			if msg.Paste || !validHostChord(msg.String()) {
				p.status = "Choose a non-text chord; Escape and Ctrl+Q are reserved"
				return p, nil
			}
			p.propose(p.currentTarget().Action, []string{msg.String()})
			p.waitingForKey = false
			return p, nil
		}
		switch msg.String() {
		case "up":
			p.sel = max(0, p.sel-1)
		case "down":
			p.sel = min(max(0, len(p.rows)-1), p.sel+1)
		case "e":
			if len(p.rows) > 0 {
				p.waitingForKey = true
				p.status = ""
			}
		case "r":
			if len(p.rows) > 0 {
				p.propose(p.currentTarget().Action, p.currentTarget().DefaultKeys)
			}
		case "R":
			p.proposal = &bindingProposal{keys: NewKeyMap(), description: "Reset all shortcuts in this workspace"}
		}
	}
	return p, nil
}

func (p *KeybindingPane) propose(action string, chords []string) {
	keys := p.keys.clone()
	proposal := &bindingProposal{keys: keys, description: action + " → " + strings.Join(chords, ", ")}
	for _, target := range buildKeybindingTargets(keys) {
		if target.Action == action {
			target.Binding.SetKeys(chords...)
			continue
		}
		remaining := slices.Clone(target.bindingKeys())
		for _, chord := range chords {
			if slices.Contains(remaining, chord) {
				proposal.conflicts = append(proposal.conflicts, target.Action+" loses "+chord)
				remaining = slices.DeleteFunc(remaining, func(value string) bool { return value == chord })
			}
		}
		target.Binding.SetKeys(remaining...)
	}
	p.proposal = proposal
}

func (p *KeybindingPane) persist() tea.Cmd {
	proposal, workspace, demo := p.proposal, p.workspace, p.demo
	p.working = true
	entries := make([]config.RuntimeKeybindingEntry, 0, 10)
	for _, target := range buildKeybindingTargets(proposal.keys) {
		entries = append(entries, config.RuntimeKeybindingEntry{Action: target.Action, Keys: slices.Clone(target.bindingKeys()), Scope: target.Scope, Description: target.Description, DefaultKeys: slices.Clone(target.DefaultKeys)})
	}
	return func() tea.Msg {
		if demo {
			return keybindingsSavedMsg{Keys: proposal.keys, Status: "DEMO: shortcuts changed in this session's memory only"}
		}
		if workspace == "" {
			return keybindingsSavedMsg{Keys: proposal.keys, Status: "Shortcuts active in this session only; workspace storage is unavailable"}
		}
		backup, err := config.SaveRuntimeKeybindingConfigWithBackup(config.New(workspace).RuntimeKeybindingsFile(), config.RuntimeKeybindingConfig{Bindings: entries})
		return keybindingsSavedMsg{Keys: proposal.keys, Err: err, Status: "Shortcuts saved and active; backup: " + backup}
	}
}

func (p *KeybindingPane) load() {
	if p.demo || p.workspace == "" {
		return
	}
	stored, err := config.LoadRuntimeKeybindingConfig(config.New(p.workspace).RuntimeKeybindingsFile())
	if err != nil {
		if !os.IsNotExist(err) {
			p.status = "Bindings could not be loaded: " + err.Error()
		}
		return
	}
	candidate := p.keys.clone()
	targets := buildKeybindingTargets(candidate)
	seen := make(map[string]string)
	seenActions := make(map[string]bool)
	for _, entry := range stored.Bindings {
		if seenActions[entry.Action] {
			p.status = "Bindings repeat an action; defaults retained"
			return
		}
		seenActions[entry.Action] = true
		index := slices.IndexFunc(targets, func(target keybindingTarget) bool { return target.Action == entry.Action })
		if index < 0 {
			p.status = "Bindings contain an unknown action: " + entry.Action + "; defaults retained"
			return
		}
		for _, chord := range entry.Keys {
			if !validHostChord(chord) {
				p.status = "Bindings contain a reserved/empty chord; defaults retained"
				return
			}
			if other, ok := seen[chord]; ok && other != entry.Action {
				p.status = "Bindings conflict: " + chord + "; defaults retained"
				return
			}
			seen[chord] = entry.Action
		}
		targets[index].Binding.SetKeys(entry.Keys...)
	}
	// Check the complete resulting map, including defaults omitted from the file.
	clear(seen)
	for _, target := range targets {
		for _, chord := range target.bindingKeys() {
			if _, ok := seen[chord]; ok {
				p.status = "Bindings conflict with defaults; defaults retained"
				return
			}
			seen[chord] = target.Action
		}
	}
	*p.keys = *candidate
}

func (p *KeybindingPane) View() string {
	lines := []string{"Settings / Keybindings", ""}
	if p.waitingForKey {
		target := p.currentTarget()
		lines = append(lines, "Rebind "+target.Action, "Current: "+strings.Join(target.bindingKeys(), ", "), "", "Press the new key chord.", "Escape cancels · Ctrl+Q always quits")
	} else if p.proposal != nil {
		lines = append(lines, "Review shortcut change", "", p.proposal.description)
		lines = append(lines, p.proposal.conflicts...)
		lines = append(lines, "", "Enter applies this change · Escape cancels")
	} else {
		budget := max(1, p.height-7)
		start := max(0, p.sel-budget+1)
		for i := start; i < min(len(p.rows), start+budget); i++ {
			target := p.rows[i]
			marker := " "
			if i == p.sel {
				marker = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %-22s %s", marker, target.Action, strings.Join(target.bindingKeys(), ", ")))
		}
		lines = append(lines, "", "↑/↓ select · e capture · r reset one · R reset all")
	}
	if p.working {
		lines = append(lines, "", "Saving; previous bindings remain active…")
	}
	if p.status != "" {
		lines = append(lines, "", terminalText(p.status))
	}
	return boundedScreen(strings.Join(lines, "\n"), max(1, p.width), max(1, p.height))
}
