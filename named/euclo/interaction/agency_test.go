package interaction

import "testing"

func TestAgencyResolver_ExactMatch(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:      []string{"verify", "verify this"},
		CapabilityID: "euclo:verify.change",
		PhaseJump:    "verify",
		Description:  "Re-run verification",
	})

	trigger, ok := r.Resolve("code", "verify")
	if !ok {
		t.Fatal("expected match")
	}
	if trigger.PhaseJump != "verify" {
		t.Errorf("phase_jump: got %q, want %q", trigger.PhaseJump, "verify")
	}
}

func TestAgencyResolver_CaseInsensitive(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:   []string{"debug this"},
		PhaseJump: "debug",
	})

	trigger, ok := r.Resolve("code", "Debug This")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
	if trigger.PhaseJump != "debug" {
		t.Errorf("phase_jump: got %q", trigger.PhaseJump)
	}
}

func TestAgencyResolver_FuzzyContains(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("planning", AgencyTrigger{
		Phrases:     []string{"alternatives"},
		Description: "Show more alternatives",
	})

	trigger, ok := r.Resolve("planning", "show me some alternatives please")
	if !ok {
		t.Fatal("expected fuzzy match")
	}
	if trigger.Description != "Show more alternatives" {
		t.Errorf("description: got %q", trigger.Description)
	}
}

func TestAgencyResolver_ModeFiltering(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:   []string{"verify"},
		PhaseJump: "verify",
	})
	r.RegisterTrigger("debug", AgencyTrigger{
		Phrases:   []string{"investigate"},
		PhaseJump: "localize",
	})

	// "verify" should not match in debug mode.
	_, ok := r.Resolve("debug", "verify")
	if ok {
		t.Error("verify should not match in debug mode")
	}

	// "investigate" should match in debug mode.
	trigger, ok := r.Resolve("debug", "investigate")
	if !ok {
		t.Fatal("expected match in debug mode")
	}
	if trigger.PhaseJump != "localize" {
		t.Errorf("phase_jump: got %q", trigger.PhaseJump)
	}
}

func TestAgencyResolver_RequiresMode(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("", AgencyTrigger{
		Phrases:      []string{"help"},
		RequiresMode: "code",
		Description:  "Code help",
	})

	_, ok := r.Resolve("debug", "help")
	if ok {
		t.Error("should not match when RequiresMode doesn't match")
	}

	trigger, ok := r.Resolve("code", "help")
	if !ok {
		t.Fatal("expected match when RequiresMode matches")
	}
	if trigger.Description != "Code help" {
		t.Errorf("description: got %q", trigger.Description)
	}
}

func TestAgencyResolver_EmptyText(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases: []string{"verify"},
	})

	_, ok := r.Resolve("code", "")
	if ok {
		t.Error("empty text should not match")
	}

	_, ok = r.Resolve("code", "   ")
	if ok {
		t.Error("whitespace-only text should not match")
	}
}

func TestAgencyResolver_NoTriggers(t *testing.T) {
	r := NewAgencyResolver()
	_, ok := r.Resolve("code", "verify")
	if ok {
		t.Error("empty resolver should not match")
	}
}

func TestAgencyResolver_TriggersForMode(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:     []string{"verify"},
		Description: "Run verification",
	})
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:     []string{"debug this"},
		Description: "Switch to debug",
	})
	r.RegisterTrigger("debug", AgencyTrigger{
		Phrases:     []string{"investigate"},
		Description: "Investigate deeper",
	})
	r.RegisterTrigger("", AgencyTrigger{
		Phrases:     []string{"help"},
		Description: "Show help",
	})

	codeTriggers := r.TriggersForMode("code")
	if len(codeTriggers) != 3 { // 2 code-specific + 1 global
		t.Errorf("code triggers: got %d, want 3", len(codeTriggers))
	}

	debugTriggers := r.TriggersForMode("debug")
	if len(debugTriggers) != 2 { // 1 debug-specific + 1 global
		t.Errorf("debug triggers: got %d, want 2", len(debugTriggers))
	}
}

func TestAgencyResolver_ExactMatchPriority(t *testing.T) {
	r := NewAgencyResolver()
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:     []string{"verify"},
		Description: "exact verify",
	})
	r.RegisterTrigger("code", AgencyTrigger{
		Phrases:     []string{"verify all"},
		Description: "verify all",
	})

	// Exact match for "verify" should win over fuzzy match for "verify all".
	trigger, ok := r.Resolve("code", "verify")
	if !ok {
		t.Fatal("expected match")
	}
	if trigger.Description != "exact verify" {
		t.Errorf("expected exact match, got %q", trigger.Description)
	}
}
