package interaction

import "time"

// FrameType represents the type of interaction frame.
type FrameType string

const (
	FrameCandidates         FrameType = "candidates"
	FrameComparison         FrameType = "comparison"
	FrameDraft              FrameType = "draft"
	FrameResultType         FrameType = "result"
	FrameStatus             FrameType = "status"
	FrameSummary            FrameType = "summary"
	FrameTransition         FrameType = "transition"
	FrameSessionList        FrameType = "session_list"
	FrameSessionListEmpty   FrameType = "session_list_empty"
	FrameSessionResuming    FrameType = "session_resuming"
	FrameSessionResumeError FrameType = "session_resume_error"

	FrameScopeConfirmation      FrameType = "scope_confirmation"
	FrameIntentClarification    FrameType = "intent_clarification"
	FrameCandidateSelection     FrameType = "candidate_selection"
	FrameThoughtRecipeSelection FrameType = "thoughtrecipe_selection"
	FrameCapabilitySelection    FrameType = "capability_selection"
	FrameHITLApproval           FrameType = "hitl_approval"
	FrameSessionResume          FrameType = "session_resume"
	FrameBackgroundJobStatus    FrameType = "background_job_status"
	FrameExecutionSummary       FrameType = "execution_summary"
	FrameVerificationEvidence   FrameType = "verification_evidence"
	FrameOutcomeFeedback        FrameType = "outcome_feedback"
)

// ActionSlot represents an action the user can take on a frame.
type ActionSlot struct {
	ID       string // Slot identifier
	Label    string // Human-readable label
	Shortcut string // Legacy shortcut key
	Action   string // Action identifier
	Risk     string // "low" | "medium" | "high"
	Default  bool   // Whether this is the default slot
}

// SelectionOption is the typed option payload used by the selection-frame
// family.
type SelectionOption struct {
	ID       string
	Label    string
	Shortcut string
	Action   string
	Risk     string
	Default  bool
}

// SelectionFrame is the typed payload for selection-style interaction frames.
type SelectionFrame struct {
	Kind     FrameType
	Question string
	Options  []SelectionOption
	Default  string
	Resume   *ClarificationResumeMetadata
}

const (
	ActionConfirm  = "confirm"
	ActionFreetext = "freetext"
)

// ShouldResumeExecution reports whether a resolved frame should immediately
// re-enter the live Euclo task execution path.
func ShouldResumeExecution(frameType FrameType) bool {
	switch frameType {
	case FrameScopeConfirmation,
		FrameIntentClarification,
		FrameCandidateSelection,
		FrameThoughtRecipeSelection,
		FrameCapabilitySelection,
		FrameHITLApproval,
		FrameSessionResume:
		return true
	default:
		return false
	}
}

// FrameMetadata preserves the timestamp field used by the legacy renderers.
type FrameMetadata struct {
	Timestamp time.Time
}

// ClarificationResumeMetadata captures the resume context for a clarification frame.
type ClarificationResumeMetadata struct {
	ActiveThoughtRecipeID string
	ResumeNodeID          string
	RouteKind             string
	RouteID               string
	StateVersion          uint64
	Unresolved            bool
	MissingFields         []string
}

// FrameResult represents the user's response to a frame.
type FrameResult struct {
	ChosenSlot  string         // The ID of the chosen slot
	ExtraData   map[string]any // Additional data provided by the user
	RespondedBy string         // Identifier of who responded
	RespondedAt time.Time      // When the response was received
}

// Candidate is a legacy candidate entry.
type Candidate struct {
	ID         string
	Summary    string
	Properties map[string]string
}

// CandidatesContent is the legacy candidate-selection payload.
type CandidatesContent struct {
	Candidates    []Candidate
	RecommendedID string
}

// ComparisonContent is the legacy comparison payload.
type ComparisonContent struct {
	Dimensions []string
	Matrix     [][]string
}

// DraftItem is one draft line item.
type DraftItem struct {
	ID       string
	Editable bool
	Content  string
}

// DraftContent is the legacy draft payload.
type DraftContent struct {
	Kind  string
	Items []DraftItem
}

// ResultContent is the legacy result payload.
type ResultContent struct {
	Status    string
	Detail    string
	Message   string
	Artifacts []string
	Evidence  []EvidenceItem
}

// EvidenceItem is a legacy result evidence entry.
type EvidenceItem struct {
	Kind   string
	Detail string
}

// HITLResponse captures a human‑in‑the‑loop approval response.
// It mirrors the essential fields of FrameResult for HITL frames.
type HITLResponse struct {
	ChosenSlot  string         // ID of the slot selected by the user (e.g., "approve" or "reject")
	ExtraData   map[string]any // Optional additional data supplied with the response
	RespondedBy string         // Identifier of the responder (e.g., user ID)
	RespondedAt time.Time      // Timestamp when the response was recorded
}

// Finding is a legacy evidence/result finding.
type Finding struct {
	Location    string
	Severity    string
	Title       string
	Summary     string
	Description string
}

// FindingsContent is the legacy findings payload.
type FindingsContent struct {
	Critical []Finding
	Warning  []Finding
	Info     []Finding
}

// StatusContent is the legacy status payload.
type StatusContent struct {
	Message string
	Detail  string
}

// SummaryContent is the legacy summary payload.
type SummaryContent struct {
	Description string
	Artifacts   []string
	Changes     []string
}

// TransitionContent is the legacy mode transition payload.
type TransitionContent struct {
	FromMode string
	ToMode   string
	Reason   string
}

// SessionListItem is a single resumable session entry.
type SessionListItem struct {
	Index         int
	WorkflowID    string
	Instruction   string
	Mode          string
	HasBKCContext bool
	LastActiveAt  string
}

// SessionListContent is the legacy session-list payload.
type SessionListContent struct {
	Workspace string
	Sessions  []SessionListItem
}

// ContextFile is a legacy sidebar entry.
type ContextFile struct {
	Path            string
	Source          string
	Summary         string
	InsertionAction string
}

// KnowledgeItem is a legacy knowledge-summary item.
type KnowledgeItem struct {
	Kind    string
	Title   string
	Summary string
}

// PipelineTrace is a legacy trace summary.
type PipelineTrace struct {
	AnchorsExtracted      int
	AnchorsConfirmed      int
	Stage1CodeResults     int
	Stage1ArchaeoResults  int
	HypotheticalGenerated bool
	HypotheticalTokens    int
	Stage3ArchaeoResults  int
	FallbackUsed          bool
	FallbackReason        string
}

// ArchaeoFinding is a legacy explore entry.
type ArchaeoFinding struct {
	ID          string
	Kind        string
	Title       string
	Description string
	AnchorRefs  []string
	Severity    string
}

// ArchaeoFindingsContent is the legacy findings payload for the archaeo pane.
type ArchaeoFindingsContent struct {
	Blobs []ArchaeoFinding
}

// InteractionFrame is a structured, durable interaction frame.
type InteractionFrame struct {
	ID            string                       // UUID-based frame ID
	Type          FrameType                    // Frame type
	TaskID        string                       // Associated task ID
	SessionID     string                       // Associated session ID
	Seq           int                          // Frame sequence number
	Slots         []ActionSlot                 // Available action slots
	DefaultSlot   string                       // ID of the default slot
	Question      string                       // Clarification question text
	Choices       []string                     // Clarification choices
	DefaultChoice string                       // Clarification default choice
	Resume        *ClarificationResumeMetadata // Resume metadata for pending clarification
	Selection     *SelectionFrame              // Typed payload for selection-style frames
	Payload       map[string]any               // Frame-specific payload data
	Content       any                          // Legacy payload field used by older renderers
	Metadata      FrameMetadata                // Legacy metadata field used by older renderers
	CreatedAt     time.Time                    // When the frame was created
	RespondedAt   *time.Time                   // When the frame was responded to (nil if pending)
	Response      *FrameResult                 // The user's response (nil if pending)
	Timeout       time.Duration                // Maximum time to wait for response
}
