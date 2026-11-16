package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
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
	header := "commands"
	if strings.TrimSpace(p.label) != "" {
		header = p.label + " + global commands"
	}
	lines := []string{p.th.Subhead().Render(header)}
	for i, item := range p.items {
		scope := item.Scope
		if scope == "" {
			scope = "global"
		}
		label := item.Usage
		if item.Description != "" {
			label += "  " + p.th.Dim().Render(item.Description)
		}
		pad := p.width - ansi.StringWidth(label) - ansi.StringWidth(scope) - 2
		if pad > 0 {
			label += strings.Repeat(" ", pad)
		} else {
			label += "  "
		}
		label += p.th.Dim().Render(scope)
		if i == p.sel {
			label = p.th.Active().Render(label)
		} else {
			label = p.th.Body().Render(label)
		}
		lines = append(lines, label)
	}
	// Keyframe 02: the palette is a sparse list above the bar — no border
	// box, one leading blank row, per-row scope labels.
	content := "\n" + strings.Join(lines, "\n")
	width := p.width
	if width < 1 {
		width = 1
	}
	return p.th.Bar().Width(width).Render(content)
}

// SetTheme sets the active semantic style source.
func (p *CommandPalette) SetTheme(th *theme.Theme) {
	if th != nil {
		p.th = th
	}
}
