package recipe

import "testing"

func TestAliasResolverUsesRecipeAliasMap(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "my recipe",
		Name: "My Recipe",
		Global: RecipeGlobal{
			Context: RecipeContext{
				Aliases: map[string]string{
					"shared": "euclo.recipe.shared.value",
				},
			},
		},
	}

	resolver := NewAliasResolver(recipe)
	if got := resolver.Resolve("shared"); got != "euclo.recipe.shared.value" {
		t.Fatalf("unexpected alias resolution: %s", got)
	}
}

func TestAliasResolverPrefixesRecipeName(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "my recipe",
		Name: "My Recipe",
	}

	resolver := NewAliasResolver(recipe)
	if got := resolver.Resolve("answer"); got != "euclo.recipe.my_recipe.answer" {
		t.Fatalf("unexpected alias resolution: %s", got)
	}
}

func TestAliasResolverPassesThroughQualifiedKeys(t *testing.T) {
	resolver := NewAliasResolver(&ThoughtRecipe{ID: "recipe"})
	if got := resolver.Resolve("euclo.recipe.custom.key"); got != "euclo.recipe.custom.key" {
		t.Fatalf("unexpected alias resolution: %s", got)
	}
}
