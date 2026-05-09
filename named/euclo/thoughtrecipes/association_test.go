package thoughtrecipe

import "testing"

func TestSupportedTriggerAssociationsExposeRecipeLocalSurface(t *testing.T) {
	if got := len(SupportedTriggerAssociations); got != 3 {
		t.Fatalf("SupportedTriggerAssociations length = %d, want 3", got)
	}
	if !IsSupportedTriggerAssociation(TriggerAssociationFamily) {
		t.Fatal("expected family association to be supported")
	}
	if !IsSupportedTriggerAssociation(TriggerAssociationKeyword) {
		t.Fatal("expected keyword association to be supported")
	}
	if !IsSupportedTriggerAssociation(TriggerAssociationHandoff) {
		t.Fatal("expected handoff association to be supported")
	}
	if IsSupportedTriggerAssociation("fallback") {
		t.Fatal("did not expect fallback association to be supported")
	}
}

func TestTriggerAssociationNormalizationPreservesDeterministicTargetMetadata(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe handoff_demo
"Demo."

trigger as capability:
  family ["debug", "debug"]
  keyword ["panic", "panic"]
  handoff ["RouteA", "routea"]
  may read workspace
`)

	meta, err := TriggerAssociationsFromDecl(doc.Declarations[0].(*TriggerDecl))
	if err != nil {
		t.Fatalf("TriggerAssociationsFromDecl failed: %v", err)
	}
	if got := meta.Families; len(got) != 1 || got[0] != "debug" {
		t.Fatalf("families = %#v, want [debug]", got)
	}
	if got := meta.Keywords; len(got) != 1 || got[0] != "panic" {
		t.Fatalf("keywords = %#v, want [panic]", got)
	}
	if got := meta.HandoffTargets; len(got) != 1 || got[0] != "routea" {
		t.Fatalf("handoff targets = %#v, want [routea]", got)
	}
	if got := meta.Tags; len(got) != 3 {
		t.Fatalf("tags = %#v, want 3 unique entries", got)
	}
}
