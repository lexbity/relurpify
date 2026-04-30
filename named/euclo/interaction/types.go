package interaction

import "time"

// FrameType represents the type of interaction frame.
type FrameType string

const (
	FrameProposal             FrameType = "proposal"
	FrameQuestion             FrameType = "question"
	FrameCandidates           FrameType = "candidates"
	FrameComparison           FrameType = "comparison"
	FrameDraft                FrameType = "draft"
	FrameResultType           FrameType = "result"
	FrameStatus               FrameType = "status"
	FrameSummary              FrameType = "summary"
	FrameTransition           FrameType = "transition"
	FrameHelp                 FrameType = "help"
	FrameSessionList          FrameType = "session_list"
	FrameSessionListEmpty     FrameType = "session_list_empty"
	FrameSessionResuming      FrameType = "session_resuming"
	FrameSessionResumeError   FrameType = "session_resume_error"

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

	// Legacy frame kinds kept for the older relurpish TUI renderers.
	FrameArchaeoFindings FrameType = "archaeo_findings"
)

// ActionSlot represents an action the user can take on a frame.
type ActionSlot struct {
	ID       string // Slot identifier
	Label    string // Human-readable label
	Shortcut  string // Legacy shortcut key
	Action    string // Action identifier
	Kind      string // Legacy action kind
	Risk      string // "low" | "medium" | "high"
	Default   bool   // Whether this is the default slot
}

// ActionInfo is a legacy help renderer helper.
type ActionInfo struct {
	Phrase      string
	Description string
}

const (
	ActionConfirm  = "confirm"
	ActionFreetext = "freetext"
)

// FrameMetadata preserves the timestamp field used by the legacy renderers.
type FrameMetadata struct {
	Timestamp time.Time
}

// FrameResult represents the user's response to a frame.
type FrameResult struct {
	ChosenSlot  string         // The ID of the chosen slot
	ExtraData   map[string]any // Additional data provided by the user
	RespondedBy string         // Identifier of who responded
	RespondedAt time.Time      // When the response was received
}

// PhaseInfo is a legacy render helper used by the old euclotui renderer.
type PhaseInfo struct {
	ID      string
	Label string
	Current bool
}

// ProposalContent is the legacy proposal payload.
type ProposalContent struct {
	Interpretation string
	Scope          []string
	Approach       string
}

// QuestionOption is a legacy multiple-choice option.
type QuestionOption struct {
	ID          string
	Label       string
	Description string
}

// QuestionContent is the legacy clarification payload.
type QuestionContent struct {
	Question    string
	Description string
	Options     []QuestionOption
}

// Candidate is a legacy candidate entry.
type Candidate struct {
	ID         string
	Summary    string
	Properties  map[string]string
}

// CandidatesContent is the legacy candidate-selection payload.
type CandidatesContent struct {
	Candidates    []Candidate
	RecommendedID  string
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
	Status   string
	Detail   string
	Message  string
	Artifacts []string
	Evidence  []EvidenceItem
}

// EvidenceItem is a legacy result evidence entry.
type EvidenceItem struct {
	Kind   string
	Detail string
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

// HelpPhase describes a phase entry in help output.
type HelpPhase struct {
	ID      string
	Label   string
	Current bool
}

// HelpAction describes an available action in help output.
type HelpAction struct {
	Phrase      string
	Description string
}

// HelpTransition describes an available transition in help output.
type HelpTransition struct {
	Phrase     string
	TargetMode string
}

// HelpContent is the legacy help payload.
type HelpContent struct {
	Mode                 string
	CurrentPhase         string
	PhaseMap             []HelpPhase
	AvailableActions     []ActionInfo
	AvailableTransitions []TransitionInfo
}

// TransitionInfo is a legacy alias used by the old renderer tests.
type TransitionInfo = HelpTransition

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
	Path           string
	Source         string
	Summary        string
	InsertionAction string
}

// ContextProposalContent is the legacy context sidebar payload.
type ContextProposalContent struct {
	AnchoredFiles []ContextFile
	ExpandedFiles []ContextFile
	KnowledgeItems []KnowledgeItem
	PipelineTrace  PipelineTrace
}

// KnowledgeItem is a legacy knowledge-summary item.
type KnowledgeItem struct {
	Kind    string
	Title   string
	Summary string
}

// PipelineTrace is a legacy trace summary.
type PipelineTrace struct {
	AnchorsExtracted     int
	AnchorsConfirmed     int
	Stage1CodeResults    int
	Stage1ArchaeoResults int
	HypotheticalGenerated bool
	HypotheticalTokens   int
	Stage3ArchaeoResults int
	FallbackUsed         bool
	FallbackReason       string
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
	ID          string         // UUID-based frame ID
	Type        FrameType      // Frame type
	Kind        FrameType      // Legacy alias for Type
	Mode        string         // Legacy renderer field
	Phase       string         // Legacy renderer field
	TaskID      string         // Associated task ID
	SessionID   string         // Associated session ID
	Seq         int            // Frame sequence number
	Slots       []ActionSlot   // Available action slots
	Actions     []ActionSlot   // Legacy alias for Slots
	DefaultSlot string         // ID of the default slot
	Payload     map[string]any // Frame-specific payload data
	Content     any            // Legacy payload field used by older renderers
	Metadata    FrameMetadata  // Legacy metadata field used by older renderers
	CreatedAt   time.Time      // When the frame was created
	RespondedAt *time.Time     // When the frame was responded to (nil if pending)
	Response    *FrameResult   // The user's response (nil if pending)
	Timeout     time.Duration  // Maximum time to wait for response
}
