// Package ports defines the consumer ports for the relurpish application.
// These are cohesive interfaces that the TUI and other consumers use to interact
// with runtime services without creating circular dependencies.
package ports

import "context"

// RecipeFile represents a recipe file with its metadata.
// This is used by the TUI to display and manage recipe files.
type RecipeFile struct {
	// ID is the recipe identifier (typically filename without extension).
	ID string
	// Path is the workspace-relative or absolute file path.
	Path string
	// Revision is the content digest (SHA-256 hash).
	Revision string
	// Source is the recipe source text.
	Source string
}

// RecipePrompt represents a prompt definition.
type RecipePrompt struct {
	ID      string
	Version string
	Origin  string
	Text    string
}

// RecipeStrategy represents a strategy definition.
type RecipeStrategy struct {
	Name  string
	Shape string
}

// RecipeCatalog contains the available strategies and prompts for recipe authoring.
type RecipeCatalog struct {
	Strategies []RecipeStrategy
	Prompts    []RecipePrompt
}

// RecipeRunResult represents the result of triggering a recipe.
type RecipeRunResult struct {
	// Revision is a monotonic counter for result updates.
	Revision uint64
	// Status is a human-readable status message.
	Status string
	// Running indicates whether an operation is currently in progress.
	Running bool
	// RunID is the ID of the started run (if any).
	RunID string
}

// RecipeLibrary supplies saved source and the installed authoring catalog.
// Reading source does not imply that saving or execution is integrated.
type RecipeLibrary interface {
	// ListRecipes returns all recipe files in the workspace.
	ListRecipes(context.Context) ([]RecipeFile, error)
	// OpenRecipe opens a recipe file and returns its content.
	OpenRecipe(context.Context, string) (RecipeFile, error)
	// RecipeCatalog returns the available strategies and prompts.
	RecipeCatalog(context.Context) (RecipeCatalog, error)
}

// RecipeWriter saves a validated source revision against its expected revision.
type RecipeWriter interface {
	// SaveRecipe saves a recipe file with revision checking.
	SaveRecipe(context.Context, RecipeFile) (RecipeFile, error)
}

// RecipeRunner admits the exact reviewed saved file with explicit task input.
type RecipeRunner interface {
	// TriggerRecipe triggers execution of a recipe file.
	// The string parameter contains task input, separate from recipe identity.
	TriggerRecipe(context.Context, RecipeFile, string) (RecipeRunResult, error)
}
