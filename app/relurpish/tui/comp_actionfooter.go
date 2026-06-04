package tui

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

// ActionFooter renders a toggleable legend of the active keyboard grammar.
// It is shown/hidden via a global toggle key (e.g. ctrl+g).
type ActionFooter struct {
	visible bool
	actions []Action
	th      *theme.Theme
}

// NewActionFooter creates a footer with the default grammar.
func NewActionFooter() *ActionFooter {
	var g ListGrammar
	return &ActionFooter{
		visible: true,
		actions: g.DefaultActions(),
	}
}

// SetTheme sets the theme for styled rendering.
func (f *ActionFooter) SetTheme(th *theme.Theme) {
	if th != nil {
		f.th = th
	}
}

// ToggleVisibility shows or hides the footer.
func (f *ActionFooter) ToggleVisibility() {
	if f != nil {
		f.visible = !f.visible
	}
}

// SetActions updates the action legend for the currently focused pane.
func (f *ActionFooter) SetActions(actions []Action) {
	if f != nil {
		f.actions = actions
	}
}

// Visible reports whether the footer is currently shown.
func (f *ActionFooter) Visible() bool {
	if f == nil {
		return false
	}
	return f.visible
}

// View renders the footer as a single line.
func (f *ActionFooter) View() string {
	if f == nil || !f.visible || len(f.actions) == 0 {
		return ""
	}
	if f.th == nil {
		f.th = theme.Default()
	}
	parts := make([]string, 0, len(f.actions))
	for _, a := range f.actions {
		parts = append(parts, fmt.Sprintf("%s %s", a.Key, a.Label))
	}
	return f.th.Dim().Render(strings.Join(parts, "  "))
}
