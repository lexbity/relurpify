package thoughtrecipe

import (
	"encoding/json"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// StepKind is a closed enum for ExecutionStep kind.
type StepKind uint8

const (
	// StepKindInvalid is the zero value and marks an unset kind.
	StepKindInvalid StepKind = iota // zero value is invalid
	// StepKindRun runs an agent.
	StepKindRun // run agent
	// StepKindDelegate delegates to a sub-thoughtrecipe.
	StepKindDelegate // delegate to sub-thoughtrecipe
	// StepKindAsk asks the user.
	StepKindAsk // ask user
	// StepKindCapability is a direct capability invocation.
	StepKindCapability // direct capability invocation
	// StepKindPipelineStage is a pipeline structural step.
	StepKindPipelineStage // pipeline structural step
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
//
// Permits truth table (single source of truth — empty never means open):
//
//	resolved  denyAll  allowed   =>  Permits
//	false     *        *         =>  false
//	true      true     *         =>  false
//	true      false    empty     =>  false
//	true      false    [t]       =>  t ∈ allowed
type ResolvedToolScope struct {
	allowed  []string
	resolved bool
	denyAll  bool
	allowAll bool
}

// scopeSentinelDenyAll is the JSON wire sentinel for an explicit deny-all
// scope. Round-trip pinned by the tool-scope golden.
const scopeSentinelDenyAll = "__deny_all__"

// scopeSentinelAllowAll is the JSON wire sentinel for an explicit
// unrestricted scope. Only AllowAll() constructs that state.
const scopeSentinelAllowAll = "__allow_all__"

// DenyAllToolScope returns a ResolvedToolScope that denies every tool.
func DenyAllToolScope() ResolvedToolScope {
	return ResolvedToolScope{resolved: true, denyAll: true}
}

// AllowTools returns a ResolvedToolScope allowing exactly the given tool
// names. Empty never means unrestricted: a resolved-but-empty scope denies
// every tool. Programmatic construction that genuinely means unrestricted
// MUST use AllowAll.
func AllowTools(names []string) ResolvedToolScope {
	return ResolvedToolScope{allowed: names, resolved: true}
}

// AllowAll returns the explicit unrestricted scope — the only writer of that
// state.
func AllowAll() ResolvedToolScope {
	return ResolvedToolScope{resolved: true, allowAll: true}
}

// IsResolved reports whether the scope has been explicitly set.
// An unresolved scope (zero value) denies every tool (A-6).
func (s ResolvedToolScope) IsResolved() bool { return s.resolved }

// AllowedToolNames returns the allowed tool names. Nil means unrestricted
// only for the explicit AllowAll scope; deny-all and empty scopes return nil
// and permit nothing.
func (s ResolvedToolScope) AllowedToolNames() []string {
	if !s.resolved || s.denyAll || s.allowAll {
		return nil
	}
	return append([]string(nil), s.allowed...)
}

// IsDenyAll reports whether this scope explicitly denies every tool.
func (s ResolvedToolScope) IsDenyAll() bool {
	return s.denyAll || !s.resolved
}

// IsAllowAll reports whether this scope is the explicit unrestricted scope.
func (s ResolvedToolScope) IsAllowAll() bool {
	return s.allowAll
}

// Permits implements the truth table: !resolved ∨ denyAll ∨ empty-allowed
// all deny; only membership or explicit AllowAll permits.
func (s ResolvedToolScope) Permits(toolName string) bool {
	if !s.resolved || s.denyAll {
		return false
	}
	if s.allowAll {
		return true
	}
	if len(s.allowed) == 0 {
		return false
	}
	for _, a := range s.allowed {
		if a == toolName {
			return true
		}
	}
	return false
}

func (s ResolvedToolScope) MarshalJSON() ([]byte, error) {
	switch {
	case s.denyAll:
		return json.Marshal([]string{scopeSentinelDenyAll})
	case s.allowAll:
		return json.Marshal([]string{scopeSentinelAllowAll})
	case !s.resolved || len(s.allowed) == 0:
		return json.Marshal(nil)
	default:
		return json.Marshal(s.allowed)
	}
}

func (s *ResolvedToolScope) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	*s = ResolvedToolScope{resolved: true}
	if names == nil {
		return nil
	}
	if len(names) == 1 && names[0] == scopeSentinelDenyAll {
		s.denyAll = true
		return nil
	}
	if len(names) == 1 && names[0] == scopeSentinelAllowAll {
		s.allowAll = true
		return nil
	}
	s.allowed = names
	return nil
}

// TypedDirective is the lowered, shape-validated form of a grammar directive
// clause or block (D2). It replaces the stringly `[]string` payload: the
// directive name validated against the bound paradigm's contract, its text
// arguments, an optional `when` predicate, nested block directives, and the
// source span for diagnostics and telemetry.
type TypedDirective struct {
	Name      string
	TextArgs  []string         // raw text of the clause arguments, in source order
	Predicate *PredicateExpr   // for predicate-bearing block directives (`revise when …`)
	Body      []TypedDirective // nested block directives, validated for placement
	Span      SourceSpan
}

// ExecutionStep carries the graph-time data for a single compiled thoughtrecipe step.
type ExecutionStep struct {
	ID              string
	Kind            StepKind
	Paradigm        string
	Scope           ResolvedToolScope
	Question        string
	Choices         []string
	ChoiceSource    string
	PipelineStages  []PipelineStageSpec
	Goal            string
	Sources         []string
	Directives      []TypedDirective
	CaptureBindings []CaptureBinding
	CapabilityID    string
	Prompt          string
	PromptID        string
	Mutation        string
	HITL            string
	Stream          *surface.ThoughtRecipeStreamSpec
	Fallback        *surface.ThoughtRecipeStepAgent
	// FallbackFor is the ID of the primary step this step is the authored
	// fallback for. Empty for every ordinary step. It is structural metadata:
	// it is how the fallback node knows to record fallback_taken and emit
	// step.fallback_activated. It is excluded from JSON so the DSL goldens do
	// not change.
	FallbackFor         string `json:"-"`
	Inherit             []string
	Capture             []string
	Dependencies        []string
	ClarificationConfig *ClarificationStepConfig
	OnError             *surface.StepErrorPolicy
	Config              map[string]any
	// RevisePredicate is the compiled `revise when` predicate of a reflection
	// step (Wave 3 D5). It is runtime plumbing, excluded from JSON so the DSL
	// goldens do not change.
	RevisePredicate *Predicate `json:"-"`
	// ReviseBody is the lowered `revise` body of a reflection step: the nested
	// run/delegate items executed sequentially in-process when the predicate
	// holds. Runtime plumbing, excluded from JSON.
	ReviseBody []ExecutionStep `json:"-"`
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
	// PredOpInvalid is the zero value and marks an unset operator.
	PredOpInvalid PredicateOp = iota
	// PredOpIs matches when state.x is <value>.
	PredOpIs // state.x is <value>
	// PredOpContains matches when state.x contains <value>.
	PredOpContains // state.x contains <value>
	// PredOpMissing matches when state.x is missing.
	PredOpMissing // missing state.x
	// PredOpPresent matches when state.x is present.
	PredOpPresent // present state.x
	// PredOpConfidenceLT matches when state.x confidence is below <percent>.
	PredOpConfidenceLT // state.x confidence below <percent>
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
