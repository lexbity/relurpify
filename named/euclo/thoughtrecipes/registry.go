package thoughtrecipe

import (
	"fmt"
	"strings"
	"sync"
)

// ThoughtRecipeEntry stores a thoughtrecipe and its compiled DSL plan.
type ThoughtRecipeEntry struct {
	Name          string
	ThoughtRecipe *ThoughtRecipe
	Plan          *ExecutionPlan
	Source        string
}

// ThoughtRecipeRegistry manages ThoughtRecipe definitions and compiled Euclo plans.
type ThoughtRecipeRegistry struct {
	mu             sync.RWMutex
	thoughtrecipes map[string]*ThoughtRecipeEntry
	order          []string
}

// NewThoughtRecipeRegistry creates a new thoughtrecipe registry.
func NewThoughtRecipeRegistry() *ThoughtRecipeRegistry {
	return &ThoughtRecipeRegistry{
		thoughtrecipes: make(map[string]*ThoughtRecipeEntry),
	}
}

// Register registers a thoughtrecipe in the registry.
func (r *ThoughtRecipeRegistry) Register(thoughtrecipe *ThoughtRecipe) error {
	_, err := r.registerThoughtRecipe(thoughtrecipe, nil, "", false)
	return err
}

// RegisterFirstWins registers a thoughtrecipe if the name is not already present.
func (r *ThoughtRecipeRegistry) RegisterFirstWins(thoughtrecipe *ThoughtRecipe) (bool, error) {
	return r.registerThoughtRecipe(thoughtrecipe, nil, "", true)
}

// RegisterCompiled registers a compiled thoughtrecipe and its source thoughtrecipe.
func (r *ThoughtRecipeRegistry) RegisterCompiled(thoughtrecipe *ThoughtRecipe, plan *ExecutionPlan, source string) error {
	_, err := r.registerThoughtRecipe(thoughtrecipe, plan, source, false)
	return err
}

// RegisterCompiledFirstWins registers a compiled thoughtrecipe only if the name is new.
func (r *ThoughtRecipeRegistry) RegisterCompiledFirstWins(thoughtrecipe *ThoughtRecipe, plan *ExecutionPlan, source string) (bool, error) {
	return r.registerThoughtRecipe(thoughtrecipe, plan, source, true)
}

func (r *ThoughtRecipeRegistry) registerThoughtRecipe(thoughtrecipe *ThoughtRecipe, plan *ExecutionPlan, source string, firstWins bool) (bool, error) {
	if thoughtrecipe == nil {
		return false, fmt.Errorf("thoughtrecipe is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := registryKeyForThoughtRecipe(thoughtrecipe)
	if key == "" {
		return false, fmt.Errorf("thoughtrecipe name is required")
	}
	if firstWins {
		if _, exists := r.thoughtrecipes[key]; exists {
			return false, nil
		}
	}
	if _, exists := r.thoughtrecipes[key]; !exists {
		r.order = append(r.order, key)
	}
	r.thoughtrecipes[key] = &ThoughtRecipeEntry{
		Name:          key,
		ThoughtRecipe: thoughtrecipe,
		Plan:          plan,
		Source:        strings.TrimSpace(source),
	}
	return true, nil
}

// Get retrieves a thoughtrecipe by ID.
func (r *ThoughtRecipeRegistry) Get(id string) (*ThoughtRecipe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.lookupEntryLocked(strings.TrimSpace(id))
	if !ok || entry == nil {
		return nil, false
	}
	return entry.ThoughtRecipe, true
}

// GetPlan retrieves a compiled plan by thoughtrecipe name.
func (r *ThoughtRecipeRegistry) GetPlan(name string) (*ExecutionPlan, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.lookupEntryLocked(strings.TrimSpace(name))
	if !ok || entry == nil || entry.Plan == nil {
		return nil, false
	}
	return entry.Plan, true
}

// List returns all registered thoughtrecipe IDs.
func (r *ThoughtRecipeRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.order))
	seen := make(map[string]bool, len(r.order))
	for _, id := range r.order {
		if seen[id] {
			continue
		}
		if _, ok := r.thoughtrecipes[id]; ok {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}

// Remove removes a thoughtrecipe from the registry.
func (r *ThoughtRecipeRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.thoughtrecipes, id)
	for i, existing := range r.order {
		if existing == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Count returns the number of registered thoughtrecipes.
func (r *ThoughtRecipeRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.thoughtrecipes)
}

// Entries returns the registered thoughtrecipe entries in insertion order.
func (r *ThoughtRecipeRegistry) Entries() []ThoughtRecipeEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ThoughtRecipeEntry, 0, len(r.order))
	for _, key := range r.order {
		entry, ok := r.thoughtrecipes[key]
		if !ok || entry == nil {
			continue
		}
		out = append(out, *entry)
	}
	return out
}

func registryKeyForThoughtRecipe(thoughtrecipe *ThoughtRecipe) string {
	if thoughtrecipe == nil {
		return ""
	}
	if name := strings.TrimSpace(thoughtrecipe.EffectiveName()); name != "" {
		return name
	}
	return strings.TrimSpace(thoughtrecipe.ID)
}

func (r *ThoughtRecipeRegistry) lookupEntryLocked(key string) (*ThoughtRecipeEntry, bool) {
	if entry, ok := r.thoughtrecipes[key]; ok {
		return entry, true
	}
	for _, entry := range r.thoughtrecipes {
		if entry == nil || entry.ThoughtRecipe == nil {
			continue
		}
		if strings.TrimSpace(entry.ThoughtRecipe.ID) == key {
			return entry, true
		}
		if strings.TrimSpace(entry.Name) == key {
			return entry, true
		}
	}
	return nil, false
}
