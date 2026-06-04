package surface

// StepStatus represents the execution status of a single recipe step.
type StepStatus string

const (
	StepPending  StepStatus = "pending"
	StepActive   StepStatus = "active"
	StepDone     StepStatus = "done"
	StepFailed   StepStatus = "failed"
	StepSkipped  StepStatus = "skipped"
)

// StepRuntime captures the live execution state of a single recipe step.
type StepRuntime struct {
	StepID     string     `json:"step_id"`
	Status     StepStatus `json:"status"`
	Index      int        `json:"index"`
	Total      int        `json:"total"`
	Paradigm   string     `json:"paradigm"`
	DurationMs int64      `json:"duration_ms,omitempty"`
	Err        string     `json:"error,omitempty"`
}
