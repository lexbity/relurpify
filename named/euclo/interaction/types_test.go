package interaction

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFrameKindConstants(t *testing.T) {
	kinds := []FrameKind{
		FrameProposal, FrameQuestion, FrameCandidates, FrameComparison,
		FrameDraft, FrameResult, FrameStatus, FrameSummary,
		FrameTransition, FrameHelp,
	}
	seen := make(map[FrameKind]bool)
	for _, k := range kinds {
		if k == "" {
			t.Error("empty FrameKind constant")
		}
		if seen[k] {
			t.Errorf("duplicate FrameKind: %s", k)
		}
		seen[k] = true
	}
}

func TestActionKindConstants(t *testing.T) {
	kinds := []ActionKind{
		ActionConfirm, ActionSelect, ActionFreetext,
		ActionToggle, ActionBatch, ActionTransition,
	}
	seen := make(map[ActionKind]bool)
	for _, k := range kinds {
		if k == "" {
			t.Error("empty ActionKind constant")
		}
		if seen[k] {
			t.Errorf("duplicate ActionKind: %s", k)
		}
		seen[k] = true
	}
}

func TestInteractionFrame_DefaultAction(t *testing.T) {
	frame := InteractionFrame{
		Actions: []ActionSlot{
			{ID: "a", Label: "First", Kind: ActionSelect},
			{ID: "b", Label: "Second", Kind: ActionConfirm, Default: true},
			{ID: "c", Label: "Third", Kind: ActionFreetext},
		},
	}
	def := frame.DefaultAction()
	if def == nil {
		t.Fatal("expected default action")
	}
	if def.ID != "b" {
		t.Errorf("expected default action ID 'b', got %q", def.ID)
	}
}

func TestInteractionFrame_DefaultAction_None(t *testing.T) {
	frame := InteractionFrame{
		Actions: []ActionSlot{
			{ID: "a", Label: "First", Kind: ActionSelect},
		},
	}
	if def := frame.DefaultAction(); def != nil {
		t.Errorf("expected nil, got action %q", def.ID)
	}
}

func TestInteractionFrame_ActionByID(t *testing.T) {
	frame := InteractionFrame{
		Actions: []ActionSlot{
			{ID: "confirm", Label: "Confirm"},
			{ID: "skip", Label: "Skip"},
		},
	}
	a := frame.ActionByID("skip")
	if a == nil {
		t.Fatal("expected action")
	}
	if a.Label != "Skip" {
		t.Errorf("expected label 'Skip', got %q", a.Label)
	}
	if frame.ActionByID("missing") != nil {
		t.Error("expected nil for missing action")
	}
}

func TestInteractionFrame_JSONRoundTrip(t *testing.T) {
	frame := InteractionFrame{
		Kind:  FrameProposal,
		Mode:  "code",
		Phase: "understand",
		Content: ProposalContent{
			Interpretation: "Add error handling to the HTTP handler",
			Scope:          []string{"server/handler.go", "server/middleware.go"},
			Approach:       "edit_verify_repair",
			Constraints:    []string{"preserve existing tests"},
		},
		Actions: []ActionSlot{
			{ID: "confirm", Label: "Confirm", Shortcut: "y", Kind: ActionConfirm, Default: true},
			{ID: "correct", Label: "Correct", Kind: ActionFreetext},
			{ID: "plan_first", Label: "Plan first", Kind: ActionTransition, TargetPhase: "planning"},
		},
		Continuable: true,
		Metadata: FrameMetadata{
			Timestamp:    time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC),
			PhaseIndex:   0,
			PhaseCount:   5,
			ArtifactRefs: []string{"art-001"},
		},
	}

	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded InteractionFrame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Kind != FrameProposal {
		t.Errorf("kind: got %q, want %q", decoded.Kind, FrameProposal)
	}
	if decoded.Mode != "code" {
		t.Errorf("mode: got %q, want %q", decoded.Mode, "code")
	}
	if decoded.Phase != "understand" {
		t.Errorf("phase: got %q, want %q", decoded.Phase, "understand")
	}
	if len(decoded.Actions) != 3 {
		t.Errorf("actions: got %d, want 3", len(decoded.Actions))
	}
	if !decoded.Continuable {
		t.Error("continuable: got false, want true")
	}
	if decoded.Metadata.PhaseIndex != 0 {
		t.Errorf("phase_index: got %d, want 0", decoded.Metadata.PhaseIndex)
	}
	if decoded.Metadata.PhaseCount != 5 {
		t.Errorf("phase_count: got %d, want 5", decoded.Metadata.PhaseCount)
	}
}

func TestFrameMetadata_JSONRoundTrip(t *testing.T) {
	meta := FrameMetadata{
		Timestamp:    time.Date(2026, 3, 17, 10, 30, 0, 0, time.UTC),
		PhaseIndex:   2,
		PhaseCount:   6,
		ArtifactRefs: []string{"explore-001", "plan-002"},
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded FrameMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.PhaseIndex != 2 {
		t.Errorf("phase_index: got %d, want 2", decoded.PhaseIndex)
	}
	if len(decoded.ArtifactRefs) != 2 {
		t.Errorf("artifact_refs: got %d, want 2", len(decoded.ArtifactRefs))
	}
}
