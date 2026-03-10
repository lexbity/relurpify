package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type HelpOverlay struct {
	width  int
	height int
}

func (h *HelpOverlay) SetSize(width, height int) {
	h.width = width
	h.height = height
}

func (h HelpOverlay) View(base string) string {
	if h.width == 0 || h.height == 0 {
		return base
	}
	boxWidth := min(max(h.width-8, 36), 72)
	content := strings.Join([]string{
		"Help",
		"",
		"Navigation",
		"  1-6              switch tabs",
		"  tab / shift+tab  next / previous tab",
		"  [ / ]            switch pane sub-tabs",
		"",
		"Input",
		"  /                filter active pane",
		"  :follow          toggle Events follow mode",
		"  :issue SUBJ SCOPE  issue token on Identity > Tokens",
		"  :revoke TOKEN      revoke token on Identity > Tokens",
		"  :enable RULE       enable rule on Security > Policies",
		"  :disable RULE      disable rule on Security > Policies",
		"  :restart CHANNEL   restart adapter on Channels > Adapters",
		"  ? or esc         close this overlay",
		"",
		"Notifications",
		"  a / x            approve / reject pairing",
		"  d                dismiss current notification",
		"",
		"General",
		"  r                refresh now",
		"  q                quit",
	}, "\n")
	box := helpOverlayStyle.Width(boxWidth).Render(content)
	return lipgloss.Place(h.width, h.height, lipgloss.Center, lipgloss.Center, box)
}
