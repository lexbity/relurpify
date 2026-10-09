package euclotypes

// FailureKind is the closed operational-failure taxonomy (Wave 2 §3.5). Every
// runtime step failure is classified into exactly one kind; the step's
// on_error policy is applied per kind, and the turn ends with a structured
// degraded result rather than a raw error or a silent continuation.
type FailureKind string

const (
	// FailureModelUnavailable: provider errors or timeouts. Default policy abort.
	FailureModelUnavailable FailureKind = "model_unavailable"
	// FailureModelInvalidOutput: unparseable or invalid planner/model output.
	// Default policy retry ×1, then abort.
	FailureModelInvalidOutput FailureKind = "model_invalid_output"
	// FailureCapabilityUnavailable: registry/policy denial or a missing handler.
	FailureCapabilityUnavailable FailureKind = "capability_unavailable"
	// FailureCapabilityDenied: governance deny (distinct from ask). No auto-retry.
	FailureCapabilityDenied FailureKind = "capability_denied"
	// FailureBudgetExhausted: token or iteration budget exhausted. Default abort.
	FailureBudgetExhausted FailureKind = "budget_exhausted"
	// FailureGroundingFailed: a step-barrier grounding failure (Wave 1 IF-1).
	// Classified via errors.Is(err, knowledge.ErrGroundingFailed) only.
	FailureGroundingFailed FailureKind = "grounding_failed"
	// FailureCancelled: context cancellation. Propagates; no frames after cancel.
	FailureCancelled FailureKind = "cancelled"
	// FailureUnknown: conservative fallback class. Default policy abort.
	FailureUnknown FailureKind = "unknown"
)

// Known reports whether the kind is one of the closed taxonomy values.
func (k FailureKind) Known() bool {
	switch k {
	case FailureModelUnavailable,
		FailureModelInvalidOutput,
		FailureCapabilityUnavailable,
		FailureCapabilityDenied,
		FailureBudgetExhausted,
		FailureGroundingFailed,
		FailureCancelled,
		FailureUnknown:
		return true
	default:
		return false
	}
}

// StepFailure is the typed, frame-visible record of one operational step
// failure. It is recorded on the envelope under euclo.step_failure and is
// carried in the step result metadata so recipe-level aggregation can build a
// structured RecipeOutcome.
type StepFailure struct {
	Kind    FailureKind `json:"kind"`
	Message string      `json:"message,omitempty"`
	// Cause is the originating error. It is not serialized; consumers use Kind
	// for decisions and Message for display.
	Cause error `json:"-"`
}
