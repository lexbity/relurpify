package reporting

import (
	"encoding/json"
	"time"
)

// EventType defines the type of reporting event.
type EventType string

const (
	EventTypeTaskStarted       EventType = "task_started"
	EventTypeTaskCompleted     EventType = "task_completed"
	EventTypeTaskFailed        EventType = "task_failed"
	EventTypeStepStarted       EventType = "step_started"
	EventTypeStepCompleted     EventType = "step_completed"
	EventTypeStepFailed        EventType = "step_failed"
	EventTypeFrameEmitted      EventType = "frame_emitted"
	EventTypeFrameResolved     EventType = "frame_resolved"
	EventTypeCapabilityInvoked EventType = "capability_invoked"
)

const (
	EventTypeIntakeComplete       EventType = "euclo.intake.complete"
	EventTypeFamilySelected       EventType = "euclo.family.selected"
	EventTypeIngestionComplete    EventType = "euclo.ingestion.complete"
	EventTypeStreamRequested      EventType = "euclo.stream.requested"
	EventTypeCapabilityClassified EventType = "euclo.capability.classified"
	EventTypeRouteSelected        EventType = "euclo.route.selected"
	EventTypeRouteCompleted       EventType = "euclo.route.completed"
	EventTypeRouteUnavailable     EventType = "euclo.route.unavailable"
	EventTypeRouteDryRun          EventType = "euclo.route.dry_run"
	EventTypeRouteFallback        EventType = "euclo.route.fallback"
	EventTypeGateResult           EventType = "euclo.gate.result"
	EventTypeFrameEmittedEuclo    EventType = "euclo.frame.emitted"
	EventTypeFrameResolvedEuclo   EventType = "euclo.frame.resolved"
	EventTypeJobSubmitted         EventType = "euclo.job.submitted"
	EventTypeJobCompleted         EventType = "euclo.job.completed"
	EventTypeStepCompletedEuclo   EventType = "euclo.step.completed"
	EventTypeExecutionComplete    EventType = "euclo.execution.complete"
	EventTypeTaskStartedEuclo     EventType = "euclo.task.started"
	EventTypeTaskCompletedEuclo   EventType = "euclo.task.completed"
	EventTypeTaskFailedEuclo      EventType = "euclo.task.failed"
)

// Event represents a reporting event.
type Event struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	TaskID    string            `json:"task_id"`
	SessionID string            `json:"session_id"`
	Data      map[string]any    `json:"data"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// EventHeader contains the shared envelope for typed Euclo events.
type EventHeader struct {
	TaskID     string    `json:"task_id"`
	SessionID  string    `json:"session_id"`
	Seq        int       `json:"seq"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventIntakeComplete signals intake pipeline completion.
type EventIntakeComplete struct {
	EventHeader
	WinningFamily        string  `json:"winning_family"`
	Confidence           float64 `json:"confidence"`
	Ambiguous            bool    `json:"ambiguous"`
	CapabilityCount      int     `json:"capability_count"`
	HasStreamResult      bool    `json:"has_stream_result"`
	ClassificationSource string  `json:"classification_source"`
}

// EventFamilySelected signals family selection.
type EventFamilySelected struct {
	EventHeader
	FamilyID   string   `json:"family_id"`
	Confidence float64  `json:"confidence"`
	Keywords   []string `json:"keywords"`
}

// EventIngestionComplete signals file ingestion completion.
type EventIngestionComplete struct {
	EventHeader
	Mode       string `json:"mode"`
	FileCount  int    `json:"file_count"`
	ChunkCount int    `json:"chunk_count"`
	ErrorCount int    `json:"error_count"`
}

// EventStreamRequested signals a context stream request.
type EventStreamRequested struct {
	EventHeader
	Query     string `json:"query"`
	MaxTokens int    `json:"max_tokens"`
	Mode      string `json:"mode"`
}

// EventCapabilityClassified signals tier-2 classification.
type EventCapabilityClassified struct {
	EventHeader
	FamilyID     string   `json:"family_id"`
	Capabilities []string `json:"capabilities"`
	Operator     string   `json:"operator"`
	LLMCalls     int      `json:"llm_calls"`
}

// EventRouteSelected signals route selection.
type EventRouteSelected struct {
	EventHeader
	FamilyID       string `json:"family_id"`
	RouteKind      string `json:"route_kind"`
	RouteID        string `json:"route_id"`
	CandidateCount int    `json:"candidate_count"`
	FallbackTaken  bool   `json:"fallback_taken"`
}

// EventGateResult signals policy gate outcome.
type EventGateResult struct {
	EventHeader
	GateID   string `json:"gate_id"`
	Passed   bool   `json:"passed"`
	Decision string `json:"decision"`
}

// EventFrameEmitted signals interaction frame emission.
type EventFrameEmitted struct {
	EventHeader
	FrameID   string `json:"frame_id"`
	FrameType string `json:"frame_type"`
	SlotCount int    `json:"slot_count"`
}

// EventFrameResolved signals interaction frame resolution.
type EventFrameResolved struct {
	EventHeader
	FrameID     string `json:"frame_id"`
	ChosenSlot  string `json:"chosen_slot"`
	RespondedBy string `json:"responded_by"`
}

// EventJobSubmitted signals background job submission.
type EventJobSubmitted struct {
	EventHeader
	JobID         string `json:"job_id"`
	RouteID       string `json:"route_id"`
	ExecutionMode string `json:"execution_mode"`
}

// EventJobCompleted signals background job completion.
type EventJobCompleted struct {
	EventHeader
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// EventStepCompleted signals recipe step completion.
type EventStepCompleted struct {
	EventHeader
	StepID     string `json:"step_id"`
	RecipeID   string `json:"recipe_id"`
	Paradigm   string `json:"paradigm"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
}

// EventExecutionComplete signals overall execution completion.
type EventExecutionComplete struct {
	EventHeader
	Outcome     string `json:"outcome"`
	OutcomeKind string `json:"outcome_kind"`
	StepCount   int    `json:"step_count"`
	LLMCalls    int    `json:"llm_calls"`
	TokenUsage  int    `json:"token_usage"`
}

// EventTaskStarted signals task start.
type EventTaskStarted struct {
	EventHeader
	Instruction string `json:"instruction,omitempty"`
}

// EventTaskCompleted signals task completion.
type EventTaskCompleted struct {
	EventHeader
	Outcome string `json:"outcome,omitempty"`
}

// EventTaskFailed signals task failure.
type EventTaskFailed struct {
	EventHeader
	Error string `json:"error,omitempty"`
}

func mergeEventData(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func eventPayloadMap(payload any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]any{"payload_error": err.Error()}
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"payload_error": err.Error()}
	}
	return out
}

// EventEmitter defines the interface for emitting events.
type EventEmitter interface {
	Emit(event *Event) error
}

// InMemoryEventEmitter is a simple in-memory event emitter.
type InMemoryEventEmitter struct {
	events []*Event
}

// NewInMemoryEventEmitter creates a new in-memory event emitter.
func NewInMemoryEventEmitter() *InMemoryEventEmitter {
	return &InMemoryEventEmitter{
		events: make([]*Event, 0),
	}
}

// Emit stores an event in memory.
func (e *InMemoryEventEmitter) Emit(event *Event) error {
	e.events = append(e.events, event)
	return nil
}

// Events returns all stored events.
func (e *InMemoryEventEmitter) Events() []*Event {
	return e.events
}

// Clear clears all stored events.
func (e *InMemoryEventEmitter) Clear() {
	e.events = make([]*Event, 0)
}
