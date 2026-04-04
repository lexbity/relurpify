package pretask

import (
	"github.com/lexcodex/relurpify/framework/retrieval"
)

// AnchorSet holds deterministic signals extracted from the query.
// These are confirmed to exist in the index — no guessing.
type AnchorSet struct {
	// SymbolNames are CamelCase / snake_case identifiers extracted from the query
	// that were confirmed to exist in the AST index.
	SymbolNames []string

	// FilePaths are explicit file paths mentioned in the query or session (@-mentions).
	FilePaths []string

	// PackageRefs are Go package paths detected in the query (e.g. "framework/capability").
	PackageRefs []string

	// SessionPins are files the user has confirmed in prior turns this session.
	SessionPins []string

	// Raw is the original query text after structural extraction.
	Raw string
}

// CodeEvidenceItem is a single file-level result from index or vector retrieval.
type CodeEvidenceItem struct {
	Path       string
	Score      float64
	Source     EvidenceSource // "anchor" | "index" | "vector"
	Summary    string         // one-line description for the confirmation frame
	Citations  []retrieval.PackedCitation
}

// KnowledgeEvidenceItem is a single result from archaeo retrieval.
type KnowledgeEvidenceItem struct {
	RefID       string
	Kind        KnowledgeKind  // "pattern" | "tension" | "decision" | "interaction"
	Title       string
	Summary     string
	Score       float64
	Source      EvidenceSource // "archaeo_topic" | "archaeo_expanded"
	RelatedRefs []string
}

type EvidenceSource string

const (
	EvidenceSourceAnchor          EvidenceSource = "anchor"
	EvidenceSourceIndex           EvidenceSource = "index"
	EvidenceSourceVector          EvidenceSource = "vector"
	EvidenceSourceArchaeoTopic    EvidenceSource = "archaeo_topic"
	EvidenceSourceArchaeoExpanded EvidenceSource = "archaeo_expanded"
)

type KnowledgeKind string

const (
	KnowledgeKindPattern     KnowledgeKind = "pattern"
	KnowledgeKindTension     KnowledgeKind = "tension"
	KnowledgeKindDecision    KnowledgeKind = "decision"
	KnowledgeKindInteraction KnowledgeKind = "interaction"
)

// Stage1Result bundles the parallel Stage 1 retrieval outputs.
type Stage1Result struct {
	CodeEvidence      []CodeEvidenceItem
	KnowledgeEvidence []KnowledgeEvidenceItem
	AnchorsMissed     []string // anchors that were extracted but not found in index
}

// HypotheticalSketch is the grounded vocabulary output from Stage 2.
type HypotheticalSketch struct {
	Text       string // raw model output
	Embedding  []float32 // computed after generation
	Grounded   bool   // false if generation was skipped / fell back
	TokenCount int
}

// EnrichedContextBundle is the merged output of all pipeline stages,
// ready for the confirmation frame.
type EnrichedContextBundle struct {
	// AnchoredFiles are high-confidence, confirmed-present files.
	AnchoredFiles []CodeEvidenceItem

	// ExpandedFiles are retrieved via index/vector/hypothetical.
	ExpandedFiles []CodeEvidenceItem

	// KnowledgeTopic comes from Stage 1 archaeo (query-matched).
	KnowledgeTopic []KnowledgeEvidenceItem

	// KnowledgeExpanded comes from Stage 3 archaeo (hypothetical-matched).
	KnowledgeExpanded []KnowledgeEvidenceItem

	// TokenEstimate is the total estimated tokens for the full bundle.
	TokenEstimate int

	// PipelineTrace records what each stage did, for observability.
	PipelineTrace PipelineTrace
}

// ConfirmedContextBundle is what the user has validated via the confirmation frame.
type ConfirmedContextBundle struct {
	Files          []CodeEvidenceItem
	KnowledgeItems []KnowledgeEvidenceItem
	SessionPins    []string // files to persist as session anchors
	Skipped        bool     // user skipped confirmation
}

// PipelineTrace records per-stage diagnostics. Written to state for observability.
type PipelineTrace struct {
	AnchorsExtracted       int
	AnchorsConfirmed       int
	Stage1CodeResults      int
	Stage1ArchaeoResults   int
	HypotheticalGenerated  bool
	HypotheticalTokens     int
	Stage3ArchaeoResults   int
	FallbackUsed           bool
	FallbackReason         string
	TotalTokenEstimate     int
}

// PipelineConfig controls pipeline behaviour.
type PipelineConfig struct {
	// MaxCodeFiles is the maximum number of code files to surface (default 6).
	MaxCodeFiles int

	// MaxKnowledgeItems is the maximum number of archaeo items to surface (default 4).
	MaxKnowledgeItems int

	// TokenBudget is the total token budget for all retrieved content (default 2000).
	TokenBudget int

	// HypotheticalMaxTokens caps the vocabulary sketch generation (default 120).
	HypotheticalMaxTokens int

	// SkipHypotheticalIfAnchorsAbove skips Stage 2 if anchor retrieval already
	// returns this many high-confidence results (default 4). Avoids unnecessary
	// model calls when anchor coverage is sufficient.
	SkipHypotheticalIfAnchorsAbove int

	// WorkflowID scopes archaeo retrieval. Empty disables archaeo passes.
	WorkflowID string

	// SessionPins are files confirmed in prior turns, always included.
	SessionPins []string
}

func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		MaxCodeFiles:                   6,
		MaxKnowledgeItems:              4,
		TokenBudget:                    2000,
		HypotheticalMaxTokens:          120,
		SkipHypotheticalIfAnchorsAbove: 4,
	}
}
