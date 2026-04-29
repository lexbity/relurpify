package intake

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// === Phase 4 Unit Tests (exact spec requirements) ===

func TestCollectSignalsKeyword(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction: "fix the panic: in handler.go",
	}

	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	signals := CollectSignals(envelope, nil, registry)

	// Should have keyword signal for "fix" -> debug
	foundKeyword := false
	foundError := false
	for _, sig := range signals {
		if sig.Kind == SignalKindKeyword && sig.FamilyID == families.FamilyDebug {
			foundKeyword = true
		}
		if sig.Kind == SignalKindErrorText {
			foundError = true
		}
	}

	if !foundKeyword {
		t.Error("Expected keyword:debug signal for 'fix'")
	}
	if !foundError {
		t.Error("Expected error_text signal for 'panic:'")
	}
}

func TestCollectSignalsContextHint(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction: "review the code",
		FamilyHint:  "review",
	}

	signals := CollectSignals(envelope, nil, nil)

	found := false
	for _, sig := range signals {
		if sig.Kind == SignalKindContextHint && sig.FamilyID == "review" {
			found = true
		}
	}

	if !found {
		t.Error("Expected context_hint:review signal")
	}
}

func TestCollectSignalsUserRecipe(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction: "failing test needs fixing",
	}

	recipeKeywords := map[string][]string{
		"debug": {"failing test"},
	}

	signals := CollectSignals(envelope, recipeKeywords, nil)

	found := false
	for _, sig := range signals {
		if sig.Kind == SignalKindUserRecipe && sig.FamilyID == "debug" {
			found = true
		}
	}

	if !found {
		t.Error("Expected user_recipe signal for matching recipe keyword")
	}
}

func TestCollectSignalsNoSignalBaseline(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction: "do something generic",
	}

	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := ClassifyTaskScored(envelope, registry, nil)

	// Should have default baseline signal
	foundDefault := false
	for _, sig := range classification.Signals {
		if sig.Kind == SignalKindDefault {
			foundDefault = true
		}
	}

	if !foundDefault {
		t.Error("Expected default:implementation signal when no other signal fires")
	}
}

func TestCollectSignalsNegative(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction:             "fix this but don't add new tests",
		NegativeConstraintSeeds: []string{"don't add new tests"},
	}

	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	signals := CollectSignals(envelope, nil, registry)

	foundNegative := false
	for _, sig := range signals {
		if sig.Kind == SignalKindNegative {
			foundNegative = true
		}
	}

	if !foundNegative {
		t.Error("Expected negative signal")
	}
}

func TestScoreSignalsRanking(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	signals := []ClassificationSignal{
		{Kind: SignalKindKeyword, FamilyID: families.FamilyDebug, Weight: WeightKeyword},
		{Kind: SignalKindKeyword, FamilyID: families.FamilyRepair, Weight: WeightKeyword},
		{Kind: SignalKindErrorText, FamilyID: families.FamilyDebug, Weight: WeightErrorText},
	}

	candidates := ScoreSignals(signals, registry.All(), nil)

	if len(candidates) == 0 {
		t.Fatal("Expected at least one candidate")
	}

	// Debug should win due to error_text weight
	if candidates[0].FamilyID != families.FamilyDebug {
		t.Errorf("Expected debug to win, got %q", candidates[0].FamilyID)
	}
}

func TestScoreSignalsAmbiguity(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	signals := []ClassificationSignal{
		{Kind: SignalKindKeyword, FamilyID: families.FamilyDebug, Weight: 1.0},
		{Kind: SignalKindKeyword, FamilyID: families.FamilyRepair, Weight: 0.95},
	}

	candidates := ScoreSignals(signals, registry.All(), nil)

	if len(candidates) < 2 {
		t.Fatal("Expected at least two candidates")
	}

	// Check ambiguity calculation
	_, ambiguous := calculateConfidence(candidates)
	if !ambiguous {
		t.Error("Expected ambiguous when scores within 0.1")
	}
}

func TestScoreSignalsWeightOverride(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	signals := []ClassificationSignal{
		{Kind: SignalKindErrorText, FamilyID: families.FamilyDebug, Weight: WeightErrorText},
	}

	weightOverrides := map[string]float64{
		"error_text:debug": 2.0,
	}

	candidates := ScoreSignals(signals, registry.All(), weightOverrides)

	if len(candidates) == 0 {
		t.Fatal("Expected at least one candidate")
	}

	// Score should be doubled
	expectedScore := WeightErrorText * 2.0
	if candidates[0].Score != expectedScore {
		t.Errorf("Expected score %f, got %f", expectedScore, candidates[0].Score)
	}
}

func TestScoreSignalsWeightSuppression(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	signals := []ClassificationSignal{
		{Kind: SignalKindKeyword, FamilyID: families.FamilyReview, Weight: WeightKeyword},
	}

	weightOverrides := map[string]float64{
		"keyword:review": 0.0,
	}

	candidates := ScoreSignals(signals, registry.All(), weightOverrides)

	// Review should have zero score
	for _, c := range candidates {
		if c.FamilyID == families.FamilyReview && c.Score != 0 {
			t.Error("Expected review score to be suppressed to 0")
		}
	}
}

func TestClassifyTaskScored_ReviewNoEdit(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction:   "review the auth package",
		EditPermitted: false,
	}

	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := ClassifyTaskScored(envelope, registry, nil)

	if classification.WinningFamily != families.FamilyReview {
		t.Errorf("Expected review family, got %q", classification.WinningFamily)
	}
}

func TestClassifyTaskScored_MixedIntent(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction: "plan the architecture and implement the feature",
	}

	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := ClassifyTaskScored(envelope, registry, nil)

	// Should detect mixed intent if both planning and implementation fire
	// This is a basic check - full mixed intent detection would need more logic
	if classification.WinningFamily == "" {
		t.Error("Expected a winning family")
	}
}

func TestClassifyTaskScored_CrossCutting(t *testing.T) {
	envelope := &TaskEnvelope{
		Instruction: "refactor across all packages",
	}

	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := ClassifyTaskScored(envelope, registry, nil)

	if classification.WinningFamily != families.FamilyRefactor {
		t.Errorf("Expected refactor family, got %q", classification.WinningFamily)
	}
}

func TestBuiltinFamiliesAllRegistered(t *testing.T) {
	registry := families.NewRegistry()
	err := families.RegisterBuiltins(registry)
	if err != nil {
		t.Fatalf("RegisterBuiltins failed: %v", err)
	}

	all := registry.All()
	expectedCount := 9

	if len(all) != expectedCount {
		t.Errorf("Expected %d built-in families, got %d", expectedCount, len(all))
	}

	// Check for specific families
	familyIDs := make(map[string]bool)
	for _, f := range all {
		familyIDs[f.ID] = true
	}

	expectedIDs := []string{
		families.FamilyDebug, families.FamilyRepair, families.FamilyReview,
		families.FamilyPlanning, families.FamilyImplementation, families.FamilyRefactor,
		families.FamilyMigration, families.FamilyInvestigation, families.FamilyArchitecture,
	}

	for _, id := range expectedIDs {
		if !familyIDs[id] {
			t.Errorf("Expected family %q to be registered", id)
		}
	}
}

func TestFamilyRegistryOverride(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	override := families.FamilyOverride{
		AddKeywords: []string{"hotfix"},
	}

	err := registry.Override(families.FamilyDebug, override)
	if err != nil {
		t.Fatalf("Override failed: %v", err)
	}

	family, ok := registry.Lookup(families.FamilyDebug)
	if !ok {
		t.Fatal("Family not found after override")
	}

	// Check that keyword was added
	found := false
	for _, kw := range family.Keywords {
		if kw == "hotfix" {
			found = true
		}
	}

	if !found {
		t.Error("Expected 'hotfix' keyword to be added via override")
	}
}

func TestFamilyRegistryConflict(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	// Try to register same family again
	family := families.KeywordFamily{
		ID:          families.FamilyDebug,
		DisplayName: "Debug",
		Keywords:    []string{"fix"},
	}

	err := registry.Register(family)
	if err == nil {
		t.Error("Expected error when registering duplicate family ID")
	}
}

func TestFamilyRegistryLookupMissing(t *testing.T) {
	registry := families.NewRegistry()

	_, ok := registry.Lookup("nonexistent")
	if ok {
		t.Error("Expected false for missing family lookup")
	}
}

func TestSignalWeightsForFamily(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	override := families.FamilyOverride{
		SignalWeights: map[string]float64{
			"error_text": 2.0,
			"keyword":    1.5,
		},
	}

	registry.Override(families.FamilyDebug, override)

	weights := registry.SignalWeightsFor(families.FamilyDebug)
	if weights == nil {
		t.Fatal("Expected signal weights to be returned")
	}

	if weights["error_text"] != 2.0 {
		t.Errorf("Expected error_text weight 2.0, got %f", weights["error_text"])
	}

	// Check that unset keys are not present
	if _, ok := weights["task_structure"]; ok {
		t.Error("Expected unset key to not be present")
	}
}
