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
	if boxWidth < 1 {
		boxWidth = 1
	}
	box := helpOverlayStyle.Width(boxWidth).Render(h.content())
	return lipgloss.Place(h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceForeground(colorDim),
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
	b.WriteString("  1-6                   switch tabs\n")
	b.WriteString("  tab / ctrl+down       focus region 1\n")
	b.WriteString("  esc                   return focus to input\n")
	b.WriteString("  f1                    help\n")
	b.WriteString("  ctrl+f                search messages\n")
	b.WriteString("  ctrl+c / ctrl+d       quit\n")
	b.WriteString("\nInput Modes\n")
	b.WriteString("  >                     agent prompt\n")
	b.WriteString("  /                     slash action autocomplete\n")
	b.WriteString("  :                     shell command autocomplete\n")
	b.WriteString("  ?                     search mode\n")
	b.WriteString("\nSidebar\n")
	b.WriteString("  ctrl+]                toggle chat context sidebar\n")
	b.WriteString("\nService Operations  (session services subtab)\n")
	b.WriteString("  s                     stop focused service\n")
	b.WriteString("  r                     restart focused service\n")
	b.WriteString("  R                     restart all services (with confirmation)\n")
	b.WriteString("\nSandbox Tab\n")
	b.WriteString("  space                 cycle allow / ask / deny\n")
	b.WriteString("  e                     edit the selected rule\n")
	b.WriteString("  s / enter             save manifest with backup\n")
	b.WriteString("  p                     switch sandbox backend\n")
	b.WriteString("\nSecurityGuard Tab\n")
	b.WriteString("  e / n / d / t         edit, add, delete, or test rules\n")
	b.WriteString("  space                 toggle ingestion guardrails\n")
	b.WriteString("\nAI Provider Tab\n")
	b.WriteString("  tab                   switch list/form focus\n")
	b.WriteString("  left/right            toggle local vs remote infrastructure\n")
	b.WriteString("  t                     test provider latency\n")
	b.WriteString("  s                     save providers.yaml with backup\n")
	b.WriteString("\nGuest Surfaces\n")
	b.WriteString("  tab                   switch tabs within the active surface\n")
	b.WriteString("  commands              depend on the active guest surface\n")
	b.WriteString("\nKeybindings Tab\n")
	b.WriteString("  e                     capture a new binding\n")
	b.WriteString("  r / R                 reset selected / all bindings\n")
	b.WriteString("\n" + dimStyle.Render("Press f1 or esc to close"))
	return b.String()
}
