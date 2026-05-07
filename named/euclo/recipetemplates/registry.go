package recipetemplates

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// LoadAll loads all embedded YAML recipe templates into a RecipeRegistry.
func LoadAll() (*recipepkg.RecipeRegistry, error) {
	registry := recipepkg.NewRecipeRegistry()
	loader := recipepkg.NewLoader()

	if err := loadTemplateDir(registry, loader, "."); err != nil {
		return registry, err
	}

	return registry, nil
}

func loadTemplateDir(registry *recipepkg.RecipeRegistry, loader *recipepkg.Loader, dir string) error {
	entries, err := templateFS.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read template directory %s: %w", dir, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			paths = append(paths, filepath.ToSlash(filepath.Join(dir, name))+"/")
			continue
		}
		if filepath.Ext(name) != ".yaml" {
			continue
		}
		paths = append(paths, filepath.ToSlash(filepath.Join(dir, name)))
	}
	sort.Strings(paths)
	for _, path := range paths {
		if strings.HasSuffix(path, "/") {
			if err := loadTemplateDir(registry, loader, strings.TrimSuffix(path, "/")); err != nil {
				return err
			}
			continue
		}
		data, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template file %s: %w", path, err)
		}
		recipe, err := loader.LoadFromBytes(data)
		if err != nil {
			return fmt.Errorf("failed to parse template file %s: %w", path, err)
		}
		if err := registry.Register(recipe); err != nil {
			return fmt.Errorf("failed to register recipe %s: %w", recipe.ID, err)
		}
	}
	return nil
}
