package intake

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

func TestCapabilityClassifierMapsFamilyToSequence(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classifier := NewCapabilityClassifier(registry)

	sel := families.FamilySelection{
		WinningFamily: families.FamilyDebug,
	}

	seq, source := classifier.ClassifyCapability(sel, nil)

	if len(seq) == 0 {
		t.Error("Expected non-empty capability sequence")
	}

	// Debug family has capability sequence: ["euclo:cap.bisect", "euclo:cap.symbol_trace"]
	if seq[0] != "euclo:cap.bisect" {
		t.Errorf("Expected first capability euclo:cap.bisect, got %s", seq[0])
	}

	if source != "family_metadata" {
		t.Errorf("Expected source family_metadata, got %s", source)
	}
}

func TestCapabilityClassifierUsesFallback(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	// Register a family without capability sequence
	registry.Register(families.KeywordFamily{
		ID:                  "test_no_seq",
		DisplayName:         "Test No Seq",
		Keywords:            []string{"test"},
		DefaultHITLPolicy:   families.HITLPolicyNever,
		DefaultVerification: families.VerificationNotRequired,
		FallbackCapability:  "euclo:cap.fallback",
	})

	classifier := NewCapabilityClassifier(registry)

	sel := families.FamilySelection{
		WinningFamily: "test_no_seq",
	}

	seq, source := classifier.ClassifyCapability(sel, nil)

	if len(seq) == 0 {
		t.Error("Expected non-empty capability sequence from fallback")
	}

	if seq[0] != "euclo:cap.fallback" {
		t.Errorf("Expected fallback capability euclo:cap.fallback, got %s", seq[0])
	}

	if source != "fallback" {
		t.Errorf("Expected source fallback, got %s", source)
	}
}

func TestCapabilityClassifierMixedIntent(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.8},
			{FamilyID: families.FamilyReview, Score: 0.6},
		},
		Confidence: 0.8,
		Ambiguous:  true,
	}

	envelope := &TaskEnvelope{
		Instruction: "fix and review the code",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if !intent.MixedIntent {
		t.Error("Expected MixedIntent to be true with multiple candidates")
	}

	if intent.CapabilityOperator != "any" {
		t.Errorf("Expected CapabilityOperator any, got %s", intent.CapabilityOperator)
	}
}

func TestCapabilityClassifierSingleFamily(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction: "fix the bug",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if intent.MixedIntent {
		t.Error("Expected MixedIntent to be false with single candidate")
	}

	if intent.CapabilityOperator != "all" {
		t.Errorf("Expected CapabilityOperator all, got %s", intent.CapabilityOperator)
	}
}

func TestResolveIntentPopulatesAllFields(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction: "fix the bug",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	// Check all required fields are populated
	if intent.WinningFamily == "" {
		t.Error("Expected WinningFamily to be populated")
	}
	if len(intent.FamilyCandidates) == 0 {
		t.Error("Expected FamilyCandidates to be populated")
	}
	if intent.Confidence == 0 {
		t.Error("Expected Confidence to be populated")
	}
	if intent.ClassificationSource == "" {
		t.Error("Expected ClassificationSource to be populated")
	}
	if intent.CapabilityOperator == "" {
		t.Error("Expected CapabilityOperator to be populated")
	}
	if intent.EditPermitted == false {
		// Debug family has HITLPolicyAsk, so edit should be permitted
		t.Error("Expected EditPermitted to be true for debug family")
	}
	if intent.RequiresVerification == false {
		// Debug family requires verification
		t.Error("Expected RequiresVerification to be true for debug family")
	}
	if intent.Scope == "" {
		t.Error("Expected Scope to be populated")
	}
	if intent.RiskLevel == "" {
		t.Error("Expected RiskLevel to be populated")
	}
	if len(intent.ReasonCodes) == 0 {
		t.Error("Expected ReasonCodes to be populated")
	}
}

func TestResolveIntentEditPermittedFromFamily(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	// Review family has HITLPolicyNever, so edit should be permitted
	classification := &ScoredClassification{
		WinningFamily: families.FamilyReview,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyReview, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction: "review the code",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if !intent.EditPermitted {
		t.Error("Expected EditPermitted to be true for review family (HITLPolicyNever)")
	}
}

func TestResolveIntentRequiresVerification(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	// Debug family requires verification
	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction: "fix the bug",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if !intent.RequiresVerification {
		t.Error("Expected RequiresVerification to be true for debug family")
	}
}

func TestResolveIntentReasonCodes(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction: "fix the bug",
		FamilyHint:   "debug",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if len(intent.ReasonCodes) == 0 {
		t.Error("Expected ReasonCodes to be populated")
	}

	// Check for expected reason codes
	hasSource := false
	hasFamilyHint := false
	hasConfident := false

	for _, code := range intent.ReasonCodes {
		if code == "source:tier1" {
			hasSource = true
		}
		if code == "family_hint:debug" {
			hasFamilyHint = true
		}
		if code == "confident" {
			hasConfident = true
		}
	}

	if !hasSource {
		t.Error("Expected reason code source:tier1")
	}
	if !hasFamilyHint {
		t.Error("Expected reason code family_hint:debug")
	}
	if !hasConfident {
		t.Error("Expected reason code confident")
	}
}

func TestResolveIntentScopeFromEnvelope(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction:    "fix the bug",
		WorkspaceScopes: []string{"backend"},
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if intent.Scope != "backend" {
		t.Errorf("Expected scope backend, got %s", intent.Scope)
	}
}

func TestResolveIntentRiskLevelFromFamily(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	// Debug family has low risk
	classification := &ScoredClassification{
		WinningFamily: families.FamilyDebug,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyDebug, Score: 0.9},
		},
		Confidence: 0.9,
		Ambiguous:  false,
	}

	envelope := &TaskEnvelope{
		Instruction: "fix the bug",
	}

	intent := ResolveIntent(classification, envelope, registry, nil, "tier1")

	if intent.RiskLevel != "low" {
		t.Errorf("Expected risk level low for debug family, got %s", intent.RiskLevel)
	}

	// Migration family has high risk
	classification.WinningFamily = families.FamilyMigration
	classification.FamilyCandidates[0].FamilyID = families.FamilyMigration

	intent = ResolveIntent(classification, envelope, registry, nil, "tier1")

	if intent.RiskLevel != "high" {
		t.Errorf("Expected risk level high for migration family, got %s", intent.RiskLevel)
	}
}

func TestCapabilityClassifierOverrideUsesCustomSequence(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	classifier := NewCapabilityClassifier(registry)

	sel := families.FamilySelection{
		WinningFamily: families.FamilyDebug,
	}

	overrides := map[string]families.FamilyOverride{
		families.FamilyDebug: {
			CapabilitySequence: []string{"euclo:cap.custom", "euclo:cap.custom2"},
		},
	}

	seq, source := classifier.ClassifyCapability(sel, overrides)

	if len(seq) == 0 {
		t.Error("Expected non-empty capability sequence from override")
	}

	if seq[0] != "euclo:cap.custom" {
		t.Errorf("Expected override capability euclo:cap.custom, got %s", seq[0])
	}

	if source != "override" {
		t.Errorf("Expected source override, got %s", source)
	}
}
