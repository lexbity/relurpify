package surface

import (
	"testing"
)

func TestThoughtRecipeValidate(t *testing.T) {
	r := &ThoughtRecipe{
		Metadata: ThoughtRecipeMetadata{
			Name:           "Test ThoughtRecipe",
			Families:       []string{"debug"},
			Keywords:       []string{"panic"},
			HandoffTargets: []string{"debug_followup"},
		},
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid thoughtrecipe to pass validation, got error: %v", err)
	}
}

func TestThoughtRecipeValidateMissingName(t *testing.T) {
	r := &ThoughtRecipe{
		Metadata: ThoughtRecipeMetadata{},
	}

	if err := r.Validate(); err == nil {
		t.Fatal("expected validation error for missing name")
	}
}

func TestThoughtRecipeValidateRejectsUnsupportedRouteKind(t *testing.T) {
	r := &ThoughtRecipe{
		RouteKind: TriggerRouteKind("bootstrap"),
		Metadata: ThoughtRecipeMetadata{
			Name: "Bad Route ThoughtRecipe",
		},
	}

	if err := r.Validate(); err == nil {
		t.Fatal("expected route kind validation error")
	}
}

func TestEffectiveName(t *testing.T) {
	tests := []struct {
		name string
		r    *ThoughtRecipe
		want string
	}{
		{name: "nil", r: nil, want: ""},
		{name: "metadata name", r: &ThoughtRecipe{Metadata: ThoughtRecipeMetadata{Name: "MetaName"}}, want: "MetaName"},
		{name: "name field", r: &ThoughtRecipe{Name: "RecipeName"}, want: "RecipeName"},
		{name: "id fallback", r: &ThoughtRecipe{ID: "recipe-id"}, want: "recipe-id"},
		{name: "metadata name wins", r: &ThoughtRecipe{Name: "RecipeName", Metadata: ThoughtRecipeMetadata{Name: "MetaName"}}, want: "MetaName"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.EffectiveName(); got != tt.want {
				t.Errorf("EffectiveName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergePolicyConstants(t *testing.T) {
	if MergePolicyAll != "all" {
		t.Errorf("MergePolicyAll = %q, want %q", MergePolicyAll, "all")
	}
	if MergePolicyAny != "any" {
		t.Errorf("MergePolicyAny = %q, want %q", MergePolicyAny, "any")
	}
	if MergePolicyFirst != "first" {
		t.Errorf("MergePolicyFirst = %q, want %q", MergePolicyFirst, "first")
	}
	if MergePolicyConcat != "concat" {
		t.Errorf("MergePolicyConcat = %q, want %q", MergePolicyConcat, "concat")
	}
}

func TestTriggerRouteKindConstants(t *testing.T) {
	if TriggerRouteKindUnknown != "" {
		t.Errorf("TriggerRouteKindUnknown = %q, want empty", TriggerRouteKindUnknown)
	}
	if TriggerRouteKindCapability != "capability" {
		t.Errorf("TriggerRouteKindCapability = %q, want %q", TriggerRouteKindCapability, "capability")
	}
	if TriggerRouteKindIntent != "intent" {
		t.Errorf("TriggerRouteKindIntent = %q, want %q", TriggerRouteKindIntent, "intent")
	}
}
