package recipe

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ThoughtRecipe defines the YAML schema for thought recipes.
//
// It supports both the legacy flat schema used by the current tests
// (`id`, `name`, `steps`) and the newer spec-shaped schema
// (`apiVersion`, `metadata.name`, `global`, `sequence`).
type ThoughtRecipe struct {
	APIVersion  string         `yaml:"apiVersion,omitempty"`
	Kind        string         `yaml:"kind,omitempty"`
	ID          string         `yaml:"id,omitempty"`
	Name        string         `yaml:"name,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Version     string         `yaml:"version,omitempty"`
	Metadata    RecipeMetadata `yaml:"metadata,omitempty"`
	Global      RecipeGlobal   `yaml:"global,omitempty"`
	Sequence    RecipeSequence `yaml:"sequence,omitempty"`
	Steps       []RecipeStep   `yaml:"steps,omitempty"`
}

// RecipeGlobal contains global configuration for the recipe.
type RecipeGlobal struct {
	Prompt        string              `yaml:"prompt,omitempty"`
	Configuration RecipeConfiguration `yaml:"configuration,omitempty"`
	Context       RecipeContext       `yaml:"context,omitempty"`

	// Legacy compatibility fields.
	Config      map[string]any    `yaml:"config,omitempty"`
	Bindings    map[string]string `yaml:"bindings,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Timeout     string            `yaml:"timeout,omitempty"`
}

// RecipeConfiguration mirrors the spec's global configuration block.
type RecipeConfiguration struct {
	Families               []string `yaml:"families,omitempty"`
	IntentKeywords         []string `yaml:"intent_keywords,omitempty"`
	TriggerPriority        int      `yaml:"trigger_priority,omitempty"`
	AllowDynamicResolution bool     `yaml:"allow_dynamic_resolution,omitempty"`
}

// RecipeContext mirrors the spec's global context block.
type RecipeContext struct {
	Stream  *RecipeStreamSpec `yaml:"stream,omitempty"`
	Ingest  *RecipeIngestSpec `yaml:"ingest,omitempty"`
	Sharing string            `yaml:"sharing,omitempty"`
	Aliases map[string]string `yaml:"aliases,omitempty"`
}

// RecipeSequence defines the sequence of steps to execute.
type RecipeSequence struct {
	Steps       []RecipeStep       `yaml:"steps,omitempty"`
	Parallel    []ParallelGroup    `yaml:"parallel,omitempty"`
	Conditional []ConditionalGroup `yaml:"conditional,omitempty"`
}

// UnmarshalYAML accepts both the spec-shaped list form and the legacy mapping
// form that wraps steps under a `steps` key.
func (s *RecipeSequence) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.SequenceNode:
		var steps []RecipeStep
		if err := value.Decode(&steps); err != nil {
			return err
		}
		s.Steps = steps
		return nil
	case yaml.MappingNode:
		type alias RecipeSequence
		var decoded alias
		if err := value.Decode(&decoded); err != nil {
			return err
		}
		*s = RecipeSequence(decoded)
		return nil
	default:
		return fmt.Errorf("sequence must be a mapping or sequence")
	}
}

// RecipeStep represents a single step in the recipe sequence.
type RecipeStep struct {
	ID           string            `yaml:"id,omitempty"`
	Type         string            `yaml:"type,omitempty"`
	Description  string            `yaml:"description,omitempty"`
	Mutation     string            `yaml:"mutation,omitempty"`
	HITL         string            `yaml:"hitl,omitempty"`
	Parent       RecipeStepAgent   `yaml:"parent,omitempty"`
	Fallback     *RecipeStepAgent  `yaml:"fallback,omitempty"`
	Context      RecipeStepContext `yaml:"context,omitempty"`
	Config       map[string]any    `yaml:"config,omitempty"`
	Captures     map[string]string `yaml:"captures,omitempty"`
	Bindings     map[string]string `yaml:"bindings,omitempty"`
	Dependencies []string          `yaml:"dependencies,omitempty"` // Legacy DAG support.

	// OnError defines error handling behavior for this step.
	OnError *StepErrorPolicy `yaml:"on_error,omitempty"`

	// Retry defines retry policy for this step.
	Retry *StepRetryPolicy `yaml:"retry,omitempty"`
}

// RecipeStepAgent describes the paradigm-specific agent invocation for a step.
type RecipeStepAgent struct {
	Paradigm string            `yaml:"paradigm,omitempty"`
	Prompt   string            `yaml:"prompt,omitempty"`
	Context  RecipeStepContext `yaml:"context,omitempty"`
}

// RecipeStepContext mirrors the spec's per-step context block.
type RecipeStepContext struct {
	Stream  *RecipeStreamSpec `yaml:"stream,omitempty"`
	Ingest  *RecipeIngestSpec `yaml:"ingest,omitempty"`
	Inherit []string          `yaml:"inherit,omitempty"`
	Capture []string          `yaml:"capture,omitempty"`
}

// RecipeStreamSpec configures a context stream request.
type RecipeStreamSpec struct {
	QueryTemplate string `yaml:"query_template,omitempty"`
	MaxTokens     int    `yaml:"max_tokens,omitempty"`
	Mode          string `yaml:"mode,omitempty"`
}

// RecipeIngestSpec configures ingestion for a recipe or step.
type RecipeIngestSpec struct {
	Mode          string   `yaml:"mode,omitempty"`
	IncludeGlobs  []string `yaml:"include_globs,omitempty"`
	ExcludeGlobs  []string `yaml:"exclude_globs,omitempty"`
	WorkspaceRoot string   `yaml:"workspace_root,omitempty"`
}

// ParallelGroup defines a group of steps to execute in parallel.
type ParallelGroup struct {
	ID    string       `yaml:"id"`
	Steps []RecipeStep `yaml:"steps"`
	Merge MergePolicy  `yaml:"merge"`
}

// ConditionalGroup defines conditional execution logic.
type ConditionalGroup struct {
	ID        string       `yaml:"id"`
	Condition string       `yaml:"condition"`
	If        []RecipeStep `yaml:"if"`
	Else      []RecipeStep `yaml:"else,omitempty"`
}

// MergePolicy defines how to merge parallel step results.
type MergePolicy string

const (
	MergePolicyAll    MergePolicy = "all"    // All branches must succeed
	MergePolicyAny    MergePolicy = "any"    // At least one branch must succeed
	MergePolicyFirst  MergePolicy = "first"  // Use first successful result
	MergePolicyConcat MergePolicy = "concat" // Concatenate all results
)

// StepErrorPolicy defines error handling for a step.
type StepErrorPolicy struct {
	Action   string `yaml:"action"` // "continue", "fail", "retry"
	RetryMax int    `yaml:"retry_max,omitempty"`
	Fallback string `yaml:"fallback,omitempty"` // Step ID to execute on failure
}

// StepRetryPolicy defines retry behavior for a step.
type StepRetryPolicy struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Delay       string `yaml:"delay"`   // e.g., "1s", "5m"
	Backoff     string `yaml:"backoff"` // "linear" or "exponential"
	MaxDelay    string `yaml:"max_delay,omitempty"`
}

// RecipeMetadata contains optional metadata about a recipe.
type RecipeMetadata struct {
	Name      string   `yaml:"name,omitempty"`
	Version   string   `yaml:"version,omitempty"`
	Author    string   `yaml:"author,omitempty"`
	Tags      []string `yaml:"tags,omitempty"`
	Category  string   `yaml:"category,omitempty"`
	CreatedAt string   `yaml:"created_at,omitempty"`
	UpdatedAt string   `yaml:"updated_at,omitempty"`
}

// StepList returns the ordered steps, preferring the spec-shaped sequence block
// and falling back to the legacy top-level steps field.
func (r *ThoughtRecipe) StepList() []RecipeStep {
	if r == nil {
		return nil
	}
	if len(r.Sequence.Steps) > 0 {
		return r.Sequence.Steps
	}
	return r.Steps
}

// EffectiveName returns the best available human-readable recipe name.
func (r *ThoughtRecipe) EffectiveName() string {
	if r == nil {
		return ""
	}
	if strings.TrimSpace(r.Metadata.Name) != "" {
		return strings.TrimSpace(r.Metadata.Name)
	}
	if strings.TrimSpace(r.Name) != "" {
		return strings.TrimSpace(r.Name)
	}
	return strings.TrimSpace(r.ID)
}

func (r *ThoughtRecipe) usesSpecSchema() bool {
	if r == nil {
		return false
	}
	if strings.TrimSpace(r.APIVersion) != "" || strings.TrimSpace(r.Kind) != "" {
		return true
	}
	if strings.TrimSpace(r.Metadata.Name) != "" || strings.TrimSpace(r.Global.Prompt) != "" {
		return true
	}
	if len(r.Sequence.Steps) > 0 || len(r.Sequence.Parallel) > 0 || len(r.Sequence.Conditional) > 0 {
		return true
	}
	if len(r.Global.Configuration.Families) > 0 || len(r.Global.Configuration.IntentKeywords) > 0 {
		return true
	}
	if r.Global.Context.Stream != nil || r.Global.Context.Ingest != nil || len(r.Global.Context.Aliases) > 0 {
		return true
	}
	for _, step := range r.StepList() {
		if strings.TrimSpace(step.Mutation) != "" || strings.TrimSpace(step.HITL) != "" || strings.TrimSpace(step.Parent.Paradigm) != "" {
			return true
		}
		if step.Parent.Context.Stream != nil || step.Parent.Context.Ingest != nil || len(step.Parent.Context.Inherit) > 0 || len(step.Parent.Context.Capture) > 0 {
			return true
		}
		if step.Fallback != nil {
			return true
		}
	}
	return false
}

// Validate validates the ThoughtRecipe schema.
func (r *ThoughtRecipe) Validate() error {
	if r == nil {
		return fmt.Errorf("recipe is nil")
	}
	if r.usesSpecSchema() {
		return r.validateSpecSchema()
	}
	return r.validateLegacySchema()
}

func (r *ThoughtRecipe) validateLegacySchema() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("recipe missing required field: id")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("recipe missing required field: name")
	}
	steps := r.StepList()
	if len(steps) == 0 && len(r.Sequence.Parallel) == 0 {
		return fmt.Errorf("recipe must have at least one step or parallel group")
	}

	seenStepIDs := make(map[string]bool)
	for i, step := range steps {
		if err := validateStep(&step, i, seenStepIDs, false); err != nil {
			return err
		}
	}
	if err := validateStepDependencies(steps, seenStepIDs); err != nil {
		return err
	}
	for i, pg := range r.Sequence.Parallel {
		if pg.ID == "" {
			return fmt.Errorf("parallel group %d missing required field: id", i)
		}
		for j, step := range pg.Steps {
			if err := validateStep(&step, j, seenStepIDs, false); err != nil {
				return fmt.Errorf("parallel group %s, step %d: %w", pg.ID, j, err)
			}
		}
	}
	return nil
}

func (r *ThoughtRecipe) validateSpecSchema() error {
	if strings.TrimSpace(r.APIVersion) == "" {
		return fmt.Errorf("recipe missing required field: apiVersion")
	}
	if strings.TrimSpace(r.EffectiveName()) == "" {
		return fmt.Errorf("recipe missing required field: metadata.name")
	}
	steps := r.StepList()
	if len(steps) == 0 {
		return fmt.Errorf("recipe must have at least one sequence step")
	}

	seenStepIDs := make(map[string]bool)
	for i, step := range steps {
		if err := validateStep(&step, i, seenStepIDs, true); err != nil {
			return err
		}
	}
	if err := validateStepDependencies(steps, seenStepIDs); err != nil {
		return err
	}
	return nil
}

// validateStep validates a single recipe step.
func validateStep(step *RecipeStep, index int, seenStepIDs map[string]bool, specMode bool) error {
	if step.ID == "" {
		return fmt.Errorf("step %d missing required field: id", index)
	}
	if seenStepIDs[step.ID] {
		return fmt.Errorf("duplicate step ID: %s", step.ID)
	}
	seenStepIDs[step.ID] = true

	if specMode {
		if err := validateStepMutation(step.Mutation); err != nil {
			return fmt.Errorf("step %s: %w", step.ID, err)
		}
		if err := validateStepHITL(step.HITL); err != nil {
			return fmt.Errorf("step %s: %w", step.ID, err)
		}
		if err := validateStepParadigm(step.Parent.Paradigm); err != nil {
			return fmt.Errorf("step %s: %w", step.ID, err)
		}
		return nil
	}

	if step.Type == "" {
		return fmt.Errorf("step %d missing required field: type", index)
	}
	if !isValidStepType(step.Type) {
		return fmt.Errorf("invalid step type: %s", step.Type)
	}
	return nil
}

func validateStepMutation(value string) error {
	switch strings.TrimSpace(value) {
	case "", "none", "required":
		return nil
	default:
		return fmt.Errorf("invalid mutation value: %s", value)
	}
}

func validateStepHITL(value string) error {
	switch strings.TrimSpace(value) {
	case "", "ask", "always", "never":
		return nil
	default:
		return fmt.Errorf("invalid hitl value: %s", value)
	}
}

func validateStepParadigm(value string) error {
	switch strings.TrimSpace(value) {
	case "", "react", "planner", "htn", "reflection", "blackboard", "chainer", "pipeline", "rewoo", "goalcon":
		return nil
	default:
		return fmt.Errorf("invalid paradigm value: %s", value)
	}
}

func validateStepDependencies(steps []RecipeStep, seen map[string]bool) error {
	for _, step := range steps {
		for _, dep := range step.Dependencies {
			if !seen[dep] {
				return fmt.Errorf("dependency %q not found for step %s", dep, step.ID)
			}
		}
	}
	return nil
}

// isValidStepType checks if a legacy step type is valid.
func isValidStepType(stepType string) bool {
	validTypes := map[string]bool{
		"llm":          true,
		"retrieve":     true,
		"ingest":       true,
		"transform":    true,
		"emit":         true,
		"gate":         true,
		"branch":       true,
		"capture":      true,
		"verify":       true,
		"policy_check": true,
		"telemetry":    true,
		"custom":       true,
	}
	return validTypes[stepType]
}
