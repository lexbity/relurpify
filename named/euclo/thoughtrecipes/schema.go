package thoughtrecipe

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// ThoughtRecipe defines the DSL-native in-memory thoughtrecipe model used by Euclo.
//
// The runtime consumes the spec-shaped DSL model and lowered execution plans.
// Execution semantics live in ExecutionPlan, not in this struct.
type ThoughtRecipe struct {
	RouteKind   TriggerRouteKind
	ID          string
	Name        string
	Description string
	Metadata    ThoughtRecipeMetadata
}

// ClarificationStepType identifies a clarification-specific thoughtrecipe step.
type ClarificationStepType string

const (
	ClarificationStepTypeClarify  ClarificationStepType = "intent_clarify"
	ClarificationStepTypeExtract  ClarificationStepType = "intent_extract"
	ClarificationStepTypeGround   ClarificationStepType = "intent_ground"
	ClarificationStepTypeProject  ClarificationStepType = "intent_project"
	ClarificationStepTypeRetrieve ClarificationStepType = "intent_retrieve"
	ClarificationStepTypeHandoff  ClarificationStepType = "intent_handoff"
)

// ClarificationStepConfig is the typed clarification contract carried in step config.
type ClarificationStepConfig struct {
	OutputSchemaID   string
	ValidationMode   intentcontext.ValidationMode
	RequiredFields   []string
	AllowedStatuses  []intentcontext.ClarificationStepStatus
	StateWriteKeys   []string
	ProjectionPolicy string
	RequeryOnSuccess bool
}

// ThoughtRecipeMetadata contains optional metadata about a thoughtrecipe.
type ThoughtRecipeMetadata struct {
	Name           string
	Version        string
	Author         string
	Tags           []string
	Families       []string
	Keywords       []string
	HandoffTargets []string
	Category       string
	CreatedAt      string
	UpdatedAt      string
}

// ThoughtRecipeStep represents a single step in the thoughtrecipe sequence (minimal DSL-native).
type ThoughtRecipeStep struct {
	ID           string
	Type         string
	CapabilityID string
	Description  string
	Prompt       string
	PromptID     string
	Mutation     string
	HITL         string
	Parent       ThoughtRecipeStepAgent
	Fallback     *ThoughtRecipeStepAgent
	Context      ThoughtRecipeStepContext
	Config       map[string]any
	Captures     map[string]string
	Bindings     map[string]string
	Dependencies []string

	// OnError defines error handling behavior for this step.
	OnError *StepErrorPolicy

	// Retry defines retry policy for this step.
	Retry *StepRetryPolicy
}

// ThoughtRecipeStepAgent describes the paradigm-specific agent invocation for a step.
type ThoughtRecipeStepAgent struct {
	Paradigm string
	Prompt   string
	Context  ThoughtRecipeStepContext
}

// ThoughtRecipeStepContext mirrors the spec's per-step context block.
type ThoughtRecipeStepContext struct {
	Stream  *ThoughtRecipeStreamSpec
	Ingest  *ThoughtRecipeIngestSpec
	Inherit []string
	Capture []string
}

// ThoughtRecipeStreamSpec configures a context stream request.
type ThoughtRecipeStreamSpec struct {
	QueryTemplate string
	MaxTokens     int
	Mode          string
}

// ThoughtRecipeIngestSpec configures ingestion for a thoughtrecipe or step.
type ThoughtRecipeIngestSpec struct {
	Mode          string
	IncludeGlobs  []string
	ExcludeGlobs  []string
	WorkspaceRoot string
}

// StepErrorPolicy defines error handling for a step.
type StepErrorPolicy struct {
	Action   string
	RetryMax int
	Fallback string
}

// StepRetryPolicy defines retry behavior for a step.
type StepRetryPolicy struct {
	MaxAttempts int
	Delay       string
	Backoff     string
	MaxDelay    string
}

// ParallelGroup defines a group of steps to execute in parallel.
type ParallelGroup struct {
	ID    string
	Steps []ThoughtRecipeStep
	Merge MergePolicy
}

// ConditionalGroup defines conditional execution logic.
type ConditionalGroup struct {
	ID        string
	Condition string
	If        []ThoughtRecipeStep
	Else      []ThoughtRecipeStep
}

// MergePolicy defines how to merge parallel step results.
type MergePolicy string

const (
	MergePolicyAll    MergePolicy = "all"    // All branches must succeed
	MergePolicyAny    MergePolicy = "any"    // At least one branch must succeed
	MergePolicyFirst  MergePolicy = "first"  // Use first successful result
	MergePolicyConcat MergePolicy = "concat" // Concatenate all results
)

// EffectiveName returns the best available human-readable thoughtrecipe name.
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

// Validate validates the ThoughtRecipe model.
func (r *ThoughtRecipe) Validate() error {
	if r == nil {
		return fmt.Errorf("thoughtrecipe is nil")
	}
	if kind := strings.TrimSpace(string(r.RouteKind)); kind != "" {
		switch TriggerRouteKind(kind) {
		case TriggerRouteKindCapability, TriggerRouteKindIntent:
		default:
			return fmt.Errorf("thoughtrecipe has unsupported route kind %q", kind)
		}
	}
	if strings.TrimSpace(r.EffectiveName()) == "" {
		return fmt.Errorf("thoughtrecipe missing required field: name")
	}
	return nil
}

// IsClarificationStepType reports whether stepType is one of the clarification-only types.
func IsClarificationStepType(stepType string) bool {
	switch strings.TrimSpace(stepType) {
	case string(ClarificationStepTypeClarify), string(ClarificationStepTypeExtract), string(ClarificationStepTypeGround), string(ClarificationStepTypeProject), string(ClarificationStepTypeRetrieve), string(ClarificationStepTypeHandoff):
		return true
	default:
		return false
	}
}

// DecodeClarificationStepConfig converts a raw step config into the typed clarification config.
func DecodeClarificationStepConfig(step ThoughtRecipeStep) (*ClarificationStepConfig, error) {
	if len(step.Config) == 0 {
		return nil, nil
	}
	cfg := &ClarificationStepConfig{}
	if value, ok := step.Config["output_schema_id"]; ok {
		cfg.OutputSchemaID = strings.TrimSpace(fmt.Sprint(value))
	}
	if value, ok := step.Config["validation_mode"]; ok {
		cfg.ValidationMode = intentcontext.ValidationMode(strings.TrimSpace(fmt.Sprint(value)))
	}
	if cfg.ValidationMode == "" {
		cfg.ValidationMode = intentcontext.ValidationModePartial
	}
	if value, ok := step.Config["required_fields"]; ok {
		cfg.RequiredFields = stringsFromAny(value)
	}
	if value, ok := step.Config["allowed_statuses"]; ok {
		for _, status := range stringsFromAny(value) {
			cfg.AllowedStatuses = append(cfg.AllowedStatuses, intentcontext.ClarificationStepStatus(strings.TrimSpace(status)))
		}
	}
	if value, ok := step.Config["state_write_keys"]; ok {
		cfg.StateWriteKeys = stringsFromAny(value)
	}
	if value, ok := step.Config["projection_policy"]; ok {
		cfg.ProjectionPolicy = strings.TrimSpace(fmt.Sprint(value))
	}
	if value, ok := step.Config["requery_on_success"]; ok {
		cfg.RequeryOnSuccess = truthyValue(value)
	}
	return cfg, validateClarificationStepConfigFields(step.Type, cfg)
}

func validateClarificationStepConfigFields(stepType string, cfg *ClarificationStepConfig) error {
	if cfg == nil {
		return nil
	}
	switch cfg.ValidationMode {
	case "", intentcontext.ValidationModeStrict, intentcontext.ValidationModePartial, intentcontext.ValidationModeRepair:
	default:
		return fmt.Errorf("invalid validation_mode: %s", cfg.ValidationMode)
	}
	if len(cfg.RequiredFields) == 0 {
		cfg.RequiredFields = nil
	}
	if len(cfg.StateWriteKeys) == 0 {
		cfg.StateWriteKeys = nil
	}
	switch ClarificationStepType(strings.TrimSpace(stepType)) {
	case ClarificationStepTypeClarify:
		// No schema required; the step emits the turn/question itself.
	case ClarificationStepTypeExtract, ClarificationStepTypeGround, ClarificationStepTypeProject, ClarificationStepTypeRetrieve:
		if strings.TrimSpace(cfg.OutputSchemaID) == "" {
			return fmt.Errorf("missing required field: config.output_schema_id")
		}
	case ClarificationStepTypeHandoff:
		// Handoff selects a downstream normal thoughtrecipe and does not require an output schema.
	}
	return nil
}

func truthyValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "no", "off", "nil", "null":
			return false
		default:
			return true
		}
	default:
		return strings.TrimSpace(fmt.Sprint(value)) != ""
	}
}

func validateStepParadigm(value string) error {
	switch strings.TrimSpace(value) {
	case "", "react", "planner", "htn", "reflection", "blackboard", "chainer", "pipeline", "rewoo", "goalcon", "euclo":
		return nil
	default:
		return fmt.Errorf("invalid paradigm value: %s", value)
	}
}

func cloneClarificationStepConfig(cfg *ClarificationStepConfig) *ClarificationStepConfig {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	if len(cfg.RequiredFields) > 0 {
		cp.RequiredFields = append([]string(nil), cfg.RequiredFields...)
	}
	if len(cfg.AllowedStatuses) > 0 {
		cp.AllowedStatuses = append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...)
	}
	if len(cfg.StateWriteKeys) > 0 {
		cp.StateWriteKeys = append([]string(nil), cfg.StateWriteKeys...)
	}
	return &cp
}
