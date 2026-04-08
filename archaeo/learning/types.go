package learning

import "time"

type InteractionKind string

const (
	InteractionPatternProposal   InteractionKind = "pattern_proposal"
	InteractionAnchorProposal    InteractionKind = "anchor_proposal"
	InteractionTensionReview     InteractionKind = "tension_review"
	InteractionIntentRefinement  InteractionKind = "intent_refinement"
	InteractionKnowledgeProposal InteractionKind = "knowledge_proposal"
)

type SubjectType string

const (
	SubjectPattern     SubjectType = "pattern"
	SubjectAnchor      SubjectType = "anchor"
	SubjectTension     SubjectType = "tension"
	SubjectExploration SubjectType = "exploration"
)

type TimeoutBehavior string

const (
	TimeoutUseDefault TimeoutBehavior = "use_default"
	TimeoutDefer      TimeoutBehavior = "defer"
	TimeoutExpire     TimeoutBehavior = "expire"
)

type InteractionStatus string

const (
	StatusPending  InteractionStatus = "pending"
	StatusResolved InteractionStatus = "resolved"
	StatusExpired  InteractionStatus = "expired"
	StatusDeferred InteractionStatus = "deferred"
)

type ResolutionKind string

const (
	ResolutionConfirm ResolutionKind = "confirm"
	ResolutionReject  ResolutionKind = "reject"
	ResolutionRefine  ResolutionKind = "refine"
	ResolutionDefer   ResolutionKind = "defer"
)

type EvidenceRef struct {
	Kind     string         `json:"kind"`
	RefID    string         `json:"ref_id"`
	Title    string         `json:"title,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Choice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type CommentInput struct {
	IntentType  string `json:"intent_type"`
	AuthorKind  string `json:"author_kind"`
	Body        string `json:"body"`
	TrustClass  string `json:"trust_class,omitempty"`
	CorpusScope string `json:"corpus_scope,omitempty"`
}

type Resolution struct {
	Kind           ResolutionKind `json:"kind"`
	ChoiceID       string         `json:"choice_id,omitempty"`
	RefinedPayload map[string]any `json:"refined_payload,omitempty"`
	CommentRef     string         `json:"comment_ref,omitempty"`
	ResolvedBy     string         `json:"resolved_by,omitempty"`
	ResolvedAt     time.Time      `json:"resolved_at"`
}

type Interaction struct {
	ID              string            `json:"id"`
	WorkflowID      string            `json:"workflow_id"`
	ExplorationID   string            `json:"exploration_id"`
	SnapshotID      string            `json:"snapshot_id,omitempty"`
	Kind            InteractionKind   `json:"kind"`
	SubjectType     SubjectType       `json:"subject_type"`
	SubjectID       string            `json:"subject_id,omitempty"`
	Title           string            `json:"title"`
	Description     string            `json:"description,omitempty"`
	Evidence        []EvidenceRef     `json:"evidence,omitempty"`
	Choices         []Choice          `json:"choices,omitempty"`
	DefaultChoice   string            `json:"default_choice,omitempty"`
	TimeoutBehavior TimeoutBehavior   `json:"timeout_behavior,omitempty"`
	Blocking        bool              `json:"blocking,omitempty"`
	Status          InteractionStatus `json:"status"`
	Resolution      *Resolution       `json:"resolution,omitempty"`
	BasedOnRevision string            `json:"based_on_revision,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type CreateInput struct {
	WorkflowID      string
	ExplorationID   string
	SnapshotID      string
	Kind            InteractionKind
	SubjectType     SubjectType
	SubjectID       string
	Title           string
	Description     string
	Evidence        []EvidenceRef
	Choices         []Choice
	DefaultChoice   string
	TimeoutBehavior TimeoutBehavior
	Blocking        bool
	BasedOnRevision string
}

type ResolveInput struct {
	WorkflowID      string
	InteractionID   string
	ExpectedStatus  InteractionStatus
	Kind            ResolutionKind
	ChoiceID        string
	RefinedPayload  map[string]any
	Comment         *CommentInput
	ResolvedBy      string
	BasedOnRevision string
}
