package thoughtrecipe

import (
	"fmt"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// Entry stores a thoughtrecipe and its compiled DSL plan.
type Entry struct {
	Name          string
	ThoughtRecipe *surface.ThoughtRecipe
	Plan          *ExecutionPlan
	Source        string
}

// Registry manages ThoughtRecipe definitions and compiled Euclo plans.
type Registry struct {
	mu             sync.RWMutex
	thoughtrecipes map[string]*Entry
	order          []string
	vocabulary     ToolVocabularyLookup
	handoffs       map[string]string
}

// SetHandoffs installs the workspace handoff table (families.yaml). Nil or
// empty clears the table: handoff resolution then degrades to its named
// runtime miss instead of consulting compiled-in data.
func (r *Registry) SetHandoffs(handoffs map[string]string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handoffs = handoffs
}

// HandoffForFamily resolves a classified family to its handoff recipe ID via
// the workspace handoff table. Empty means no mapping exists.
func (r *Registry) HandoffForFamily(family string) string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handoffs[strings.ToLower(strings.TrimSpace(family))]
}

// Handoffs returns a copy of the workspace handoff table (boot assertions
// iterate it; callers must not mutate the registry's map).
func (r *Registry) Handoffs() map[string]string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.handoffs))
	for family, target := range r.handoffs {
		out[family] = target
	}
	return out
}

// WithToolVocabulary wires the declared-vocabulary catalog used by the
// programmatic FR-14 arm: compiled plans registered through this registry
// must resolve their full `may invoke` vocabulary against it.
func (r *Registry) WithToolVocabulary(vocabulary ToolVocabularyLookup) *Registry {
	if r != nil {
		r.vocabulary = vocabulary
	}
	return r
}

// validatePlanToolVocabulary resolves every step's declared tool names
// (steps, route branches, pipeline stages) against the wired vocabulary.
func (r *Registry) validatePlanToolVocabulary(plan *ExecutionPlan) error {
	if r == nil || r.vocabulary == nil || plan == nil {
		return nil
	}
	recipeName := ""
	if plan.ThoughtRecipe != nil {
		recipeName = plan.ThoughtRecipe.Name
	}
	walk := func(steps []ExecutionStep) error {
		for _, step := range steps {
			if err := ResolveDeclaredToolNames(recipeName, step.ID, step.Scope.AllowedToolNames(), r.vocabulary); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(plan.Steps); err != nil {
		return err
	}
	for _, group := range plan.Routes {
		for _, branch := range group.Branches {
			if err := walk(branch.Steps); err != nil {
				return err
			}
		}
	}
	for _, pipeline := range plan.Pipelines {
		for _, stage := range pipeline.Stages {
			if err := walk(stage.Steps); err != nil {
				return err
			}
		}
	}
	return nil
}

// NewThoughtRecipeRegistry creates a new thoughtrecipe registry.
func NewThoughtRecipeRegistry() *Registry {
	return &Registry{
		thoughtrecipes: make(map[string]*Entry),
	}
}

// Register registers a thoughtrecipe in the registry.
func (r *Registry) Register(thoughtrecipe *surface.ThoughtRecipe) error {
	_, err := r.registerThoughtRecipe(thoughtrecipe, nil, "", false)
	return err
}

// RegisterFirstWins registers a thoughtrecipe if the name is not already present.
func (r *Registry) RegisterFirstWins(thoughtrecipe *surface.ThoughtRecipe) (bool, error) {
	return r.registerThoughtRecipe(thoughtrecipe, nil, "", true)
}

// RegisterCompiled registers a compiled thoughtrecipe and its source thoughtrecipe.
func (r *Registry) RegisterCompiled(thoughtrecipe *surface.ThoughtRecipe, plan *ExecutionPlan, source string) error {
	_, err := r.registerThoughtRecipe(thoughtrecipe, plan, source, false)
	return err
}

// RegisterCompiledFirstWins registers a compiled thoughtrecipe only if the name is new.
func (r *Registry) RegisterCompiledFirstWins(thoughtrecipe *surface.ThoughtRecipe, plan *ExecutionPlan, source string) (bool, error) {
	return r.registerThoughtRecipe(thoughtrecipe, plan, source, true)
}

func (r *Registry) registerThoughtRecipe(thoughtrecipe *surface.ThoughtRecipe, plan *ExecutionPlan, source string, firstWins bool) (bool, error) {
	if thoughtrecipe == nil {
		return false, fmt.Errorf("thoughtrecipe is nil")
	}
	// Programmatic and loader-fed plans must pass the same paradigm contract
	// validation at registration (D3): the loader already validated the AST,
	// so this is the belt that covers plans built in Go.
	if plan != nil {
		if err := ValidatePlanContracts(plan, paradigm.Registry); err != nil {
			return false, err
		}
		// FR-14 programmatic arm: a plan whose declared tool vocabulary does
		// not resolve in the capability registry is a registration error —
		// the same loudness the loader applies at parse time (D2).
		if err := r.validatePlanToolVocabulary(plan); err != nil {
			return false, err
		}
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
	r.thoughtrecipes[key] = &Entry{
		Name:          key,
		ThoughtRecipe: thoughtrecipe,
		Plan:          plan,
		Source:        strings.TrimSpace(source),
	}
	return true, nil
}

// Get retrieves a thoughtrecipe by ID.
func (r *Registry) Get(id string) (*surface.ThoughtRecipe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.lookupEntryLocked(strings.TrimSpace(id))
	if !ok || entry == nil {
		return nil, false
	}
	return entry.ThoughtRecipe, true
}

// GetPlan retrieves a compiled plan by thoughtrecipe name.
func (r *Registry) GetPlan(name string) (*ExecutionPlan, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.lookupEntryLocked(strings.TrimSpace(name))
	if !ok || entry == nil || entry.Plan == nil {
		return nil, false
	}
	return entry.Plan, true
}

// List returns all registered thoughtrecipe IDs.
func (r *Registry) List() []string {
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
func (r *Registry) Remove(id string) {
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
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.thoughtrecipes)
}

// Entries returns the registered thoughtrecipe entries in insertion order.
func (r *Registry) Entries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Entry, 0, len(r.order))
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
func (r *Registry) FindByTag(tag string) []Entry {
	return r.findByTags([]string{tag})
}

// FindByFamily returns entries whose trigger family metadata includes the provided family.
func (r *Registry) FindByFamily(family string) []Entry {
	return r.findByTags([]string{family})
}

// FindByKeyword returns entries whose trigger keyword metadata includes the provided keyword.
func (r *Registry) FindByKeyword(keyword string) []Entry {
	return r.findByTags([]string{keyword})
}

// FindByHandoffTarget returns entries whose declared handoff targets include the provided target.
func (r *Registry) FindByHandoffTarget(target string) []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := normalizeTriggerTag(target)
	if normalized == "" {
		return nil
	}

	out := make([]Entry, 0)
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
func (r *Registry) FindMatchingTags(tags ...string) []Entry {
	return r.findByTags(tags)
}

// ResolveBestMatch returns the highest-scoring thoughtrecipe match for the
// provided explicit identifier and normalized search tokens.
func (r *Registry) ResolveBestMatch(explicitID string, tokens ...string) (Entry, bool, []string) {
	if r == nil {
		return Entry{}, false, nil
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
		return Entry{}, false, nil
	}

	var (
		bestEntry   Entry
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
		return Entry{}, false, nil
	}
	return bestEntry, true, bestReasons
}

func (r *Registry) findByTags(tags []string) []Entry {
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

	out := make([]Entry, 0)
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

func recipeEntryMatchesTags(recipe *surface.ThoughtRecipe, tags map[string]struct{}) bool {
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

func recipeEntryMatchesHandoffTarget(recipe *surface.ThoughtRecipe, target string) bool {
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

func scoreThoughtRecipeEntry(recipe *surface.ThoughtRecipe, tokens map[string]struct{}) (int, []string) {
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
			return r != '_' && r != '-' && r != ':' && r != '.' && r != '/' && r != '\\' && (r < 'a' || r > 'z') && (r < '0' || r > '9')
		}) {
			if normalizedToken := normalizeTriggerTag(part); normalizedToken != "" {
				normalized[normalizedToken] = struct{}{}
			}
		}
	}
	return normalized
}

func registryKeyForThoughtRecipe(thoughtrecipe *surface.ThoughtRecipe) string {
	if thoughtrecipe == nil {
		return ""
	}
	if name := strings.TrimSpace(thoughtrecipe.EffectiveName()); name != "" {
		return name
	}
	return strings.TrimSpace(thoughtrecipe.ID)
}

func (r *Registry) lookupEntryLocked(key string) (*Entry, bool) {
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
