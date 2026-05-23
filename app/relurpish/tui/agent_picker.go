package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// AgentPicker renders and manages the active-agent chooser overlay.
type AgentPicker struct {
	open  bool
	items []string
	sel   int
}

// NewAgentPicker returns a closed picker.
func NewAgentPicker() *AgentPicker {
	return &AgentPicker{}
}

// Open loads a new item list and selects the current agent if present.
func (p *AgentPicker) Open(items []string, current string) {
	if p == nil {
		return
	}
	p.items = append(p.items[:0], items...)
	p.sel = 0
	p.open = len(p.items) > 0
	if !p.open {
		return
	}
	current = normalizeSurfaceKey(current)
	for i, item := range p.items {
		if normalizeSurfaceKey(item) == current {
			p.sel = i
			return
		}
	}
}

// Close hides the picker.
func (p *AgentPicker) Close() {
	if p == nil {
		return
	}
	p.open = false
	p.sel = 0
}

// IsOpen reports whether the picker is visible.
func (p *AgentPicker) IsOpen() bool {
	return p != nil && p.open && len(p.items) > 0
}

// Render draws the picker panel.
func (p *AgentPicker) Render(width, height int) string {
	_ = height
	if !p.IsOpen() {
		return ""
	}
	if width < 1 {
		width = 1
	}
	title := fmt.Sprintf("Agents (%d)", len(p.items))
	visible := sectionList(p.items, p.sel, 7)
	return panelStyle.Width(width).Render(strings.Join([]string{
		panelHeaderStyle.Render(title),
		visible,
	}, "\n"))
}

// HandleKey updates the selection. When a choice is confirmed, the selected
// agent name is returned.
func (p *AgentPicker) HandleKey(msg tea.KeyMsg) (string, bool) {
	if !p.IsOpen() {
		return "", false
	}
	if len(p.items) == 0 {
		return "", false
	}
	switch msg.String() {
	case "esc":
		p.Close()
		return "", true
	case "enter":
		selected := p.items[p.sel]
		p.Close()
		return selected, true
	case "up", "k":
		if p.sel > 0 {
			p.sel--
		}
		return "", true
	case "down", "j":
		if p.sel < len(p.items)-1 {
			p.sel++
		}
		return "", true
	case "tab":
		p.sel = (p.sel + 1) % len(p.items)
		return "", true
	case "shift+tab":
		p.sel = (p.sel - 1 + len(p.items)) % len(p.items)
		return "", true
	case "home":
		p.sel = 0
		return "", true
	case "end":
		p.sel = len(p.items) - 1
		return "", true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		target := strings.ToLower(string(msg.Runes[0]))
		for i, item := range p.items {
			if strings.HasPrefix(strings.ToLower(item), target) {
				p.sel = i
				return "", true
			}
		}
	}
	return "", false
}
