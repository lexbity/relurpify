package knowledge

import "context"

// PatternInstance locates a pattern occurrence in a file.
type PatternInstance struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Excerpt   string `json:"excerpt,omitempty"`
	SymbolID  string `json:"symbol_id,omitempty"`
}

// PatternKind classifies pattern types.
type PatternKind string

const (
	PatternKindStructural PatternKind = "structural"
	PatternKindSemantic   PatternKind = "semantic"
	PatternKindBehavioral PatternKind = "behavioral"
	PatternKindBoundary   PatternKind = "boundary"
)

// PatternStatus tracks pattern lifecycle state.
type PatternStatus string

const (
	PatternStatusProposed   PatternStatus = "proposed"
	PatternStatusConfirmed  PatternStatus = "confirmed"
	PatternStatusRejected   PatternStatus = "rejected"
	PatternStatusSuperseded PatternStatus = "superseded"
)

// PatternRecord represents a discovered pattern with metadata.
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
	ConfirmedAt  *Timestamp        `json:"confirmed_at,omitempty"`
	SupersededBy string            `json:"superseded_by,omitempty"`
	Confidence   float64           `json:"confidence"`
	CreatedAt    Timestamp         `json:"created_at"`
	UpdatedAt    Timestamp         `json:"updated_at"`
}

// PatternStore persists pattern records.
type PatternStore interface {
	Save(ctx context.Context, record PatternRecord) error
	Load(ctx context.Context, id string) (*PatternRecord, error)
	ListByStatus(ctx context.Context, status PatternStatus, corpusScope string) ([]PatternRecord, error)
	ListByKind(ctx context.Context, kind PatternKind, corpusScope string) ([]PatternRecord, error)
	UpdateStatus(ctx context.Context, id string, status PatternStatus, confirmedBy string) error
	Supersede(ctx context.Context, oldID string, replacement PatternRecord) error
}

// CommentIntentType classifies comment purposes.
type CommentIntentType string

const (
	CommentIntentional        CommentIntentType = "intentional"
	CommentDeferred           CommentIntentType = "deferred"
	CommentOpenQuestion       CommentIntentType = "open-question"
	CommentSuperseding        CommentIntentType = "superseding"
	CommentBoundaryConstraint CommentIntentType = "boundary-constraint"
)

// AuthorKind identifies the comment author.
type AuthorKind string

const (
	AuthorKindHuman AuthorKind = "human"
	AuthorKindAgent AuthorKind = "agent"
)

// TrustClass classifies trust for a comment.
type TrustClass string

const (
	TrustClassWorkspace TrustClass = "workspace_trusted"
	TrustClassBuiltin   TrustClass = "builtin_trusted"
)

// CommentRecord represents a human or agent comment on code or patterns.
type CommentRecord struct {
	CommentID   string            `json:"comment_id"`
	PatternID   string            `json:"pattern_id,omitempty"`
	AnchorID    string            `json:"anchor_id,omitempty"`
	TensionID   string            `json:"tension_id,omitempty"`
	FilePath    string            `json:"file_path,omitempty"`
	SymbolID    string            `json:"symbol_id,omitempty"`
	IntentType  CommentIntentType `json:"intent_type"`
	Body        string            `json:"body"`
	AuthorKind  AuthorKind        `json:"author_kind"`
	TrustClass  TrustClass        `json:"trust_class"`
	AnchorRef   string            `json:"anchor_ref,omitempty"`
	CorpusScope string            `json:"corpus_scope,omitempty"`
	CreatedAt   Timestamp         `json:"created_at"`
	UpdatedAt   Timestamp         `json:"updated_at"`
}

// CommentStore persists code comments.
type CommentStore interface {
	Save(ctx context.Context, record CommentRecord) error
	Load(ctx context.Context, id string) (*CommentRecord, error)
	ListForPattern(ctx context.Context, patternID string) ([]CommentRecord, error)
	ListForAnchor(ctx context.Context, anchorID string) ([]CommentRecord, error)
	ListForTension(ctx context.Context, tensionID string) ([]CommentRecord, error)
	ListForSymbol(ctx context.Context, symbolID string) ([]CommentRecord, error)
}

// Timestamp is a Unix timestamp in seconds.
type Timestamp = int64
