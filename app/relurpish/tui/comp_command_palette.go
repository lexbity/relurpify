package tui

import (
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
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
	label string
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewCommandPalette returns an empty palette.
func NewCommandPalette() *CommandPalette {
	return &CommandPalette{th: theme.Default()}
}

// Sync mirrors the current palette state from InputBar.
func (p *CommandPalette) Sync(open bool, items []commandItem, sel int, width int, label string) {
	p.open = open && len(items) > 0
	p.items = append(p.items[:0], items...)
	p.sel = sel
	p.width = width
	p.label = label
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
	p.label = ""
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
	header := "Commands"
	if strings.TrimSpace(p.label) != "" {
		header = p.label
	}
	lines := []string{p.th.Subhead().Render(header)}
	for i, item := range p.items {
		label := item.Usage
		if item.Description != "" {
			label += "  " + p.th.Dim().Render(item.Description)
		}
		if i == p.sel {
			label = p.th.Active().Render(label)
		} else {
			label = p.th.Body().Render(label)
		}
		lines = append(lines, label)
	}
	content := strings.Join(lines, "\n")
	width := p.width
	if width < 1 {
		width = 1
	}
	return p.th.Panel().Width(width).Render(content)
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

// SetTheme sets the active semantic style source.
func (p *CommandPalette) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
