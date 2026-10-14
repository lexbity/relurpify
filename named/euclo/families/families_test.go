package families

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistryStartsEmpty(t *testing.T) {
	registry := NewRegistry()
	assert.Empty(t, registry.All())
	_, found := registry.Lookup("debug")
	assert.False(t, found, "a fresh registry holds no families")
	assert.Nil(t, registry.SignalWeightsFor("debug"))
}

func TestRegisterRejectsDuplicateID(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(KeywordFamily{ID: "debug", DisplayName: "Debug"}))

	err := registry.Register(KeywordFamily{ID: "debug", DisplayName: "Debug II"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `family "debug" already registered`)

	family, found := registry.Lookup("debug")
	require.True(t, found)
	assert.Equal(t, "Debug", family.DisplayName, "the first registration wins")
}

func TestOverrideRequiresExistingFamily(t *testing.T) {
	registry := NewRegistry()
	err := registry.Override("missing", FamilyOverride{AddKeywords: []string{"x"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `family "missing" not found`)
}

func TestLookupAppliesOverridePatches(t *testing.T) {
	base := KeywordFamily{
		ID:                  "debug",
		DisplayName:         "Debug",
		Keywords:            []string{"fix", "trace"},
		IntentKeywords:      []string{"panic"},
		DefaultHITLPolicy:   HITLPolicyAsk,
		DefaultVerification: VerificationRequired,
	}
	registry := NewRegistry()
	require.NoError(t, registry.Register(base))

	t.Run("without override the base family is returned", func(t *testing.T) {
		family, found := registry.Lookup("debug")
		require.True(t, found)
		assert.Equal(t, base, family)
	})

	never := HITLPolicyNever
	configurable := VerificationConfigurable
	require.NoError(t, registry.Override("debug", FamilyOverride{
		AddKeywords:         []string{"diagnose"},
		RemoveKeywords:      []string{"trace"},
		AddIntentKeywords:   []string{"failing test"},
		ReplaceHITLPolicy:   &never,
		ReplaceVerification: &configurable,
		SignalWeights:       map[string]float64{"error_text": 2.0},
	}))

	family, found := registry.Lookup("debug")
	require.True(t, found)
	assert.Equal(t, []string{"fix", "diagnose"}, family.Keywords)
	assert.Equal(t, []string{"panic", "failing test"}, family.IntentKeywords)
	assert.Equal(t, HITLPolicyNever, family.DefaultHITLPolicy)
	assert.Equal(t, VerificationConfigurable, family.DefaultVerification)
	assert.Equal(t, "Debug", family.DisplayName, "override never touches identity fields")

	t.Run("base family stored in the registry is untouched", func(t *testing.T) {
		raw, found := registry.Lookup("debug")
		require.True(t, found)
		assert.Equal(t, []string{"fix", "diagnose"}, raw.Keywords)
	})
}

func TestLookupMissingFamily(t *testing.T) {
	registry := NewRegistry()
	family, found := registry.Lookup("nope")
	assert.False(t, found)
	assert.Equal(t, KeywordFamily{}, family)
}

func TestAllReturnsFamiliesSortedByID(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(KeywordFamily{ID: "review", DisplayName: "Review"}))
	require.NoError(t, registry.Register(KeywordFamily{ID: "debug", DisplayName: "Debug"}))
	require.NoError(t, registry.Register(KeywordFamily{ID: "repair", DisplayName: "Repair"}))

	all := registry.All()
	require.Len(t, all, 3)
	assert.Equal(t, []string{"debug", "repair", "review"},
		[]string{all[0].ID, all[1].ID, all[2].ID})
}

func TestSignalWeightsForReturnsOverrideWeights(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(KeywordFamily{ID: "debug"}))

	weights := map[string]float64{"keyword:debug": 0.0, "error_text": 1.5}
	require.NoError(t, registry.Override("debug", FamilyOverride{SignalWeights: weights}))

	assert.Equal(t, weights, registry.SignalWeightsFor("debug"))
	assert.Nil(t, registry.SignalWeightsFor("repair"), "families without overrides report nil weights")
}

func TestFilterStringsRemovesOnlyListed(t *testing.T) {
	// filterStrings is the engine behind RemoveKeywords overrides.
	result := filterStrings([]string{"fix", "trace", "panic"}, []string{"trace", "absent"})
	assert.Equal(t, []string{"fix", "panic"}, result)
	assert.Empty(t, filterStrings([]string{"fix"}, []string{"fix"}))
	assert.Equal(t, []string{"fix"}, filterStrings([]string{"fix"}, nil))
}

func TestRegisterBuiltinsLoadsNineFamilies(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, RegisterBuiltins(registry))

	wantIDs := []string{
		FamilyArchitecture, FamilyDebug, FamilyImplementation, FamilyInvestigation,
		FamilyMigration, FamilyPlanning, FamilyRefactor, FamilyRepair, FamilyReview,
	}
	all := registry.All()
	require.Len(t, all, len(wantIDs))
	gotIDs := make([]string, 0, len(all))
	for _, family := range all {
		gotIDs = append(gotIDs, family.ID)
	}
	assert.Equal(t, wantIDs, gotIDs)

	// Re-registering on a populated registry must fail loudly.
	err := RegisterBuiltins(registry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegisterBuiltinsFamiliesAreWellFormed(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, RegisterBuiltins(registry))

	validPolicies := map[HITLPolicy]bool{HITLPolicyNever: true, HITLPolicyAsk: true, HITLPolicyAlways: true}
	validVerification := map[VerificationReq]bool{
		VerificationRequired: true, VerificationNotRequired: true, VerificationConfigurable: true,
	}

	for _, family := range registry.All() {
		assert.NotEmptyf(t, family.DisplayName, "family %s needs a display name", family.ID)
		assert.NotEmptyf(t, family.Keywords, "family %s needs keywords", family.ID)
		assert.NotEmptyf(t, family.IntentKeywords, "family %s needs intent keywords", family.ID)
		assert.NotEmptyf(t, family.RetrievalTemplate, "family %s needs a retrieval template", family.ID)
		assert.Containsf(t, family.RetrievalTemplate, "{{.Instruction}}",
			"family %s retrieval template must interpolate the instruction", family.ID)
		assert.Truef(t, validPolicies[family.DefaultHITLPolicy],
			"family %s has unknown HITL policy %q", family.ID, family.DefaultHITLPolicy)
		assert.Truef(t, validVerification[family.DefaultVerification],
			"family %s has unknown verification requirement %q", family.ID, family.DefaultVerification)
	}
}

func TestRegisterBuiltinsFallbackTargets(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, RegisterBuiltins(registry))

	withFallback := map[string]string{
		FamilyDebug:          "euclo:cap.bisect",
		FamilyRepair:         "euclo:cap.targeted_refactor",
		FamilyReview:         "euclo:cap.code_review",
		FamilyImplementation: "euclo:cap.targeted_refactor",
		FamilyRefactor:       "euclo:cap.targeted_refactor",
		FamilyMigration:      "euclo:cap.api_compat",
		FamilyInvestigation:  "euclo:cap.symbol_trace",
	}
	for id, want := range withFallback {
		family, found := registry.Lookup(id)
		require.Truef(t, found, "family %s missing", id)
		assert.Equalf(t, want, family.FallbackCapability, "family %s fallback changed", id)
	}

	// Planning and architecture deliberately ship without a fallback target.
	for _, id := range []string{FamilyPlanning, FamilyArchitecture} {
		family, found := registry.Lookup(id)
		require.Truef(t, found, "family %s missing", id)
		assert.Emptyf(t, family.FallbackCapability, "family %s should have no fallback", id)
	}
}

func TestSelectFamiliesScoresKeywordsAndIntentKeywords(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(KeywordFamily{
		ID:             "light",
		Keywords:       []string{"fix"},
		IntentKeywords: []string{"quick fix"},
	}))
	require.NoError(t, registry.Register(KeywordFamily{
		ID:             "heavy",
		Keywords:       []string{"fix", "patch"},
		IntentKeywords: []string{"quick fix"},
	}))

	result := SelectFamilies(registry, SelectionRequest{
		Keywords:       []string{"fix", "patch"},
		IntentKeywords: []string{"quick fix"},
	})

	require.Len(t, result.Families, 2)
	assert.Equal(t, "heavy", result.Families[0].Family.ID)
	assert.InDelta(t, 4.0, result.Families[0].Score, 1e-9, "two keyword hits (1.0 each) plus one intent hit (2.0)")
	assert.Equal(t, "light", result.Families[1].Family.ID)
	assert.InDelta(t, 3.0, result.Families[1].Score, 1e-9, "one keyword hit plus one intent hit")
}

func TestSelectFamiliesSkipsZeroScoresAndLimitsResults(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(KeywordFamily{ID: "debug", Keywords: []string{"fix"}}))
	require.NoError(t, registry.Register(KeywordFamily{ID: "review", Keywords: []string{"audit"}}))

	empty := SelectFamilies(registry, SelectionRequest{Keywords: []string{"unrelated"}})
	assert.Empty(t, empty.Families, "families without keyword overlap are not selected")

	limited := SelectFamilies(registry, SelectionRequest{
		Keywords:    []string{"fix", "audit"},
		MaxFamilies: 1,
	})
	require.Len(t, limited.Families, 1)
	assert.Equal(t, "debug", limited.Families[0].Family.ID, "higher score survives the cap")

	uncapped := SelectFamilies(registry, SelectionRequest{Keywords: []string{"fix", "audit"}})
	assert.Len(t, uncapped.Families, 2, "MaxFamilies<=0 selects every matching family")
}

func TestSelectFamiliesBreaksScoreTiesByID(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(KeywordFamily{ID: "zeta", Keywords: []string{"fix"}}))
	require.NoError(t, registry.Register(KeywordFamily{ID: "alpha", Keywords: []string{"fix"}}))
	require.NoError(t, registry.Register(KeywordFamily{ID: "mid", Keywords: []string{"fix"}}))

	result := SelectFamilies(registry, SelectionRequest{Keywords: []string{"fix"}})
	require.Len(t, result.Families, 3)
	got := []string{result.Families[0].Family.ID, result.Families[1].Family.ID, result.Families[2].Family.ID}
	assert.Equal(t, []string{"alpha", "mid", "zeta"}, got, "equal scores order by family ID")
}

func TestSelectFamiliesAppliesOverridesBeforeScoring(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, RegisterBuiltins(registry))

	// Move the "fix" keyword from debug to repair via overrides; scoring must
	// see the patched keyword sets, not the registered base families.
	require.NoError(t, registry.Override(FamilyDebug, FamilyOverride{RemoveKeywords: []string{"fix"}}))
	require.NoError(t, registry.Override(FamilyRepair, FamilyOverride{AddKeywords: []string{"fix"}}))

	result := SelectFamilies(registry, SelectionRequest{Keywords: []string{"fix"}})

	for _, scored := range result.Families {
		assert.NotEqual(t, FamilyDebug, scored.Family.ID, "family stripped of all matching keywords must not be selected")
	}
	require.NotEmpty(t, result.Families)
	assert.Equal(t, FamilyRepair, result.Families[0].Family.ID,
		"the family that gained the keyword via override wins the keyword score")
}

func TestSelectFamiliesNilRegistryReturnsEmpty(t *testing.T) {
	result := SelectFamilies(nil, SelectionRequest{Keywords: []string{"fix"}})
	assert.Empty(t, result.Families)
}

func TestFamilySelectionTypesCarryFields(t *testing.T) {
	selection := FamilySelection{
		WinningFamily: FamilyDebug,
		Candidates:    []FamilyCandidate{{FamilyID: FamilyDebug, Score: 3}, {FamilyID: FamilyRepair, Score: 1}},
		Confidence:    0.75,
		Ambiguous:     false,
	}
	assert.Equal(t, FamilyDebug, selection.WinningFamily)
	assert.Len(t, selection.Candidates, 2)
	assert.InDelta(t, 0.75, selection.Confidence, 1e-9)
	assert.False(t, selection.Ambiguous)
}
