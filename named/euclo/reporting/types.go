package reporting

// OutcomeCategory represents the category of task outcome.
type OutcomeCategory string

const (
	OutcomeCategorySuccess   OutcomeCategory = "success"
	OutcomeCategoryFailure   OutcomeCategory = "failure"
	OutcomeCategoryPartial   OutcomeCategory = "partial"
	OutcomeCategoryBlocked   OutcomeCategory = "blocked"
	OutcomeCategoryCancelled OutcomeCategory = "cancelled"
	// OutcomeCategoryAwaitingInteraction: the execution graph finished but
	// the turn ended on a live interaction frame. Neither success nor
	// failure — the turn's work continues when the frame is answered.
	OutcomeCategoryAwaitingInteraction OutcomeCategory = "awaiting_interaction"
	// OutcomeCategoryUnverified reports a route that ran to completion
	// without failure while the work is not evidenced: an edit-shaped task
	// closed without an executed write or test observation, so the model's
	// assertion of success was the only support. Degraded, not trusted.
	OutcomeCategoryUnverified OutcomeCategory = "unverified"
)

// Outcome represents the final outcome of task execution.
type Outcome struct {
	Category   OutcomeCategory
	Reason     string
	Details    map[string]string
	Completed  bool
	ErrorCount int
}

// RouteOutcome describes the outcome state of a Euclo route execution.
type RouteOutcome string

const (
	RouteOutcomeSuccess         RouteOutcome = "success"
	RouteOutcomeFailure         RouteOutcome = "failure"
	RouteOutcomeDryRun          RouteOutcome = "dry_run"
	RouteOutcomeApprovalPending RouteOutcome = "approval_pending"
)

// TelemetryEvent represents a telemetry event.
type TelemetryEvent struct {
	Name      string
	Timestamp int64
	Data      map[string]any
}

// TelemetryContext holds telemetry context for a task execution.
type TelemetryContext struct {
	TaskID    string
	SessionID string
	Events    []TelemetryEvent
	Metrics   map[string]float64
}
