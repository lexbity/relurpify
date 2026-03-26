package guidance

import "time"

type GuidanceKind string

const (
	GuidanceAmbiguity      GuidanceKind = "ambiguity"
	GuidanceConfidence     GuidanceKind = "confidence"
	GuidanceScopeExpansion GuidanceKind = "scope_expansion"
	GuidanceRecovery       GuidanceKind = "recovery"
	GuidanceContradiction  GuidanceKind = "contradiction"
	GuidanceApproach       GuidanceKind = "approach"
)

type GuidanceTimeoutBehavior string

const (
	GuidanceTimeoutUseDefault GuidanceTimeoutBehavior = "use_default"
	GuidanceTimeoutDefer      GuidanceTimeoutBehavior = "defer"
	GuidanceTimeoutFail       GuidanceTimeoutBehavior = "fail"
)

type GuidanceState string

const (
	GuidanceStatePending  GuidanceState = "pending"
	GuidanceStateResolved GuidanceState = "resolved"
	GuidanceStateDeferred GuidanceState = "deferred"
	GuidanceStateExpired  GuidanceState = "expired"
)

type GuidanceChoice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default,omitempty"`
}

type GuidanceRequest struct {
	ID              string                  `json:"id"`
	Kind            GuidanceKind            `json:"kind"`
	Title           string                  `json:"title"`
	Description     string                  `json:"description"`
	Choices         []GuidanceChoice        `json:"choices"`
	Context         map[string]any          `json:"context,omitempty"`
	Timeout         time.Duration           `json:"timeout,omitempty"`
	TimeoutBehavior GuidanceTimeoutBehavior `json:"timeout_behavior,omitempty"`
	RequestedAt     time.Time               `json:"requested_at"`
	State           GuidanceState           `json:"state"`
}

type GuidanceDecision struct {
	RequestID string    `json:"request_id"`
	ChoiceID  string    `json:"choice_id"`
	Freetext  string    `json:"freetext,omitempty"`
	DecidedBy string    `json:"decided_by"`
	DecidedAt time.Time `json:"decided_at"`
}

type GuidanceEventType string

const (
	GuidanceEventRequested GuidanceEventType = "requested"
	GuidanceEventResolved  GuidanceEventType = "resolved"
	GuidanceEventDeferred  GuidanceEventType = "deferred"
	GuidanceEventExpired   GuidanceEventType = "expired"
)

type GuidanceEvent struct {
	Type     GuidanceEventType
	Request  *GuidanceRequest
	Decision *GuidanceDecision
	Error    string
}
