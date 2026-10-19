package thoughtrecipe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoRecipeDir reports that the resolved workspace has no
// relurpify_cfg/euclo directory. After workspace-truth resolution (D-8) the
// loader never sees a zero-value workspace, so a missing recipe directory is
// a boot error, never a tolerated empty registry.
var ErrNoRecipeDir = errors.New("no recipes")

// Loader scans Euclo DSL thoughtrecipe sources from the workspace.
type Loader struct {
	PromptRegistry PromptRegistryLookup
	RecipeRegistry RegistryLookup
	// CapabilityRegistry is the single declared-vocabulary catalog (D1/FR-5):
	// `do relurpic` references, coordination metadata, and the D2
	// `may invoke` load check all resolve through it. Production wiring
	// passes the composed capability registry — the same instance step-time
	// scoping resolves against.
	CapabilityRegistry ToolVocabularyLookup
}

// LoadWarningKind classifies a non-fatal loader diagnostic so surfaces (the
// TUI recipe list, boot telemetry) can react to kinds, not parse messages.
type LoadWarningKind string

const (
	// LoadWarningKindDirectory marks a directory ignored by the top-level scan.
	LoadWarningKindDirectory LoadWarningKind = "ignored_directory"
	// LoadWarningKindExtension marks a file with an unsupported extension.
	LoadWarningKindExtension LoadWarningKind = "ignored_extension"
	// LoadWarningKindDuplicate marks a duplicate recipe name; first wins.
	LoadWarningKindDuplicate LoadWarningKind = "duplicate_ignored"
	// LoadWarningKindVocabularyNotCallable marks a declared `may invoke` name
	// that resolves only to a governed capability — legal vocabulary, but not
	// a model-callable tool, so the model cannot invoke it.
	LoadWarningKindVocabularyNotCallable LoadWarningKind = "vocabulary_not_model_callable"
)

// LoadWarning captures a non-fatal loader diagnostic.
type LoadWarning struct {
	Path    string
	Message string
	Kind    LoadWarningKind
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
	Registry   *Registry
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
func (l *Loader) WithRecipeRegistry(reg RegistryLookup) *Loader {
	l.RecipeRegistry = reg
	return l
}

// WithCapabilityRegistry wires capability lookup into the loader's semantic checks.
func (l *Loader) WithCapabilityRegistry(reg ToolVocabularyLookup) *Loader {
	l.CapabilityRegistry = reg
	return l
}

// LoadWorkspace scans the Euclo source root under workspaceRoot and returns the
// candidate thoughtrecipe files in lexical order.
func (l *Loader) LoadWorkspace(workspaceRoot string) (*LoadResult, error) {
	trimmedRoot := strings.TrimSpace(workspaceRoot)
	if trimmedRoot == "" {
		return nil, fmt.Errorf("recipe workspace root is required; refusing CWD-relative scan")
	}
	root := filepath.Join(trimmedRoot, ThoughtRecipeSourceRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s missing under workspace %q", ErrNoRecipeDir, ThoughtRecipeSourceRoot, trimmedRoot)
		}
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
		Root:       trimmedRoot,
		SourceRoot: root,
		Registry:   NewThoughtRecipeRegistry(),
	}
	result.Registry.WithToolVocabulary(l.CapabilityRegistry)

	for _, name := range names {
		entry := byName[name]
		fullPath := filepath.Join(root, name)
		if entry.IsDir() {
			result.Warnings = append(result.Warnings, LoadWarning{
				Path:    fullPath,
				Message: "ignored directory during top-level-only thoughtrecipe scan",
				Kind:    LoadWarningKindDirectory,
			})
			continue
		}

		ext := filepath.Ext(name)
		if !IsAcceptedThoughtRecipeExtension(ext) {
			// families.yaml is the workspace handoff table read alongside
			// the recipes — its presence here is expected, not a mistake.
			if name == FamiliesFileName {
				continue
			}
			result.Warnings = append(result.Warnings, LoadWarning{
				Path:    fullPath,
				Message: fmt.Sprintf("ignored unsupported thoughtrecipe extension %q", ext),
				Kind:    LoadWarningKindExtension,
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

	// The workspace handoff table (families.yaml) is workspace data read
	// alongside the recipes; the clarification handoff resolves through it.
	handoffs, err := LoadHandoffsFile(root)
	if err != nil {
		return nil, err
	}
	result.Registry.SetHandoffs(handoffs)

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
	plan, err := LowerDocumentWithRegistry(doc, l.CapabilityRegistry)
	if err != nil {
		return err
	}

	if err := validateCapabilityStepScopes(plan, source.Path); err != nil {
		return err
	}
	if ok, err := result.Registry.RegisterCompiledFirstWins(plan.ThoughtRecipe, plan, source.Path); err != nil {
		return err
	} else if !ok {
		result.Warnings = append(result.Warnings, LoadWarning{
			Path:    source.Path,
			Message: fmt.Sprintf("duplicate thoughtrecipe name %q ignored; first registration wins", plan.ThoughtRecipe.Name),
			Kind:    LoadWarningKindDuplicate,
		})
	}
	result.Warnings = append(result.Warnings, l.vocabularyVisibilityWarnings(plan, source.Path)...)
	return nil
}

// vocabularyVisibilityWarnings reports declared `may invoke` names that resolve
// in the registry only as governed capabilities: legal vocabulary, but the
// model's tool list never contains them, so react steps cannot invoke them.
// The recipe still loads (the name is enforceable scope), and boot telemetry
// surfaces the warning so the disarmed expectation is observable (P1).
func (l *Loader) vocabularyVisibilityWarnings(plan *ExecutionPlan, sourcePath string) []LoadWarning {
	if l.CapabilityRegistry == nil || plan == nil {
		return nil
	}
	recipeName := ""
	if plan.ThoughtRecipe != nil {
		recipeName = plan.ThoughtRecipe.Name
	}
	var warnings []LoadWarning
	walk := func(steps []ExecutionStep) {
		for _, step := range steps {
			for _, tool := range FindNonCallableDeclaredTools(step.Scope.AllowedToolNames(), l.CapabilityRegistry) {
				warnings = append(warnings, LoadWarning{
					Path: sourcePath,
					Message: fmt.Sprintf(
						"recipe %q step %q: may invoke %q resolves to a governed capability, not a model-callable tool; the model tool list does not contain it, so react steps cannot invoke it (invoke via a do step or expose a callable tool)",
						recipeName, step.ID, tool),
					Kind: LoadWarningKindVocabularyNotCallable,
				})
			}
		}
	}
	walk(plan.Steps)
	for _, group := range plan.Routes {
		for _, branch := range group.Branches {
			walk(branch.Steps)
		}
	}
	return warnings
}

// validateCapabilityStepScopes is the belt-to-the-lowering's-suspenders load
// invariant: every capability step MUST carry a resolved scope. The lowering
// always sets one (the named capability), so a violation means a lowering
// regression, not author error — fail the load naming file and position.
func validateCapabilityStepScopes(plan *ExecutionPlan, sourcePath string) error {
	if plan == nil {
		return nil
	}
	walk := func(steps []ExecutionStep) error {
		for _, step := range steps {
			if step.Kind == StepKindCapability && !step.Scope.IsResolved() {
				return fmt.Errorf("%s: capability_step_missing_scope: capability step %q has no resolved scope", sourcePath, step.ID)
			}
		}
		return nil
	}
	if err := walk(plan.Steps); err != nil {
		return err
	}
	for _, route := range plan.Routes {
		for _, branch := range route.Branches {
			if err := walk(branch.Steps); err != nil {
				return err
			}
		}
	}
	return nil
}
