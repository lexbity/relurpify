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

// FindByTag returns entries whose trigger metadata includes the provided tag.
func (r *ThoughtRecipeRegistry) FindByTag(tag string) []ThoughtRecipeEntry {
	return r.findByTags([]string{tag})
}

// FindByFamily returns entries whose trigger family metadata includes the provided family.
func (r *ThoughtRecipeRegistry) FindByFamily(family string) []ThoughtRecipeEntry {
	return r.findByTags([]string{family})
}

// FindByKeyword returns entries whose trigger keyword metadata includes the provided keyword.
func (r *ThoughtRecipeRegistry) FindByKeyword(keyword string) []ThoughtRecipeEntry {
	return r.findByTags([]string{keyword})
}

// FindByHandoffTarget returns entries whose declared handoff targets include the provided target.
func (r *ThoughtRecipeRegistry) FindByHandoffTarget(target string) []ThoughtRecipeEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := normalizeTriggerTag(target)
	if normalized == "" {
		return nil
	}

	out := make([]ThoughtRecipeEntry, 0)
	for _, key := range r.order {
		entry, ok := r.thoughtrecipes[key]
		if !ok || entry == nil || entry.ThoughtRecipe == nil {
			continue
		}
		if recipeEntryMatchesHandoffTarget(entry.ThoughtRecipe, normalized) {
			out = append(out, *entry)
		}
	}
	return out
}

// FindMatchingTags returns entries matching any of the provided tags.
func (r *ThoughtRecipeRegistry) FindMatchingTags(tags ...string) []ThoughtRecipeEntry {
	return r.findByTags(tags)
}

// ResolveBestMatch returns the highest-scoring thoughtrecipe match for the
// provided explicit identifier and normalized search tokens.
func (r *ThoughtRecipeRegistry) ResolveBestMatch(explicitID string, tokens ...string) (ThoughtRecipeEntry, bool, []string) {
	if r == nil {
		return ThoughtRecipeEntry{}, false, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	if normalizedID := normalizeTriggerTag(explicitID); normalizedID != "" {
		if entry, ok := r.lookupEntryLocked(normalizedID); ok && entry != nil {
			return *entry, true, []string{"explicit_thoughtrecipe"}
		}
	}

	normalizedTokens := normalizeSearchTokens(tokens...)
	if len(normalizedTokens) == 0 {
		return ThoughtRecipeEntry{}, false, nil
	}

	var (
		bestEntry   ThoughtRecipeEntry
		bestScore   int
		bestReasons []string
		tied        bool
	)
	for _, key := range r.order {
		entry, ok := r.thoughtrecipes[key]
		if !ok || entry == nil || entry.ThoughtRecipe == nil {
			continue
		}
		score, reasons := scoreThoughtRecipeEntry(entry.ThoughtRecipe, normalizedTokens)
		if score <= 0 {
			continue
		}
		if score > bestScore {
			bestEntry = *entry
			bestScore = score
			bestReasons = append([]string(nil), reasons...)
			tied = false
			continue
		}
		if score == bestScore {
			tied = true
		}
	}
	if bestScore == 0 || tied {
		return ThoughtRecipeEntry{}, false, nil
	}
	return bestEntry, true, bestReasons
}

func (r *ThoughtRecipeRegistry) findByTags(tags []string) []ThoughtRecipeEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := make(map[string]struct{})
	for _, tag := range tags {
		if normalizedTag := normalizeTriggerTag(tag); normalizedTag != "" {
			normalized[normalizedTag] = struct{}{}
		}
	}
	if len(normalized) == 0 {
		return nil
	}

	out := make([]ThoughtRecipeEntry, 0)
	for _, key := range r.order {
		entry, ok := r.thoughtrecipes[key]
		if !ok || entry == nil || entry.ThoughtRecipe == nil {
			continue
		}
		if recipeEntryMatchesTags(entry.ThoughtRecipe, normalized) {
			out = append(out, *entry)
		}
	}
	return out
}

func recipeEntryMatchesTags(recipe *ThoughtRecipe, tags map[string]struct{}) bool {
	if recipe == nil {
		return false
	}
	for _, tag := range recipe.Metadata.Families {
		if _, ok := tags[normalizeTriggerTag(tag)]; ok {
			return true
		}
	}
	for _, tag := range recipe.Metadata.Keywords {
		if _, ok := tags[normalizeTriggerTag(tag)]; ok {
			return true
		}
	}
	for _, tag := range recipe.Metadata.Tags {
		if _, ok := tags[normalizeTriggerTag(tag)]; ok {
			return true
		}
	}
	return false
}

func recipeEntryMatchesHandoffTarget(recipe *ThoughtRecipe, target string) bool {
	if recipe == nil {
		return false
	}
	for _, candidate := range recipe.Metadata.HandoffTargets {
		if normalizeTriggerTag(candidate) == target {
			return true
		}
	}
	return false
}

func scoreThoughtRecipeEntry(recipe *ThoughtRecipe, tokens map[string]struct{}) (int, []string) {
	if recipe == nil {
		return 0, nil
	}
	score := 0
	reasons := make([]string, 0, 4)
	id := normalizeTriggerTag(recipe.ID)
	name := normalizeTriggerTag(recipe.EffectiveName())
	if id != "" {
		if _, ok := tokens[id]; ok {
			score += 100
			reasons = append(reasons, "thoughtrecipe_id")
		}
	}
	if name != "" {
		if _, ok := tokens[name]; ok {
			score += 90
			reasons = append(reasons, "thoughtrecipe_name")
		}
	}
	for _, tag := range recipe.Metadata.HandoffTargets {
		if _, ok := tokens[normalizeTriggerTag(tag)]; ok {
			score += 80
			reasons = append(reasons, "handoff_target")
			break
		}
	}
	for _, tag := range recipe.Metadata.Families {
		if _, ok := tokens[normalizeTriggerTag(tag)]; ok {
			score += 40
			reasons = append(reasons, "family")
			break
		}
	}
	for _, tag := range recipe.Metadata.Keywords {
		if _, ok := tokens[normalizeTriggerTag(tag)]; ok {
			score += 30
			reasons = append(reasons, "keyword")
			break
		}
	}
	for _, tag := range recipe.Metadata.Tags {
		if _, ok := tokens[normalizeTriggerTag(tag)]; ok {
			score += 20
			reasons = append(reasons, "tag")
			break
		}
	}
	return score, reasons
}

func normalizeSearchTokens(tokens ...string) map[string]struct{} {
	normalized := make(map[string]struct{})
	for _, token := range tokens {
		for _, part := range strings.FieldsFunc(strings.ToLower(strings.TrimSpace(token)), func(r rune) bool {
			return !(r == '_' || r == '-' || r == ':' || r == '.' || r == '/' || r == '\\' || ('a' <= r && r <= 'z') || ('0' <= r && r <= '9'))
		}) {
			if normalizedToken := normalizeTriggerTag(part); normalizedToken != "" {
				normalized[normalizedToken] = struct{}{}
			}
		}
	}
	return normalized
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
