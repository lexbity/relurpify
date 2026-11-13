// Package runtime provides the relurpish application runtime services.
package runtime

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
)

// RecipeWorkspaceAdapter adapts RecipeService to the tui.RecipeWorkspace interface.
// This enables the TUI to use the runtime recipe service without circular dependencies.
type RecipeWorkspaceAdapter struct {
	service *RecipeService
}

// NewRecipeWorkspaceAdapter creates a new adapter.
func NewRecipeWorkspaceAdapter(service *RecipeService) *RecipeWorkspaceAdapter {
	return &RecipeWorkspaceAdapter{service: service}
}

// ListRecipes returns all recipe files in the workspace.
func (a *RecipeWorkspaceAdapter) ListRecipes(ctx context.Context) ([]ports.RecipeFile, error) {
	infos, err := a.service.ListRecipes(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ports.RecipeFile, len(infos))
	for i, info := range infos {
		result[i] = ports.RecipeFile{
			ID:       info.ID,
			Path:     info.WorkspacePath,
			Revision: info.Revision,
			Source:   "", // Source not included in FileInfo
		}
	}
	return result, nil
}

// OpenRecipe opens a recipe file and returns its content.
func (a *RecipeWorkspaceAdapter) OpenRecipe(ctx context.Context, path string) (ports.RecipeFile, error) {
	file, err := a.service.OpenRecipe(ctx, path)
	if err != nil {
		return ports.RecipeFile{}, err
	}

	return ports.RecipeFile{
		ID:       file.ID,
		Path:     file.WorkspacePath,
		Revision: file.Revision,
		Source:   file.Source,
	}, nil
}

// RecipeCatalog returns the available strategies and prompts.
func (a *RecipeWorkspaceAdapter) RecipeCatalog(ctx context.Context) (ports.RecipeCatalog, error) {
	// For P04, we return an empty catalog or a basic one.
	// The full implementation will come later.
	return ports.RecipeCatalog{
		Strategies: []ports.RecipeStrategy{},
		Prompts:    []ports.RecipePrompt{},
	}, nil
}

// SaveRecipe saves a recipe file.
func (a *RecipeWorkspaceAdapter) SaveRecipe(ctx context.Context, file ports.RecipeFile) (ports.RecipeFile, error) {
	// Convert ports.RecipeFile to runtime.RecipeFile
	runtimeFile := &RecipeFile{
		ID:           file.ID,
		Path:         file.Path,
		WorkspacePath: file.Path,
		Name:         file.ID, // Use ID as name if not available
		Source:       file.Source,
		Revision:     file.Revision,
		Bytes:        []byte(file.Source),
	}

	saved, err := a.service.SaveRecipe(ctx, runtimeFile)
	if err != nil {
		return ports.RecipeFile{}, err
	}

	return ports.RecipeFile{
		ID:       saved.ID,
		Path:     saved.WorkspacePath,
		Revision: saved.Revision,
		Source:   saved.Source,
	}, nil
}

// TriggerRecipe triggers execution of a recipe.
// For P04, this just validates and prepares the recipe for execution.
func (a *RecipeWorkspaceAdapter) TriggerRecipe(ctx context.Context, file ports.RecipeFile, entry string) (ports.RecipeRunResult, error) {
	// Convert ports.RecipeFile to runtime.RecipeFile
	runtimeFile := &RecipeFile{
		ID:           file.ID,
		Path:         file.Path,
		WorkspacePath: file.Path,
		Name:         file.ID,
		Source:       file.Source,
		Revision:     file.Revision,
		Bytes:        []byte(file.Source),
	}

	// For P04, we just prepare the recipe run and return a simple result
	// The actual execution will be handled by the runtime
	_, err := a.service.PrepareRecipeRun(ctx, runtimeFile)
	if err != nil {
		return ports.RecipeRunResult{}, fmt.Errorf("prepare recipe: %w", err)
	}

	// Return a simple result indicating the recipe is ready
	return ports.RecipeRunResult{
		Revision: 1,
		Status:   fmt.Sprintf("Recipe %s validated and ready for execution", file.ID),
		Running:  false,
	}, nil
}
