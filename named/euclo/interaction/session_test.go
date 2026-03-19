package interaction

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestExtractSessionResume_NoState(t *testing.T) {
	if ExtractSessionResume(nil) != nil {
		t.Error("nil state should return nil resume")
	}
	state := core.NewContext()
	if ExtractSessionResume(state) != nil {
		t.Error("empty state should return nil resume")
	}
}

func TestExtractSessionResume_FromInteractionState(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.interaction_state", InteractionState{
		Mode:         "code",
		CurrentPhase: "propose",
		PhaseStates:  map[string]any{"scope.confirmed": true},
		Selections:   map[string]string{"intent": "fix"},
		SkippedPhases: []string{"clarify"},
	})

	resume := ExtractSessionResume(state)
	if resume == nil {
		t.Fatal("expected non-nil resume")
	}
	if resume.Mode != "code" {
		t.Errorf("mode: got %q", resume.Mode)
	}
	if resume.LastPhase != "propose" {
		t.Errorf("last phase: got %q", resume.LastPhase)
	}
	if len(resume.CompletedPhases) != 0 {
		t.Errorf("completed: got %v, want none", resume.CompletedPhases)
	}
	if len(resume.SkippedPhases) != 1 || resume.SkippedPhases[0] != "clarify" {
		t.Errorf("skipped: got %v", resume.SkippedPhases)
	}
}

func TestExtractSessionResume_FromMapAny(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.interaction_state", map[string]any{
		"mode":           "debug",
		"current_phase":  "localize",
		"phase_states":   map[string]any{"intake.done": true},
		"selections":     map[string]any{"strategy": "trace"},
		"phases_executed": []any{"intake", "reproduce"},
		"skipped_phases": []any{"reproduce"},
	})

	resume := ExtractSessionResume(state)
	if resume == nil {
		t.Fatal("expected non-nil resume")
	}
	if resume.Mode != "debug" {
		t.Errorf("mode: got %q", resume.Mode)
	}
	if resume.LastPhase != "localize" {
		t.Errorf("last phase: got %q", resume.LastPhase)
	}
	if len(resume.CompletedPhases) != 2 || resume.CompletedPhases[0] != "intake" {
		t.Errorf("completed: got %v", resume.CompletedPhases)
	}
}

func TestBuildResumeFrame(t *testing.T) {
	resume := &SessionResume{
		Mode:      "code",
		LastPhase: "propose",
	}
	frame := BuildResumeFrame(resume)
	if frame.Kind != FrameSessionResume {
		t.Errorf("kind: got %q", frame.Kind)
	}
	if frame.Phase != "resume" {
		t.Errorf("phase: got %q", frame.Phase)
	}
	content, ok := frame.Content.(QuestionContent)
	if !ok {
		t.Fatal("expected QuestionContent")
	}
	if len(content.Options) != 3 {
		t.Errorf("options: got %d", len(content.Options))
	}
	// First action should be default.
	if !frame.Actions[0].Default {
		t.Error("expected first action to be default")
	}
}

func TestHandleResumeResponse(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"resume", "resume"},
		{"restart", "restart"},
		{"switch", "switch"},
		{"unknown", "resume"},
		{"", "resume"},
	}
	for _, tc := range cases {
		got := HandleResumeResponse(UserResponse{ActionID: tc.input})
		if got != tc.expected {
			t.Errorf("HandleResumeResponse(%q): got %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestApplySessionResume(t *testing.T) {
	machine := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "code",
		Emitter: &NoopEmitter{},
		Phases: []PhaseDefinition{
			{ID: "scope"},
			{ID: "propose"},
			{ID: "commit"},
		},
	})
	resume := &SessionResume{
		Mode:            "code",
		LastPhase:       "propose",
		CompletedPhases: []string{"scope"},
		PhaseStates:     map[string]any{"scope.confirmed": true},
	}
	ApplySessionResume(machine, resume)
	if machine.State()["session.resumed"] != true {
		t.Error("expected session.resumed = true")
	}
	if machine.State()["session.last_phase"] != "propose" {
		t.Error("expected session.last_phase = propose")
	}
	if machine.State()["scope.confirmed"] != true {
		t.Error("expected phase state restored")
	}
	if machine.CurrentPhase() != "propose" {
		t.Errorf("expected current phase propose, got %q", machine.CurrentPhase())
	}
}
