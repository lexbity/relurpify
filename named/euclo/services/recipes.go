package services

import (
	"errors"
	"os"
	"strings"

	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// defaultThoughtRecipeLoader implements ThoughtRecipeLoader using the Euclo DSL source scan.
type defaultThoughtRecipeLoader struct{}

func (r *defaultThoughtRecipeLoader) LoadAll(workspace string, caps thoughtrecipepkg.CapabilityRegistryLookup) (*thoughtrecipepkg.LoadResult, error) {
	loader := thoughtrecipepkg.NewLoader().WithCapabilityRegistry(caps)
	result, err := loader.LoadWorkspace(strings.TrimSpace(workspace))
	if err == nil && result != nil {
		return result, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &thoughtrecipepkg.LoadResult{
		Registry: thoughtrecipepkg.NewThoughtRecipeRegistry(),
	}, nil
}
