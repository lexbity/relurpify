package tui

import "context"

// RecipeWorkspace owns source retrieval, revision checks, persistence and
// explicit file triggering. Authoring is separate from intent/chat admission.
type RecipeWorkspace interface {
	ListRecipes(context.Context) ([]RecipeFile, error)
	OpenRecipe(context.Context, string) (RecipeFile, error)
	RecipeCatalog(context.Context) (RecipeCatalog, error)
	SaveRecipe(context.Context, RecipeFile) (RecipeFile, error)
	TriggerRecipe(context.Context, RecipeFile, string) (FrontendSnapshot, error)
}

type RecipeFile struct{ ID, Path, Revision, Source string }
type RecipePrompt struct{ ID, Version, Origin, Text string }
type RecipeStrategy struct{ Name, Shape string }
type RecipeCatalog struct {
	Strategies []RecipeStrategy
	Prompts    []RecipePrompt
}
