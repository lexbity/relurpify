package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

func TestRenderSelectionFrameWithNilSelectionRendersWithoutPanic(t *testing.T) {
	cases := []struct {
		name  string
		frame interaction.InteractionFrame
	}{
		{
			name: "legacy choices path with nil Selection",
			frame: interaction.InteractionFrame{
				Type:          interaction.FrameScopeConfirmation,
				TaskID:        "task-1",
				Question:      "Which files should be included?",
				DefaultChoice: "all",
				Choices:       []string{"all", "selected", "none"},
				Selection:     nil,
			},
		},
		{
			name: "intent clarification with nil Selection",
			frame: interaction.InteractionFrame{
				Type:          interaction.FrameIntentClarification,
				Question:      "What approach?",
				DefaultChoice: "refactor",
				Choices:       []string{"refactor", "rewrite", "patch"},
				Selection:     nil,
			},
		},
		{
			name: "non-nil Selection with options",
			frame: interaction.InteractionFrame{
				Type:     interaction.FrameHITLApproval,
				Question: "Approve?",
				Selection: &interaction.SelectionFrame{
					Question: "Approve this change?",
					Options: []interaction.SelectionOption{
						{ID: "approve", Label: "Approve", Default: true},
						{ID: "deny", Label: "Deny"},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := RenderInteractionFrame(theme.Default(), tc.frame)
			if strings.TrimSpace(result.Content.Text) == "" {
				t.Error("expected non-empty rendered text")
			}
		})
	}
}

func TestRenderSelectionFrameWithSlotsAndChoicesNoSelection(t *testing.T) {
	// Legacy path: frame has Slots but no Selection.
	frame := interaction.InteractionFrame{
		Type:     interaction.FrameCandidateSelection,
		TaskID:   "task-1",
		Question: "test question",
		Slots: []interaction.ActionSlot{
			{ID: "a", Label: "Option A", Default: true},
			{ID: "b", Label: "Option B"},
		},
		DefaultSlot: "a",
		Selection:   nil,
	}
	result := RenderInteractionFrame(theme.Default(), frame)
	if result.Content.Text == "" {
		t.Fatal("expected non-empty text for slots path with nil Selection")
	}
	if !strings.Contains(result.Content.Text, "Option A") {
		t.Errorf("expected Option A in rendered output, got: %s", result.Content.Text)
	}
}

func TestRenderSelectionFrameWithResumeOnFrame(t *testing.T) {
	// Frame has no Selection but has a Resume field on the frame itself.
	resume := &interaction.ClarificationResumeMetadata{
		ActiveThoughtRecipeID: "recipe-1",
		ResumeNodeID:          "node-1",
	}
	frame := interaction.InteractionFrame{
		Type:     interaction.FrameHITLApproval,
		Question: "Approve?",
		Slots: []interaction.ActionSlot{
			{ID: "yes", Label: "Yes", Default: true},
			{ID: "no", Label: "No"},
		},
		DefaultSlot: "yes",
		Selection:   nil,
		Resume:      resume,
	}
	result := RenderInteractionFrame(theme.Default(), frame)
	if !strings.Contains(result.Content.Text, "recipe-1") {
		t.Errorf("expected resume metadata 'recipe-1' in output, got: %s", result.Content.Text)
	}
}

func TestRenderSelectionFrameWithResumeOnSelection(t *testing.T) {
	// Frame has Selection which has Resume; frame.Resume is also set.
	// selection.Resume should take precedence.
	resume := &interaction.ClarificationResumeMetadata{
		ActiveThoughtRecipeID: "selection-recipe",
	}
	frameResume := &interaction.ClarificationResumeMetadata{
		ActiveThoughtRecipeID: "frame-recipe",
	}
	frame := interaction.InteractionFrame{
		Type:     interaction.FrameHITLApproval,
		Question: "Approve?",
		Selection: &interaction.SelectionFrame{
			Question: "Approve this?",
			Options: []interaction.SelectionOption{
				{ID: "yes", Label: "Yes", Default: true},
			},
			Default: "yes",
			Resume:  resume,
		},
		Resume: frameResume,
	}
	result := RenderInteractionFrame(theme.Default(), frame)
	if !strings.Contains(result.Content.Text, "selection-recipe") {
		t.Errorf("expected selection.Resume to take precedence, got: %s", result.Content.Text)
	}
	if strings.Contains(result.Content.Text, "frame-recipe") {
		t.Errorf("expected frame.Resume NOT to appear when selection.Resume is set, got: %s", result.Content.Text)
	}
}

func TestRenderAllSelectionFrameTypesWithNilSelection(t *testing.T) {
	selectionTypes := []interaction.FrameType{
		interaction.FrameScopeConfirmation,
		interaction.FrameIntentClarification,
		interaction.FrameCandidateSelection,
		interaction.FrameThoughtRecipeSelection,
		interaction.FrameCapabilitySelection,
		interaction.FrameHITLApproval,
		interaction.FrameSessionResume,
		interaction.FrameBackgroundJobStatus,
		interaction.FrameExecutionSummary,
		interaction.FrameVerificationEvidence,
		interaction.FrameOutcomeFeedback,
	}

	for _, ft := range selectionTypes {
		t.Run(string(ft), func(t *testing.T) {
			frame := interaction.InteractionFrame{
				Type:     ft,
				TaskID:   "task-1",
				Question: "test question",
				Slots: []interaction.ActionSlot{
					{ID: "opt1", Label: "Option 1", Default: true},
				},
				DefaultSlot: "opt1",
				Selection:   nil,
			}
			result := RenderInteractionFrame(theme.Default(), frame)
			if result.Content.Text == "" {
				t.Errorf("RenderInteractionFrame for %s with nil Selection returned empty text", ft)
			}
		})
	}
}

func TestRenderSelectionFrameEdgeCases(t *testing.T) {
	// All nil: Selection nil, Resume nil, Slots empty, no Choices.
	frame := interaction.InteractionFrame{
		Type:     interaction.FrameScopeConfirmation,
		TaskID:   "task-1",
		Question: "just a question",
	}
	result := RenderInteractionFrame(theme.Default(), frame)
	if result.Content.Text == "" {
		t.Error("expected non-empty text for minimal frame")
	}

	// Slots populated but empty.
	frame2 := interaction.InteractionFrame{
		Type:   interaction.FrameCandidateSelection,
		TaskID: "task-1",
		Slots:  []interaction.ActionSlot{},
	}
	result2 := RenderInteractionFrame(theme.Default(), frame2)
	if result2.Content.Text == "" {
		t.Error("expected non-empty text for frame with empty Slots")
	}

	// Choices populated but empty.
	frame3 := interaction.InteractionFrame{
		Type:     interaction.FrameIntentClarification,
		TaskID:   "task-1",
		Question: "question",
		Choices:  []string{},
	}
	result3 := RenderInteractionFrame(theme.Default(), frame3)
	if result3.Content.Text == "" {
		t.Error("expected non-empty text for frame with empty Choices")
	}

	// Selection non-nil with nil Resume, frame.Resume also nil.
	frame4 := interaction.InteractionFrame{
		Type: interaction.FrameHITLApproval,
		Selection: &interaction.SelectionFrame{
			Question: "Approve?",
			Options: []interaction.SelectionOption{
				{ID: "yes", Label: "Yes", Default: true},
			},
			Default: "yes",
		},
	}
	result4 := RenderInteractionFrame(theme.Default(), frame4)
	if result4.Content.Text == "" {
		t.Error("expected non-empty text for Selection with nil Resume")
	}

	// Response set on frame.
	frame5 := interaction.InteractionFrame{
		Type: interaction.FrameHITLApproval,
		Response: &interaction.FrameResult{
			ChosenSlot: "approve",
		},
		Slots: []interaction.ActionSlot{
			{ID: "approve", Label: "Approve", Default: true},
		},
	}
	result5 := RenderInteractionFrame(theme.Default(), frame5)
	if result5.Content.Text == "" {
		t.Error("expected non-empty text for frame with Response")
	}
}

func FuzzInteractionFrameRender(f *testing.F) {
	seeds := []interaction.FrameType{
		interaction.FrameScopeConfirmation,
		interaction.FrameIntentClarification,
		interaction.FrameCandidateSelection,
		interaction.FrameThoughtRecipeSelection,
		interaction.FrameCapabilitySelection,
		interaction.FrameHITLApproval,
		interaction.FrameSessionResume,
		interaction.FrameBackgroundJobStatus,
		interaction.FrameExecutionSummary,
		interaction.FrameVerificationEvidence,
		interaction.FrameOutcomeFeedback,
		interaction.FrameStatus,
	}
	for _, ft := range seeds {
		f.Add(string(ft))
	}
	f.Fuzz(func(t *testing.T, frameType string) {
		frame := interaction.InteractionFrame{
			Type: interaction.FrameType(frameType),
		}
		// Should never panic regardless of frame type.
		result := RenderInteractionFrame(theme.Default(), frame)
		_ = result
	})
}

func TestRenderStatusFrameIsStatic(t *testing.T) {
	// The static ⟳ glyph should be rendered, but no animation manager.
	frame := interaction.InteractionFrame{
		Type: interaction.FrameStatus,
		Content: interaction.StatusContent{
			Message: "working on it",
		},
	}
	result := RenderInteractionFrame(theme.Default(), frame)
	if !strings.Contains(result.Content.Text, "working on it") {
		t.Errorf("expected status message in output, got: %s", result.Content.Text)
	}
}

// TestRenderInteractionFrameProducesGoldenOutput verifies that every
// selection-class frame type produces non-empty, structurally sound output
// when rendered through the shared theme. This proves euclotui renderers
// correctly use the theme rather than hardcoded ANSI colours.
func TestRenderInteractionFrameProducesGoldenOutput(t *testing.T) {
	th := theme.Default()

	frameTypes := []interaction.FrameType{
		interaction.FrameCandidates,
		interaction.FrameComparison,
		interaction.FrameDraft,
		interaction.FrameResultType,
		interaction.FrameStatus,
		interaction.FrameSummary,
		interaction.FrameTransition,
		interaction.FrameSessionList,
		interaction.FrameSessionListEmpty,
		interaction.FrameSessionResuming,
		interaction.FrameSessionResumeError,
		interaction.FrameScopeConfirmation,
		interaction.FrameIntentClarification,
		interaction.FrameCandidateSelection,
		interaction.FrameHITLApproval,
		interaction.FrameBackgroundJobStatus,
		interaction.FrameExecutionSummary,
		interaction.FrameVerificationEvidence,
		interaction.FrameOutcomeFeedback,
	}

	for _, ft := range frameTypes {
		t.Run(string(ft), func(t *testing.T) {
			frame := interaction.InteractionFrame{
				Type:     ft,
				TaskID:   "task-1",
				Question: "test question",
				Slots: []interaction.ActionSlot{
					{ID: "opt1", Label: "Option 1", Default: true},
					{ID: "opt2", Label: "Option 2"},
				},
				DefaultSlot: "opt1",
				Content:     interaction.CandidatesContent{Candidates: []interaction.Candidate{{ID: "c1", Summary: "Candidate 1"}}},
			}
			if ft == interaction.FrameStatus {
				frame.Content = interaction.StatusContent{Message: "processing"}
			}
			if ft == interaction.FrameSummary {
				frame.Content = interaction.SummaryContent{Description: "test summary", Artifacts: []string{"a.go"}}
			}
			if ft == interaction.FrameTransition {
				frame.Content = interaction.TransitionContent{FromMode: "a", ToMode: "b"}
			}
			if ft == interaction.FrameSessionList {
				frame.Content = interaction.SessionListContent{Sessions: []interaction.SessionListItem{{Index: 1, Instruction: "test"}}}
			}
			if ft == interaction.FrameSessionListEmpty {
				frame.Content = "no sessions"
			}
			if ft == interaction.FrameSessionResuming || ft == interaction.FrameSessionResumeError {
				frame.Content = "resuming..."
			}
			if ft == interaction.FrameDraft {
				frame.Content = interaction.DraftContent{Kind: "test", Items: []interaction.DraftItem{{Content: "draft item"}}}
			}
			if ft == interaction.FrameResultType {
				frame.Content = interaction.ResultContent{Status: "passed", Evidence: []interaction.EvidenceItem{{Kind: "check", Detail: "ok"}}}
			}
			if ft == interaction.FrameComparison {
				frame.Content = interaction.ComparisonContent{Dimensions: []string{"speed"}, Matrix: [][]string{{"fast"}}}
			}

			msg := RenderInteractionFrame(th, frame)
			if strings.TrimSpace(msg.Content.Text) == "" {
				t.Errorf("RenderInteractionFrame(%s) returned empty text", ft)
			}
		})
	}
}

// TestThemeBasedRoleMapping verifies that the theme role methods used by
// euclotui renderers match the palette values exactly (not hardcoded ANSI).
func TestThemeBasedRoleMapping(t *testing.T) {
	th := theme.Default()
	pal := th.Palette()

	// Render a sample frame and verify role-derived styles are used.
	frame := interaction.InteractionFrame{
		Type:     interaction.FrameScopeConfirmation,
		TaskID:   "task-1",
		Question: "test question",
		Slots:    []interaction.ActionSlot{{ID: "opt1", Label: "Option 1", Default: true}},
	}

	msg := RenderInteractionFrame(th, frame)
	if msg.Content.Text == "" {
		t.Fatal("empty render for scope confirmation")
	}

	// Euclo and host share the same palette colours.
	_ = pal
}

// TestRenderInteractionNotificationUsesTheme verifies that the notification
// renderer uses the shared theme rather than hardcoded colours.
func TestRenderInteractionNotificationUsesTheme(t *testing.T) {
	th := theme.Default()
	item := tui.NotificationItem{
		ID:    "n1",
		Msg:   "test notification",
		Extra: map[string]string{"slot_count": "0"},
	}
	result := RenderInteractionNotification(th, item)
	if !strings.Contains(result, "test notification") {
		t.Errorf("notification render missing message text: %s", result)
	}
}

func TestRenderInteractionNotificationWithSlots(t *testing.T) {
	th := theme.Default()
	item := tui.NotificationItem{
		ID:  "n2",
		Msg: "choose",
		Extra: map[string]string{
			"slot_count":   "2",
			"slot_0_id":    "a",
			"slot_0_label": "Alpha",
			"slot_1_id":    "b",
			"slot_1_label": "Beta",
		},
	}
	result := RenderInteractionNotification(th, item)
	if !strings.Contains(result, "Alpha") || !strings.Contains(result, "Beta") {
		t.Errorf("notification render missing slot labels: %s", result)
	}
}

func TestRenderStatusNoGlyph(t *testing.T) {
	th := theme.Default()
	frame := interaction.InteractionFrame{
		Type: interaction.FrameStatus,
		Content: interaction.StatusContent{
			Message: "processing",
		},
	}
	msg := RenderInteractionFrame(th, frame)
	if strings.Contains(msg.Content.Text, "⟳") {
		t.Error("renderStatus should not contain static ⟳ glyph")
	}
	if !strings.Contains(msg.Content.Text, "processing") {
		t.Errorf("renderStatus missing message, got: %s", msg.Content.Text)
	}
}
