package recipe

import (
	"fmt"
	"sync"
)

// RecipeRegistry manages ThoughtRecipe definitions.
type RecipeRegistry struct {
	mu      sync.RWMutex
	recipes map[string]*ThoughtRecipe
}

// NewRecipeRegistry creates a new recipe registry.
func NewRecipeRegistry() *RecipeRegistry {
	return &RecipeRegistry{
		recipes: make(map[string]*ThoughtRecipe),
	}
}

// Register registers a recipe in the registry.
func (r *RecipeRegistry) Register(recipe *ThoughtRecipe) error {
	if err := recipe.Validate(); err != nil {
		return fmt.Errorf("invalid recipe: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.recipes[recipe.ID] = recipe
	return nil
}

// Get retrieves a recipe by ID.
func (r *RecipeRegistry) Get(id string) (*ThoughtRecipe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recipe, ok := r.recipes[id]
	return recipe, ok
}

// List returns all registered recipe IDs.
func (r *RecipeRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.recipes))
	for id := range r.recipes {
		ids = append(ids, id)
	}
	return ids
}

// Remove removes a recipe from the registry.
func (r *RecipeRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.recipes, id)
}

// Count returns the number of registered recipes.
func (r *RecipeRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.recipes)
}
