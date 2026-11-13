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

// RecipeWorkspace is the consumer port for recipe file operations.
// It provides lossless editing and file execution capabilities.
// Implementations must be stateless with respect to editing state.
type RecipeWorkspace interface {
	// ListRecipes returns all recipe files in the workspace.
	ListRecipes(context.Context) ([]RecipeFile, error)
	// OpenRecipe opens a recipe file and returns its content.
	OpenRecipe(context.Context, string) (RecipeFile, error)
	// RecipeCatalog returns the available strategies and prompts.
	RecipeCatalog(context.Context) (RecipeCatalog, error)
	// SaveRecipe saves a recipe file with revision checking.
	SaveRecipe(context.Context, RecipeFile) (RecipeFile, error)
	// TriggerRecipe triggers execution of a recipe file.
	// The entry parameter specifies the entry point (may be empty for default).
	// For P04, this returns a simple status result.
	TriggerRecipe(context.Context, RecipeFile, string) (RecipeRunResult, error)
}
