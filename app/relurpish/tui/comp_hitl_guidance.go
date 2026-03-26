package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GuidanceTriggerKind distinguishes the source that opened the guidance panel.
type GuidanceTriggerKind string

const (
	// GuidanceTriggerAmbiguity is opened when the agent needs clarification
	// before proceeding.
	GuidanceTriggerAmbiguity GuidanceTriggerKind = "ambiguity"
	// GuidanceTriggerDeferred is opened when a deferred comment is surfaced
	// for the current scope.
	GuidanceTriggerDeferred GuidanceTriggerKind = "deferred"
	// GuidanceTriggerLearning is opened when the agent proposes an annotation
	// for user confirmation.
	GuidanceTriggerLearning GuidanceTriggerKind = "learning"
	// GuidanceTriggerPermission is the existing HITL permission trigger (kept
	// for routing completeness; visual rendering stays in the notification bar).
	GuidanceTriggerPermission GuidanceTriggerKind = "permission"
)

// Messages emitted by GuidancePanel.

// GuidancePanelSubmitMsg is emitted when the user submits a response (enter).
// The panel closes after this.
type GuidancePanelSubmitMsg struct {
	RequestID string
	Response  string
}

// GuidancePanelAnnotateMsg is emitted when the user presses [a] to save the
// response as a pattern annotation. The panel stays open.
type GuidancePanelAnnotateMsg struct {
	RequestID string
	Body      string
	IntentType string
}

// GuidancePanelDeferMsg is emitted when the user presses [d] to defer.
// The guidance is accumulated as an engineering observation; panel closes.
type GuidancePanelDeferMsg struct {
	RequestID string
}

// GuidancePanelJumpExploreMsg is emitted when the user presses [v] to jump to
// the explore subtab at the relevant pattern. Panel stays open.
type GuidancePanelJumpExploreMsg struct {
	PatternID string
}

// intentTypes is the ordered list of intent type labels cycled with [tab]
// when the panel is in annotate mode.
var intentTypes = []string{"intentional", "aspirational", "constraint", "observation"}

// GuidancePanel is an in-layout overlay that appears above the input bar when
// the agent requires user guidance. It handles ambiguity questions, deferred
// comment surfacing, and learning annotations.
//
// Since lipgloss.NewLayer is not available in the current version, the panel
// renders as an additional element inserted between the main pane and input bar.
// The main pane height is reduced by GuidancePanel.Height() while it is open.
type GuidancePanel struct {
	open      bool
	kind      GuidanceTriggerKind
	requestID string
	title     string
	body      string

	// relatedComments surfaces deferred/open-question comments alongside the
	// ambiguity question so both can be addressed in one interaction.
	relatedComments []CommentRef

	// confidenceDelta is populated when addressing the request would change a
	// plan step's confidence, e.g. "0.61 → ~0.83".
	confidenceDelta string

	// patternID is used by GuidancePanelJumpExploreMsg when [v] is pressed.
	patternID string

	// annotateMode is true while [a] has been pressed; the intent type is
	// cycled with tab.
	annotateMode bool
	intentSel    int

	input textinput.Model
	width int
}

// newGuidancePanel creates a closed panel with a focused input field.
func newGuidancePanel() GuidancePanel {
	ti := textinput.New()
	ti.Placeholder = "type response, enter to submit"
	return GuidancePanel{input: ti}
}

// Open opens the panel with the given trigger information.
func (p *GuidancePanel) Open(
	kind GuidanceTriggerKind,
	requestID, title, body string,
	comments []CommentRef,
	confidenceDelta, patternID string,
) {
	p.open = true
	p.kind = kind
	p.requestID = requestID
	p.title = title
	p.body = body
	p.relatedComments = comments
	p.confidenceDelta = confidenceDelta
	p.patternID = patternID
	p.annotateMode = false
	p.intentSel = 0
	p.input.SetValue("")
	p.input.Focus()
}

// Close resets the panel to its closed state.
func (p *GuidancePanel) Close() {
	p.open = false
	p.annotateMode = false
	p.input.SetValue("")
	p.input.Blur()
}

// IsOpen returns true when the panel is visible.
func (p *GuidancePanel) IsOpen() bool { return p.open }

// RequestID returns the guidance request ID currently displayed.
func (p *GuidancePanel) RequestID() string { return p.requestID }

// SetWidth updates the panel width (called on resize).
func (p *GuidancePanel) SetWidth(w int) {
	p.width = w
	inner := w - 4 // account for border + padding
	if inner < 10 {
		inner = 10
	}
	p.input.Width = inner
}

// Height returns the number of rows the panel occupies in the layout.
// Used by the layout engine to reduce main pane height.
func (p *GuidancePanel) Height() int {
	if !p.open {
		return 0
	}
	lines := 3 // border (2) + title + body
	if p.body != "" {
		lines += strings.Count(p.body, "\n")
	}
	lines += len(p.relatedComments)
	if p.confidenceDelta != "" {
		lines++
	}
	lines++ // input row
	if lines < 6 {
		lines = 6
	}
	if lines > 12 {
		lines = 12
	}
	return lines
}

// Update handles key events when the panel is open. Returns the panel and any
// message command to emit.
func (p *GuidancePanel) Update(msg tea.Msg) (GuidancePanel, tea.Cmd) {
	if !p.open {
		return *p, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			response := strings.TrimSpace(p.input.Value())
			rid := p.requestID
			p.Close()
			return *p, func() tea.Msg {
				return GuidancePanelSubmitMsg{RequestID: rid, Response: response}
			}

		case "a":
			if !p.annotateMode {
				// Switch input prompt to annotation mode; panel stays open.
				p.annotateMode = true
				p.input.Placeholder = "annotation text, enter to save"
				p.input.Focus()
				return *p, nil
			}
			// In annotate mode [a] acts as submit annotation.
			body := strings.TrimSpace(p.input.Value())
			rid := p.requestID
			intentType := intentTypes[p.intentSel%len(intentTypes)]
			p.input.SetValue("")
			p.annotateMode = false
			p.input.Placeholder = "type response, enter to submit"
			return *p, func() tea.Msg {
				return GuidancePanelAnnotateMsg{RequestID: rid, Body: body, IntentType: intentType}
			}

		case "tab":
			if p.annotateMode {
				p.intentSel = (p.intentSel + 1) % len(intentTypes)
				return *p, nil
			}

		case "v":
			pid := p.patternID
			if pid != "" {
				return *p, func() tea.Msg {
					return GuidancePanelJumpExploreMsg{PatternID: pid}
				}
			}

		case "d":
			rid := p.requestID
			p.Close()
			return *p, func() tea.Msg {
				return GuidancePanelDeferMsg{RequestID: rid}
			}

		case "esc":
			p.Close()
			return *p, nil
		}
	}
	// Pass all other messages to the textinput.
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return *p, cmd
}

// View renders the guidance panel as a bordered box. Returns empty string when closed.
func (p *GuidancePanel) View() string {
	if !p.open {
		return ""
	}

	var b strings.Builder

	// Kind badge + title.
	kindBadge := guidanceKindStyle(p.kind).Render(string(p.kind))
	b.WriteString(kindBadge + "  " + headerStyle.Render(p.title) + "\n")

	// Body text.
	if p.body != "" {
		b.WriteString(p.body + "\n")
	}

	// Confidence delta (when relevant).
	if p.confidenceDelta != "" {
		b.WriteString(dimStyle.Render("confidence: ") + inProgressStyle.Render(p.confidenceDelta) + "\n")
	}

	// Related deferred comments.
	for _, c := range p.relatedComments {
		b.WriteString(dimStyle.Render("  ⚑ ") +
			filePathStyle.Render(c.PatternTitle) +
			dimStyle.Render(" ["+c.IntentType+"]") + "\n")
		if c.Body != "" {
			b.WriteString(dimStyle.Render("    "+truncate(c.Body, 60)) + "\n")
		}
	}

	// Annotate mode intent selector.
	if p.annotateMode {
		intentLabel := intentTypes[p.intentSel%len(intentTypes)]
		b.WriteString(inProgressStyle.Render("  intent: ") + intentLabel + dimStyle.Render("  [tab] cycle") + "\n")
	}

	// Input row.
	b.WriteString(p.input.View() + "\n")

	// Key hint.
	hints := "[enter] submit  [d] defer  [esc] dismiss"
	if p.patternID != "" {
		hints += "  [v] explore"
	}
	hints += "  [a] annotate"
	b.WriteString(dimStyle.Render(hints))

	inner := b.String()
	w := p.width
	if w == 0 {
		w = 80
	}
	return guidancePanelStyle.Width(w - 2).Render(inner)
}

// guidanceKindStyle returns a per-kind badge style.
func guidanceKindStyle(k GuidanceTriggerKind) lipgloss.Style {
	switch k {
	case GuidanceTriggerAmbiguity:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("202")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)
	case GuidanceTriggerDeferred:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("18")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)
	case GuidanceTriggerLearning:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)
	default:
		return dimStyle
	}
}

// truncate cuts s to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
