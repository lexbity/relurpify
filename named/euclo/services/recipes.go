package services

import (
	"errors"
	"os"

	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// defaultThoughtRecipeLoader implements ThoughtRecipeLoader using the Euclo DSL source scan.
type defaultThoughtRecipeLoader struct{}

func (r *defaultThoughtRecipeLoader) LoadAll() (*thoughtrecipepkg.LoadResult, error) {
	loader := thoughtrecipepkg.NewLoader()
	result, err := loader.LoadWorkspace(".")
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
