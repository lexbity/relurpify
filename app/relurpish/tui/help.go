package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpOverlay renders a centered help box over the current view.
type HelpOverlay struct {
	width, height int
}

// SetSize updates the terminal dimensions used for centering.
func (h *HelpOverlay) SetSize(w, ht int) {
	h.width = w
	h.height = ht
}

// View renders the help overlay centered over base. When dimensions are
// unknown it just returns the base view unchanged.
func (h HelpOverlay) View(base string) string {
	if h.width == 0 || h.height == 0 {
		return base
	}
	boxWidth := h.width - 4
	if boxWidth > 70 {
		boxWidth = 70
	}
	box := helpOverlayStyle.Width(boxWidth).Render(h.content())
	return lipgloss.Place(h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceForeground(lipgloss.Color("238")),
		lipgloss.WithWhitespaceChars("·"),
	)
}

func (h HelpOverlay) content() string {
	cmds := listCommandsSorted()
	var b strings.Builder
	b.WriteString("Help\n\n")
	b.WriteString("Commands\n")
	for _, cmd := range cmds {
		b.WriteString(fmt.Sprintf("  %-22s %s\n", cmd.Usage, cmd.Description))
	}
	b.WriteString("\nNavigation\n")
	b.WriteString("  1-5                   switch tabs\n")
	b.WriteString("  tab / shift+tab       next / prev tab (input empty)\n")
	b.WriteString("  ctrl+t                toggle title bar\n")
	b.WriteString("  ctrl+f                search messages\n")
	b.WriteString("  ctrl+c / ctrl+d       quit\n")
	b.WriteString("\nSidebar\n")
	b.WriteString("  ctrl+]                toggle sidebar (chat context / archaeo plan)\n")
	b.WriteString("  tab                   focus sidebar / main area (archaeo plan)\n")
	b.WriteString("\nBlob Operations  (archaeo plan sidebar)\n")
	b.WriteString("  enter                 add focused blob to plan\n")
	b.WriteString("  x / d                remove blob from plan\n")
	b.WriteString("  e                    expand / collapse blob detail\n")
	b.WriteString("\nExplore Operations  (archaeo explore subtab)\n")
	b.WriteString("  enter                 stage / unstage focused blob\n")
	b.WriteString("  /promote-all          stage all proposed blobs\n")
	b.WriteString("\nService Operations  (session services subtab)\n")
	b.WriteString("  s                     stop focused service\n")
	b.WriteString("  r                     restart focused service\n")
	b.WriteString("  R                     restart all services (with confirmation)\n")
	b.WriteString("\n" + dimStyle.Render("Press ? or esc to close"))
	return b.String()
}
