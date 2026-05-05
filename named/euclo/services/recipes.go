package services

import (
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	"codeburg.org/lexbit/relurpify/named/euclo/recipetemplates"
)

// defaultRecipeLoader implements RecipeLoader using Euclo's built‑in recipe templates.
type defaultRecipeLoader struct{}

func (r *defaultRecipeLoader) LoadAll() (*recipepkg.RecipeRegistry, error) {
	return recipetemplates.LoadAll()
}
