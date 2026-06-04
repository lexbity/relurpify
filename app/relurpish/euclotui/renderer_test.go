package euclotui

import (
	"strings"
	"testing"

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
			result := RenderInteractionFrame(tc.frame)
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
	result := RenderInteractionFrame(frame)
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
	result := RenderInteractionFrame(frame)
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
	result := RenderInteractionFrame(frame)
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
			result := RenderInteractionFrame(frame)
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
	result := RenderInteractionFrame(frame)
	if result.Content.Text == "" {
		t.Error("expected non-empty text for minimal frame")
	}

	// Slots populated but empty.
	frame2 := interaction.InteractionFrame{
		Type:   interaction.FrameCandidateSelection,
		TaskID: "task-1",
		Slots:  []interaction.ActionSlot{},
	}
	result2 := RenderInteractionFrame(frame2)
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
	result3 := RenderInteractionFrame(frame3)
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
	result4 := RenderInteractionFrame(frame4)
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
	result5 := RenderInteractionFrame(frame5)
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
		result := RenderInteractionFrame(frame)
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
	result := RenderInteractionFrame(frame)
	if !strings.Contains(result.Content.Text, "working on it") {
		t.Errorf("expected status message in output, got: %s", result.Content.Text)
	}
}
