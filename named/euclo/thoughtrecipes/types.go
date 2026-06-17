package thoughtrecipe

import (
	"encoding/json"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// StepKind is a closed enum for ExecutionStep kind.
type StepKind uint8

const (
	StepKindInvalid      StepKind = iota // zero value is invalid
	StepKindRun                          // run agent
	StepKindDelegate                     // delegate to sub-thoughtrecipe
	StepKindAsk                          // ask user
	StepKindCapability                   // direct capability invocation
	StepKindPipelineStage                // pipeline structural step
)

func (k StepKind) String() string {
	switch k {
	case StepKindRun:
		return "run"
	case StepKindDelegate:
		return "delegate"
	case StepKindAsk:
		return "ask"
	case StepKindCapability:
		return "capability"
	case StepKindPipelineStage:
		return "pipeline"
	default:
		return "invalid"
	}
}

func (k StepKind) valid() bool {
	return k >= StepKindRun && k <= StepKindPipelineStage
}

func (k StepKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func (k *StepKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*k = stepKindFromString(s)
	return nil
}

func stepKindFromString(s string) StepKind {
	switch s {
	case "run":
		return StepKindRun
	case "delegate":
		return StepKindDelegate
	case "ask":
		return StepKindAsk
	case "capability":
		return StepKindCapability
	case "pipeline":
		return StepKindPipelineStage
	default:
		return StepKindInvalid
	}
}

// ExecutionPlan is the spec-shaped compilation result for a DSL thoughtrecipe.
type ExecutionPlan struct {
	ThoughtRecipe *surface.ThoughtRecipe
	Agents        map[string]AgentBinding
	ToolScopes    []ToolScopeFrame
	Steps         []ExecutionStep
	Routes        []CompiledRouteGroup
	Pipelines     []CompiledPipelineGroup
	Warnings      []SemanticWarning
	RouteKind     surface.TriggerRouteKind
}

// AgentBinding captures a thoughtrecipe-local agent declaration lowered to a runtime paradigm.
type AgentBinding struct {
	Name     string
	Paradigm string
	Span     SourceSpan
}

// ResolvedToolScope is the resolved tool scope for a step.
// The zero value denies every tool (fail-closed, A-6).
type ResolvedToolScope struct {
	allowed  []string
	resolved bool
	denyAll  bool
}

// DenyAllToolScope returns a ResolvedToolScope that denies every tool.
func DenyAllToolScope() ResolvedToolScope {
	return ResolvedToolScope{resolved: true, denyAll: true}
}

// AllowTools returns a ResolvedToolScope allowing the given tool names.
// A nil/empty slice means unrestricted (the runtime defaults to allow-all).
func AllowTools(names []string) ResolvedToolScope {
	return ResolvedToolScope{allowed: names, resolved: true}
}

// IsResolved reports whether the scope has been explicitly set.
// An unresolved scope (zero value) denies every tool (A-6).
func (s ResolvedToolScope) IsResolved() bool { return s.resolved }

// AllowedToolNames returns the list of allowed tool names.
// Nil means deny-all when IsDenyAll() is true, or unrestricted otherwise.
func (s ResolvedToolScope) AllowedToolNames() []string {
	if !s.resolved || s.denyAll {
		return nil
	}
	return append([]string(nil), s.allowed...)
}

// IsDenyAll reports whether this scope explicitly denies every tool.
func (s ResolvedToolScope) IsDenyAll() bool {
	return s.denyAll || !s.resolved
}

// Permits reports whether the given tool is allowed by this scope.
func (s ResolvedToolScope) Permits(toolName string) bool {
	if !s.resolved || s.denyAll {
		return false
	}
	if len(s.allowed) == 0 {
		return true // nil/empty = unrestricted
	}
	for _, a := range s.allowed {
		if a == toolName {
			return true
		}
	}
	return false
}

func (s ResolvedToolScope) MarshalJSON() ([]byte, error) {
	if s.denyAll {
		return json.Marshal([]string{"__deny_all__"})
	}
	if !s.resolved || len(s.allowed) == 0 {
		return json.Marshal(nil)
	}
	return json.Marshal(s.allowed)
}

func (s *ResolvedToolScope) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	if names == nil {
		s.resolved = true
		s.allowed = nil
		s.denyAll = false
	} else if len(names) == 1 && names[0] == "__deny_all__" {
		s.resolved = true
		s.allowed = nil
		s.denyAll = true
	} else {
		s.resolved = true
		s.allowed = names
		s.denyAll = false
	}
	return nil
}

// ExecutionStep carries the graph-time data for a single compiled thoughtrecipe step.
type ExecutionStep struct {
	ID       string
	Kind     StepKind
	Paradigm string
	Scope    ResolvedToolScope
	Question string
	Choices             []string
	ChoiceSource        string
	PipelineStages      []PipelineStageSpec
	Goal                string
	Sources             []string
	Directives          []string
	CaptureBindings     []CaptureBinding
	CapabilityID        string
	Prompt              string
	PromptID            string
	Mutation            string
	HITL                string
	Stream              *surface.ThoughtRecipeStreamSpec
	Fallback            *surface.ThoughtRecipeStepAgent
	Inherit             []string
	Capture             []string
	Dependencies        []string
	ClarificationConfig *ClarificationStepConfig
	OnError             *surface.StepErrorPolicy
	Config              map[string]any
}

// ToSurfaceStep projects the typed ExecutionStep back to the surface
// ThoughtRecipeStep shape consumed by recipe projections and telemetry.
func (s ExecutionStep) ToSurfaceStep() surface.ThoughtRecipeStep {
	return surface.ThoughtRecipeStep{
		ID:           s.ID,
		Type:         s.Kind.String(),
		CapabilityID: s.CapabilityID,
		Prompt:       s.Prompt,
		PromptID:     s.PromptID,
		Mutation:     s.Mutation,
		HITL:         s.HITL,
		Parent:       surface.ThoughtRecipeStepAgent{Paradigm: s.Paradigm},
		Dependencies: append([]string(nil), s.Dependencies...),
		Config:       s.Config,
		OnError:      s.OnError,
	}
}

// ToolScopeFrame captures one lexical tool allowlist contribution.
type ToolScopeFrame struct {
	ScopeKind string
	ToolNames []string
	Span      SourceSpan
}

// CompiledRouteGroup is a compiled constrained route block.
type CompiledRouteGroup struct {
	Group    *RouteGroup
	Branches []CompiledRouteBranch
}

// CompiledRouteBranch is a compiled route branch with normalized predicate and body.
type CompiledRouteBranch struct {
	Predicate *Predicate
	Steps     []ExecutionStep
	IsElse    bool
}

// RouteGroup identifies a lowered route block.
type RouteGroup struct {
	positioned
	ID string
}

// CompiledPipelineGroup is a compiled pipeline block with ordered stages.
type CompiledPipelineGroup struct {
	Group  *PipelineGroup
	Stages []CompiledPipelineStage
}

// CompiledPipelineStage is a compiled pipeline stage with lowered steps.
type CompiledPipelineStage struct {
	Stage *PipelineStage
	Steps []ExecutionStep
}

// PipelineGroup identifies a lowered pipeline block.
type PipelineGroup struct {
	positioned
	ID string
}

// PipelineStageSpec carries a runtime-friendly pipeline stage description.
type PipelineStageSpec struct {
	Name  string
	Span  SourceSpan
	Steps []ExecutionStep
}

// PredicateOp is a closed enum for predicate operators.
type PredicateOp uint8

const (
	PredOpInvalid     PredicateOp = iota
	PredOpIs                      // state.x is <value>
	PredOpContains                // state.x contains <value>
	PredOpMissing                 // missing state.x
	PredOpPresent                 // present state.x
	PredOpConfidenceLT            // state.x confidence below <percent>
)

func (o PredicateOp) String() string {
	switch o {
	case PredOpIs:
		return "is"
	case PredOpContains:
		return "contains"
	case PredOpMissing:
		return "missing"
	case PredOpPresent:
		return "present"
	case PredOpConfidenceLT:
		return "confidence_below"
	default:
		return "invalid"
	}
}

func (o PredicateOp) valid() bool {
	return o >= PredOpIs && o <= PredOpConfidenceLT
}

func (o PredicateOp) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

func (o *PredicateOp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*o = predicateOpFromString(s)
	return nil
}

func predicateOpFromString(s string) PredicateOp {
	switch s {
	case "is":
		return PredOpIs
	case "contains":
		return PredOpContains
	case "missing":
		return PredOpMissing
	case "present":
		return PredOpPresent
	case "confidence_below":
		return PredOpConfidenceLT
	default:
		return PredOpInvalid
	}
}

// PredicateValue carries the typed value for a predicate expression.
type PredicateValue struct {
	StringVal string `json:"string_val,omitempty"`
	Percent   int    `json:"percent,omitempty"`
}

// Predicate is a typed, compiled predicate fragment.
type Predicate struct {
	Subject string         // envelope lookup key, e.g. "state.intent"
	Op      PredicateOp    `json:"Op"`
	Value   PredicateValue `json:"Value"`
	Label   string         `json:"Label"` // diagnostics only — never the eval source
}


