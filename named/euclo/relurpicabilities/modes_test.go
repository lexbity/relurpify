package relurpicabilities

import (
	"testing"
)

// TestAllModesHaveDescriptors verifies every mode returned by AllModes()
// has at least one descriptor registered in DefaultRegistry() with that mode in ModeFamilies.
// Note: Currently only debug, planning, and chat have dedicated descriptors.
// review, code, and tdd modes reuse capabilities from other modes.
func TestAllModesHaveDescriptors(t *testing.T) {
	reg := DefaultRegistry()

	// Modes that must have dedicated descriptors in DefaultRegistry()
	requiredModes := []string{ModeDebug, ModePlanning, ModeChat}

	for _, mode := range requiredModes {
		hasDescriptor := false
		for _, desc := range reg.IDs() {
			d, _ := reg.Lookup(desc)
			if containsString(d.ModeFamilies, mode) {
				hasDescriptor = true
				break
			}
		}
		if !hasDescriptor {
			t.Errorf("mode %q has no descriptors registered in DefaultRegistry()", mode)
		}
	}
}

// TestIsValidMode validates the mode validation function.
func TestIsValidMode(t *testing.T) {
	tests := []struct {
		mode     string
		expected bool
	}{
		{ModeDebug, true},
		{ModeReview, true},
		{ModePlanning, true},
		{ModeTDD, true},
		{ModeCode, true},
		{ModeChat, true},
		{"invalid", false},
		{"", false},
		{"DEBUG", false}, // case-sensitive
		{"Chat", false},  // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := IsValidMode(tt.mode)
			if got != tt.expected {
				t.Errorf("IsValidMode(%q) = %v, want %v", tt.mode, got, tt.expected)
			}
		})
	}
}

// TestAllModesReturnsExpectedModes verifies AllModes returns modes in expected order.
func TestAllModesReturnsExpectedModes(t *testing.T) {
	expected := []string{ModeDebug, ModeReview, ModePlanning, ModeTDD, ModeCode, ModeChat}
	got := AllModes()

	if len(got) != len(expected) {
		t.Fatalf("AllModes() returned %d modes, want %d", len(got), len(expected))
	}

	for i, mode := range expected {
		if got[i] != mode {
			t.Errorf("AllModes()[%d] = %q, want %q", i, got[i], mode)
		}
	}
}

// TestPrimaryModeAccessor verifies the PrimaryMode() helper works correctly.
func TestPrimaryModeAccessor(t *testing.T) {
	tests := []struct {
		name        string
		families    []string
		wantPrimary string
	}{
		{
			name:        "single mode",
			families:    []string{"debug"},
			wantPrimary: "debug",
		},
		{
			name:        "multiple modes",
			families:    []string{"debug", "review", "chat"},
			wantPrimary: "debug",
		},
		{
			name:        "empty families",
			families:    []string{},
			wantPrimary: "",
		},
		{
			name:        "nil families",
			families:    nil,
			wantPrimary: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Descriptor{ModeFamilies: tt.families}
			got := d.PrimaryMode()
			if got != tt.wantPrimary {
				t.Errorf("PrimaryMode() = %q, want %q", got, tt.wantPrimary)
			}
		})
	}
}
