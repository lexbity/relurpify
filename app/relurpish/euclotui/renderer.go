package euclotui

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// RenderInteractionFrame converts an InteractionFrame into a tui.Message.
func RenderInteractionFrame(frame interaction.InteractionFrame) tui.Message {
	msg := tui.Message{
		ID:        tui.GenerateID(),
		Role:      tui.RoleAgent,
		Timestamp: frame.Metadata.Timestamp,
		Content: tui.MessageContent{
			Expanded: map[string]bool{},
		},
	}

	switch frame.Type {
	case interaction.FrameCandidates:
		msg.Content.Text = renderCandidates(frame)
	case interaction.FrameComparison:
		msg.Content.Text = renderComparison(frame)
	case interaction.FrameDraft:
		msg.Content.Text = renderDraft(frame)
	case interaction.FrameResultType:
		msg.Content.Text = renderFrameResult(frame)
	case interaction.FrameStatus:
		msg.Content.Text = renderStatus(frame)
	case interaction.FrameSummary:
		msg.Content.Text = renderSummary(frame)
	case interaction.FrameTransition:
		msg.Content.Text = renderTransition(frame)
	case interaction.FrameSessionList:
		msg.Content.Text = renderSessionList(frame)
	case interaction.FrameSessionListEmpty:
		msg.Content.Text = renderSessionListEmpty(frame)
	case interaction.FrameSessionResuming:
		msg.Content.Text = renderSessionResuming(frame)
	case interaction.FrameSessionResumeError:
		msg.Content.Text = renderSessionResumeError(frame)
	case interaction.FrameScopeConfirmation:
		msg.Content.Text = renderSelectionFrame(frame, "Scope Confirmation")
	case interaction.FrameIntentClarification:
		msg.Content.Text = renderSelectionFrame(frame, "Clarification")
	case interaction.FrameCandidateSelection:
		msg.Content.Text = renderSelectionFrame(frame, "Candidate Selection")
	case interaction.FrameThoughtRecipeSelection:
		msg.Content.Text = renderSelectionFrame(frame, "Thoughtrecipe Selection")
	case interaction.FrameCapabilitySelection:
		msg.Content.Text = renderSelectionFrame(frame, "Capability Selection")
	case interaction.FrameHITLApproval:
		msg.Content.Text = renderSelectionFrame(frame, "Approval Required")
	case interaction.FrameSessionResume:
		msg.Content.Text = renderSelectionFrame(frame, "Session Resume")
	case interaction.FrameBackgroundJobStatus:
		msg.Content.Text = renderSelectionFrame(frame, "Background Job Status")
	case interaction.FrameExecutionSummary:
		msg.Content.Text = renderSelectionFrame(frame, "Execution Summary")
	case interaction.FrameVerificationEvidence:
		msg.Content.Text = renderSelectionFrame(frame, "Verification Evidence")
	case interaction.FrameOutcomeFeedback:
		msg.Content.Text = renderSelectionFrame(frame, "Outcome Feedback")
	default:
		msg.Content.Text = fmt.Sprintf("[%s]", frame.Type)
	}

	return msg
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

func renderSelectionFrame(frame interaction.InteractionFrame, title string) string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render(title) + "\n")
	selection := frame.Selection
	var typed interaction.SelectionFrame
	if selection != nil {
		typed = *selection
	}
	question := strings.TrimSpace(typed.Question)
	if question == "" {
		question = strings.TrimSpace(frame.Question)
	}
	if q := question; q != "" {
		b.WriteString(q + "\n")
	}
	options := typed.Options
	if len(options) == 0 && len(frame.Slots) > 0 {
		options = selectionOptionsFromSlots(frame.Slots)
	}
	defaultChoice := strings.TrimSpace(typed.Default)
	if defaultChoice == "" {
		defaultChoice = strings.TrimSpace(frame.DefaultChoice)
	}
	if len(options) > 0 {
		b.WriteString("\n")
		for i, slot := range options {
			label := strings.TrimSpace(slot.Label)
			if label == "" {
				label = slot.ID
			}
			prefix := fmt.Sprintf("[%d]", i+1)
			if slot.Default {
				prefix = headerStyle.Render(prefix + "*")
			} else {
				prefix = headerStyle.Render(prefix)
			}
			line := fmt.Sprintf("  %s %s", prefix, label)
			if slot.Risk != "" {
				line += fmt.Sprintf(" %s", dimStyle.Render("risk:"+slot.Risk))
			}
			b.WriteString(line + "\n")
		}
	} else if len(frame.Choices) > 0 {
		b.WriteString("\n")
		for i, choice := range frame.Choices {
			prefix := fmt.Sprintf("[%d]", i+1)
			if choice == defaultChoice {
				prefix = headerStyle.Render(prefix + "*")
			} else {
				prefix = headerStyle.Render(prefix)
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", prefix, choice))
		}
	}
	if defaultChoice != "" {
		b.WriteString("\n" + dimStyle.Render("default: ") + defaultChoice + "\n")
	}
	resume := frame.Resume
	if selection != nil && selection.Resume != nil {
		resume = selection.Resume
	}
	if resume != nil {
		b.WriteString("\n" + dimStyle.Render("resume: ") + renderResumeMetadata(*resume) + "\n")
	}
	if frame.Response != nil {
		if choice := strings.TrimSpace(frame.Response.ChosenSlot); choice != "" {
			b.WriteString(dimStyle.Render("response: ") + choice + "\n")
		}
	}
	if len(frame.Payload) > 0 {
		if payload := renderFramePayload(frame.Payload); payload != "" {
			b.WriteString("\n" + payload)
		}
	}
	return eucloFrameStyle.Render(strings.TrimSpace(b.String()))
}

func selectionOptionsFromSlots(slots []interaction.ActionSlot) []interaction.SelectionOption {
	if len(slots) == 0 {
		return nil
	}
	out := make([]interaction.SelectionOption, 0, len(slots))
	for _, slot := range slots {
		out = append(out, interaction.SelectionOption{
			ID:       slot.ID,
			Label:    slot.Label,
			Shortcut: slot.Shortcut,
			Action:   slot.Action,
			Risk:     slot.Risk,
			Default:  slot.Default,
		})
	}
	return out
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

func renderSessionList(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.SessionListContent)
	if !ok {
		return "[session list]"
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Resume Session") + "\n")
	if content.Workspace != "" {
		b.WriteString(dimStyle.Render("Workspace: ") + content.Workspace + "\n")
	}
	b.WriteString(dimStyle.Render("Select a previous session to resume, or skip to start new:") + "\n\n")
	for _, s := range content.Sessions {
		index := headerStyle.Render(fmt.Sprintf("[%d]", s.Index))
		mode := ""
		if s.Mode != "" {
			mode = dimStyle.Render("(" + s.Mode + ")")
		}
		status := ""
		if s.HasBKCContext {
			status = completedStyle.Render(" ✓BKC")
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n", index, s.Instruction, mode))
		b.WriteString(dimStyle.Render(fmt.Sprintf("    ID: %s%s\n", s.WorkflowID, status)))
		if s.LastActiveAt != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    Last active: %s\n", s.LastActiveAt)))
		}
		b.WriteString("\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderSessionListEmpty(frame interaction.InteractionFrame) string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Resume Session") + "\n")
	if content, ok := frame.Content.(string); ok && content != "" {
		b.WriteString(content + "\n")
	} else {
		b.WriteString("No previous sessions found for this workspace.\n")
	}
	return eucloFrameStyle.Render(b.String())
}

func renderSessionResuming(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(string)
	if !ok {
		content = "Resuming session..."
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Session Resume") + "\n")
	b.WriteString(inProgressStyle.Render("⟳ ") + content + "\n")
	return eucloFrameStyle.Render(b.String())
}

func renderSessionResumeError(frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(string)
	if !ok {
		content = "Could not resume session."
	}
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Session Resume") + "\n")
	b.WriteString(diffRemoveStyle.Render("✗ ") + content + "\n")
	return eucloFrameStyle.Render(b.String())
}

func renderResumeMetadata(resume interaction.ClarificationResumeMetadata) string {
	parts := make([]string, 0, 5)
	if resume.ActiveThoughtRecipeID != "" {
		parts = append(parts, "thoughtrecipe="+resume.ActiveThoughtRecipeID)
	}
	if resume.RouteKind != "" {
		parts = append(parts, "route_kind="+resume.RouteKind)
	}
	if resume.RouteID != "" {
		parts = append(parts, "route_id="+resume.RouteID)
	}
	if resume.ResumeNodeID != "" {
		parts = append(parts, "resume_node="+resume.ResumeNodeID)
	}
	if resume.Unresolved {
		parts = append(parts, "unresolved=true")
	}
	if len(resume.MissingFields) > 0 {
		parts = append(parts, "missing="+strings.Join(resume.MissingFields, ","))
	}
	return strings.Join(parts, " ")
}

func renderFramePayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		switch key {
		case "question", "choices", "default_choice", "resume", "response", "response_extra":
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("  %s %s\n", dimStyle.Render(key+":"), fmt.Sprint(payload[key])))
	}
	return b.String()
}

// RenderChatProjection renders the human-sized milestone feed for the chat
// surface.
func RenderChatProjection(p ChatProjection) string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Chat Projection") + "\n")
	if len(p.Milestones) == 0 && len(p.Outputs) == 0 && len(p.Frames) == 0 {
		return eucloFrameStyle.Render(strings.TrimSpace(b.String()))
	}
	for _, line := range p.Milestones {
		b.WriteString("  " + headerStyle.Render("●") + " " + line + "\n")
	}
	for _, line := range p.Outputs {
		b.WriteString("  " + dimStyle.Render("LLM") + " " + line + "\n")
	}
	for _, frame := range p.Frames {
		b.WriteString("  " + dimStyle.Render("frame") + " " + frameLabel(frame) + "\n")
	}
	return eucloFrameStyle.Render(strings.TrimSpace(b.String()))
}
