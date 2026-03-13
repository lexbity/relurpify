package rewoo

// StepOnFailure defines how executor failures are handled.
type StepOnFailure string

const (
	// StepOnFailureSkip records the failure and continues.
	StepOnFailureSkip StepOnFailure = "skip"
	// StepOnFailureAbort aborts execution immediately.
	StepOnFailureAbort StepOnFailure = "abort"
	// StepOnFailureReplan aborts the current run and asks the agent to replan.
	StepOnFailureReplan StepOnFailure = "replan"
)

// RewooStep is a single tool-backed plan step.
type RewooStep struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Tool        string         `json:"tool"`
	Params      map[string]any `json:"params"`
	DependsOn   []string       `json:"depends_on"`
	OnFailure   StepOnFailure  `json:"on_failure"`
}

// RewooPlan is the planner output consumed by the executor.
type RewooPlan struct {
	Goal  string      `json:"goal"`
	Steps []RewooStep `json:"steps"`
}

// RewooStepResult stores the mechanical outcome of one tool step.
type RewooStepResult struct {
	StepID  string         `json:"step_id"`
	Tool    string         `json:"tool"`
	Success bool           `json:"success"`
	Output  map[string]any `json:"output,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// RewooOptions controls execution limits and fallback behavior.
type RewooOptions struct {
	MaxReplanAttempts int
	OnFailure         StepOnFailure
	MaxSteps          int
}
