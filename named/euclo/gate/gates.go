package gate

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// Phase identifies a named execution phase within an execution profile.
type Phase string

const (
	PhaseExplore    Phase = "explore"
	PhasePlan       Phase = "plan"
	PhaseEdit       Phase = "edit"
	PhaseVerify     Phase = "verify"
	PhaseReport     Phase = "report"
	PhaseReproduce  Phase = "reproduce"
	PhaseLocalize   Phase = "localize"
	PhasePatch      Phase = "patch"
	PhasePlanTests  Phase = "plan_tests"
	PhaseImplement  Phase = "implement"
	PhaseReview     Phase = "review"
	PhaseSummarize  Phase = "summarize"
	PhaseTrace      Phase = "trace"
	PhaseAnalyze    Phase = "analyze"
	PhaseStage      Phase = "stage"
)

// GateFailPolicy determines how a failed gate is handled.
type GateFailPolicy string

const (
	// GateFailBlock prevents advancement to the next phase.
	GateFailBlock GateFailPolicy = "block"
	// GateFailWarn allows advancement but records a warning.
	GateFailWarn GateFailPolicy = "warn"
	// GateFailSkip skips the target phase entirely.
	GateFailSkip GateFailPolicy = "skip"
)

// ValidatorCheck identifies the type of check an ArtifactValidator performs.
type ValidatorCheck string

const (
	// ValidatorCheckNotEmpty checks that the artifact payload is non-nil
	// and non-empty (for strings, maps, and slices).
	ValidatorCheckNotEmpty ValidatorCheck = "not_empty"
	// ValidatorCheckHasKey checks that a map payload contains a specific key.
	ValidatorCheckHasKey ValidatorCheck = "has_key"
	// ValidatorCheckMinCount checks that a slice payload has at least N items.
	ValidatorCheckMinCount ValidatorCheck = "min_count"
)

// ArtifactValidator defines a single validation rule applied to an artifact's
// payload. Field selects a top-level key within a map payload; if empty the
// entire payload is checked.
type ArtifactValidator struct {
	Kind  euclotypes.ArtifactKind
	Field string         // top-level key within map payload; "" means whole payload
	Check ValidatorCheck // not_empty / has_key / min_count
	Value any            // key name for has_key, int for min_count
}

// ArtifactGate defines the artifact requirements that must be met before
// a phase transition can proceed.
type ArtifactGate struct {
	RequiredKinds []euclotypes.ArtifactKind
	Validators    []ArtifactValidator
}

// IsEmpty returns true if the gate has no requirements.
func (g ArtifactGate) IsEmpty() bool {
	return len(g.RequiredKinds) == 0 && len(g.Validators) == 0
}

// PhaseGate defines evidence requirements for advancing from one phase
// to the next. ModeGates allows mode-specific overrides of the default gate.
type PhaseGate struct {
	From        Phase
	To          Phase
	ModeGates   map[string]ArtifactGate // mode_id → gate override
	DefaultGate ArtifactGate
	OnFail      GateFailPolicy
}

// GateEvaluation captures the result of evaluating a single phase gate.
type GateEvaluation struct {
	From             Phase
	To               Phase
	ModeID           string
	Passed           bool
	Missing          []euclotypes.ArtifactKind
	FailedValidators []string
	Policy           GateFailPolicy
}

// gateForMode returns the mode-specific gate if one exists, otherwise
// the default gate.
func (g PhaseGate) gateForMode(modeID string) ArtifactGate {
	if g.ModeGates != nil {
		if gate, ok := g.ModeGates[strings.TrimSpace(strings.ToLower(modeID))]; ok {
			return gate
		}
	}
	return g.DefaultGate
}

// EvaluateGate checks whether the artifact state satisfies a phase gate
// for the given mode.
func EvaluateGate(gate PhaseGate, modeID string, artifacts euclotypes.ArtifactState) GateEvaluation {
	eval := GateEvaluation{
		From:   gate.From,
		To:     gate.To,
		ModeID: modeID,
		Passed: true,
		Policy: gate.OnFail,
	}

	effective := gate.gateForMode(modeID)
	if effective.IsEmpty() {
		return eval
	}

	// Check required artifact kinds.
	for _, kind := range effective.RequiredKinds {
		if !artifacts.Has(kind) {
			eval.Missing = append(eval.Missing, kind)
			eval.Passed = false
		}
	}

	// Run validators.
	for _, v := range effective.Validators {
		if err := runValidator(v, artifacts); err != "" {
			eval.FailedValidators = append(eval.FailedValidators, err)
			eval.Passed = false
		}
	}

	return eval
}

// EvaluateGateSequence evaluates gates in order. It returns all evaluations
// and whether all gates passed (or were non-blocking).
func EvaluateGateSequence(gates []PhaseGate, modeID string, artifacts euclotypes.ArtifactState) ([]GateEvaluation, bool) {
	var evals []GateEvaluation
	allPassed := true
	for _, gate := range gates {
		eval := EvaluateGate(gate, modeID, artifacts)
		evals = append(evals, eval)
		if !eval.Passed {
			switch eval.Policy {
			case GateFailBlock:
				allPassed = false
				return evals, false
			case GateFailWarn:
				// continue but record
			case GateFailSkip:
				// continue
			default:
				allPassed = false
				return evals, false
			}
		}
	}
	return evals, allPassed
}

// runValidator evaluates a single validator against the artifact state.
// Returns an empty string on success, or a description of the failure.
func runValidator(v ArtifactValidator, artifacts euclotypes.ArtifactState) string {
	matching := artifacts.OfKind(v.Kind)
	if len(matching) == 0 {
		return fmt.Sprintf("%s: no artifacts of kind %s", v.Check, v.Kind)
	}

	// Apply check to the first matching artifact's payload.
	payload := matching[0].Payload
	if v.Field != "" {
		payload = extractField(payload, v.Field)
	}

	switch v.Check {
	case ValidatorCheckNotEmpty:
		if isEmpty(payload) {
			field := string(v.Kind)
			if v.Field != "" {
				field += "." + v.Field
			}
			return fmt.Sprintf("not_empty: %s is empty", field)
		}
	case ValidatorCheckHasKey:
		key, _ := v.Value.(string)
		if key == "" {
			return "has_key: no key specified"
		}
		m, ok := payload.(map[string]any)
		if !ok {
			return fmt.Sprintf("has_key: %s payload is not a map", v.Kind)
		}
		if _, exists := m[key]; !exists {
			return fmt.Sprintf("has_key: %s missing key %q", v.Kind, key)
		}
	case ValidatorCheckMinCount:
		minCount := toInt(v.Value)
		if minCount <= 0 {
			return "min_count: invalid count"
		}
		count := sliceLen(payload)
		if count < minCount {
			return fmt.Sprintf("min_count: %s has %d items, need %d", v.Kind, count, minCount)
		}
	default:
		return fmt.Sprintf("unknown check: %s", v.Check)
	}
	return ""
}

// extractField pulls a top-level key from a map payload. Returns nil if
// the payload is not a map or the key is absent.
func extractField(payload any, field string) any {
	m, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	return m[field]
}

// isEmpty checks whether a value is considered empty for gate purposes.
func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case []map[string]any:
		return len(typed) == 0
	case []euclotypes.Artifact:
		return len(typed) == 0
	}
	return false
}

// sliceLen returns the length of a slice-typed value.
func sliceLen(v any) int {
	switch typed := v.(type) {
	case []any:
		return len(typed)
	case []string:
		return len(typed)
	case []map[string]any:
		return len(typed)
	}
	return 0
}

// toInt converts a value to int, handling common numeric types.
func toInt(v any) int {
	switch typed := v.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return 0
}

// ============================================================================
// Phase Gate Definitions
// ============================================================================

// DefaultPhaseGates returns the default phase gate definitions for each
// execution profile. Gates define what artifact evidence must exist before
// advancing from one phase to the next. Mode-specific overrides allow
// stricter evidence requirements in modes like debug or tdd.
func DefaultPhaseGates() map[string][]PhaseGate {
	return map[string][]PhaseGate{
		"edit_verify_repair":        editVerifyRepairGates(),
		"reproduce_localize_patch":  reproduceLocalizePatchGates(),
		"test_driven_generation":    testDrivenGenerationGates(),
		"review_suggest_implement":  reviewSuggestImplementGates(),
		"plan_stage_execute":        planStageExecuteGates(),
		"trace_execute_analyze":     traceExecuteAnalyzeGates(),
	}
}

// edit_verify_repair: explore → plan → edit → verify
func editVerifyRepairGates() []PhaseGate {
	return []PhaseGate{
		{
			From: PhaseExplore,
			To:   PhasePlan,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			},
			ModeGates: map[string]ArtifactGate{
				"debug": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindExplore, Check: ValidatorCheckNotEmpty},
					},
				},
			},
			OnFail: GateFailBlock,
		},
		{
			From: PhasePlan,
			To:   PhaseEdit,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			},
			OnFail: GateFailBlock,
		},
		{
			From: PhaseEdit,
			To:   PhaseVerify,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
			},
			OnFail: GateFailBlock,
		},
	}
}

// reproduce_localize_patch: reproduce → localize → patch → verify
func reproduceLocalizePatchGates() []PhaseGate {
	return []PhaseGate{
		{
			From: PhaseReproduce,
			To:   PhaseLocalize,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			},
			ModeGates: map[string]ArtifactGate{
				"debug": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindExplore, Check: ValidatorCheckNotEmpty},
						{Kind: euclotypes.ArtifactKindExplore, Field: "status", Check: ValidatorCheckNotEmpty},
					},
				},
			},
			OnFail: GateFailBlock,
		},
		{
			From: PhaseLocalize,
			To:   PhasePatch,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
			},
			ModeGates: map[string]ArtifactGate{
				"debug": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindAnalyze, Check: ValidatorCheckNotEmpty},
					},
				},
			},
			OnFail: GateFailBlock,
		},
		{
			From: PhasePatch,
			To:   PhaseVerify,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
			},
			OnFail: GateFailBlock,
		},
	}
}

// test_driven_generation: plan_tests → implement → verify
func testDrivenGenerationGates() []PhaseGate {
	return []PhaseGate{
		{
			From: PhasePlanTests,
			To:   PhaseImplement,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			},
			ModeGates: map[string]ArtifactGate{
				"tdd": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindPlan, Check: ValidatorCheckNotEmpty},
						{Kind: euclotypes.ArtifactKindPlan, Field: "steps", Check: ValidatorCheckMinCount, Value: 1},
					},
				},
			},
			OnFail: GateFailBlock,
		},
		{
			From: PhaseImplement,
			To:   PhaseVerify,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
			},
			OnFail: GateFailBlock,
		},
	}
}

// review_suggest_implement: review → summarize
func reviewSuggestImplementGates() []PhaseGate {
	return []PhaseGate{
		{
			From: PhaseReview,
			To:   PhaseSummarize,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
			},
			ModeGates: map[string]ArtifactGate{
				"review": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindAnalyze, Check: ValidatorCheckNotEmpty},
					},
				},
			},
			OnFail: GateFailWarn,
		},
	}
}

// plan_stage_execute: plan → stage → summarize
func planStageExecuteGates() []PhaseGate {
	return []PhaseGate{
		{
			From: PhasePlan,
			To:   PhaseStage,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			},
			ModeGates: map[string]ArtifactGate{
				"planning": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindPlan, Check: ValidatorCheckNotEmpty},
					},
				},
			},
			OnFail: GateFailBlock,
		},
		{
			From: PhaseStage,
			To:   PhaseSummarize,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
			},
			OnFail: GateFailWarn,
		},
	}
}

// trace_execute_analyze: trace → analyze
func traceExecuteAnalyzeGates() []PhaseGate {
	return []PhaseGate{
		{
			From: PhaseTrace,
			To:   PhaseAnalyze,
			DefaultGate: ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			},
			ModeGates: map[string]ArtifactGate{
				"debug": {
					RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
					Validators: []ArtifactValidator{
						{Kind: euclotypes.ArtifactKindExplore, Check: ValidatorCheckNotEmpty},
					},
				},
			},
			OnFail: GateFailBlock,
		},
	}
}
