package contextmgr

import "testing"

func TestStrategyProfilesMatchExpectedValues(t *testing.T) {
	tests := []struct {
		name    string
		profile StrategyProfile
		check   func(*testing.T, StrategyProfile)
	}{
		{
			name:    "aggressive",
			profile: AggressiveProfile,
			check: func(t *testing.T, got StrategyProfile) {
				if got.Name != "aggressive" {
					t.Fatalf("unexpected name: %q", got.Name)
				}
				if got.CompressThreshold != 5 {
					t.Fatalf("unexpected compress threshold: %d", got.CompressThreshold)
				}
				if got.PrioritizationMode != PrioritizationRecency {
					t.Fatalf("unexpected prioritization mode: %v", got.PrioritizationMode)
				}
				if got.TokenBudgetFraction != 0.25 {
					t.Fatalf("unexpected token budget fraction: %v", got.TokenBudgetFraction)
				}
				if !got.ASTExportedOnly {
					t.Fatal("expected ASTExportedOnly to be true")
				}
				if got.FileDetailLevel != DetailSignatureOnly {
					t.Fatalf("unexpected file detail level: %v", got.FileDetailLevel)
				}
				if got.FilePinned {
					t.Fatal("expected FilePinned to be false")
				}
				if got.SearchMaxResults != 0 {
					t.Fatalf("unexpected search max results: %d", got.SearchMaxResults)
				}
				if got.LoadMemory {
					t.Fatal("expected LoadMemory to be false")
				}
				if got.ExpandTrigger != ExpandOnErrorType {
					t.Fatalf("unexpected expand trigger: %v", got.ExpandTrigger)
				}
			},
		},
		{
			name:    "conservative",
			profile: ConservativeProfile,
			check: func(t *testing.T, got StrategyProfile) {
				if got.Name != "conservative" {
					t.Fatalf("unexpected name: %q", got.Name)
				}
				if got.CompressThreshold != 15 {
					t.Fatalf("unexpected compress threshold: %d", got.CompressThreshold)
				}
				if got.PrioritizationMode != PrioritizationRelevance {
					t.Fatalf("unexpected prioritization mode: %v", got.PrioritizationMode)
				}
				if got.TokenBudgetFraction != 0.75 {
					t.Fatalf("unexpected token budget fraction: %v", got.TokenBudgetFraction)
				}
				if got.ASTExportedOnly {
					t.Fatal("expected ASTExportedOnly to be false")
				}
				if got.FileDetailLevel != DetailDetailed {
					t.Fatalf("unexpected file detail level: %v", got.FileDetailLevel)
				}
				if !got.FilePinned {
					t.Fatal("expected FilePinned to be true")
				}
				if !got.LoadDependencies {
					t.Fatal("expected LoadDependencies to be true")
				}
				if got.SearchMaxResults != 20 {
					t.Fatalf("unexpected search max results: %d", got.SearchMaxResults)
				}
				if !got.SearchUseFullInstruction {
					t.Fatal("expected SearchUseFullInstruction to be true")
				}
				if !got.LoadMemory {
					t.Fatal("expected LoadMemory to be true")
				}
				if got.MemoryMaxResults != 10 {
					t.Fatalf("unexpected memory max results: %d", got.MemoryMaxResults)
				}
				if got.ExpandTrigger != ExpandOnToolUse {
					t.Fatalf("unexpected expand trigger: %v", got.ExpandTrigger)
				}
			},
		},
		{
			name:    "balanced",
			profile: BalancedProfile,
			check: func(t *testing.T, got StrategyProfile) {
				if got.Name != "balanced" {
					t.Fatalf("unexpected name: %q", got.Name)
				}
				if got.CompressThreshold != 10 {
					t.Fatalf("unexpected compress threshold: %d", got.CompressThreshold)
				}
				if got.PrioritizationMode != PrioritizationWeighted {
					t.Fatalf("unexpected prioritization mode: %v", got.PrioritizationMode)
				}
				if got.RelevanceWeight != 0.6 || got.RecencyWeight != 0.4 {
					t.Fatalf("unexpected weighting: relevance=%v recency=%v", got.RelevanceWeight, got.RecencyWeight)
				}
				if got.TokenBudgetFraction != 0.5 {
					t.Fatalf("unexpected token budget fraction: %v", got.TokenBudgetFraction)
				}
				if !got.ASTExportedOnly {
					t.Fatal("expected ASTExportedOnly to be true")
				}
				if got.FileDetailLevel != DetailConcise {
					t.Fatalf("unexpected file detail level: %v", got.FileDetailLevel)
				}
				if got.PinFirstN != 2 {
					t.Fatalf("unexpected pin first n: %d", got.PinFirstN)
				}
				if got.SearchMaxResults != 10 {
					t.Fatalf("unexpected search max results: %d", got.SearchMaxResults)
				}
				if got.LoadMemory {
					t.Fatal("expected LoadMemory to be false")
				}
				if got.ExpandTrigger != ExpandOnFailureOrUncertainty {
					t.Fatalf("unexpected expand trigger: %v", got.ExpandTrigger)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, tc.profile)
		})
	}
}

func TestValidateProfileBands(t *testing.T) {
	for name, profile := range map[string]StrategyProfile{
		"aggressive":   AggressiveProfile,
		"conservative": ConservativeProfile,
		"balanced":     BalancedProfile,
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProfileBands(profile); err != nil {
				t.Fatalf("ValidateProfileBands(%s): %v", name, err)
			}
		})
	}
}

func TestLookupProfileRegistry(t *testing.T) {
	cases := map[string]StrategyProfile{
		"aggressive":           AggressiveProfile,
		"balanced":             BalancedProfile,
		"conservative":         ConservativeProfile,
		"narrow_to_wide":       ConservativeProfile,
		"localize_then_expand": BalancedProfile,
		"targeted":             AggressiveProfile,
		"read_heavy":           ConservativeProfile,
		"expand_carefully":     BalancedProfile,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := LookupProfile(name)
			if !ok {
				t.Fatalf("expected profile %q to be registered", name)
			}
			if got.Name != want.Name {
				t.Fatalf("unexpected profile for %q: got %q want %q", name, got.Name, want.Name)
			}
		})
	}
}

func TestLookupProfileUnknown(t *testing.T) {
	if _, ok := LookupProfile("does-not-exist"); ok {
		t.Fatal("expected unknown profile lookup to fail")
	}
}

func TestValidateProfileBandsRejectsMisorderedBands(t *testing.T) {
	profile := StrategyProfile{
		DetailBands: []DetailBand{
			{MinRelevance: 0.5, Level: DetailDetailed},
			{MinRelevance: 0.9, Level: DetailFull},
		},
	}
	if err := ValidateProfileBands(profile); err == nil {
		t.Fatal("expected misordered bands to be rejected")
	}
}

func TestZeroValueStrategyProfileIsSafe(t *testing.T) {
	var profile StrategyProfile
	if profile.Name != "" || profile.CompressThreshold != 0 || profile.DetailBands != nil {
		t.Fatalf("unexpected zero value profile: %#v", profile)
	}
	if err := ValidateProfileBands(profile); err != nil {
		t.Fatalf("ValidateProfileBands(zero): %v", err)
	}
}
