package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// CommandPalette renders context-aware slash command suggestions above the
// input bar. Selection and filtering are driven by InputBar, while RootModel
// owns placement and rendering as an overlay.
type CommandPalette struct {
	open  bool
	items []commandItem
	sel   int
	width int
}

// NewCommandPalette returns an empty palette.
func NewCommandPalette() *CommandPalette {
	return &CommandPalette{}
}

// Sync mirrors the current palette state from InputBar.
func (p *CommandPalette) Sync(open bool, items []commandItem, sel int, width int) {
	p.open = open && len(items) > 0
	p.items = append(p.items[:0], items...)
	p.sel = sel
	p.width = width
	if p.sel < 0 {
		p.sel = 0
	}
	if p.sel >= len(p.items) {
		p.sel = len(p.items) - 1
	}
}

// Close hides the palette.
func (p *CommandPalette) Close() {
	p.open = false
	p.items = nil
	p.sel = 0
}

// IsOpen reports whether the palette should be rendered.
func (p *CommandPalette) IsOpen() bool {
	return p != nil && p.open && len(p.items) > 0
}

// Height reports the rendered row count when open.
func (p *CommandPalette) Height() int {
	if !p.IsOpen() {
		return 0
	}
	return len(p.items) + 2
}

// View renders the palette panel.
func (p *CommandPalette) View() string {
	if !p.IsOpen() {
		return ""
	}
	lines := []string{panelHeaderStyle.Render("Commands")}
	for i, item := range p.items {
		label := item.Usage
		if item.Description != "" {
			label += "  " + dimStyle.Render(item.Description)
		}
		if i == p.sel {
			label = panelItemActiveStyle.Render(label)
		} else {
			label = panelItemStyle.Render(label)
		}
		lines = append(lines, label)
	}
	content := strings.Join(lines, "\n")
	return panelStyle.Width(max(24, p.width)).Render(content)
}

// statusBarText renders a compact, context-sensitive key hint line.
func (m RootModel) statusBarText() string {
	switch {
	case m.hitlPanel.IsOpen():
		return "guidance: enter submit | a annotate | d defer | v explore | esc close"
	case m.notifBar != nil && m.notifBar.Active():
		return "notification: y approve | n deny | d dismiss"
	case m.cmdPalette != nil && m.cmdPalette.IsOpen():
		return "commands: ↑↓ select | tab complete | enter run | esc cancel"
	case m.inputBar != nil && m.inputBar.IsFilePickerActive():
		return "files: ↑↓ select | enter choose | esc cancel"
	case m.activeTab == TabSession:
		return "session: [ ] subtabs | / commands | ctrl+f filter"
	case m.activeTab == TabConfig:
		return "config: / commands | tab cycle sections | r refresh"
	default:
		return "chat: / commands | @ files | ctrl+f search | ? help"
	}
}

func (m RootModel) renderStatusBar() string {
	return statusStyle.Width(m.width).Render(m.statusBarText())
}

func overlayPanelView(parts ...string) string {
	var visible []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			visible = append(visible, part)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, visible...)
}
