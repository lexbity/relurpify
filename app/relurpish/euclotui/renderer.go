package euclotui

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"github.com/charmbracelet/lipgloss"
)

func RenderInteractionFrame(th *theme.Theme, frame interaction.InteractionFrame) tui.Message {
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
		msg.Content.Text = renderCandidates(th, frame)
	case interaction.FrameComparison:
		msg.Content.Text = renderComparison(th, frame)
	case interaction.FrameDraft:
		msg.Content.Text = renderDraft(th, frame)
	case interaction.FrameResultType:
		msg.Content.Text = renderFrameResult(th, frame)
	case interaction.FrameStatus:
		msg.Content.Text = renderStatus(th, frame)
	case interaction.FrameSummary:
		msg.Content.Text = renderSummary(th, frame)
	case interaction.FrameTransition:
		msg.Content.Text = renderTransition(th, frame)
	case interaction.FrameSessionList:
		msg.Content.Text = renderSessionList(th, frame)
	case interaction.FrameSessionListEmpty:
		msg.Content.Text = renderSessionListEmpty(th, frame)
	case interaction.FrameSessionResuming:
		msg.Content.Text = renderSessionResuming(th, frame)
	case interaction.FrameSessionResumeError:
		msg.Content.Text = renderSessionResumeError(th, frame)
	case interaction.FrameScopeConfirmation:
		msg.Content.Text = renderSelectionFrame(th, frame, "Scope Confirmation")
	case interaction.FrameIntentClarification:
		msg.Content.Text = renderSelectionFrame(th, frame, "Clarification")
	case interaction.FrameCandidateSelection:
		msg.Content.Text = renderSelectionFrame(th, frame, "Candidate Selection")
	case interaction.FrameThoughtRecipeSelection:
		msg.Content.Text = renderSelectionFrame(th, frame, "Thoughtrecipe Selection")
	case interaction.FrameCapabilitySelection:
		msg.Content.Text = renderSelectionFrame(th, frame, "Capability Selection")
	case interaction.FrameHITLApproval:
		msg.Content.Text = renderSelectionFrame(th, frame, "Approval Required")
	case interaction.FrameSessionResume:
		msg.Content.Text = renderSelectionFrame(th, frame, "Session Resume")
	case interaction.FrameBackgroundJobStatus:
		msg.Content.Text = renderSelectionFrame(th, frame, "Background Job Status")
	case interaction.FrameExecutionSummary:
		msg.Content.Text = renderSelectionFrame(th, frame, "Execution Summary")
	case interaction.FrameVerificationEvidence:
		msg.Content.Text = renderSelectionFrame(th, frame, "Verification Evidence")
	case interaction.FrameOutcomeFeedback:
		msg.Content.Text = renderSelectionFrame(th, frame, "Outcome Feedback")
	default:
		msg.Content.Text = fmt.Sprintf("[%s]", frame.Type)
	}
	return msg
}

func renderCandidates(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.CandidatesContent)
	if !ok {
		return "[candidates]"
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Candidates") + "\n")
	for i, c := range content.Candidates {
		label := fmt.Sprintf("[%d] %s", i+1, c.ID)
		b.WriteString(th.Header().Render(label) + "\n")
		b.WriteString("  " + c.Summary + "\n")
		for k, v := range c.Properties {
			b.WriteString(fmt.Sprintf("  %s %s\n", th.Dim().Render(k+":"), v))
		}
	}
	if content.RecommendedID != "" {
		b.WriteString(th.Dim().Render("\nRecommended: ") + content.RecommendedID + "\n")
	}
	return th.Panel().Render(b.String())
}

func renderSelectionFrame(th *theme.Theme, frame interaction.InteractionFrame, title string) string {
	var b strings.Builder
	b.WriteString(th.Subhead().Render(title) + "\n")
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
				prefix = th.Header().Render(prefix + "*")
			} else {
				prefix = th.Header().Render(prefix)
			}
			line := fmt.Sprintf("  %s %s", prefix, label)
			if slot.Risk != "" {
				line += fmt.Sprintf(" %s", th.Dim().Render("risk:"+slot.Risk))
			}
			b.WriteString(line + "\n")
		}
	} else if len(frame.Choices) > 0 {
		b.WriteString("\n")
		for i, choice := range frame.Choices {
			prefix := fmt.Sprintf("[%d]", i+1)
			if choice == defaultChoice {
				prefix = th.Header().Render(prefix + "*")
			} else {
				prefix = th.Header().Render(prefix)
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", prefix, choice))
		}
	}
	if defaultChoice != "" {
		b.WriteString("\n" + th.Dim().Render("default: ") + defaultChoice + "\n")
	}
	resume := frame.Resume
	if selection != nil && selection.Resume != nil {
		resume = selection.Resume
	}
	if resume != nil {
		b.WriteString("\n" + th.Dim().Render("resume: ") + renderResumeMetadata(*resume) + "\n")
	}
	if frame.Response != nil {
		if choice := strings.TrimSpace(frame.Response.ChosenSlot); choice != "" {
			b.WriteString(th.Dim().Render("response: ") + choice + "\n")
		}
	}
	if len(frame.Payload) > 0 {
		if payload := renderFramePayload(th, frame.Payload); payload != "" {
			b.WriteString("\n" + payload)
		}
	}
	return th.Panel().Render(strings.TrimSpace(b.String()))
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

func renderComparison(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.ComparisonContent)
	if !ok {
		return "[comparison]"
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Comparison") + "\n")
	if len(content.Dimensions) > 0 {
		b.WriteString(fmt.Sprintf("  %-12s", ""))
		for _, dim := range content.Dimensions {
			b.WriteString(fmt.Sprintf("%-15s", th.Dim().Render(dim)))
		}
		b.WriteString("\n")
		for i, row := range content.Matrix {
			label := fmt.Sprintf("Option %d", i+1)
			b.WriteString(fmt.Sprintf("  %-12s", th.Header().Render(label)))
			for _, cell := range row {
				b.WriteString(fmt.Sprintf("%-15s", cell))
			}
			b.WriteString("\n")
		}
	}
	return th.Panel().Render(b.String())
}

func renderDraft(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.DraftContent)
	if !ok {
		return "[draft]"
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Draft") + "\n")
	if content.Kind != "" {
		b.WriteString(th.Dim().Render("("+content.Kind+")") + "\n")
	}
	for i, item := range content.Items {
		marker := fmt.Sprintf("%d.", i+1)
		if item.Editable {
			marker = "~" + marker
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", th.Dim().Render(marker), item.Content))
	}
	return th.Panel().Render(b.String())
}

func renderFrameResult(th *theme.Theme, frame interaction.InteractionFrame) string {
	switch content := frame.Content.(type) {
	case interaction.ResultContent:
		return renderResultContent(th, content)
	case interaction.FindingsContent:
		return renderFindingsContent(th, content)
	default:
		return th.Panel().Render("[result]")
	}
}

func renderResultContent(th *theme.Theme, content interaction.ResultContent) string {
	var b strings.Builder
	statusLabel := content.Status
	switch content.Status {
	case "passed", "completed":
		statusLabel = th.Success().Render("✓ " + content.Status)
	case "failed":
		statusLabel = th.Error().Render("✗ " + content.Status)
	case "partial":
		statusLabel = th.Warning().Render("◐ " + content.Status)
	}
	b.WriteString(th.Subhead().Render("Result") + " " + statusLabel + "\n")
	for _, ev := range content.Evidence {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			th.Dim().Render(ev.Kind+":"),
			ev.Detail,
		))
	}
	return th.Panel().Render(b.String())
}

func renderFindingsContent(th *theme.Theme, content interaction.FindingsContent) string {
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Findings") + "\n")
	for _, f := range content.Critical {
		b.WriteString(th.Error().Bold(true).Render("  CRITICAL "))
		if f.Location != "" {
			b.WriteString(th.Subhead().Render(f.Location) + " ")
		}
		b.WriteString(f.Description + "\n")
	}
	for _, f := range content.Warning {
		b.WriteString(th.Warning().Render("  WARNING  "))
		if f.Location != "" {
			b.WriteString(th.Subhead().Render(f.Location) + " ")
		}
		b.WriteString(f.Description + "\n")
	}
	for _, f := range content.Info {
		b.WriteString(th.Dim().Render("  INFO     "))
		if f.Location != "" {
			b.WriteString(th.Subhead().Render(f.Location) + " ")
		}
		b.WriteString(f.Description + "\n")
	}
	return th.Panel().Render(b.String())
}

func renderStatus(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.StatusContent)
	if !ok {
		return "[status]"
	}
	return th.Dim().Render(content.Message)
}

func renderSummary(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.SummaryContent)
	if !ok {
		return "[summary]"
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Summary") + "\n")
	if content.Description != "" {
		b.WriteString(content.Description + "\n")
	}
	if len(content.Artifacts) > 0 {
		b.WriteString(th.Dim().Render("  Artifacts: ") + strings.Join(content.Artifacts, ", ") + "\n")
	}
	if len(content.Changes) > 0 {
		b.WriteString(th.Dim().Render("  Changes: ") + strings.Join(content.Changes, ", ") + "\n")
	}
	return th.Panel().Render(b.String())
}

func renderTransition(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.TransitionContent)
	if !ok {
		return "[transition]"
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Mode Transition") + "\n")
	b.WriteString(fmt.Sprintf("  %s → %s\n",
		th.Header().Render(content.FromMode),
		th.Warning().Render(content.ToMode),
	))
	if content.Reason != "" {
		b.WriteString(th.Dim().Render("  "+content.Reason) + "\n")
	}
	return th.Panel().Render(b.String())
}

func renderSessionList(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(interaction.SessionListContent)
	if !ok {
		return "[session list]"
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Resume Session") + "\n")
	if content.Workspace != "" {
		b.WriteString(th.Dim().Render("Workspace: ") + content.Workspace + "\n")
	}
	b.WriteString(th.Dim().Render("Select a previous session to resume, or skip to start new:") + "\n\n")
	for _, s := range content.Sessions {
		index := th.Header().Render(fmt.Sprintf("[%d]", s.Index))
		mode := ""
		if s.Mode != "" {
			mode = th.Dim().Render("(" + s.Mode + ")")
		}
		status := ""
		if s.HasBKCContext {
			status = th.Success().Render(" ✓BKC")
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n", index, s.Instruction, mode))
		b.WriteString(th.Dim().Render(fmt.Sprintf("    ID: %s%s\n", s.WorkflowID, status)))
		if s.LastActiveAt != "" {
			b.WriteString(th.Dim().Render(fmt.Sprintf("    Last active: %s\n", s.LastActiveAt)))
		}
		b.WriteString("\n")
	}
	return th.Panel().Render(b.String())
}

func renderSessionListEmpty(th *theme.Theme, frame interaction.InteractionFrame) string {
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Resume Session") + "\n")
	if content, ok := frame.Content.(string); ok && content != "" {
		b.WriteString(content + "\n")
	} else {
		b.WriteString("No previous sessions found for this workspace.\n")
	}
	return th.Panel().Render(b.String())
}

func renderSessionResuming(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(string)
	if !ok {
		content = "Resuming session..."
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Session Resume") + "\n")
	b.WriteString(th.Warning().Render("⟳ ") + content + "\n")
	return th.Panel().Render(b.String())
}

func renderSessionResumeError(th *theme.Theme, frame interaction.InteractionFrame) string {
	content, ok := frame.Content.(string)
	if !ok {
		content = "Could not resume session."
	}
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Session Resume") + "\n")
	b.WriteString(th.Error().Render("✗ ") + content + "\n")
	return th.Panel().Render(b.String())
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

func renderFramePayload(th *theme.Theme, payload map[string]any) string {
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
		b.WriteString(fmt.Sprintf("  %s %s\n", th.Dim().Render(key+":"), fmt.Sprint(payload[key])))
	}
	return b.String()
}

func RenderChatProjection(th *theme.Theme, snap EucloProjectionSnapshot) string {
	var b strings.Builder
	b.WriteString(th.Subhead().Render("Chat Projection") + "\n")
	if len(snap.Chat.Milestones) == 0 && len(snap.Chat.Outputs) == 0 && len(snap.Chat.Frames) == 0 {
		return th.Panel().Render(strings.TrimSpace(b.String()))
	}
	// Render phase stepper bar.
	if snap.StepperPhase != PhaseIdle {
		b.WriteString("  " + renderStepper(th, snap.StepperPhase) + "\n\n")
	}
	for _, line := range snap.Chat.Milestones {
		b.WriteString("  " + th.Header().Render("●") + " " + line + "\n")
	}
	for _, line := range snap.Chat.Outputs {
		b.WriteString("  " + th.Dim().Render("LLM") + " " + line + "\n")
	}
	for _, frame := range snap.Chat.Frames {
		b.WriteString("  " + th.Dim().Render("frame") + " " + frameLabel(frame) + "\n")
	}
	return th.Panel().Render(strings.TrimSpace(b.String()))
}

func renderStepper(th *theme.Theme, current Phase) string {
	if th == nil || current == PhaseIdle {
		return ""
	}
	ordered := []Phase{PhaseIntake, PhasePlan, PhaseExecute, PhaseVerify, PhaseDone}
	parts := make([]string, 0, len(ordered))
	for _, p := range ordered {
		label := p.String()
		var style lipgloss.Style
		if p < current {
			style = th.Success()
		} else if p == current {
			style = th.Active()
		} else {
			style = th.Pending()
		}
		parts = append(parts, style.Render(label))
	}
	return strings.Join(parts, " → ")
}
