package services

import (
	"strings"

	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// defaultThoughtRecipeLoader implements ThoughtRecipeLoader using the Euclo DSL source scan.
type defaultThoughtRecipeLoader struct{}

// LoadAll loads every thoughtrecipe under the given workspace. The workspace
// is always resolved (D-8): a missing relurpify_cfg/euclo directory propagates
// thoughtrecipes.ErrNoRecipeDir instead of degrading to an empty registry.
func (r *defaultThoughtRecipeLoader) LoadAll(workspace string, caps thoughtrecipepkg.ToolVocabularyLookup) (*thoughtrecipepkg.LoadResult, error) {
	// One catalog (FR-5): the same registry instance backs the loader's
	// capability references and the declared-vocabulary load check.
	loader := thoughtrecipepkg.NewLoader().WithCapabilityRegistry(caps)
	return loader.LoadWorkspace(strings.TrimSpace(workspace))
}
