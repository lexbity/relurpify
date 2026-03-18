package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestRenderInteractionFrame_Proposal(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "code",
		Phase: "intent",
		Content: interaction.ProposalContent{
			Interpretation: "Fix the authentication bug",
			Scope:          []string{"auth.go", "auth_test.go"},
			Approach:       "edit_verify",
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if msg.Role != RoleAgent {
		t.Errorf("role: got %q", msg.Role)
	}
	if !strings.Contains(msg.Content.Text, "Proposal") {
		t.Error("expected Proposal header")
	}
	if !strings.Contains(msg.Content.Text, "Fix the authentication bug") {
		t.Error("expected interpretation text")
	}
	if !strings.Contains(msg.Content.Text, "auth.go") {
		t.Error("expected scope")
	}
}

func TestRenderInteractionFrame_Question(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameQuestion,
		Mode:  "planning",
		Phase: "clarify",
		Content: interaction.QuestionContent{
			Question: "Which approach do you prefer?",
			Options: []interaction.QuestionOption{
				{ID: "a", Label: "Option A", Description: "Detailed A"},
				{ID: "b", Label: "Option B"},
			},
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "Question") {
		t.Error("expected Question header")
	}
	if !strings.Contains(msg.Content.Text, "[1]") {
		t.Error("expected numbered options")
	}
	if !strings.Contains(msg.Content.Text, "Detailed A") {
		t.Error("expected option description")
	}
}

func TestRenderInteractionFrame_Result(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameResult,
		Mode:  "code",
		Phase: "verify",
		Content: interaction.ResultContent{
			Status: "passed",
			Evidence: []interaction.EvidenceItem{
				{Kind: "test", Detail: "All tests pass"},
			},
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "passed") {
		t.Error("expected passed status")
	}
	if !strings.Contains(msg.Content.Text, "All tests pass") {
		t.Error("expected evidence")
	}
}

func TestRenderInteractionFrame_Findings(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameResult,
		Mode:  "review",
		Phase: "triage",
		Content: interaction.FindingsContent{
			Critical: []interaction.Finding{{Location: "auth.go:15", Description: "SQL injection"}},
			Warning:  []interaction.Finding{{Description: "Missing check"}},
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "CRITICAL") {
		t.Error("expected CRITICAL label")
	}
	if !strings.Contains(msg.Content.Text, "SQL injection") {
		t.Error("expected finding description")
	}
}

func TestRenderInteractionFrame_Status(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  "code",
		Phase: "execute",
		Content: interaction.StatusContent{
			Message: "Applying edits...",
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "Applying edits...") {
		t.Error("expected status message")
	}
}

func TestRenderInteractionFrame_Transition(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameTransition,
		Mode:  "code",
		Phase: "verify",
		Content: interaction.TransitionContent{
			FromMode: "code",
			ToMode:   "debug",
			Reason:   "Verification failures exceeded threshold",
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "code") {
		t.Error("expected from mode")
	}
	if !strings.Contains(msg.Content.Text, "debug") {
		t.Error("expected to mode")
	}
}

func TestRenderInteractionFrame_Help(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameHelp,
		Mode:  "code",
		Phase: "execute",
		Content: interaction.HelpContent{
			Mode:         "code",
			CurrentPhase: "execute",
			PhaseMap: []interaction.PhaseInfo{
				{ID: "intent", Label: "Intent"},
				{ID: "execute", Label: "Execute", Current: true},
				{ID: "verify", Label: "Verify"},
			},
			AvailableActions: []interaction.ActionInfo{
				{Phrase: "verify", Description: "Run verification"},
			},
			AvailableTransitions: []interaction.TransitionInfo{
				{Phrase: "debug this", TargetMode: "debug"},
			},
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "Help") {
		t.Error("expected Help header")
	}
	if !strings.Contains(msg.Content.Text, "Execute") {
		t.Error("expected phase label")
	}
	if !strings.Contains(msg.Content.Text, "verify") {
		t.Error("expected action phrase")
	}
}

func TestRenderInteractionFrame_Draft(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameDraft,
		Mode:  "planning",
		Phase: "refine",
		Content: interaction.DraftContent{
			Kind: "plan",
			Items: []interaction.DraftItem{
				{ID: "1", Content: "Step one", Editable: true},
				{ID: "2", Content: "Step two", Editable: false},
			},
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "Draft") {
		t.Error("expected Draft header")
	}
	if !strings.Contains(msg.Content.Text, "Step one") {
		t.Error("expected draft item")
	}
}

func TestRenderInteractionFrame_Summary(t *testing.T) {
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameSummary,
		Mode:  "code",
		Phase: "present",
		Content: interaction.SummaryContent{
			Description: "Task completed successfully",
			Changes:     []string{"auth.go", "handler.go"},
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	msg := RenderInteractionFrame(frame)
	if !strings.Contains(msg.Content.Text, "Summary") {
		t.Error("expected Summary header")
	}
	if !strings.Contains(msg.Content.Text, "auth.go") {
		t.Error("expected changes")
	}
}

func TestRenderPhaseProgress(t *testing.T) {
	labels := []interaction.PhaseInfo{
		{ID: "intent", Label: "Intent"},
		{ID: "propose", Label: "Propose"},
		{ID: "execute", Label: "Execute"},
		{ID: "verify", Label: "Verify"},
	}
	result := RenderPhaseProgress("code", 2, 4, labels)
	if !strings.Contains(result, "[code]") {
		t.Error("expected mode label")
	}
	if !strings.Contains(result, "●Execute") {
		t.Error("expected current phase marker")
	}
}

func TestRenderPhaseProgress_NoLabels(t *testing.T) {
	result := RenderPhaseProgress("code", 1, 3, nil)
	if !strings.Contains(result, "phase 2/3") {
		t.Errorf("got %q, expected fallback format", result)
	}
}

func TestRenderActionSlots(t *testing.T) {
	actions := []interaction.ActionSlot{
		{ID: "confirm", Label: "Confirm", Shortcut: "y", Default: true},
		{ID: "skip", Label: "Skip"},
	}
	result := RenderActionSlots(actions)
	if !strings.Contains(result, "[y]") {
		t.Error("expected shortcut key")
	}
	if !strings.Contains(result, "Confirm*") {
		t.Error("expected default marker")
	}
	if !strings.Contains(result, "[2]") {
		t.Error("expected number key for second action")
	}
}

func TestRenderActionSlots_Empty(t *testing.T) {
	result := RenderActionSlots(nil)
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
