package intake

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

func TestResolveIntentDoesNotInventCapabilitySequences(t *testing.T) {
	registry := families.NewRegistry()
	_ = families.RegisterBuiltins(registry)

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

	intent := ResolveIntent(classification, envelope, registry, "tier1")

	if intent.WinningFamily != families.FamilyDebug {
		t.Fatalf("unexpected winning family: %q", intent.WinningFamily)
	}
	if intent.ClassificationSource != "tier1" {
		t.Fatalf("unexpected classification source: %q", intent.ClassificationSource)
	}
	if !intent.EditPermitted {
		t.Fatal("expected debug family to permit edits")
	}
	if !intent.RequiresVerification {
		t.Fatal("expected debug family to require verification")
	}
	if intent.Scope != "workspace" {
		t.Fatalf("unexpected scope: %q", intent.Scope)
	}
	if len(intent.ReasonCodes) == 0 {
		t.Fatal("expected reason codes to be populated")
	}
}

func TestResolveIntentDeterministicFamilyMetadata(t *testing.T) {
	registry := families.NewRegistry()
	_ = families.RegisterBuiltins(registry)

	classification := &ScoredClassification{
		WinningFamily: families.FamilyReview,
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: families.FamilyReview, Score: 0.9},
		},
		Confidence: 0.8,
		Ambiguous:  true,
	}
	envelope := &TaskEnvelope{
		Instruction: "review the code",
		FamilyHint:  "review",
	}

	intent := ResolveIntent(classification, envelope, registry, "tier1")

	if intent.RiskLevel != "low" {
		t.Fatalf("unexpected risk level: %q", intent.RiskLevel)
	}
	if !intent.MixedIntent && len(classification.FamilyCandidates) > 1 {
		t.Fatal("expected mixed intent when multiple candidates exist")
	}
	foundHint := false
	for _, code := range intent.ReasonCodes {
		if code == "family_hint:review" {
			foundHint = true
			break
		}
	}
	if !foundHint {
		t.Fatalf("expected family hint reason code, got %#v", intent.ReasonCodes)
	}
}
