package patterns

import "time"

type PatternKind string

const (
	PatternKindStructural PatternKind = "structural"
	PatternKindSemantic   PatternKind = "semantic"
	PatternKindBehavioral PatternKind = "behavioral"
	PatternKindBoundary   PatternKind = "boundary"
)

type PatternStatus string

const (
	PatternStatusProposed   PatternStatus = "proposed"
	PatternStatusConfirmed  PatternStatus = "confirmed"
	PatternStatusRejected   PatternStatus = "rejected"
	PatternStatusSuperseded PatternStatus = "superseded"
)

type GapSeverity string

const (
	GapSeverityMinor       GapSeverity = "minor"
	GapSeveritySignificant GapSeverity = "significant"
	GapSeverityCritical    GapSeverity = "critical"
)

type CommentIntentType string

const (
	CommentIntentional        CommentIntentType = "intentional"
	CommentDeferred           CommentIntentType = "deferred"
	CommentOpenQuestion       CommentIntentType = "open-question"
	CommentSuperseding        CommentIntentType = "superseding"
	CommentBoundaryConstraint CommentIntentType = "boundary-constraint"
)

type AuthorKind string

const (
	AuthorKindHuman AuthorKind = "human"
	AuthorKindAgent AuthorKind = "agent"
)

type TrustClass string

const (
	TrustClassWorkspaceTrusted TrustClass = "workspace_trusted"
	TrustClassBuiltinTrusted   TrustClass = "builtin_trusted"
)

type PatternInstance struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Excerpt   string `json:"excerpt,omitempty"`
	SymbolID  string `json:"symbol_id,omitempty"`
}

type PatternProposal struct {
	ID           string            `json:"id"`
	Kind         PatternKind       `json:"kind"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Instances    []PatternInstance `json:"instances,omitempty"`
	Confidence   float64           `json:"confidence"`
	CorpusScope  string            `json:"corpus_scope"`
	CorpusSource string            `json:"corpus_source"`
	CreatedAt    time.Time         `json:"created_at"`
}

type PatternRecord struct {
	ID           string            `json:"id"`
	Kind         PatternKind       `json:"kind"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Status       PatternStatus     `json:"status"`
	Instances    []PatternInstance `json:"instances,omitempty"`
	CommentIDs   []string          `json:"comment_ids,omitempty"`
	AnchorRefs   []string          `json:"anchor_refs,omitempty"`
	CorpusScope  string            `json:"corpus_scope"`
	CorpusSource string            `json:"corpus_source"`
	ConfirmedBy  string            `json:"confirmed_by,omitempty"`
	ConfirmedAt  *time.Time        `json:"confirmed_at,omitempty"`
	SupersededBy string            `json:"superseded_by,omitempty"`
	Confidence   float64           `json:"confidence"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type IntentGap struct {
	GapID          string      `json:"gap_id"`
	AnchorID       string      `json:"anchor_id,omitempty"`
	AnchorTerm     string      `json:"anchor_term"`
	FilePath       string      `json:"file_path"`
	SymbolID       string      `json:"symbol_id,omitempty"`
	Description    string      `json:"description"`
	Severity       GapSeverity `json:"severity"`
	EvidenceLines  []int       `json:"evidence_lines,omitempty"`
	CorpusScope    string      `json:"corpus_scope"`
	DetectionRunID string      `json:"detection_run_id,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
}

type CommentRecord struct {
	CommentID   string            `json:"comment_id"`
	PatternID   string            `json:"pattern_id,omitempty"`
	AnchorID    string            `json:"anchor_id,omitempty"`
	FilePath    string            `json:"file_path,omitempty"`
	SymbolID    string            `json:"symbol_id,omitempty"`
	IntentType  CommentIntentType `json:"intent_type"`
	Body        string            `json:"body"`
	AuthorKind  AuthorKind        `json:"author_kind"`
	TrustClass  TrustClass        `json:"trust_class"`
	AnchorRef   string            `json:"anchor_ref,omitempty"`
	CorpusScope string            `json:"corpus_scope,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
