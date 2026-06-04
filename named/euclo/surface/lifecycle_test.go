package surface

import "testing"

func TestMacroPhaseString(t *testing.T) {
	tests := []struct {
		phase MacroPhase
		want  string
	}{
		{MacroIdle, "idle"},
		{MacroIntake, "intake"},
		{MacroRoute, "route"},
		{MacroExecute, "execute"},
		{MacroVerify, "verify"},
		{MacroDone, "done"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("MacroPhase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestMacroPhaseUnknownString(t *testing.T) {
	if got := MacroPhase(99).String(); got != "unknown" {
		t.Errorf("MacroPhase(99).String() = %q, want %q", got, "unknown")
	}
}

func TestMacroPhaseOrdering(t *testing.T) {
	ordered := []MacroPhase{MacroIdle, MacroIntake, MacroRoute, MacroExecute, MacroVerify, MacroDone}
	for i, current := range ordered {
		for j, other := range ordered {
			if i < j {
				if !current.Before(other) {
					t.Errorf("%s.Before(%s) = false, want true", current, other)
				}
				if other.After(current) != true {
					t.Errorf("%s.After(%s) = false, want true", other, current)
				}
			} else if i == j {
				if current.Before(other) {
					t.Errorf("%s.Before(%s) = true, want false (same phase)", current, other)
				}
				if current.After(other) {
					t.Errorf("%s.After(%s) = true, want false (same phase)", current, other)
				}
			} else {
				if current.Before(other) {
					t.Errorf("%s.Before(%s) = true, want false", current, other)
				}
				if !other.Before(current) {
					t.Errorf("%s.Before(%s) = false, want true", other, current)
				}
			}
		}
	}
}

func TestMacroPhaseBeforeAfterBoundaries(t *testing.T) {
	if MacroIdle.Before(MacroIntake) != true {
		t.Error("Idle should be before Intake")
	}
	if MacroDone.After(MacroVerify) != true {
		t.Error("Done should be after Verify")
	}
	if MacroIdle.Before(MacroIdle) {
		t.Error("Idle should not be before itself")
	}
}
