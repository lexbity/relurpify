package thoughtrecipe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Loader scans Euclo DSL thoughtrecipe sources from the workspace.
type Loader struct {
	PromptRegistry    PromptRegistryLookup
	RecipeRegistry    ThoughtRecipeRegistryLookup
	CapabilityRegistry CapabilityRegistryLookup
}

// LoadWarning captures a non-fatal loader diagnostic.
type LoadWarning struct {
	Path    string
	Message string
}

// SourceFile describes a candidate Euclo thoughtrecipe source discovered in the
// workspace.
type SourceFile struct {
	Path      string
	Name      string
	Extension string
}

// LoadResult captures the workspace scan result before DSL parsing is added.
type LoadResult struct {
	Root       string
	SourceRoot string
	Sources    []SourceFile
	Warnings   []LoadWarning
	Registry   *ThoughtRecipeRegistry
}

// NewLoader creates a new thoughtrecipe loader.
func NewLoader() *Loader {
	return &Loader{}
}

// WithPromptRegistry wires prompt lookup into the loader.
func (l *Loader) WithPromptRegistry(reg PromptRegistryLookup) *Loader {
	l.PromptRegistry = reg
	return l
}

// WithRecipeRegistry wires thoughtrecipe lookup into the loader.
func (l *Loader) WithRecipeRegistry(reg ThoughtRecipeRegistryLookup) *Loader {
	l.RecipeRegistry = reg
	return l
}

// WithCapabilityRegistry wires capability lookup into the loader's semantic checks.
func (l *Loader) WithCapabilityRegistry(reg CapabilityRegistryLookup) *Loader {
	l.CapabilityRegistry = reg
	return l
}

// LoadWorkspace scans the Euclo source root under workspaceRoot and returns the
// candidate thoughtrecipe files in lexical order.
func (l *Loader) LoadWorkspace(workspaceRoot string) (*LoadResult, error) {
	root := filepath.Join(strings.TrimSpace(workspaceRoot), ThoughtRecipeSourceRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read euclo thoughtrecipe source root %q: %w", root, err)
	}

	names := make([]string, 0, len(entries))
	byName := make(map[string]os.DirEntry, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		names = append(names, name)
		byName[name] = entry
	}
	sort.Strings(names)

	result := &LoadResult{
		Root:       strings.TrimSpace(workspaceRoot),
		SourceRoot: root,
		Registry:   NewThoughtRecipeRegistry(),
	}

	for _, name := range names {
		entry := byName[name]
		fullPath := filepath.Join(root, name)
		if entry.IsDir() {
			result.Warnings = append(result.Warnings, LoadWarning{
				Path:    fullPath,
				Message: "ignored directory during top-level-only thoughtrecipe scan",
			})
			continue
		}

		ext := filepath.Ext(name)
		if !IsAcceptedThoughtRecipeExtension(ext) {
			result.Warnings = append(result.Warnings, LoadWarning{
				Path:    fullPath,
				Message: fmt.Sprintf("ignored unsupported thoughtrecipe extension %q", ext),
			})
			continue
		}

		result.Sources = append(result.Sources, SourceFile{
			Path:      fullPath,
			Name:      strings.TrimSuffix(name, ext),
			Extension: ext,
		})
	}

	for _, source := range result.Sources {
		if err := l.loadThoughtRecipeSource(result, source); err != nil {
			return nil, fmt.Errorf("load thoughtrecipe source %q: %w", source.Path, err)
		}
	}

	return result, nil
}

func (l *Loader) loadThoughtRecipeSource(result *LoadResult, source SourceFile) error {
	if result == nil {
		return fmt.Errorf("load result is nil")
	}
	contents, err := os.ReadFile(source.Path)
	if err != nil {
		return fmt.Errorf("read thoughtrecipe source: %w", err)
	}

	doc, err := ParseSource(source.Path, string(contents))
	if err != nil {
		return err
	}
	if strings.TrimSpace(doc.Name) == "" {
		return fmt.Errorf("thoughtrecipe name is required")
	}

	if err := NewTypeSystem(doc).Validate(); err != nil {
		return err
	}
	symbols := NewSymbolTable(doc)
	symbols.WithCapabilityRegistry(l.CapabilityRegistry)
	symbols.WithPromptRegistry(l.PromptRegistry)
	if l.RecipeRegistry != nil {
		symbols.WithRecipeRegistry(l.RecipeRegistry)
	} else if result.Registry != nil {
		symbols.WithRecipeRegistry(result.Registry)
	}
	if err := symbols.Resolve(); err != nil {
		return err
	}
	plan, err := LowerDocument(doc)
	if err != nil {
		return err
	}

	if ok, err := result.Registry.RegisterCompiledFirstWins(plan.ThoughtRecipe, plan, source.Path); err != nil {
		return err
	} else if !ok {
		result.Warnings = append(result.Warnings, LoadWarning{
			Path:    source.Path,
			Message: fmt.Sprintf("duplicate thoughtrecipe name %q ignored; first registration wins", plan.ThoughtRecipe.Name),
		})
	}
	return nil
}
