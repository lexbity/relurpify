package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ──────────────────────────────────────────────────────────────
// Styles specific to euclo interaction rendering
// ──────────────────────────────────────────────────────────────

var (
	eucloFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	eucloPhaseStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	eucloPhaseActiveStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)

	eucloPhaseCompletedStyle = lipgloss.NewStyle().
					Foreground(colorSuccess)

	eucloPhasePendingStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	eucloFindingCriticalStyle = lipgloss.NewStyle().
					Foreground(colorError).
					Bold(true)

	eucloFindingWarningStyle = lipgloss.NewStyle().
					Foreground(colorWarning)

	eucloFindingInfoStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	eucloNotifStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)
)

// ──────────────────────────────────────────────────────────────
// Frame rendering
// ──────────────────────────────────────────────────────────────

// RenderInteractionFrame converts an InteractionFrame into a TUI Message.
func RenderInteractionFrame(frame interaction.InteractionFrame) Message {
	msg := Message{
		ID:        generateID(),
		Role:      RoleAgent,
		Timestamp: frame.Metadata.Timestamp,
		Content: MessageContent{
			Expanded: map[string]bool{},
		},
	}

	switch frame.Kind {
	case interaction.FrameProposal:
		// Check if it's a ContextProposalContent
		if content, ok := frame.Content.(interaction.ContextProposalContent); ok {
			msg.Content.Text = renderContextProposal(content)
		} else {
			msg.Content.Text = renderProposal(frame)
		}
	case interaction.FrameQuestion:
		msg.Content.Text = renderQuestion(frame)
	case interaction.FrameCandidates:
		msg.Content.Text = renderCandidates(frame)
	case interaction.FrameComparison:
		msg.Content.Text = renderComparison(frame)
	case interaction.FrameDraft:
		msg.Content.Text = renderDraft(frame)
	case interaction.FrameResult:
		msg.Content.Text = renderFrameResult(frame)
	case interaction.FrameStatus:
		msg.Content.Text = renderStatus(frame)
	case interaction.FrameSummary:
		msg.Content.Text = renderSummary(frame)
	case interaction.FrameTransition:
		msg.Content.Text = renderTransition(frame)
	case interaction.FrameHelp:
		msg.Content.Text = renderHelp(frame)
	default:
		msg.Content.Text = fmt.Sprintf("[%s] %s/%s", frame.Kind, frame.Mode, frame.Phase)
	}

	return msg
}

// RenderPhaseProgress produces a breadcrumb trail like:
//
//	[code] intent → ●propose → execute → verify → present
func RenderPhaseProgress(mode string, phaseIndex, phaseCount int, phaseLabels []interaction.PhaseInfo) string {
	if len(phaseLabels) == 0 {
		return fmt.Sprintf("[%s] phase %d/%d", mode, phaseIndex+1, phaseCount)
	}
	var b strings.Builder
	b.WriteString(eucloPhaseStyle.Render(fmt.Sprintf("[%s] ", mode)))
	for i, p := range phaseLabels {
		if i > 0 {
			b.WriteString(dimStyle.Render(" → "))
		}
		label := p.Label
		if i < phaseIndex {
			b.WriteString(eucloPhaseCompletedStyle.Render(label))
		} else if i == phaseIndex {
			b.WriteString(eucloPhaseActiveStyle.Render("●" + label))
		} else {
			b.WriteString(eucloPhasePendingStyle.Render(label))
		}
	}
	return b.String()
}

// ──────────────────────────────────────────────────────────────
// Per-kind renderers
// ──────────────────────────────────────────────────────────────

func renderProposal(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.ProposalContent)
	if !ok {
		return "[proposal]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Proposal") + "\n")
	if content.Interpretation != "" {
		b.WriteString(content.Interpretation + "\n")
	}
	if len(content.Scope) > 0 {
		b.WriteString(dimStyle.Render("Scope: ") + strings.Join(content.Scope, ", ") + "\n")
	}
	if content.Approach != "" {
		b.WriteString(dimStyle.Render("Approach: ") + content.Approach + "\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderQuestion(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.QuestionContent)
	if !ok {
		return "[question]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Question") + "\n")
	b.WriteString(content.Question + "\n")
	if content.Description != "" {
		b.WriteString(dimStyle.Render(content.Description) + "\n")
	}
	for i, opt := range content.Options {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			headerStyle.Render(fmt.Sprintf("[%d]", i+1)),
			opt.Label,
		))
		if opt.Description != "" {
			b.WriteString(dimStyle.Render("      "+opt.Description) + "\n")
		}
	}
	return eucloFrameStyle.Render(b.String())
}

func renderCandidates(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.CandidatesContent)
	if !ok {
		return "[candidates]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Candidates") + "\n")
	for i, c := range content.Candidates {
		label := fmt.Sprintf("[%d] %s", i+1, c.ID)
		b.WriteString(headerStyle.Render(label) + "\n")
		b.WriteString("  " + c.Summary + "\n")
		for k, v := range c.Properties {
			b.WriteString(fmt.Sprintf("  %s %s\n", dimStyle.Render(k+":"), v))
		}
	}
	if content.RecommendedID != "" {
		b.WriteString(dimStyle.Render("\nRecommended: ") + content.RecommendedID + "\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderComparison(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.ComparisonContent)
	if !ok {
		return "[comparison]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Comparison") + "\n")
	if len(content.Dimensions) > 0 {
		// Header row: dimensions as column headers.
		b.WriteString(fmt.Sprintf("  %-12s", ""))
		for _, dim := range content.Dimensions {
			b.WriteString(fmt.Sprintf("%-15s", dimStyle.Render(dim)))
		}
		b.WriteString("\n")
		// Matrix rows.
		for i, row := range content.Matrix {
			label := fmt.Sprintf("Option %d", i+1)
			b.WriteString(fmt.Sprintf("  %-12s", headerStyle.Render(label)))
			for _, cell := range row {
				b.WriteString(fmt.Sprintf("%-15s", cell))
			}
			b.WriteString("\n")
		}
	}
	return eucloFrameStyle.Render(b.String())
}

func renderDraft(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.DraftContent)
	if !ok {
		return "[draft]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Draft") + "\n")
	if content.Kind != "" {
		b.WriteString(dimStyle.Render("("+content.Kind+")") + "\n")
	}
	for i, item := range content.Items {
		marker := fmt.Sprintf("%d.", i+1)
		if item.Editable {
			marker = "~" + marker
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", dimStyle.Render(marker), item.Content))
	}
	return eucloFrameStyle.Render(b.String())
}

func renderFrameResult(frame interaction.InteractionFrame) string {
	// Handle both ResultContent and FindingsContent.
	switch content := frame.Content.(type) {
	case interaction.ResultContent:
		return renderResultContent(content)
	case interaction.FindingsContent:
		return renderFindingsContent(content)
	default:
		return eucloFrameStyle.Render("[result]")
	}
}

func renderResultContent(content interaction.ResultContent) string {
	var b strings.Builder
	statusLabel := content.Status
	switch content.Status {
	case "passed", "completed":
		statusLabel = completedStyle.Render("✓ " + content.Status)
	case "failed":
		statusLabel = diffRemoveStyle.Render("✗ " + content.Status)
	case "partial":
		statusLabel = inProgressStyle.Render("◐ " + content.Status)
	}
	b.WriteString(sectionHeaderStyle.Render("Result") + " " + statusLabel + "\n")
	for _, ev := range content.Evidence {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			dimStyle.Render(ev.Kind+":"),
			ev.Detail,
		))
	}
	return eucloFrameStyle.Render(b.String())
}

func renderFindingsContent(content interaction.FindingsContent) string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Findings") + "\n")
	for _, f := range content.Critical {
		b.WriteString(eucloFindingCriticalStyle.Render("  CRITICAL "))
		if f.Location != "" {
			b.WriteString(filePathStyle.Render(f.Location) + " ")
		}
		b.WriteString(f.Description + "\n")
	}
	for _, f := range content.Warning {
		b.WriteString(eucloFindingWarningStyle.Render("  WARNING  "))
		if f.Location != "" {
			b.WriteString(filePathStyle.Render(f.Location) + " ")
		}
		b.WriteString(f.Description + "\n")
	}
	for _, f := range content.Info {
		b.WriteString(eucloFindingInfoStyle.Render("  INFO     "))
		if f.Location != "" {
			b.WriteString(filePathStyle.Render(f.Location) + " ")
		}
		b.WriteString(f.Description + "\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderStatus(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.StatusContent)
	if !ok {
		return "[status]"
	}
	return dimStyle.Render("⟳ " + content.Message)
}

func renderSummary(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.SummaryContent)
	if !ok {
		return "[summary]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Summary") + "\n")
	if content.Description != "" {
		b.WriteString(content.Description + "\n")
	}
	if len(content.Artifacts) > 0 {
		b.WriteString(dimStyle.Render("  Artifacts: ") + strings.Join(content.Artifacts, ", ") + "\n")
	}
	if len(content.Changes) > 0 {
		b.WriteString(dimStyle.Render("  Changes: ") + strings.Join(content.Changes, ", ") + "\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderTransition(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.TransitionContent)
	if !ok {
		return "[transition]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Mode Transition") + "\n")
	b.WriteString(fmt.Sprintf("  %s → %s\n",
		eucloPhaseStyle.Render(content.FromMode),
		eucloPhaseActiveStyle.Render(content.ToMode),
	))
	if content.Reason != "" {
		b.WriteString(dimStyle.Render("  "+content.Reason) + "\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderHelp(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.HelpContent)
	if !ok {
		return "[help]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Help") + " ")
	b.WriteString(eucloPhaseStyle.Render(content.Mode) + "\n\n")

	if len(content.PhaseMap) > 0 {
		b.WriteString(dimStyle.Render("Phases:") + "\n")
		for _, p := range content.PhaseMap {
			marker := "  "
			if p.Current {
				marker = "● "
			}
			label := p.Label
			if p.Current {
				label = eucloPhaseActiveStyle.Render(label)
			}
			b.WriteString(marker + label + "\n")
		}
		b.WriteString("\n")
	}

	if len(content.AvailableActions) > 0 {
		b.WriteString(dimStyle.Render("Actions:") + "\n")
		for _, a := range content.AvailableActions {
			b.WriteString(fmt.Sprintf("  %s  %s\n",
				headerStyle.Render(a.Phrase),
				dimStyle.Render(a.Description),
			))
		}
		b.WriteString("\n")
	}

	if len(content.AvailableTransitions) > 0 {
		b.WriteString(dimStyle.Render("Transitions:") + "\n")
		for _, t := range content.AvailableTransitions {
			b.WriteString(fmt.Sprintf("  \"%s\" → %s\n",
				headerStyle.Render(t.Phrase),
				t.TargetMode,
			))
		}
	}

	return eucloFrameStyle.Render(b.String())
}

// ──────────────────────────────────────────────────────────────
// Action slot rendering for notification bar
// ──────────────────────────────────────────────────────────────

func renderContextProposal(content interaction.ContextProposalContent) string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Context Proposal") + "\n")
	
	if len(content.AnchoredFiles) > 0 {
		b.WriteString(dimStyle.Render("Anchored Files:") + "\n")
		for _, file := range content.AnchoredFiles {
			b.WriteString(fmt.Sprintf("  %s (%s) - %s\n", 
				filePathStyle.Render(file.Path),
				file.Source,
				file.Summary,
			))
		}
	}
	
	if len(content.ExpandedFiles) > 0 {
		b.WriteString(dimStyle.Render("Expanded Files:") + "\n")
		for _, file := range content.ExpandedFiles {
			b.WriteString(fmt.Sprintf("  %s (%s) - %s\n",
				filePathStyle.Render(file.Path),
				file.Source,
				file.Summary,
			))
		}
	}
	
	if len(content.KnowledgeItems) > 0 {
		b.WriteString(dimStyle.Render("Knowledge Items:") + "\n")
		for _, item := range content.KnowledgeItems {
			b.WriteString(fmt.Sprintf("  [%s] %s - %s\n",
				item.Kind,
				item.Title,
				item.Summary,
			))
		}
	}
	
	// Add pipeline trace info
	trace := content.PipelineTrace
	b.WriteString(dimStyle.Render("Pipeline Trace:") + "\n")
	b.WriteString(fmt.Sprintf("  Anchors: %d extracted, %d confirmed\n", 
		trace.AnchorsExtracted, trace.AnchorsConfirmed))
	b.WriteString(fmt.Sprintf("  Stage1: %d code, %d archaeo\n",
		trace.Stage1CodeResults, trace.Stage1ArchaeoResults))
	if trace.HypotheticalGenerated {
		b.WriteString(fmt.Sprintf("  Hypothetical: %d tokens\n", trace.HypotheticalTokens))
	}
	b.WriteString(fmt.Sprintf("  Stage3: %d archaeo\n", trace.Stage3ArchaeoResults))
	if trace.FallbackUsed {
		b.WriteString(fmt.Sprintf("  Fallback: %s\n", trace.FallbackReason))
	}
	
	return eucloFrameStyle.Render(b.String())
}

// RenderActionSlots formats action slots for the notification bar hint.
func RenderActionSlots(actions []interaction.ActionSlot) string {
	if len(actions) == 0 {
		return ""
	}
	var parts []string
	for i, a := range actions {
		key := a.Shortcut
		if key == "" {
			// Use number key for first 9 actions, fallback to action ID.
			if i < 9 {
				key = fmt.Sprintf("%d", i+1)
			} else {
				key = a.ID
			}
		}
		label := a.Label
		if a.Default {
			label += "*"
		}
		parts = append(parts, fmt.Sprintf("[%s] %s", key, label))
	}
	return dimStyle.Render("  " + strings.Join(parts, "  "))
}
