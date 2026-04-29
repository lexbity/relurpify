package recipe

import (
	"fmt"
	"strings"
)

// AliasResolver maps human-readable capture names to envelope keys.
type AliasResolver struct {
	RecipeName string
	Aliases    map[string]string
}

// NewAliasResolver creates a resolver for a specific recipe.
func NewAliasResolver(recipe *ThoughtRecipe) *AliasResolver {
	if recipe == nil {
		return &AliasResolver{}
	}
	recipeName := recipe.ID
	if strings.TrimSpace(recipeName) == "" {
		recipeName = recipe.EffectiveName()
	}
	aliases := make(map[string]string, len(recipe.Global.Context.Aliases))
	for k, v := range recipe.Global.Context.Aliases {
		aliases[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return &AliasResolver{
		RecipeName: recipeName,
		Aliases:    aliases,
	}
}

// Resolve returns the working-memory key for an alias name.
func (r *AliasResolver) Resolve(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	if r != nil {
		if resolved := strings.TrimSpace(r.Aliases[alias]); resolved != "" {
			return resolved
		}
		if strings.HasPrefix(alias, "euclo.recipe.") {
			return alias
		}
		if strings.TrimSpace(r.RecipeName) != "" {
			return fmt.Sprintf("euclo.recipe.%s.%s", sanitizeAliasComponent(r.RecipeName), sanitizeAliasComponent(alias))
		}
	}
	return fmt.Sprintf("euclo.recipe.%s", sanitizeAliasComponent(alias))
}

func sanitizeAliasComponent(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}
