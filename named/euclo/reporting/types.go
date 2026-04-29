package reporting

// OutcomeCategory represents the category of task outcome.
type OutcomeCategory string

const (
	OutcomeCategorySuccess      OutcomeCategory = "success"
	OutcomeCategoryFailure      OutcomeCategory = "failure"
	OutcomeCategoryPartial      OutcomeCategory = "partial"
	OutcomeCategoryBlocked      OutcomeCategory = "blocked"
	OutcomeCategoryCancelled    OutcomeCategory = "cancelled"
)

// Outcome represents the final outcome of task execution.
type Outcome struct {
	Category    OutcomeCategory
	Reason      string
	Details     map[string]string
	Completed   bool
	ErrorCount  int
}

// TelemetryEvent represents a telemetry event.
type TelemetryEvent struct {
	Name      string
	Timestamp int64
	Data      map[string]interface{}
}

// TelemetryContext holds telemetry context for a task execution.
type TelemetryContext struct {
	TaskID      string
	SessionID   string
	Events      []TelemetryEvent
	Metrics     map[string]float64
}
