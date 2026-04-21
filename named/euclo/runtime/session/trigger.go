package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// SessionResumeTrigger returns an AgencyTrigger that fires when the user
// expresses intent to resume a past session.
func SessionResumeTrigger() interaction.AgencyTrigger {
	return interaction.AgencyTrigger{
		Phrases: []string{
			"resume session",
			"continue session",
			"load session",
			"restore session",
			"continue last session",
			"resume last session",
			"pick up where i left off",
		},
		CapabilityID: "", // handled by PhaseJump, not a relurpic capability
		PhaseJump:    "session_select",
		RequiresMode: "", // mode-agnostic
		Description:  "Resume a previous coding session with restored semantic context",
	}
}

// SessionSelectPhase implements interaction.PhaseHandler for the
// session_select phase.
type SessionSelectPhase struct {
	Index    *SessionIndex
	Resolver *SessionResumeResolver
}

// Execute implements the session selection phase.
// It lists sessions, emits a frame, awaits user response, resolves the selection,
// and returns the resume context in a single pass.
func (p *SessionSelectPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// 1. List sessions
	workspace := stringFromState(mc.State, "euclo.workspace")
	list, err := p.Index.List(ctx, workspace, 10)
	if err != nil {
		return interaction.PhaseOutcome{Advance: true}, nil // non-fatal
	}

	// 2. Check for empty sessions
	if len(list.Sessions) == 0 {
		emitEmptyFrame(ctx, mc)
		return interaction.PhaseOutcome{Advance: true}, nil
	}

	// 3. Emit session list and await response
	emitListFrame(ctx, mc, list)
	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{Advance: true}, nil // non-fatal
	}

	// 4. Parse the response action ID to a workflow ID
	workflowID := parseActionToWorkflowID(resp, list)
	if workflowID == "" {
		// "skip" or unrecognized → proceed without resuming
		return interaction.PhaseOutcome{Advance: true}, nil
	}

	// 5. Resolve the selected session
	resumeCtx, err := p.Resolver.Resolve(ctx, workflowID)
	if err != nil {
		emitResumeErrorFrame(ctx, mc, workflowID, err)
		return interaction.PhaseOutcome{Advance: true}, nil
	}

	// 6. Emit resuming frame and advance with resume context
	emitResumingFrame(ctx, mc, resumeCtx.WorkflowID)
	return interaction.PhaseOutcome{
		Advance:      true,
		StateUpdates: map[string]any{"euclo.session_resume_context": resumeCtx},
	}, nil
}

// formatSessionListContent converts SessionList to SessionListContent.
func formatSessionListContent(list SessionList) interaction.SessionListContent {
	entries := make([]interaction.SessionListEntry, len(list.Sessions))
	for i, s := range list.Sessions {
		entries[i] = interaction.SessionListEntry{
			Index:         i + 1, // 1-based for user selection
			WorkflowID:    s.WorkflowID,
			Instruction:   s.Instruction,
			Mode:          s.Mode,
			Status:        s.Status,
			HasBKCContext: s.HasBKCContext,
			LastActiveAt:  s.LastActiveAt.Format(time.RFC3339),
		}
	}
	return interaction.SessionListContent{
		Sessions:  entries,
		Workspace: list.Workspace,
	}
}

// buildSessionListActions creates action slots for session selection.
func buildSessionListActions(count int) []interaction.ActionSlot {
	actions := make([]interaction.ActionSlot, 0, count+1)
	for i := 1; i <= count; i++ {
		actions = append(actions, interaction.ActionSlot{
			ID:       fmt.Sprintf("select_%d", i),
			Label:    fmt.Sprintf("Select %d", i),
			Kind:     interaction.ActionSelect,
			Shortcut: strconv.Itoa(i),
		})
	}
	// Add skip action
	actions = append(actions, interaction.ActionSlot{
		ID:      "skip",
		Label:   "Skip (new session)",
		Kind:    interaction.ActionConfirm,
		Default: true,
	})
	return actions
}

// stringFromState extracts a string value from state map.
func stringFromState(state map[string]any, key string) string {
	if state == nil {
		return ""
	}
	raw, ok := state[key]
	if !ok || raw == nil {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// ParseSessionSelection resolves a user response string to a workflow ID
// from the provided session list. Returns "" if no match is found.
//
// The parser tries in order:
// 1. Numeric index (1, 2, 3...)
// 2. Exact workflow ID match
// 3. Workflow ID prefix match
// 4. Instruction substring match
func ParseSessionSelection(response string, list SessionList) string {
	response = strings.TrimSpace(response)
	if response == "" {
		return ""
	}
	responseLower := strings.ToLower(response)

	// 1. Try numeric index
	if idx, err := strconv.Atoi(response); err == nil && idx >= 1 && idx <= len(list.Sessions) {
		return list.Sessions[idx-1].WorkflowID
	}

	// 2. Try exact workflow ID match (case-insensitive)
	for _, s := range list.Sessions {
		if strings.EqualFold(s.WorkflowID, response) {
			return s.WorkflowID
		}
	}

	// 3. Try workflow ID prefix match
	for _, s := range list.Sessions {
		if strings.HasPrefix(strings.ToLower(s.WorkflowID), responseLower) {
			return s.WorkflowID
		}
	}

	// 4. Try instruction substring match
	for _, s := range list.Sessions {
		if strings.Contains(strings.ToLower(s.Instruction), responseLower) {
			return s.WorkflowID
		}
	}

	return ""
}

// emitEmptyFrame emits a FrameSessionListEmpty frame.
func emitEmptyFrame(ctx context.Context, mc interaction.PhaseMachineContext) {
	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameSessionListEmpty,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: "No previous sessions found for this workspace.",
		Actions: []interaction.ActionSlot{
			{ID: "continue", Label: "Continue", Kind: interaction.ActionConfirm, Default: true},
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	_ = mc.Emitter.Emit(ctx, frame)
}

// emitListFrame emits a FrameSessionList frame.
func emitListFrame(ctx context.Context, mc interaction.PhaseMachineContext, list SessionList) {
	content := formatSessionListContent(list)
	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameSessionList,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: buildSessionListActions(len(list.Sessions)),
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	_ = mc.Emitter.Emit(ctx, frame)
}

// parseActionToWorkflowID parses the user response to a workflow ID.
// Handles numeric action IDs like "select_1", "select_2", and freetext fallback.
func parseActionToWorkflowID(resp interaction.UserResponse, list SessionList) string {
	if resp.ActionID == "skip" {
		return ""
	}
	// Numeric action IDs: "select_1", "select_2", ...
	if strings.HasPrefix(resp.ActionID, "select_") {
		idxStr := strings.TrimPrefix(resp.ActionID, "select_")
		if idx, err := strconv.Atoi(idxStr); err == nil && idx >= 1 && idx <= len(list.Sessions) {
			return list.Sessions[idx-1].WorkflowID
		}
	}
	// Freetext fallback (when ActionID is empty or unrecognized, and Text is provided)
	if resp.Text != "" {
		return ParseSessionSelection(resp.Text, list)
	}
	return ""
}

// emitResumeErrorFrame emits a FrameSessionResumeError frame.
func emitResumeErrorFrame(ctx context.Context, mc interaction.PhaseMachineContext, workflowID string, err error) {
	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameSessionResumeError,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: fmt.Sprintf("Could not resume session %s: %s", workflowID, err),
		Actions: []interaction.ActionSlot{
			{ID: "continue", Label: "Continue", Kind: interaction.ActionConfirm, Default: true},
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	_ = mc.Emitter.Emit(ctx, frame)
}

// emitResumingFrame emits a FrameSessionResuming frame.
func emitResumingFrame(ctx context.Context, mc interaction.PhaseMachineContext, workflowID string) {
	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameSessionResuming,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: fmt.Sprintf("Resuming session: %s", workflowID),
		Actions: []interaction.ActionSlot{
			{ID: "continue", Label: "Continue", Kind: interaction.ActionConfirm, Default: true},
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	_ = mc.Emitter.Emit(ctx, frame)
}
