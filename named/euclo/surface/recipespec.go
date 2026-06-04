package surface

import (
	"fmt"
	"strings"
)

// ThoughtRecipe defines the DSL-native in-memory thoughtrecipe model used by Euclo.
type ThoughtRecipe struct {
	RouteKind   TriggerRouteKind
	ID          string
	Name        string
	Description string
	Metadata    ThoughtRecipeMetadata
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

// ThoughtRecipeStep represents a single step in the thoughtrecipe sequence.
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
	OnError      *StepErrorPolicy
	Retry        *StepRetryPolicy
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
	MergePolicyAll    MergePolicy = "all"
	MergePolicyAny    MergePolicy = "any"
	MergePolicyFirst  MergePolicy = "first"
	MergePolicyConcat MergePolicy = "concat"
)

// TriggerRouteKind identifies the route contract declared by a trigger.
type TriggerRouteKind string

const (
	TriggerRouteKindUnknown    TriggerRouteKind = ""
	TriggerRouteKindCapability TriggerRouteKind = "capability"
	TriggerRouteKindIntent     TriggerRouteKind = "intent"
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
