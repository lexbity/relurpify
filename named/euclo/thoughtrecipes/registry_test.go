package thoughtrecipe

import "testing"

func TestThoughtRecipeRegistryUsesThoughtRecipeNameAndPreservesFirstWins(t *testing.T) {
	reg := NewThoughtRecipeRegistry()

	first := &ThoughtRecipe{
		ID:   "thoughtrecipe-a",
		Name: "shared",
		Metadata: ThoughtRecipeMetadata{
			Name: "shared",
		},
	}
	second := &ThoughtRecipe{
		ID:   "thoughtrecipe-b",
		Name: "shared",
		Metadata: ThoughtRecipeMetadata{
			Name: "shared",
		},
	}

	ok, err := reg.RegisterCompiledFirstWins(first, &ExecutionPlan{ThoughtRecipe: first}, "first.euclo")
	if err != nil {
		t.Fatalf("RegisterCompiledFirstWins first failed: %v", err)
	}
	if !ok {
		t.Fatal("expected first registration to succeed")
	}
	ok, err = reg.RegisterCompiledFirstWins(second, &ExecutionPlan{ThoughtRecipe: second}, "second.euclo")
	if err != nil {
		t.Fatalf("RegisterCompiledFirstWins second failed: %v", err)
	}
	if ok {
		t.Fatal("expected duplicate registration to be ignored")
	}
	if got := reg.Count(); got != 1 {
		t.Fatalf("registry count = %d, want 1", got)
	}
	if got := reg.List(); len(got) != 1 || got[0] != "shared" {
		t.Fatalf("registry list = %#v, want [shared]", got)
	}
	if thoughtrecipe, ok := reg.Get("shared"); !ok || thoughtrecipe == nil || thoughtrecipe.ID != "thoughtrecipe-a" {
		t.Fatalf("expected first thoughtrecipe by name, got %#v (ok=%v)", thoughtrecipe, ok)
	}
	if plan, ok := reg.GetPlan("shared"); !ok || plan == nil || plan.ThoughtRecipe != first {
		t.Fatalf("expected first plan by name, got %#v (ok=%v)", plan, ok)
	}
}

func TestThoughtRecipeRegistrySearchesTriggerMetadataTags(t *testing.T) {
	reg := NewThoughtRecipeRegistry()

	recipe := &ThoughtRecipe{
		ID:   "debug_recipe",
		Name: "debug_recipe",
		Metadata: ThoughtRecipeMetadata{
			Name:           "debug_recipe",
			Families:       []string{"debug"},
			Keywords:       []string{"panic", "trace"},
			HandoffTargets: []string{"debug_followup"},
			Tags:           []string{"debug", "panic", "trace", "debug_followup"},
		},
	}
	if _, err := reg.RegisterCompiledFirstWins(recipe, &ExecutionPlan{ThoughtRecipe: recipe}, "debug.euclo"); err != nil {
		t.Fatalf("RegisterCompiledFirstWins failed: %v", err)
	}

	if got := reg.FindByFamily("debug"); len(got) != 1 || got[0].Name != "debug_recipe" {
		t.Fatalf("FindByFamily returned %#v, want debug_recipe", got)
	}
	if got := reg.FindByKeyword("panic"); len(got) != 1 || got[0].Name != "debug_recipe" {
		t.Fatalf("FindByKeyword returned %#v, want debug_recipe", got)
	}
	if got := reg.FindMatchingTags("trace"); len(got) != 1 || got[0].Name != "debug_recipe" {
		t.Fatalf("FindMatchingTags returned %#v, want debug_recipe", got)
	}
	if got := reg.FindByHandoffTarget("debug_followup"); len(got) != 1 || got[0].Name != "debug_recipe" {
		t.Fatalf("FindByHandoffTarget returned %#v, want debug_recipe", got)
	}
}
