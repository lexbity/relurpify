package planner

// resultfields.go is the single vocabulary for the planner's result-field
// contract and its backing envelope keys. Capture bindings read the result
// fields (the fields of the final execution.Result payload); the runner writes
// the envelope keys. Tests and conformance cases share these names so a renamed
// field breaks one place, not many.

// Result fields surfaced on the final planner step result, bindable by recipe
// capture.
const (
	ResultResults            = "results"
	ResultSkippedTools       = "skipped_tools"
	ResultSummary            = "summary"
	ResultPlanOrigin         = "plan_origin"
	ResultPlanObjective      = "plan_objective"
	ResultStepsCompleted     = "steps_completed"
	ResultVerification       = "verification"
	ResultVerificationIssues = "verification_issues"
	ResultResult             = "result"
	ResultRaw                = "result_raw"
)

// Envelope keys written and read by the planner phases.
const (
	EnvelopeKeyPlan               = "planner.plan"
	EnvelopeKeyPlanOrigin         = "planner.plan_origin"
	EnvelopeKeyPlanObjective      = "planner.plan_objective"
	EnvelopeKeyResults            = "planner.results"
	EnvelopeKeySkippedTools       = "planner.skipped_tools"
	EnvelopeKeySummary            = "planner.summary"
	EnvelopeKeyStepsCompleted     = "planner.steps_completed"
	EnvelopeKeyVerification       = "planner.verification"
	EnvelopeKeyVerificationIssues = "planner.verification_issues"
	EnvelopeKeyResult             = "planner.result"
	EnvelopeKeyResultRaw          = "planner.result_raw"
	// EnvelopeKeyCompletedSteps is the cross-paradigm completed-step resume
	// vocabulary (shared with the plan executor and HTN).
	EnvelopeKeyCompletedSteps = "plan.completed_steps"
)

// Plan provenance values recorded at phase 1 and surfaced as plan_origin.
const (
	planOriginAuthored  = "authored"
	planOriginGenerated = "generated"
)

// DefaultGeneratedPlanBound is the maximum number of steps a generated planner
// plan may contain (D3/NFR-2).
const DefaultGeneratedPlanBound = 12

// MaxPlanStepTextChars is the maximum authored or generated step text length.
const MaxPlanStepTextChars = 200

// maxVerifyResultChars bounds the per-step result text fed to the verify and
// summarize prompts so the prompt stays deterministic and bounded.
const maxVerifyResultChars = 2000
