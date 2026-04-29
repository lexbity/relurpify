package interaction

import "time"

// FrameType represents the type of interaction frame.
type FrameType string

const (
	FrameScopeConfirmation    FrameType = "scope_confirmation"
	FrameIntentClarification  FrameType = "intent_clarification"
	FrameCandidateSelection   FrameType = "candidate_selection"
	FrameRecipeSelection      FrameType = "recipe_selection"
	FrameCapabilitySelection  FrameType = "capability_selection"
	FrameHITLApproval         FrameType = "hitl_approval"
	FrameSessionResume        FrameType = "session_resume"
	FrameBackgroundJobStatus  FrameType = "background_job_status"
	FrameExecutionSummary     FrameType = "execution_summary"
	FrameVerificationEvidence FrameType = "verification_evidence"
	FrameOutcomeFeedback      FrameType = "outcome_feedback"
)

// ActionSlot represents an action the user can take on a frame.
type ActionSlot struct {
	ID      string // Slot identifier
	Label   string // Human-readable label
	Action  string // Action identifier
	Risk    string // "low" | "medium" | "high"
	Default bool   // Whether this is the default slot
}

// FrameResult represents the user's response to a frame.
type FrameResult struct {
	ChosenSlot  string            // The ID of the chosen slot
	ExtraData   map[string]any    // Additional data provided by the user
	RespondedBy string            // Identifier of who responded
	RespondedAt time.Time         // When the response was received
}

// InteractionFrame is a structured, durable interaction frame.
type InteractionFrame struct {
	ID        string       // UUID-based frame ID
	Type      FrameType    // Frame type
	TaskID    string       // Associated task ID
	SessionID string       // Associated session ID
	Seq       int          // Frame sequence number
	Slots     []ActionSlot // Available action slots
	DefaultSlot string     // ID of the default slot
	Payload   map[string]any // Frame-specific payload data
	CreatedAt time.Time    // When the frame was created
	RespondedAt *time.Time // When the frame was responded to (nil if pending)
	Response   *FrameResult // The user's response (nil if pending)
	Timeout   time.Duration // Maximum time to wait for response
}

// HITLDecision represents a human-in-the-loop decision.
type HITLDecision struct {
	Approved   bool      // Whether the action was approved
	Scope      string    // The scope of the approval
	RespondedAt time.Time // When the decision was made
	Reason     string    // Reason for the decision
}
