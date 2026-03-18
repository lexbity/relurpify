package interaction

import "testing"

func TestBuildHelpFrame(t *testing.T) {
	resolver := NewAgencyResolver()
	resolver.RegisterTrigger("code", AgencyTrigger{
		Phrases:     []string{"verify"},
		Description: "Re-run verification",
	})
	resolver.RegisterTrigger("code", AgencyTrigger{
		Phrases:     []string{"debug this"},
		Description: "Switch to debug mode",
	})

	phaseMap := NewModePhaseMap()
	phaseMap.Register("code", []PhaseInfo{
		{ID: "understand", Label: "Understand"},
		{ID: "propose", Label: "Propose"},
		{ID: "execute", Label: "Execute"},
		{ID: "verify", Label: "Verify"},
		{ID: "present", Label: "Present"},
	})

	transitions := DefaultModeTransitions()

	frame := BuildHelpFrame("code", "execute", resolver, phaseMap, transitions)

	if frame.Kind != FrameHelp {
		t.Errorf("kind: got %q, want help", frame.Kind)
	}
	if frame.Mode != "code" {
		t.Errorf("mode: got %q, want code", frame.Mode)
	}
	if frame.Phase != "execute" {
		t.Errorf("phase: got %q, want execute", frame.Phase)
	}

	content, ok := frame.Content.(HelpContent)
	if !ok {
		t.Fatal("expected HelpContent")
	}
	if content.Mode != "code" {
		t.Errorf("content.mode: got %q", content.Mode)
	}
	if content.CurrentPhase != "execute" {
		t.Errorf("content.current_phase: got %q", content.CurrentPhase)
	}

	// Phase map should have current marker on execute.
	if len(content.PhaseMap) != 5 {
		t.Fatalf("phase_map: got %d, want 5", len(content.PhaseMap))
	}
	for _, p := range content.PhaseMap {
		if p.ID == "execute" && !p.Current {
			t.Error("execute phase should be marked current")
		}
		if p.ID != "execute" && p.Current {
			t.Errorf("phase %q should not be marked current", p.ID)
		}
	}

	// Available actions from resolver.
	if len(content.AvailableActions) != 2 {
		t.Errorf("actions: got %d, want 2", len(content.AvailableActions))
	}

	// Available transitions.
	if len(content.AvailableTransitions) != 3 {
		t.Errorf("transitions: got %d, want 3", len(content.AvailableTransitions))
	}
}

func TestBuildHelpFrame_NilComponents(t *testing.T) {
	frame := BuildHelpFrame("code", "verify", nil, nil, nil)

	if frame.Kind != FrameHelp {
		t.Errorf("kind: got %q, want help", frame.Kind)
	}

	content, ok := frame.Content.(HelpContent)
	if !ok {
		t.Fatal("expected HelpContent")
	}
	if len(content.PhaseMap) != 0 {
		t.Errorf("phase_map: got %d, want 0", len(content.PhaseMap))
	}
	if len(content.AvailableActions) != 0 {
		t.Errorf("actions: got %d, want 0", len(content.AvailableActions))
	}
	if len(content.AvailableTransitions) != 0 {
		t.Errorf("transitions: got %d, want 0", len(content.AvailableTransitions))
	}
}

func TestDefaultModeTransitions(t *testing.T) {
	transitions := DefaultModeTransitions()

	tests := []struct {
		mode  string
		count int
	}{
		{"code", 3},
		{"debug", 2},
		{"planning", 1},
		{"tdd", 2},
		{"review", 2},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := transitions.Get(tt.mode)
			if len(got) != tt.count {
				t.Errorf("transitions for %q: got %d, want %d", tt.mode, len(got), tt.count)
			}
		})
	}
}

func TestModePhaseMap(t *testing.T) {
	m := NewModePhaseMap()

	if m.Get("code") != nil {
		t.Error("empty map should return nil")
	}

	m.Register("code", []PhaseInfo{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B"},
	})

	phases := m.Get("code")
	if len(phases) != 2 {
		t.Errorf("phases: got %d, want 2", len(phases))
	}
}

func TestModeTransitions(t *testing.T) {
	mt := NewModeTransitions()

	if mt.Get("code") != nil {
		t.Error("empty transitions should return nil")
	}

	mt.Register("code", []TransitionInfo{
		{Phrase: "debug", TargetMode: "debug"},
	})

	got := mt.Get("code")
	if len(got) != 1 {
		t.Errorf("transitions: got %d, want 1", len(got))
	}
}

func TestRegisterHelpTriggers(t *testing.T) {
	resolver := NewAgencyResolver()
	RegisterHelpTriggers(resolver)

	// Help triggers are global (empty mode).
	trigger, ok := resolver.Resolve("code", "help")
	if !ok {
		t.Fatal("expected help trigger")
	}
	if trigger.Description == "" {
		t.Error("help trigger should have description")
	}

	// Should also work in other modes.
	trigger, ok = resolver.Resolve("debug", "what can I do")
	if !ok {
		t.Fatal("expected help trigger in debug mode")
	}
}

func TestRegisterHelpTriggers_NilResolver(t *testing.T) {
	RegisterHelpTriggers(nil) // should not panic
}

func TestHelpTriggerPhrases(t *testing.T) {
	if len(HelpTriggerPhrases) != 3 {
		t.Errorf("trigger phrases: got %d, want 3", len(HelpTriggerPhrases))
	}
}
