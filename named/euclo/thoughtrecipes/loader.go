package thoughtrecipes

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	// ExpectedAPIVersion is the supported API version for thought recipes.
	ExpectedAPIVersion = "euclo/v1alpha1"
	// ExpectedKind is the expected Kind for thought recipes.
	ExpectedKind = "ThoughtRecipe"
	// DefaultTriggerPriority is the default trigger priority.
	DefaultTriggerPriority = 5
	// MaxTriggerPriority is the maximum allowed trigger priority.
	MaxTriggerPriority = 100
	// MinTriggerPriority is the minimum allowed trigger priority.
	MinTriggerPriority = 1
)

// Supported paradigms for validation.
var supportedParadigms = map[string]bool{
	"react":      true,
	"planner":    true,
	"htn":        true,
	"reflection": true,
	"goalcon":    true,
	"blackboard": true,
	"architect":  true,
	"chainer":    true,
}

// validNameRegex matches recipe names: [a-z][a-z0-9-]*
var validNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// LoadResult contains the result of loading recipes from a directory.
type LoadResult struct {
	Plans    []*ExecutionPlan
	Warnings []string
	Errors   []error
}

// LoadFile loads and validates a single thought recipe YAML file.
// Returns the ExecutionPlan on success, or an error on validation failure.
func LoadFile(path string) (*ExecutionPlan, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	recipe := &ThoughtRecipe{}
	if err := yaml.Unmarshal(data, recipe); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML %s: %w", path, err)
	}

	// Validate and compile
	plan, warnings, err := CompileExecutionPlan(recipe, path)
	if err != nil {
		return nil, nil, err
	}

	return plan, warnings, nil
}

// LoadAll loads all thought recipe YAML files from a directory.
// Returns successfully loaded recipes, warnings, and errors for invalid files.
func LoadAll(dir string) (*LoadResult, error) {
	result := &LoadResult{
		Plans:    make([]*ExecutionPlan, 0),
		Warnings: make([]string, 0),
		Errors:   make([]error, 0),
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Empty directory is not an error
			return result, nil
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Only process .yaml and .yml files
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(dir, name)
		plan, warnings, err := LoadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", name, err))
			continue
		}

		if len(warnings) > 0 {
			// Prefix warnings with filename
			for _, w := range warnings {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", name, w))
			}
		}

		result.Plans = append(result.Plans, plan)
	}

	return result, nil
}

// Validate checks a ThoughtRecipe for schema compliance.
// Returns a slice of validation errors. Empty slice = valid.
func Validate(recipe *ThoughtRecipe) []error {
	var errors []error

	if recipe == nil {
		return []error{fmt.Errorf("recipe is nil")}
	}

	// API version check
	if recipe.APIVersion != ExpectedAPIVersion {
		errors = append(errors, fmt.Errorf("apiVersion must be %q, got %q", ExpectedAPIVersion, recipe.APIVersion))
	}

	// Kind check
	if recipe.Kind != ExpectedKind {
		errors = append(errors, fmt.Errorf("kind must be %q, got %q", ExpectedKind, recipe.Kind))
	}

	// Metadata validation
	if recipe.Metadata.Name == "" {
		errors = append(errors, fmt.Errorf("metadata.name is required"))
	} else if !validNameRegex.MatchString(recipe.Metadata.Name) {
		errors = append(errors, fmt.Errorf("metadata.name %q must match [a-z][a-z0-9-]*", recipe.Metadata.Name))
	}

	if recipe.Metadata.Description == "" {
		errors = append(errors, fmt.Errorf("metadata.description is required"))
	}

	// Sequence validation
	if len(recipe.Sequence) == 0 {
		errors = append(errors, fmt.Errorf("sequence must have at least one step"))
	}

	// Step validation
	stepIDs := make(map[string]bool)
	for i, step := range recipe.Sequence {
		if step.ID == "" {
			errors = append(errors, fmt.Errorf("step %d: id is required", i))
		} else {
			if stepIDs[step.ID] {
				errors = append(errors, fmt.Errorf("step %d: duplicate step id %q", i, step.ID))
			}
			stepIDs[step.ID] = true
		}

		// Paradigm validation
		if step.Parent.Paradigm == "" {
			errors = append(errors, fmt.Errorf("step %s: parent.paradigm is required", step.ID))
		} else if !supportedParadigms[step.Parent.Paradigm] {
			errors = append(errors, fmt.Errorf("step %s: unsupported paradigm %q", step.ID, step.Parent.Paradigm))
		}
	}

	return errors
}

// CompileExecutionPlan validates a ThoughtRecipe and compiles it into an ExecutionPlan.
// Warnings are non-fatal issues (like aliases shadowing standard aliases).
func CompileExecutionPlan(recipe *ThoughtRecipe, sourcePath string) (*ExecutionPlan, []string, error) {
	// First validate
	if errs := Validate(recipe); len(errs) > 0 {
		return nil, nil, errs[0] // Return first error
	}

	warnings := make([]string, 0)

	// Build alias resolver for validation
	resolver := NewAliasResolver(recipe.Global.Context.Aliases)

	// Check for shadowed aliases
	shadowed := resolver.ListShadowed()
	if len(shadowed) > 0 {
		warnings = append(warnings, fmt.Sprintf("custom aliases shadow standard aliases: %v", shadowed))
	}

	// Clamp trigger priority
	priority := recipe.Global.Configuration.TriggerPriority
	if priority == 0 {
		priority = DefaultTriggerPriority
	} else if priority > MaxTriggerPriority {
		warnings = append(warnings, fmt.Sprintf("trigger_priority %d clamped to %d", priority, MaxTriggerPriority))
		priority = MaxTriggerPriority
	} else if priority < MinTriggerPriority {
		warnings = append(warnings, fmt.Sprintf("trigger_priority %d clamped to %d", priority, MinTriggerPriority))
		priority = MinTriggerPriority
	}

	// Determine sharing mode
	sharingMode, _ := ValidateSharingMode(recipe.Global.Context.Sharing.Default)
	if sharingMode == "" {
		sharingMode = SharingModeExplicit // Default per spec
	}

	// Parse enrichment sources
	globalEnrichment := make([]EnrichmentSource, 0, len(recipe.Global.Context.Enrichment))
	for _, s := range recipe.Global.Context.Enrichment {
		if src, ok := ValidateEnrichmentSource(s); ok {
			globalEnrichment = append(globalEnrichment, src)
		} else {
			warnings = append(warnings, fmt.Sprintf("unknown enrichment source %q ignored", s))
		}
	}

	// Compile global prompt (template not yet rendered)
	globalPrompt := recipe.Global.Prompt

	// Build execution steps
	steps := make([]ExecutionStep, 0, len(recipe.Sequence))
	for _, step := range recipe.Sequence {
		execStep := compileStep(step, recipe.Global, warnings)
		steps = append(steps, execStep)

		// Check for child on non-delegating paradigm
		if step.Child != nil && step.Child.Paradigm != "" {
			if !IsParadigmWithDelegation(step.Parent.Paradigm) {
				warnings = append(warnings, fmt.Sprintf("step %s: child paradigm %q specified but parent %q has no delegation slot",
					step.ID, step.Child.Paradigm, step.Parent.Paradigm))
			}
		}
	}

	plan := &ExecutionPlan{
		Name:               recipe.Metadata.Name,
		Description:        recipe.Metadata.Description,
		Version:            recipe.Metadata.Version,
		FilePath:           sourcePath,
		GlobalCapabilities: recipe.Global.Capabilities.Allowed,
		GlobalEnrichment:   globalEnrichment,
		GlobalSharing:      sharingMode,
		GlobalAliases:      recipe.Global.Context.Aliases,
		GlobalPrompt:       globalPrompt,
		Resolver:           resolver,
		Steps:              steps,
		TriggerPriority:    priority,
		Modes:              recipe.Global.Configuration.Modes,
		IntentKeywords:     recipe.Global.Configuration.IntentKeywords,
		Warnings:           warnings,
	}

	return plan, warnings, nil
}

// compileStep converts a RecipeStep to an ExecutionStep.
func compileStep(step RecipeStep, global RecipeGlobal, warnings []string) ExecutionStep {
	execStep := ExecutionStep{
		ID: step.ID,
		Parent: ExecutionStepAgent{
			Paradigm: step.Parent.Paradigm,
			Prompt:   step.Parent.Prompt,
		},
		Inherit: step.Parent.Context.Inherit,
		Capture: step.Parent.Context.Capture,
	}

	// Compile parent capabilities (intersect with global)
	parentCaps := step.Parent.Capabilities.Allowed
	if len(parentCaps) == 0 {
		// Inherit from global
		execStep.AllowedCapabilities = global.Capabilities.Allowed
	} else if len(global.Capabilities.Allowed) == 0 {
		// No global restriction, use step restriction
		execStep.AllowedCapabilities = parentCaps
	} else {
		// Intersect step allowed with global allowed
		globalSet := make(map[string]bool)
		for _, c := range global.Capabilities.Allowed {
			globalSet[c] = true
		}
		intersected := make([]string, 0)
		for _, c := range parentCaps {
			if globalSet[c] {
				intersected = append(intersected, c)
			}
		}
		execStep.AllowedCapabilities = intersected
	}

	// Compile parent enrichment (additive to global)
	parentEnrichment := make([]EnrichmentSource, 0)
	for _, s := range step.Parent.Context.Enrichment {
		if src, ok := ValidateEnrichmentSource(s); ok {
			parentEnrichment = append(parentEnrichment, src)
		}
	}
	execStep.Parent.Enrichment = parentEnrichment

	// Compile child if present
	if step.Child != nil && step.Child.Paradigm != "" {
		child := &ExecutionStepAgent{
			Paradigm: step.Child.Paradigm,
			Prompt:   step.Child.Prompt,
		}

		// Child capabilities (intersect with global)
		if len(step.Child.Capabilities.Allowed) > 0 {
			child.Capabilities = step.Child.Capabilities.Allowed
		}

		// Child enrichment
		for _, s := range step.Child.Context.Enrichment {
			if src, ok := ValidateEnrichmentSource(s); ok {
				child.Enrichment = append(child.Enrichment, src)
			}
		}

		execStep.Child = child
	}

	// Compile fallback if present
	if step.Fallback != nil && step.Fallback.Paradigm != "" {
		fallback := &ExecutionStepAgent{
			Paradigm: step.Fallback.Paradigm,
			Prompt:   step.Fallback.Prompt,
		}

		if len(step.Fallback.Capabilities.Allowed) > 0 {
			fallback.Capabilities = step.Fallback.Capabilities.Allowed
		}

		execStep.Fallback = fallback
	}

	return execStep
}

// ResolveInherit returns the state keys for a list of inherit aliases.
func ResolveInherit(aliases []string, resolver *AliasResolver) []string {
	if resolver == nil {
		resolver = NewAliasResolver(nil)
	}

	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		stateKey, ok := resolver.Resolve(alias)
		if ok {
			result = append(result, stateKey)
		} else {
			// Unknown alias - use as-is
			result = append(result, alias)
		}
	}

	return result
}

// ResolveCapture returns the state keys for a list of capture aliases.
func ResolveCapture(aliases []string, resolver *AliasResolver) []string {
	// Same logic as inherit
	return ResolveInherit(aliases, resolver)
}

// PlanRegistry is a collection of loaded ExecutionPlans indexed by name.
type PlanRegistry struct {
	plans map[string]*ExecutionPlan
	mu    sync.RWMutex
}

// NewPlanRegistry creates a new empty plan registry.
func NewPlanRegistry() *PlanRegistry {
	return &PlanRegistry{
		plans: make(map[string]*ExecutionPlan),
	}
}

// Register adds a plan to the registry.
func (r *PlanRegistry) Register(plan *ExecutionPlan) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if plan.Name == "" {
		return fmt.Errorf("plan name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.plans[plan.Name] = plan
	return nil
}

// Get retrieves a plan by name.
func (r *PlanRegistry) Get(name string) (*ExecutionPlan, bool) {
	if r == nil {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	plan, ok := r.plans[name]
	return plan, ok
}

// List returns all registered plan names (sorted).
func (r *PlanRegistry) List() []string {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.plans))
	for name := range r.plans {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// Count returns the number of registered plans.
func (r *PlanRegistry) Count() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.plans)
}
