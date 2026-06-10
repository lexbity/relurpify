package interaction

import (
	"fmt"
	"strings"
	"time"

	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// generateID creates a simple unique ID without external dependencies.
func generateID() string {
	return fmt.Sprintf("frame-%d", time.Now().UnixNano())
}

// NewScopeConfirmationFrame creates a scope confirmation frame for ingestion.
func NewScopeConfirmationFrame(taskID, sessionID string) *InteractionFrame {
	return newClarificationFrame(taskID, sessionID, FrameScopeConfirmation, "Confirm the scope to use for this request.", []string{
		"use_selected_files",
		"scan_changed",
		"scan_full",
	}, nil, map[string]any{
		"selection": "files_only",
	}, []ActionSlot{
		{
			ID:      "use_selected_files",
			Label:   "Use my selected files",
			Action:  "files_only",
			Risk:    "low",
			Default: true,
		},
		{
			ID:      "scan_changed",
			Label:   "Scan changed files (incremental)",
			Action:  "incremental",
			Risk:    "medium",
			Default: false,
		},
		{
			ID:      "scan_full",
			Label:   "Scan full workspace",
			Action:  "full",
			Risk:    "high",
			Default: false,
		},
	}, "use_selected_files", 5*time.Minute)
}

// NewHITLApprovalFrame creates a HITL approval frame.
func NewHITLApprovalFrame(taskID, sessionID string, permission string, risk string) *InteractionFrame {
	return newClarificationFrame(taskID, sessionID, FrameHITLApproval, fmt.Sprintf("Approve permission %s?", strings.TrimSpace(permission)), []string{
		"approve",
		"reject",
	}, nil, map[string]any{
		"permission": permission,
		"risk":       risk,
	}, []ActionSlot{
		{
			ID:      "approve",
			Label:   "Approve",
			Action:  "approve",
			Risk:    risk,
			Default: false,
		},
		{
			ID:      "reject",
			Label:   "Reject",
			Action:  "reject",
			Risk:    "low",
			Default: false,
		},
	}, "", 5*time.Minute)
}

// NewCandidateSelectionFrame creates a candidate selection frame for ambiguous classification.
func NewCandidateSelectionFrame(taskID, sessionID string, candidates []string) *InteractionFrame {
	slots := make([]ActionSlot, len(candidates))
	for i, candidate := range candidates {
		slots[i] = ActionSlot{
			ID:      candidate,
			Label:   candidate,
			Action:  candidate,
			Risk:    "low",
			Default: i == 0,
		}
	}

	defaultChoice := ""
	if len(candidates) > 0 {
		defaultChoice = candidates[0]
	}
	return newClarificationFrame(taskID, sessionID, FrameCandidateSelection, "Select the best candidate.", append([]string(nil), candidates...), nil, map[string]any{
		"candidates": candidates,
	}, slots, defaultChoice, 5*time.Minute)
}

// NewThoughtRecipeSelectionFrame creates a selection frame for choosing a thoughtrecipe.
// Each recipe projection is carried in the payload so the frontend can render
// structured recipe info (steps, paradigms, HITL gates, groups, etc.).
func NewThoughtRecipeSelectionFrame(taskID, sessionID string, recipes []surface.RecipeProjection) *InteractionFrame {
	n := len(recipes)
	candidates := make([]string, n)
	slots := make([]ActionSlot, n)
	var projs []surface.RecipeProjection
	if n > 0 {
		projs = make([]surface.RecipeProjection, n)
	}
	for i, r := range recipes {
		name := r.Name
		if name == "" {
			name = r.RecipeID
		}
		candidates[i] = name
		if projs != nil {
			projs[i] = r
		}
		slots[i] = ActionSlot{
			ID:      r.RecipeID,
			Label:   name,
			Action:  r.RecipeID,
			Risk:    "low",
			Default: i == 0,
		}
	}
	defaultChoice := ""
	if n > 0 {
		defaultChoice = candidates[0]
	}
	payload := map[string]any{
		"candidates": candidates,
	}
	if len(projs) > 0 {
		payload["recipes"] = projs
	}
	return newClarificationFrame(taskID, sessionID, FrameThoughtRecipeSelection, "Select a thoughtrecipe.", candidates, nil, payload, slots, defaultChoice, 5*time.Minute)
}

// NewOutcomeFeedbackFrame creates an outcome feedback frame.
func NewOutcomeFeedbackFrame(taskID, sessionID string, outcome string) *InteractionFrame {
	return newClarificationFrame(taskID, sessionID, FrameOutcomeFeedback, "Was this outcome helpful?", []string{
		"positive",
		"partial",
		"negative",
	}, nil, map[string]any{
		"outcome": outcome,
	}, []ActionSlot{
		{
			ID:      "positive",
			Label:   "Yes, solved my problem",
			Action:  "positive",
			Risk:    "low",
			Default: true,
		},
		{
			ID:      "partial",
			Label:   "Partially helpful",
			Action:  "partial",
			Risk:    "low",
			Default: false,
		},
		{
			ID:      "negative",
			Label:   "No, not helpful",
			Action:  "negative",
			Risk:    "low",
			Default: false,
		},
	}, "positive", 30*time.Second)
}

// NewClarificationFrame creates a clarification frame with explicit question, choices, and resume metadata.
func NewClarificationFrame(taskID, sessionID, question string, choices []string, resume *ClarificationResumeMetadata) *InteractionFrame {
	return newClarificationFrame(taskID, sessionID, FrameIntentClarification, question, choices, resume, nil, nil, "", 5*time.Minute)
}

func newClarificationFrame(taskID, sessionID string, frameType FrameType, question string, choices []string, resume *ClarificationResumeMetadata, payload map[string]any, slots []ActionSlot, defaultChoice string, timeout time.Duration) *InteractionFrame {
	now := time.Now().UTC()
	frame := &InteractionFrame{
		ID:            generateID(),
		Type:          frameType,
		TaskID:        taskID,
		SessionID:     sessionID,
		Seq:           0,
		Question:      strings.TrimSpace(question),
		Choices:       NormalizeChoices(choices),
		DefaultChoice: strings.TrimSpace(defaultChoice),
		Resume:        CloneClarificationResumeMetadata(resume),
		Payload:       map[string]any{},
	}
	selection := SelectionFrame{
		Kind:     frameType,
		Question: strings.TrimSpace(question),
		Resume:   CloneClarificationResumeMetadata(resume),
	}
	if len(frame.Choices) > 0 {
		if frame.DefaultChoice == "" {
			frame.DefaultChoice = frame.Choices[0]
		}
	}
	if len(slots) > 0 {
		frame.Slots = append([]ActionSlot(nil), slots...)
		selection.Options = selectionOptionsFromSlots(slots)
		if frame.DefaultChoice == "" && len(slots) > 0 {
			frame.DefaultChoice = strings.TrimSpace(slots[0].ID)
		}
	} else {
		frame.Slots = buildActionSlotsFromChoices(frame.Choices)
		selection.Options = selectionOptionsFromSlots(frame.Slots)
		if frame.DefaultChoice == "" && len(frame.Slots) > 0 {
			frame.DefaultChoice = strings.TrimSpace(frame.Slots[0].ID)
		}
	}
	selection.Default = strings.TrimSpace(defaultChoice)
	if selection.Default == "" && len(selection.Options) > 0 {
		selection.Default = strings.TrimSpace(selection.Options[0].ID)
	}
	if payload != nil {
		for k, v := range payload {
			frame.Payload[k] = v
		}
	}
	if frame.Question != "" {
		frame.Payload["question"] = frame.Question
	}
	if len(frame.Choices) > 0 {
		frame.Payload["choices"] = append([]string(nil), frame.Choices...)
	}
	if frame.DefaultChoice != "" {
		frame.Payload["default_choice"] = frame.DefaultChoice
		frame.DefaultSlot = frame.DefaultChoice
	}
	if frame.Resume != nil {
		frame.Payload["resume"] = map[string]any{
			"active_thoughtrecipe_id": frame.Resume.ActiveThoughtRecipeID,
			"resume_node_id":          frame.Resume.ResumeNodeID,
			"route_kind":              frame.Resume.RouteKind,
			"route_id":                frame.Resume.RouteID,
			"state_version":           frame.Resume.StateVersion,
			"unresolved":              frame.Resume.Unresolved,
			"missing_fields":          append([]string(nil), frame.Resume.MissingFields...),
		}
	}
	frame.Selection = &selection
	frame.CreatedAt = now
	frame.Metadata.Timestamp = now
	frame.Timeout = timeout
	return frame
}

func selectionOptionsFromSlots(slots []ActionSlot) []SelectionOption {
	if len(slots) == 0 {
		return nil
	}
	out := make([]SelectionOption, 0, len(slots))
	for _, slot := range slots {
		out = append(out, SelectionOption(slot))
	}
	return out
}

// SetResponse records a user's response on the frame and mirrors it into payload.
func (f *InteractionFrame) SetResponse(choice string, extra map[string]any, responder string, respondedAt time.Time) {
	if f == nil {
		return
	}
	if respondedAt.IsZero() {
		respondedAt = time.Now().UTC()
	}
	payload := make(map[string]any, len(extra)+2)
	for k, v := range extra {
		payload[k] = v
	}
	if trimmed := strings.TrimSpace(choice); trimmed != "" {
		payload["answer"] = trimmed
	}
	if trimmed := strings.TrimSpace(responder); trimmed != "" {
		payload["responded_by"] = trimmed
	}
	f.Response = &FrameResult{
		ChosenSlot:  strings.TrimSpace(choice),
		ExtraData:   payload,
		RespondedBy: strings.TrimSpace(responder),
		RespondedAt: respondedAt,
	}
	f.RespondedAt = &respondedAt
	if f.Payload == nil {
		f.Payload = make(map[string]any)
	}
	f.Payload["response"] = map[string]any{
		"choice":       strings.TrimSpace(choice),
		"responded_by": strings.TrimSpace(responder),
		"responded_at": respondedAt.UTC().Format(time.RFC3339Nano),
	}
	if len(payload) > 0 {
		f.Payload["response_extra"] = payload
	}
}

// ClarificationTurnFromFrame projects a clarification frame into a persisted turn record.
func ClarificationTurnFromFrame(frame *InteractionFrame, stateVersion uint64) *intentcontext.ClarificationTurn {
	if frame == nil {
		return nil
	}
	answer, ok := ResponseValue(frame)
	if !ok {
		answer = ""
	}
	turn := &intentcontext.ClarificationTurn{
		TurnID:       strings.TrimSpace(frame.ID),
		PromptID:     strings.TrimSpace(frame.PayloadString("prompt_id")),
		PromptFamily: string(frame.Type),
		Question:     strings.TrimSpace(frame.Question),
		Answer:       strings.TrimSpace(answer),
		ResponseKind: intentcontext.ResponseKindFreeText,
		StateVersion: stateVersion,
		SourceTurnID: strings.TrimSpace(frame.ID),
		CreatedAt:    frame.CreatedAt,
		UpdatedAt:    frame.CreatedAt,
	}
	if frame.Resume != nil {
		if strings.TrimSpace(frame.Resume.RouteID) != "" && turn.PromptID == "" {
			turn.PromptID = strings.TrimSpace(frame.Resume.RouteID)
		}
		if strings.TrimSpace(frame.Resume.ActiveThoughtRecipeID) != "" && turn.SourceTurnID == "" {
			turn.SourceTurnID = strings.TrimSpace(frame.Resume.ActiveThoughtRecipeID)
		}
	}
	if frame.Response != nil {
		if strings.TrimSpace(frame.Response.ChosenSlot) != "" {
			turn.ResponseKind = intentcontext.ResponseKindChoice
		}
		if strings.TrimSpace(frame.Response.ChosenSlot) == "" && len(frame.Response.ExtraData) > 0 {
			turn.ResponseKind = intentcontext.ResponseKindFreeText
		}
		if !frame.Response.RespondedAt.IsZero() {
			turn.UpdatedAt = frame.Response.RespondedAt
		}
	}
	turn.Normalize(frame.TaskID, frame.SessionID)
	return turn
}

func buildActionSlotsFromChoices(choices []string) []ActionSlot {
	if len(choices) == 0 {
		return []ActionSlot{{
			ID:      "answer",
			Label:   "Answer",
			Action:  "answer",
			Risk:    "low",
			Default: true,
		}}
	}
	slots := make([]ActionSlot, 0, len(choices))
	for i, choice := range choices {
		choice = strings.TrimSpace(choice)
		if choice == "" {
			continue
		}
		slots = append(slots, ActionSlot{
			ID:      choice,
			Label:   choice,
			Action:  choice,
			Risk:    "low",
			Default: i == 0,
		})
	}
	if len(slots) == 0 {
		return []ActionSlot{{
			ID:      "answer",
			Label:   "Answer",
			Action:  "answer",
			Risk:    "low",
			Default: true,
		}}
	}
	return slots
}

func NormalizeChoices(choices []string) []string {
	if len(choices) == 0 {
		return nil
	}
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if trimmed := strings.TrimSpace(choice); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CloneClarificationResumeMetadata returns a deep copy of clarification resume metadata.
func CloneClarificationResumeMetadata(resume *ClarificationResumeMetadata) *ClarificationResumeMetadata {
	if resume == nil {
		return nil
	}
	out := *resume
	out.MissingFields = append([]string(nil), resume.MissingFields...)
	return &out
}

func (f *InteractionFrame) PayloadString(key string) string {
	if f == nil || f.Payload == nil {
		return ""
	}
	if value, ok := f.Payload[key]; ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

// RecipeProjections returns the recipe projections carried by a thoughtrecipe
// selection frame, if any.
func (f *InteractionFrame) RecipeProjections() []surface.RecipeProjection {
	if f == nil || f.Payload == nil {
		return nil
	}
	raw, ok := f.Payload["recipes"]
	if !ok {
		return nil
	}
	projs, ok := raw.([]surface.RecipeProjection)
	if !ok || len(projs) == 0 {
		return nil
	}
	return projs
}
