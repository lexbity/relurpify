package interaction

// ProposalContent is the typed payload for FrameProposal frames.
// The system proposes its interpretation of the user's instruction.
type ProposalContent struct {
	Interpretation string   `json:"interpretation"`
	Scope          []string `json:"scope"`
	Approach       string   `json:"approach,omitempty"`
	Constraints    []string `json:"constraints,omitempty"`
}

// QuestionContent is the typed payload for FrameQuestion frames.
type QuestionContent struct {
	Question      string           `json:"question"`
	Description   string           `json:"description,omitempty"`
	Options       []QuestionOption `json:"options,omitempty"`
	AllowFreetext bool             `json:"allow_freetext"`
}

// QuestionOption is a single selectable option in a question frame.
type QuestionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// CandidatesContent is the typed payload for FrameCandidates frames.
type CandidatesContent struct {
	Candidates    []Candidate `json:"candidates"`
	RecommendedID string      `json:"recommended_id,omitempty"`
}

// Candidate represents a single candidate in a selection frame.
type Candidate struct {
	ID         string            `json:"id"`
	Summary    string            `json:"summary"`
	Properties map[string]string `json:"properties,omitempty"`
}

// ComparisonContent is the typed payload for FrameComparison frames.
type ComparisonContent struct {
	Dimensions []string   `json:"dimensions"`
	Matrix     [][]string `json:"matrix"` // candidates × dimensions
}

// DraftContent is the typed payload for FrameDraft frames.
type DraftContent struct {
	Kind    string      `json:"kind"` // plan, test_list, edit_proposal, fix_proposal, finding_triage
	Items   []DraftItem `json:"items"`
	Addable bool        `json:"addable"`
}

// DraftItem is a single editable item in a draft frame.
type DraftItem struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Editable  bool   `json:"editable"`
	Removable bool   `json:"removable"`
}

// ResultContent is the typed payload for FrameResult frames.
type ResultContent struct {
	Status   string         `json:"status"` // passed, failed, partial, all_red
	Evidence []EvidenceItem `json:"evidence,omitempty"`
	Gaps     []string       `json:"gaps,omitempty"`
}

// EvidenceItem represents a single piece of evidence in a result frame.
type EvidenceItem struct {
	Kind       string  `json:"kind"` // stacktrace, code_read, git_blame, test_correlation, diff_analysis
	Detail     string  `json:"detail"`
	Location   string  `json:"location,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// FindingsContent is the typed payload for FrameResult frames that contain
// categorized findings (used by review mode triage).
type FindingsContent struct {
	Critical []Finding `json:"critical,omitempty"`
	Warning  []Finding `json:"warning,omitempty"`
	Info     []Finding `json:"info,omitempty"`
}

// Finding represents a single review finding.
type Finding struct {
	Location    string `json:"location"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// StatusContent is the typed payload for FrameStatus frames.
type StatusContent struct {
	Message  string  `json:"message"`
	Progress float64 `json:"progress,omitempty"` // 0.0-1.0, optional
	Phase    string  `json:"phase,omitempty"`
}

// SummaryContent is the typed payload for FrameSummary frames.
type SummaryContent struct {
	Description string   `json:"description"`
	Artifacts   []string `json:"artifacts,omitempty"`
	Changes     []string `json:"changes,omitempty"`
}

// TransitionContent is the typed payload for FrameTransition frames.
type TransitionContent struct {
	FromMode  string   `json:"from_mode"`
	ToMode    string   `json:"to_mode"`
	Reason    string   `json:"reason"`
	Artifacts []string `json:"artifacts,omitempty"` // what carries over
}

// HelpContent is the typed payload for FrameHelp frames.
type HelpContent struct {
	Mode                 string           `json:"mode"`
	CurrentPhase         string           `json:"current_phase"`
	PhaseMap             []PhaseInfo      `json:"phase_map"`
	AvailableActions     []ActionInfo     `json:"available_actions,omitempty"`
	AvailableTransitions []TransitionInfo `json:"available_transitions,omitempty"`
}

// PhaseInfo describes a phase in the mode's phase map.
type PhaseInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Current bool   `json:"current,omitempty"`
}

// ActionInfo describes an available user action in the help surface.
type ActionInfo struct {
	Phrase      string `json:"phrase"`
	Description string `json:"description"`
}

// TransitionInfo describes an available mode transition.
type TransitionInfo struct {
	Phrase     string `json:"phrase"`
	TargetMode string `json:"target_mode"`
}

// ActionKind constants
const (
	ActionKindPrimary   ActionKind = "primary"
	ActionKindSecondary ActionKind = "secondary"
)

// PipelineTrace records per-stage diagnostics. Written to state for observability.
type PipelineTrace struct {
	AnchorsExtracted      int    `json:"anchors_extracted"`
	AnchorsConfirmed      int    `json:"anchors_confirmed"`
	Stage1CodeResults     int    `json:"stage1_code_results"`
	Stage1ArchaeoResults  int    `json:"stage1_archaeo_results"`
	HypotheticalGenerated bool   `json:"hypothetical_generated"`
	HypotheticalTokens    int    `json:"hypothetical_tokens"`
	Stage3ArchaeoResults  int    `json:"stage3_archaeo_results"`
	FallbackUsed          bool   `json:"fallback_used"`
	FallbackReason        string `json:"fallback_reason,omitempty"`
	TotalTokenEstimate    int    `json:"total_token_estimate"`
}

// ContextProposalContent is the typed payload for context enrichment proposal frames.
// The host UI renders this however appropriate for its surface.
type ContextProposalContent struct {
	// AnchoredFiles are high-confidence files (user-selected or session pins).
	AnchoredFiles []ContextFileEntry `json:"anchored_files,omitempty"`

	// ExpandedFiles are structurally or semantically retrieved files.
	ExpandedFiles []ContextFileEntry `json:"expanded_files,omitempty"`

	// KnowledgeItems are archaeo-sourced patterns, tensions, and decisions.
	KnowledgeItems []ContextKnowledgeEntry `json:"knowledge_items,omitempty"`

	// PipelineTrace is the per-stage diagnostic summary.
	PipelineTrace PipelineTrace `json:"pipeline_trace"`
}

// ContextFileEntry is a single file entry in a context proposal.
type ContextFileEntry struct {
	Path            string  `json:"path"`
	Summary         string  `json:"summary,omitempty"` // one-line description
	Score           float64 `json:"score,omitempty"`
	Source          string  `json:"source"`                     // "anchor" | "index" | "vector"
	InsertionAction string  `json:"insertion_action,omitempty"` // "direct" | "summarized" | "metadata-only"
}

// ArchaeoFindingsContent is the typed payload for FrameArchaeoFindings frames.
// The archaeology relurpic capability emits this when it produces tension,
// pattern, or learning interaction proposals during an explore run.
type ArchaeoFindingsContent struct {
	// Blobs holds the proposed tensions, patterns, and learning interactions.
	Blobs []ArchaeoFindingBlob `json:"blobs"`
	// RunID identifies the explore run that produced these findings (optional).
	RunID string `json:"run_id,omitempty"`
}

// ArchaeoFindingBlob is a single proposed blob in an archaeo findings frame.
type ArchaeoFindingBlob struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"` // "tension" | "pattern" | "learning"
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	AnchorRefs  []string `json:"anchor_refs,omitempty"`
	Severity    string   `json:"severity,omitempty"` // for tensions: high/med/low
}

// ContextKnowledgeEntry is a single archaeo knowledge item in a context proposal.
type ContextKnowledgeEntry struct {
	RefID           string `json:"ref_id"`
	Kind            string `json:"kind"` // "pattern" | "tension" | "decision" | "interaction"
	Title           string `json:"title"`
	Summary         string `json:"summary,omitempty"`
	Source          string `json:"source"`                     // "archaeo_topic" | "archaeo_expanded"
	InsertionAction string `json:"insertion_action,omitempty"` // "direct" | "summarized" | "metadata-only"
}
