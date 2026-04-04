# Chat Context Enrichment Pipeline

**Status**: Planned — pre-beta euclo requirement  
**Priority**: High — closes a fundamental gap in chat mode pre-task augmentation  
**Depends on**: IndexManager wiring confirmed correct (see `docs/issues/recurring-indexmanager-wiring-gap.md`); `ayenitd` composition root (phases 1–5 complete)  
**Target**: `named/euclo/` chat mode execution path  
**Authored**: 2026-04-03  
**Updated**: 2026-04-04 — ayenitd integration, file selection ownership, embedding strategy, all open questions resolved

---

## Motivation

Euclo chat mode currently gives the LLM the raw user query plus whatever is already in context. No retrieval fires for `chat` mode. No structural enrichment happens before the first LLM call. For a 14B model, this means the model starts from near-zero codebase knowledge for every new question — and hallucinates to fill the gap.

The correct solution is a pre-task context enrichment pipeline that:
1. Deterministically extracts structural anchors from the query (no model required)
2. Retrieves grounded context from both the AST index and the archaeo knowledge layer
3. Uses *that grounded context* to generate a vocabulary-rich hypothetical for expanded retrieval
4. Returns the merged result to the user for confirmation before the LLM sees the task

This pipeline treats archaeo not just as a workflow execution paradigm but as a **knowledge layer** available to all modes.

---

## Design Principles

1. **User-selected files are the highest-confidence anchors.** Files the user explicitly selects in the current interaction (via @mention or file picker) — whether previously loaded or not — enter the anchor set unconditionally. No index confirmation required. The user's intent overrides everything.
2. **Anchors are deterministic and always run first.** No model guessing before we have confirmed evidence.
3. **HyDE is always grounded.** The hypothetical is generated *after* anchor retrieval, never cold. A small model reasoning from real code is reliable. A small model guessing from nothing is not.
4. **Two archaeo passes, different purposes.** Stage 1: topic-matching against the raw query. Stage 3: structural + historical matching against the grounded hypothetical. They surface different knowledge.
5. **User confirmation is part of the answer.** The confirmation frame is not friction — it is a first-class part of the chat interaction. The user's corrections become session anchors.
6. **Silent degradation must not occur.** Each stage has an explicit fallback. If archaeo is unavailable, the pipeline continues. If hypothetical generation fails, the pipeline continues with anchor results only.
7. **Euclo is UI-agnostic.** The pipeline and interaction phase emit structured frames and await responses. Rendering, timing, and layout are the host UI's (relurpish's) responsibility. Euclo must not encode presentation logic.

---

## Pipeline Architecture

```
  User query + current-turn file selections (@mentions, file picker)
         │
         │  PipelineInput{Query, CurrentTurnFiles, SessionPins, WorkflowID}
         ▼
┌─────────────────────────────────────────────────────┐
│ Stage 0 — AnchorExtractor                           │
│                                                     │
│  Priority 1: CurrentTurnFiles (user-selected,       │
│              not yet loaded) — trusted unconditionally│
│  Priority 2: SessionPins (confirmed prior turns)    │
│  Priority 3: CamelCase/path symbols in query        │
│              confirmed against AST index            │
└─────────────────────────────────────────────────────┘
         │  AnchorSet
         ▼
┌─────────────────────────────────────────────────────┐
│ Stage 1 — parallel                                  │
│                                                     │
│  IndexRetriever          ArchaeoRetriever           │
│  .Retrieve(anchors)      .RetrieveTopic(query,wfID) │
│                                                     │
│  structural expansion    topic-matched knowledge    │
│  from anchor symbols:    patterns, tensions,        │
│  deps, call graph,       decisions matching the     │
│  related files           raw query                  │
│                                                     │
│  → CodeEvidence          → KnowledgeEvidence        │
└─────────────────────────────────────────────────────┘
         │  Stage1Result{CodeEvidence, KnowledgeEvidence}
         ▼
┌─────────────────────┐
│ Stage 2             │
│ HypotheticalGen     │  model call — grounded in Stage 1
│ .Generate(...)      │  vocabulary sketch ~120 tokens
│                     │  skipped if anchor coverage high
└─────────────────────┘
         │  HypotheticalSketch (Embedding + Text)
         ▼
┌─────────────────────┐
│ Stage 3             │
│ ArchaeoRetriever    │  second archaeo pass
│ .RetrieveExpanded() │  query = hypothetical embedding
│                     │  structurally-matched knowledge
│                     │  surfaces different items than Stage 1
└─────────────────────┘
         │  ExpandedKnowledgeEvidence
         ▼
┌─────────────────────┐
│ Merge + Rank        │
│ ResultMerger.Merge()│  dedupe, score, budget-enforce
└─────────────────────┘
         │  EnrichedContextBundle
         ▼
┌─────────────────────┐
│ ContextProposalPhase│  emits ContextProposalFrame
│ .Execute(...)       │  awaits UserResponse
│                     │  UI-agnostic — host renders
└─────────────────────┘
         │  ConfirmedContextBundle
         ▼
┌─────────────────────┐
│ ProgressiveLoader   │  loads confirmed files at
│ .Load(confirmed)    │  appropriate detail level
└─────────────────────┘
```

---

## Component Specifications

### New Package: `named/euclo/runtime/pretask/`

All new pipeline code lives here. Keeps the orchestration isolated from the existing runtime types.

---

### `pretask/types.go`

```go
package pretask

import (
    "github.com/lexcodex/relurpify/framework/retrieval"
    eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
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
    Files             []CodeEvidenceItem
    KnowledgeItems    []KnowledgeEvidenceItem
    SessionPins       []string // files to persist as session anchors
    Skipped           bool     // user skipped confirmation
}

// PipelineTrace records per-stage diagnostics. Written to state for observability.
type PipelineTrace struct {
    AnchorsExtracted    int
    AnchorsConfirmed    int
    Stage1CodeResults   int
    Stage1ArchaeoResults int
    HypotheticalGenerated bool
    HypotheticalTokens  int
    Stage3ArchaeoResults int
    FallbackUsed        bool
    FallbackReason      string
    TotalTokenEstimate  int
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
```

---

### `pretask/anchor.go`

```go
package pretask

// AnchorExtractor extracts deterministic structural signals from a query.
// It does not call the LLM. All results are confirmed against the AST index.
type AnchorExtractor struct {
    index  IndexQuerier  // interface over ast.IndexManager
    config AnchorConfig
}

type AnchorConfig struct {
    // MinSymbolLength filters out very short tokens (default 3).
    MinSymbolLength int
    // MaxSymbols caps how many symbols to confirm against the index (default 12).
    MaxSymbols int
}

// IndexQuerier is the narrow interface the extractor needs from ast.IndexManager.
// Using an interface makes this unit-testable without a real index.
type IndexQuerier interface {
    QuerySymbol(pattern string) ([]*ast.Node, error)
    SearchNodes(query ast.NodeQuery) ([]*ast.Node, error)
}

// Extract builds an AnchorSet from the full pipeline input.
//
// Extraction order (priority):
//   1. input.CurrentTurnFiles — user-selected this turn, not yet loaded.
//      Added unconditionally. No index confirmation. These are the user's
//      explicit intent and take precedence over all other signals.
//   2. input.SessionPins — confirmed in prior turns. Also unconditional.
//   3. @-mentioned file paths parsed from input.Query (trust user-provided paths).
//   4. CamelCase identifiers extracted from input.Query, confirmed against index.
//   5. Package-path-style tokens (e.g. "framework/capability"), confirmed against index.
//
// Files that are already loaded (present in ProgressiveLoader.loadedFiles) are
// still included in the AnchorSet — they were confirmed previously and drive
// retrieval expansion. The loader skips re-reading them (cache hit).
func (e *AnchorExtractor) Extract(input PipelineInput) AnchorSet
```

**Algorithm — symbol extraction:**
1. Add all `input.CurrentTurnFiles` directly to `FilePaths` (highest trust, no confirmation)
2. Add all `input.SessionPins` directly to `SessionPins` (already confirmed, no re-check)
3. Parse `input.Query` for `@`-prefixed paths → add to `FilePaths`
4. Split query on word boundaries; collect `[A-Z][a-zA-Z0-9]{2,}` (CamelCase) tokens
5. Collect `[a-z]+/[a-z][a-z0-9/]+` (package-path style) tokens
6. For each candidate symbol: call `index.QuerySymbol(candidate)` — keep only confirmed hits
7. Cap confirmed symbols at `config.MaxSymbols`

---

### `pretask/retrieval.go`

#### Index retrieval (Stage 1a)

```go
// IndexRetriever performs structural retrieval from the AST index.
type IndexRetriever struct {
    index    IndexQuerier
    deps     DependencyQuerier
    loader   FileLoader
    config   IndexRetrieverConfig
}

type DependencyQuerier interface {
    GetDependencyGraph(symbol string) (*ast.DependencyGraph, error)
    GetCallGraph(symbol string) (*ast.CallGraph, error)
}

type IndexRetrieverConfig struct {
    // DependencyHops is how many hops to expand from each anchor symbol (default 1).
    DependencyHops int
    // MaxFilesPerSymbol caps expansion per anchor (default 3).
    MaxFilesPerSymbol int
}

// Retrieve returns code evidence for the given anchor set.
// For each confirmed symbol in anchors.SymbolNames:
//   - Loads the file containing the symbol (DetailSignatureOnly)
//   - Expands DependencyHops hops via GetDependencyGraph
//   - Caps at MaxFilesPerSymbol additional files per symbol
// For each path in anchors.FilePaths: loads at DetailSignatureOnly.
// Deduplicates across symbols.
func (r *IndexRetriever) Retrieve(ctx context.Context, anchors AnchorSet) ([]CodeEvidenceItem, error)
```

#### Archaeo retrieval (Stage 1b and Stage 3)

```go
// ArchaeoRetriever queries the archaeo knowledge layer.
// It is used twice: once with the raw query (Stage 1b), once with the
// hypothetical sketch embedding (Stage 3).
type ArchaeoRetriever struct {
    tensionSvc   TensionQuerier
    patternSvc   PatternQuerier
    retriever    retrieval.RetrieverService // for semantic search over archaeo corpus
    config       ArchaeoRetrieverConfig
}

// TensionQuerier is the narrow interface needed from archaeo/tensions.
type TensionQuerier interface {
    ActiveByWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error)
    SummaryByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.TensionSummary, error)
}

// PatternQuerier is the narrow interface needed from archaeo patterns.
type PatternQuerier interface {
    ListByWorkflow(ctx context.Context, workflowID string) ([]patterns.PatternRecord, error)
}

type ArchaeoRetrieverConfig struct {
    WorkflowID        string
    MaxItems          int
    MaxTokens         int
}

// RetrieveTopic performs Stage 1b: query-driven archaeo retrieval.
// Uses the raw query text as the retrieval query against the archaeo corpus.
// Also lists active tensions and patterns for the workflow and scores them
// against the query via keyword overlap.
func (r *ArchaeoRetriever) RetrieveTopic(ctx context.Context, query string) ([]KnowledgeEvidenceItem, error)

// RetrieveExpanded performs Stage 3: hypothetical-driven archaeo retrieval.
// Uses the hypothetical sketch embedding (if available) or its text as the
// retrieval query. Returns different results than RetrieveTopic because the
// query is now code-vocabulary-rich rather than natural-language-rich.
func (r *ArchaeoRetriever) RetrieveExpanded(ctx context.Context, sketch HypotheticalSketch) ([]KnowledgeEvidenceItem, error)
```

---

### `pretask/hypothetical.go`

```go
// HypotheticalGenerator generates a grounded vocabulary sketch.
// It requires Stage 1 results to be populated — never called cold.
type HypotheticalGenerator struct {
    model    core.LanguageModel
    embedder retrieval.Embedder
    config   HypotheticalConfig
}

type HypotheticalConfig struct {
    MaxTokens   int     // cap on generated sketch (default 120)
    Temperature float64 // low temperature for consistency (default 0.1)
}

// Generate produces a vocabulary sketch grounded in Stage 1 evidence.
//
// Prompt strategy:
//   "Given this question and the following code signatures/knowledge from
//    this codebase, list the additional function names, types, and packages
//    that are likely relevant. Be terse. Use names that exist in this codebase."
//
// The prompt includes:
//   - The original query
//   - Signatures from Stage 1 CodeEvidence (DetailSignatureOnly — cheap tokens)
//   - Titles/summaries from Stage 1 KnowledgeEvidence
//
// The output is embedded immediately. Both Text and Embedding are returned.
//
// If model is nil, stage1 is empty, or generation fails, returns a
// HypotheticalSketch with Grounded=false. The pipeline continues gracefully.
func (g *HypotheticalGenerator) Generate(
    ctx context.Context,
    query string,
    stage1 Stage1Result,
) (HypotheticalSketch, error)
```

**Why this works for small models:**
The model is not generating a hypothetical answer to the question. It is generating a *vocabulary expansion* from code it just read. Pattern recognition from evidence. This task is well within 14B model capability because it does not require broad parametric knowledge — it requires reading the signatures in context and naming related things.

**Token cost estimate:**
- Input: ~400 tokens (query + top 5 signatures + top 3 knowledge titles)
- Output: ~120 tokens
- Total: ~520 tokens per enrichment call
- At Ollama speeds for qwen2.5-coder:14b: ~2-4 seconds generation + ~0.3-0.5s embedding

**Embedding is synchronous and sequential.** With a fixed Ollama instance pool (typically 1 locally), embedding and generation compete for the same endpoint and cannot be meaningfully parallelized. The embedding call is a single forward pass (no token generation) and adds minimal latency relative to the generation step. Async embedding with a text-based fallback was considered and rejected: it adds implementation complexity without latency benefit under the single-instance constraint, and text-based Stage 3 retrieval produces qualitatively different (worse) results than vector retrieval — making the fallback path a quality regression rather than a graceful degradation.

**On embedding failure:** `HypotheticalSketch.Grounded = false`, Stage 3 skipped. This is true graceful degradation (no retrieval attempt) rather than a silent quality downgrade.

---

### `pretask/merger.go`

```go
// ResultMerger merges pipeline stage outputs into a single ranked bundle.
type ResultMerger struct {
    config MergerConfig
}

type MergerConfig struct {
    TokenBudget       int
    MaxCodeFiles      int
    MaxKnowledgeItems int
}

// Merge produces an EnrichedContextBundle from pipeline stage outputs.
//
// Merge strategy:
//   1. Anchored files (source="anchor" or "session_pin") always included
//   2. Remaining code slots filled by score, descending, deduped by path
//   3. KnowledgeTopic items scored by keyword overlap with query
//   4. KnowledgeExpanded items scored by retrieval score
//   5. Deduplication across topic/expanded by RefID
//   6. Token budget enforced: demote to DetailMinimal before dropping
//   7. PipelineTrace populated from all stage inputs
func (m *ResultMerger) Merge(
    query string,
    anchors AnchorSet,
    stage1 Stage1Result,
    sketch HypotheticalSketch,
    expanded []KnowledgeEvidenceItem,
) EnrichedContextBundle
```

---

### `pretask/pipeline.go`

```go
package pretask

// Pipeline orchestrates the full pre-task context enrichment flow.
type Pipeline struct {
    anchorExtractor  *AnchorExtractor
    indexRetriever   *IndexRetriever
    archaeoRetriever *ArchaeoRetriever
    hypotheticalGen  *HypotheticalGenerator
    merger           *ResultMerger
    config           PipelineConfig
}

// PipelineInput is everything the pipeline needs from the call site.
type PipelineInput struct {
    Query string

    // CurrentTurnFiles are files the user explicitly selected in this interaction
    // turn via @mention or file picker. These become highest-priority anchors
    // unconditionally — no index confirmation required.
    // Includes files not yet loaded into context.
    CurrentTurnFiles []string

    // SessionPins are files confirmed by the user in prior turns this session.
    // These also bypass index confirmation.
    SessionPins []string

    // WorkflowID scopes archaeo retrieval. Empty string disables both archaeo passes.
    WorkflowID string
}

// Run executes the full pipeline and returns an EnrichedContextBundle.
//
// Execution order:
//   Stage 0: AnchorExtractor.Extract(input)
//            — CurrentTurnFiles and SessionPins are included unconditionally
//   Stage 1: parallel — IndexRetriever.Retrieve(anchors)
//                     + ArchaeoRetriever.RetrieveTopic(input.Query, input.WorkflowID)
//   Stage 2: HypotheticalGenerator.Generate(input.Query, stage1)
//            — skipped if anchor + stage1 code coverage >= SkipHypotheticalIfAnchorsAbove
//   Stage 3: ArchaeoRetriever.RetrieveExpanded(sketch)
//            — skipped if input.WorkflowID empty or Stage 2 was skipped
//   Merge:   ResultMerger.Merge
//
// Any stage error is logged and the pipeline continues with partial results.
// The PipelineTrace in the returned bundle records what was skipped and why.
func (p *Pipeline) Run(ctx context.Context, input PipelineInput) (EnrichedContextBundle, error)

// NewPipeline constructs a pipeline from a WorkspaceEnvironment and an optional
// TensionQuerier. All service fields are optional — nil dependencies cause the
// relevant stage to be skipped gracefully. See Environment Pre-initialization section.
func NewPipeline(
    env ayenitd.WorkspaceEnvironment,
    tensions TensionQuerier,  // optional — nil degrades archaeo topic retrieval to pattern-only
    config PipelineConfig,
) *Pipeline
```

---

### `pretask/resolver.go`

Sits between `UserResponse` and the pipeline. Validates and normalizes user-provided file paths before they enter `AnchorExtractor`. No changes to `platform/fs`.

```go
package pretask

// FileResolver validates and normalizes file paths from user input.
// It wraps framework/contextmgr.ExtractFileReferences for @mention parsing.
type FileResolver struct {
    workspace string // absolute workspace root
}

// ResolvedFiles holds the output of a resolution pass.
type ResolvedFiles struct {
    Paths []string // validated absolute paths within workspace
    Skipped []string // paths that failed validation (logged, not fatal)
}

// Resolve processes file picker selections and @mentions from a user response.
// - selections: UserResponse.Selections (file picker results)
// - text: UserResponse.Text (parsed for @-prefixed mentions)
// All paths are validated to be within the workspace root.
// Symlinks are not followed. Paths escaping workspace are dropped into Skipped.
func (r *FileResolver) Resolve(selections []string, text string) ResolvedFiles

// computeFileDelta returns the files added and removed relative to prior.
// Used to produce the incremental update for each turn.
func computeFileDelta(prior, current []string) (added, removed []string)
```

**Session pin accumulation** (`ContextProposalPhase` per turn):
```go
// 1. Load prior accumulated set
prior := loadPinnedFiles(memory, MemoryScopeSession)
// 2. Resolve new selections from incoming UserResponse
resolved := fileResolver.Resolve(resp.Selections, resp.Text)
// 3. Compute delta — only new files enter the pipeline this turn
added, _ := computeFileDelta(prior, resolved.Paths)
input.CurrentTurnFiles = added
// 4. Upsert new files as pinned — ProgressiveLoader skips re-loading unchanged files
for _, path := range added {
    contextMgr.UpsertFileItem(&core.FileContextItem{Path: path, Pinned: true})
}
// 5. Persist updated accumulated set for next turn
memory.Remember(ctx, "context.pinned_files", allFiles, MemoryScopeSession)
```

---

### New Interaction Phase: `named/euclo/interaction/modes/context_proposal.go`

Euclo emits a structured frame and awaits a response. It does not encode any presentation logic — that is the host UI's responsibility (relurpish for the TUI, or any other future surface).

```go
package modes

// ContextProposalPhase emits a ContextProposalFrame and awaits the user's
// response before the main execution phase runs.
//
// Slots into the chat mode interaction machine as the first phase (before intent).
// ChatMode receives the pipeline as a constructor parameter.
type ContextProposalPhase struct {
    Pipeline     ContextEnrichmentPipeline
    FileResolver *pretask.FileResolver
}

// ContextEnrichmentPipeline is the narrow interface the phase needs.
type ContextEnrichmentPipeline interface {
    Run(ctx context.Context, input pretask.PipelineInput) (pretask.EnrichedContextBundle, error)
}

// Execute runs the pipeline, emits the proposal frame, and collects the response.
//
// Input resolution (euclo owns this — relurpish does not pre-seed state):
//   - CurrentTurnFiles: resolved from the incoming UserResponse directly.
//     UserResponse.Selections (file picker) + @mentions parsed from UserResponse.Text
//     via FileResolver.Resolve(). Delta against HybridMemory["context.pinned_files"]
//     so only newly added files enter the pipeline this turn.
//   - SessionPins: loaded from HybridMemory["context.pinned_files"] (MemoryScopeSession)
//   - WorkflowID: from state key "euclo.workflow_id"
//
// Frame emitted: FrameProposal carrying ContextProposalContent (see below).
//
// Actions accepted:
//   "confirm" — accept bundle as-is
//   "add"     — response carries added file paths in resp.Paths
//   "remove"  — response carries removed file paths in resp.Paths
//   "skip"    — proceed without enrichment; no frame emitted on subsequent turns
//
// StateUpdates on any advance:
//   "context.confirmed_files"   []string               — paths to load
//   "context.knowledge_items"   []KnowledgeEvidenceItem — archaeo results
//   "context.pipeline_trace"    PipelineTrace          — observability
//
// Memory updates on any advance:
//   HybridMemory["context.pinned_files"] (MemoryScopeSession) — accumulated session pins
//   ContextManager.UpsertFileItem(Pinned=true) for each confirmed new file
func (p *ContextProposalPhase) Execute(
    ctx context.Context,
    mc interaction.PhaseMachineContext,
) (interaction.PhaseOutcome, error)
```

**`ContextProposalContent` — new content type in `named/euclo/interaction/content.go`:**

```go
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
    PipelineTrace pretask.PipelineTrace `json:"pipeline_trace"`
}

// ContextFileEntry is a single file entry in a context proposal.
type ContextFileEntry struct {
    Path    string `json:"path"`
    Summary string `json:"summary,omitempty"` // one-line description
    Score   float64 `json:"score,omitempty"`
    Source  string `json:"source"` // "anchor" | "index" | "vector"
}

// ContextKnowledgeEntry is a single archaeo knowledge item in a context proposal.
type ContextKnowledgeEntry struct {
    RefID   string `json:"ref_id"`
    Kind    string `json:"kind"`    // "pattern" | "tension" | "decision" | "interaction"
    Title   string `json:"title"`
    Summary string `json:"summary,omitempty"`
    Source  string `json:"source"` // "archaeo_topic" | "archaeo_expanded"
}
```

**Relurpish rendering responsibility:**
Relurpish receives the `ContextProposalContent` and renders the three-tier layout (Confirmed / Expanded / Project knowledge), handles auto-confirm timeout, and manages keyboard navigation. These are relurpish concerns and must not leak into euclo. Extend `euclo_renderer.go` with a renderer for `ContextProposalContent`.

---

## Environment Pre-initialization

All services the pipeline depends on must be instantiated **before any agent loads**. `ayenitd.Open()` is the composition root — it returns a fully-initialized `WorkspaceEnvironment` that the euclo agent receives directly. The euclo agent does not assemble services itself.

### No separate `EucloEnvironment` type

`ayenitd.WorkspaceEnvironment` already carries every field the pipeline needs:

| Field | Already in WorkspaceEnvironment? |
|---|---|
| `Model`, `Registry`, `IndexManager`, `SearchEngine`, `Memory`, `Config` | ✓ |
| `Embedder retrieval.Embedder` | ✓ (OllamaEmbedder; interface for future backends) |
| `WorkflowStore`, `PlanStore`, `PatternStore`, `RetrievalDB` | ✓ |
| `KnowledgeStore memory.KnowledgeStore` | Add to `ayenitd.WorkspaceEnvironment` |

No new environment type is needed. `NewPipeline` accepts `ayenitd.WorkspaceEnvironment` directly.

### `KnowledgeStore` addition

Add `KnowledgeStore memory.KnowledgeStore` to `ayenitd.WorkspaceEnvironment` and wire it in `ayenitd.Open()`. Used by `ArchaeoRetriever` as a fallback when workflow-scoped retrieval returns insufficient results.

### `TensionService` wiring

`WorkspaceEnvironment` does not carry a `TensionQuerier` — euclo's tension service is constructed internally via `archaeoBinding()` (see `named/euclo/agent.go`). `NewPipeline` therefore accepts it as a separate optional parameter:

```go
func NewPipeline(
    env ayenitd.WorkspaceEnvironment,
    tensions TensionQuerier,  // optional — nil degrades archaeo topic retrieval to pattern-only
    config PipelineConfig,
) *Pipeline
```

Euclo wires it in `InitializeEnvironment`:

```go
a.ContextPipeline = pretask.NewPipeline(a.Environment, a.tensionService(), pretask.DefaultPipelineConfig())
```

### Bootstrap responsibility

`ayenitd.Open()` (composition root, `ayenitd/open.go`) instantiates all stores and services before returning. `BootstrapAgentRuntime` (`ayenitd/bootstrap_extract.go`) handles agent-level wiring on top. Neither `app/relurpish/runtime/` nor `InitializeEnvironment` constructs platform services.

## Integration Points

### Where the pipeline is constructed

`named/euclo/agent.go` — `InitializeEnvironment` receives `ayenitd.WorkspaceEnvironment` and wires the pipeline with the optional tension service:

```go
a.ContextPipeline = pretask.NewPipeline(a.Environment, a.tensionService(), pretask.DefaultPipelineConfig())
```

`NewPipeline` extracts what it needs from the environment. Nil fields degrade gracefully.

### Where current-turn files come from

Euclo owns file selection — relurpish does not pre-seed state. `UserResponse` already carries:

```go
type UserResponse struct {
    ActionID   string
    Text       string   // @mention parsing source
    Selections []string // file picker results
}
```

`ContextProposalPhase.Execute` resolves current-turn files directly from the incoming `UserResponse`:
1. Parse `@`-prefixed paths from `UserResponse.Text` via `pretask.FileResolver`
2. Take `UserResponse.Selections` (file picker) directly
3. Merge and validate both via `FileResolver.Resolve(workspace)`
4. Feed the result as `PipelineInput.CurrentTurnFiles`

Relurpish migrates to populate `UserResponse.Selections` from its file picker UI rather than writing to agent state directly.

### File selection caching — no re-send on subsequent turns

Accumulated session pins are stored in `HybridMemory` (`MemoryScopeSession`) under key `"context.pinned_files"`. Each turn:
1. Load prior set from `HybridMemory`
2. `computeFileDelta(prior, current)` → only the added files enter `PipelineInput.CurrentTurnFiles`
3. `ContextManager.UpsertFileItem(path, Pinned=true)` for each new addition
4. `HybridMemory.Remember("context.pinned_files", updatedSet, MemoryScopeSession)`

`ProgressiveLoader.loadedFiles` ensures already-loaded files are not re-read from disk. `FileContextItem.Pinned = true` protects confirmed files from context pruning.

### Where the phase is inserted

`named/euclo/interaction/modes/chat.go` — `ContextProposalPhase` is inserted as the first phase (before `intent`). `ChatMode(...)` receives the pipeline as a parameter, threading it into the phase constructor. Confirmed results are written to state keys for downstream phases.

### Where retrieval policy changes

`named/euclo/runtime/retrieval.go` — `ResolveRetrievalPolicy`: set `WidenWhenNoLocal = true` unconditionally. This is a one-line fix, independent of the new pipeline, that enables workflow retrieval as a floor for all modes.

### Where confirmed context is loaded

`named/euclo/runtime/context/runtime_impl.go` — `BuildContextRuntime` checks for `"context.confirmed_files"` in state and loads them via `ProgressiveLoader` before `InitialLoad` runs:

```go
if confirmed, ok := state.Get("context.confirmed_files").([]string); ok {
    for _, path := range confirmed {
        _ = policy.Progressive.DrillDown(path)
    }
}
```

### Where SemanticInputBundle is populated

`named/euclo/runtime/work/work.go` — when building `UnitOfWork`, if `"context.knowledge_items"` is in state, convert `[]KnowledgeEvidenceItem` to `[]SemanticFindingSummary` and inject into `SemanticInputBundle`. This bridges pipeline output into the existing archaeo provenance system used by all execution modes.

---

## Configuration

The pipeline is configurable per-session via manifest `skill_config`:

**Euclo manifest `skill_config`** — controls pipeline behavior:

```yaml
skill_config:
  context_enrichment:
    enabled: true
    max_code_files: 6
    max_knowledge_items: 4
    token_budget: 2000
    hypothetical_max_tokens: 120
    skip_hypothetical_if_anchors_above: 4
    show_confirmation_frame: true   # false = silent enrichment, skip the proposal phase
```

`show_confirmation_frame: false` causes the pipeline to run silently — results are loaded into context without emitting a proposal frame. Useful for automated contexts or power users.

**Relurpish-specific configuration** (not euclo's concern):
- Auto-confirm timeout duration
- Countdown display style
- Keyboard shortcut bindings for confirm/add/remove/skip

These belong in relurpish settings, not the euclo manifest.

---

## Testing Infrastructure

### Unit tests — `named/euclo/runtime/pretask/`

#### `anchor_test.go`

```go
// TestAnchorExtractor_CurrentTurnFilesHighestPriority
//   input.CurrentTurnFiles: ["framework/capability/runtime_policy.go"]
//   input.Query: "explain this file"
//   Mock index returns nothing (no symbols in query)
//   Assert: AnchorSet.FilePaths contains "framework/capability/runtime_policy.go"
//   Assert: index.QuerySymbol never called for this path (no confirmation needed)

// TestAnchorExtractor_CurrentTurnFilesNotYetLoaded
//   input.CurrentTurnFiles: ["some/new/file.go"]  (not in ProgressiveLoader)
//   Assert: file is still included — user selection bypasses load-state check

// TestAnchorExtractor_CamelCaseExtraction
//   input.Query: "How does PermissionManager decide whether to allow a tool call"
//   Mock index returns nodes for "PermissionManager", "CheckPermission"
//   Assert: AnchorSet.SymbolNames contains both confirmed symbols

// TestAnchorExtractor_UnknownSymbolFiltered
//   input.Query: "How does FooBarBaz work"
//   Mock index returns no nodes for "FooBarBaz"
//   Assert: AnchorSet.SymbolNames = []  (not confirmed, filtered out)

// TestAnchorExtractor_SessionPinsPassthrough
//   input.SessionPins: ["agents/react/react.go"]
//   Assert: AnchorSet.SessionPins includes "agents/react/react.go"
//   Assert: index.QuerySymbol NOT called for session pins (already confirmed)

// TestAnchorExtractor_PriorityOrder
//   input.CurrentTurnFiles: ["a.go"]
//   input.SessionPins: ["b.go"]
//   input.Query: "explain @c.go and CamelSymbol"
//   Mock index confirms "CamelSymbol"
//   Assert: AnchorSet.FilePaths order: ["a.go", "c.go"] (current turn before @mentions)
//   Assert: AnchorSet.SessionPins = ["b.go"]
//   Assert: AnchorSet.SymbolNames = ["CamelSymbol"]

// TestAnchorExtractor_EmptyInput
//   input = PipelineInput{}
//   Assert: AnchorSet is zero-value, no panic
```

#### `hypothetical_test.go`

```go
// TestHypotheticalGenerator_GroundedOutput
//   Mock model returns "PermissionManager.CheckPermission, TrustClass, AllowDeny"
//   Assert: HypotheticalSketch.Grounded = true
//   Assert: HypotheticalSketch.Text contains expected tokens

// TestHypotheticalGenerator_NilModelFallback
//   model = nil
//   Assert: HypotheticalSketch.Grounded = false, no panic

// TestHypotheticalGenerator_EmptyStage1Fallback
//   stage1 = Stage1Result{} (no evidence)
//   Assert: HypotheticalSketch.Grounded = false
//   (does not call model when there is nothing to ground from)

// TestHypotheticalGenerator_SkipWhenAnchorsCoverageHigh
//   config.SkipHypotheticalIfAnchorsAbove = 4
//   stage1.CodeEvidence has 5 items
//   Assert: generation skipped, HypotheticalSketch.Grounded = false
//   Assert: PipelineTrace.FallbackReason = "anchor_coverage_sufficient"
```

#### `merger_test.go`

```go
// TestResultMerger_AnchoredFilesAlwaysIncluded
//   anchors has 2 files, code budget would normally limit to 1
//   Assert: both anchored files present in AnchoredFiles

// TestResultMerger_Deduplication
//   Same file appears in anchor + index retrieval results
//   Assert: file appears once in output, source = "anchor" (higher priority)

// TestResultMerger_TokenBudgetEnforced
//   config.TokenBudget = 100
//   input has 10 large files
//   Assert: total TokenEstimate <= 100

// TestResultMerger_KnowledgeDeduplication
//   Same RefID appears in KnowledgeTopic + KnowledgeExpanded
//   Assert: item appears once

// TestResultMerger_PipelineTracePopulated
//   Full input provided
//   Assert: all PipelineTrace fields are set
```

#### `pipeline_test.go`

```go
// TestPipeline_FullFlow
//   Mock all dependencies
//   Assert: all 4 stages run in order
//   Assert: EnrichedContextBundle has items from each stage
//   Assert: PipelineTrace reflects all stages

// TestPipeline_NoArchaeoWhenWorkflowIDEmpty
//   config.WorkflowID = ""
//   Assert: archaeo retriever not called (both passes)
//   Assert: PipelineTrace.Stage1ArchaeoResults = 0

// TestPipeline_ContinuesOnStageError
//   Stage 1 index retriever returns error
//   Assert: pipeline continues, returns partial results
//   Assert: PipelineTrace.FallbackUsed = true

// TestPipeline_NilIndexHandled
//   index = nil
//   Assert: Stage 0 + Stage 1a skipped gracefully
//   Assert: no panic

// TestPipeline_AsyncConcurrency
//   Stage 1 runs index + archaeo in parallel
//   Mock both with artificial delay
//   Assert: total elapsed < sum of individual delays (confirms parallelism)
```

### Integration tests — `named/euclo/runtime/pretask/integration_test.go`

These require a real SQLite index and a real (but small) workspace fixture.

```go
// TestPipelineIntegration_RealIndexRealQuery
//   Setup: index a small Go fixture package with known symbols
//   Run: Pipeline.Run with query containing a known symbol name
//   Assert: AnchorSet.SymbolNames contains the known symbol
//   Assert: CodeEvidence includes the file containing the symbol
//   Assert: PipelineTrace.AnchorsConfirmed > 0

// TestPipelineIntegration_DependencyExpansion
//   Setup: fixture with file A importing file B
//   Run: pipeline with query referencing a symbol in file A
//   Assert: file B appears in ExpandedFiles (1-hop dependency)

// TestPipelineIntegration_GroundedHypotheticalImproves
//   Setup: fixture where raw query has poor vocabulary match
//   Run 1: pipeline with HypotheticalMaxTokens=0 (skip hypothetical)
//   Run 2: pipeline with full hypothetical
//   Compare: Run 2 ExpandedFiles has higher avg score or more results
//   (demonstrates that grounded HyDE improves retrieval)
```

### Interaction phase tests — `named/euclo/interaction/modes/context_proposal_test.go`

```go
// TestContextProposalPhase_ConfirmAction
//   Mock pipeline returns bundle with 2 anchored + 2 expanded files
//   Mock emitter receives FrameProposal, user responds "confirm"
//   Assert: StateUpdates["context.confirmed_files"] has all 4 paths
//   Assert: StateUpdates["context.session_pins"] includes anchored files

// TestContextProposalPhase_SkipAction
//   User responds "skip"
//   Assert: PhaseOutcome.Advance = true
//   Assert: StateUpdates["context.confirmed_files"] = nil (nothing loaded)
//   Assert: no session pins set

// TestContextProposalPhase_AddFile
//   User responds with add action + file path
//   Assert: added file appears in confirmed_files
//   Assert: added file appears in session_pins

// TestContextProposalPhase_RemoveFile
//   User responds with remove action targeting an expanded file
//   Assert: removed file absent from confirmed_files
//   Assert: anchored files still present (cannot remove anchors without explicit session pin clear)

// TestContextProposalPhase_EmptyBundle
//   Pipeline returns empty EnrichedContextBundle
//   Assert: frame is still emitted (with empty state message)
//   Assert: PhaseOutcome advances normally

// TestContextProposalPhase_PipelineError
//   Pipeline.Run returns error
//   Assert: phase emits status frame with error summary
//   Assert: PhaseOutcome.Advance = true (skips enrichment, does not block)
```

### Regression tests — `named/euclo/runtime/pretask/regression_test.go`

Record known query → expected anchor symbols and evidence files using a committed fixture workspace. These catch vocabulary extraction regressions without needing Ollama.

```go
// TestRegressionAnchorExtraction
//   Table-driven: []struct{query string; expectedSymbols []string}
//   Uses fixture index at testdata/fixture_workspace/
//   Confirms stable extraction across changes to AnchorExtractor

// TestRegressionRetrievalPolicy
//   Confirms code mode now has WidenWhenNoLocal=true after the one-line fix
//   Prevents regression of the retrieval policy gap
```

---

## Implementation Phases

### Phase 1 — Foundation

**Steps:**
1. Add `KnowledgeStore memory.KnowledgeStore` to `ayenitd.WorkspaceEnvironment` and wire in `ayenitd.Open()`
2. Create `named/euclo/runtime/pretask/` package: `types.go`, `anchor.go`, `merger.go`, `pipeline.go`, `resolver.go`
3. Implement `FileResolver` — @mention parsing (wraps `contextmgr.ExtractFileReferences`), path validation, `computeFileDelta`
4. Implement `AnchorExtractor` — `CurrentTurnFiles` highest priority, session pins second, query symbols third
5. Implement `ResultMerger` with unit tests
6. Fix `ResolveRetrievalPolicy` code mode (one-line `WidenWhenNoLocal` fix)
7. All unit tests pass without any LLM or embedder

**Files modified:**
- `ayenitd/environment.go` — add `KnowledgeStore` field to `WorkspaceEnvironment`
- `ayenitd/open.go` — wire `KnowledgeStore` during `Open()`
- `ayenitd/stores.go` — open `KnowledgeStore` alongside other runtime stores
- `named/euclo/runtime/retrieval.go` — one-line `WidenWhenNoLocal = true` fix in `ResolveRetrievalPolicy`

**Files created (new package):**
- `named/euclo/runtime/pretask/types.go`
- `named/euclo/runtime/pretask/anchor.go`
- `named/euclo/runtime/pretask/merger.go`
- `named/euclo/runtime/pretask/pipeline.go`
- `named/euclo/runtime/pretask/resolver.go`
- `named/euclo/runtime/pretask/anchor_test.go`
- `named/euclo/runtime/pretask/merger_test.go`

**Packages read (no changes):**
- `framework/contextmgr/context_policy_types.go` — `ExtractFileReferences` wrapped by `FileResolver`
- `framework/ast/index_manager.go` — `IndexManager`, `QuerySymbol`, `SearchNodes` (behind `IndexQuerier` interface)
- `framework/core/context_item.go` — `FileContextItem` type reference

**Unit tests:**

`named/euclo/runtime/pretask/resolver_test.go` — new, all tests in this phase:
```go
// TestFileResolver_AtMentionExtraction
//   text: "can you explain @framework/capability/registry.go and how it relates to PermissionManager"
//   Assert: Paths contains "framework/capability/registry.go"
//   Assert: "PermissionManager" not in Paths (it's a symbol, not a path)

// TestFileResolver_SelectionsMergedWithMentions
//   selections: ["named/euclo/agent.go"]
//   text: "also look at @framework/core/types.go"
//   Assert: Paths contains both files, no duplicates

// TestFileResolver_PathOutsideWorkspaceDropped
//   selections: ["../../etc/passwd", "/absolute/path/outside.go"]
//   Assert: both dropped into Skipped, Paths = []

// TestFileResolver_RelativePathNormalized
//   workspace: "/home/user/project"
//   selections: ["named/euclo/../euclo/agent.go"]
//   Assert: Paths contains "named/euclo/agent.go" (cleaned)

// TestFileResolver_EmptyInput
//   selections: [], text: "just a plain question"
//   Assert: Paths = [], Skipped = [], no panic

// TestComputeFileDelta_NewFiles
//   prior: ["a.go", "b.go"], current: ["a.go", "b.go", "c.go"]
//   Assert: added = ["c.go"], removed = []

// TestComputeFileDelta_RemovedFile
//   prior: ["a.go", "b.go"], current: ["a.go"]
//   Assert: added = [], removed = ["b.go"]

// TestComputeFileDelta_NoChange
//   prior == current
//   Assert: added = [], removed = []

// TestComputeFileDelta_EmptyPrior
//   prior: [], current: ["a.go", "b.go"]
//   Assert: added = ["a.go", "b.go"], removed = []
```

`named/euclo/runtime/pretask/anchor_test.go` — defined in Testing Infrastructure section (all cases apply here).

`named/euclo/runtime/pretask/merger_test.go` — defined in Testing Infrastructure section (all cases apply here).

`ayenitd/environment_test.go` — extend existing suite:
```go
// TestWorkspaceEnvironment_KnowledgeStoreWired
//   Open() a workspace with a real (temp) SQLite path
//   Assert: env.KnowledgeStore != nil
//   Assert: env.KnowledgeStore.Close() returns nil (basic health)
```

`named/euclo/runtime/retrieval_test.go` — existing file, add:
```go
// TestResolveRetrievalPolicy_ChatModeWidensWhenNoLocal
//   Call ResolveRetrievalPolicy for "chat" mode with no local files
//   Assert: policy.WidenWhenNoLocal = true
//   (regression guard for the one-line fix)
```

---

### Phase 2 — Retrieval stages

**Steps:**
1. Implement `IndexRetriever` — structural expansion via AST dependency/call graph
2. Implement `ArchaeoRetriever` — `RetrieveTopic` (with optional `TensionQuerier`, degrades to pattern-only if nil) and `RetrieveExpanded`
3. Implement `Pipeline.Run` stages 0, 1, 3 (hypothetical skipped — `Grounded=false`)
4. `NewPipeline(env WorkspaceEnvironment, tensions TensionQuerier, config)` wired to environment services
5. Integration tests against fixture workspace

**Files modified:**
- `named/euclo/runtime/pretask/pipeline.go` — implement `Pipeline.Run`, `NewPipeline`
- `named/euclo/agent.go` — `InitializeEnvironment` wires `NewPipeline(a.Environment, a.tensionService(), ...)`

**Files created:**
- `named/euclo/runtime/pretask/retrieval.go` — `IndexRetriever`, `ArchaeoRetriever`
- `named/euclo/runtime/pretask/retrieval_test.go`
- `named/euclo/runtime/pretask/pipeline_test.go`
- `named/euclo/runtime/pretask/integration_test.go`
- `named/euclo/runtime/pretask/testdata/fixture_workspace/` — fixture Go package for integration tests

**Packages read (no changes):**
- `framework/ast/index_manager.go` — `GetDependencyGraph`, `GetCallGraph`
- `framework/ast/graphschema.go` — `DependencyGraph`, `CallGraph` types
- `framework/patterns/store.go` — `PatternStore` (behind `PatternQuerier` interface)
- `framework/retrieval/service.go` — `RetrieverService` for archaeo semantic search
- `framework/retrieval/embedder.go` — `Embedder` interface
- `archaeo/tensions/service.go` — `Service.ActiveByWorkflow`, `SummaryByWorkflow` (behind `TensionQuerier` interface)
- `archaeo/bindings/euclo/service.go` — source of `tensionService()` passed to `NewPipeline`
- `ayenitd/environment.go` — `WorkspaceEnvironment` consumed by `NewPipeline`

**Unit tests:**

`named/euclo/runtime/pretask/retrieval_test.go` — new, all tests in this phase:
```go
// TestIndexRetriever_SymbolExpansion
//   anchors.SymbolNames = ["PermissionManager"]
//   Mock index returns file "framework/authorization/manager.go" for symbol
//   Mock dep graph returns 1 dependency file "framework/authorization/policy.go"
//   Assert: result contains both files
//   Assert: "framework/authorization/manager.go" has source="anchor"
//   Assert: "framework/authorization/policy.go" has source="index"

// TestIndexRetriever_FilePathDirectLoad
//   anchors.FilePaths = ["named/euclo/agent.go"]
//   Assert: result contains "named/euclo/agent.go" with source="anchor"
//   Assert: no dependency expansion attempted for file-path anchors

// TestIndexRetriever_DeduplicatesAcrossSymbols
//   Two symbols both resolve to the same file
//   Assert: file appears once in result

// TestIndexRetriever_MaxFilesPerSymbolCapped
//   config.MaxFilesPerSymbol = 2
//   Mock dep graph returns 5 dependency files for symbol
//   Assert: result contains at most 3 files (1 symbol file + 2 deps)

// TestIndexRetriever_NilIndexDegrades
//   index = nil
//   Assert: returns empty result, no panic

// TestArchaeoRetriever_RetrieveTopicWithTensions
//   Mock TensionQuerier returns 2 active tensions matching query keywords
//   Mock PatternQuerier returns 1 pattern
//   Assert: result contains all 3 items, kinds set correctly
//   Assert: tensions scored higher than pattern (keyword overlap)

// TestArchaeoRetriever_NilTensionsDegradesToPatternsOnly
//   tensions = nil (TensionQuerier not provided)
//   Mock PatternQuerier returns 2 patterns
//   Assert: result contains 2 patterns, no panics
//   Assert: PipelineTrace.FallbackReason not set (patterns are not a fallback)

// TestArchaeoRetriever_EmptyWorkflowIDSkipsRetrieval
//   config.WorkflowID = ""
//   Assert: neither TensionQuerier nor PatternQuerier called
//   Assert: returns empty result

// TestArchaeoRetriever_RetrieveExpandedUsesSketch
//   sketch.Grounded = true, sketch.Text = "PermissionManager CheckPermission"
//   Mock retrieval service returns 2 knowledge items for sketch text
//   Assert: result contains both items with source="archaeo_expanded"

// TestArchaeoRetriever_RetrieveExpandedSkipsWhenNotGrounded
//   sketch.Grounded = false
//   Assert: retrieval service not called
//   Assert: returns empty result
```

`named/euclo/runtime/pretask/pipeline_test.go` — defined in Testing Infrastructure section (all cases apply here).

`named/euclo/runtime/pretask/integration_test.go` — defined in Testing Infrastructure section (`TestPipelineIntegration_RealIndexRealQuery`, `TestPipelineIntegration_DependencyExpansion` apply here; `TestPipelineIntegration_GroundedHypotheticalImproves` deferred to Phase 3).

---

### Phase 3 — Hypothetical generation

**Steps:**
1. Implement `HypotheticalGenerator` — synchronous: LLM generate → embed → return `HypotheticalSketch`
2. Add Stage 2 to `Pipeline.Run`; wire skip threshold (`SkipHypotheticalIfAnchorsAbove`)
3. Integration test demonstrating retrieval improvement vs anchor-only baseline

**Files modified:**
- `named/euclo/runtime/pretask/pipeline.go` — add Stage 2 to `Pipeline.Run`, add `hypotheticalGen` field

**Files created:**
- `named/euclo/runtime/pretask/hypothetical.go` — `HypotheticalGenerator`
- `named/euclo/runtime/pretask/hypothetical_test.go`

**Packages read (no changes):**
- `framework/core/` — `LanguageModel` interface (model call for sketch generation)
- `framework/retrieval/embedder.go` — `Embedder.Embed` (synchronous, called immediately after generation)
- `framework/retrieval/ollama_embedder.go` — concrete implementation provided via `WorkspaceEnvironment.Embedder`

**Unit tests:**

`named/euclo/runtime/pretask/hypothetical_test.go` — defined in Testing Infrastructure section (all four cases apply here).

`named/euclo/runtime/pretask/pipeline_test.go` — extend with:
```go
// TestPipeline_Stage2RunsWhenAnchorsBelowThreshold
//   config.SkipHypotheticalIfAnchorsAbove = 4
//   stage1 returns 2 CodeEvidence items
//   Assert: HypotheticalGenerator.Generate called
//   Assert: PipelineTrace.HypotheticalGenerated = true

// TestPipeline_Stage2SkippedWhenAnchorsAboveThreshold
//   config.SkipHypotheticalIfAnchorsAbove = 4
//   stage1 returns 5 CodeEvidence items
//   Assert: HypotheticalGenerator.Generate NOT called
//   Assert: PipelineTrace.HypotheticalGenerated = false
//   Assert: Stage 3 also skipped (no grounded sketch)

// TestPipeline_Stage3SkippedWhenEmbeddingFails
//   Mock embedder returns error
//   Assert: HypotheticalSketch.Grounded = false
//   Assert: ArchaeoRetriever.RetrieveExpanded NOT called
//   Assert: pipeline returns partial results without Stage 3
```

`named/euclo/runtime/pretask/integration_test.go` — add:
```go
// TestPipelineIntegration_GroundedHypotheticalImproves
//   (defined in Testing Infrastructure section — activated in this phase)
```

---

### Phase 4 — Interaction phase

**Steps:**
1. Add `ContextProposalContent`, `ContextFileEntry`, `ContextKnowledgeEntry` to `interaction/content.go`
2. Implement `ContextProposalPhase` — resolves `CurrentTurnFiles` from `UserResponse` directly (Selections + @mention parse), computes delta via `computeFileDelta` against `HybridMemory["context.pinned_files"]`
3. Wire `ContextProposalPhase` into `ChatMode(...)` as the first phase; thread pipeline as constructor parameter
4. Wire confirmed files into `BuildContextRuntime` via `ProgressiveLoader.UpsertFileItem(Pinned=true)`
5. Wire `KnowledgeEvidenceItems` into `SemanticInputBundle` in `work.go`
6. Relurpish: extend `euclo_renderer.go` to render `ContextProposalContent`
7. Relurpish: populate `UserResponse.Selections` from file picker UI (migration from prior state-writing approach)

**Files modified:**
- `named/euclo/interaction/content.go` — add `ContextProposalContent`, `ContextFileEntry`, `ContextKnowledgeEntry`
- `named/euclo/interaction/modes/chat.go` — thread `Pipeline` parameter into `ChatMode(...)`, prepend `ContextProposalPhase`
- `named/euclo/runtime/context/runtime_impl.go` — `BuildContextRuntime` loads confirmed files via `ProgressiveLoader`
- `named/euclo/runtime/work/work.go` — inject `KnowledgeEvidenceItems` into `SemanticInputBundle`
- `app/relurpish/tui/euclo_renderer.go` — render `ContextProposalContent` frame
- `app/relurpish/euclotui/renderer.go` — coordinate file picker `UserResponse.Selections` population

**Files created:**
- `named/euclo/interaction/modes/context_proposal.go` — `ContextProposalPhase`
- `named/euclo/interaction/modes/context_proposal_test.go`

**Packages read (no changes):**
- `named/euclo/interaction/emitter.go` — `UserResponse` type (`.Selections`, `.Text`)
- `named/euclo/interaction/machine.go` — `PhaseMachine`, `PhaseDefinition`, `PhaseMachineContext`
- `framework/contextmgr/context_manager.go` — `UpsertFileItem`
- `framework/contextmgr/progressive_loader.go` — `loadedFiles`, file caching behaviour
- `framework/core/context_item.go` — `FileContextItem.Pinned`

**Unit tests:**

`named/euclo/interaction/modes/context_proposal_test.go` — defined in Testing Infrastructure section (all six cases apply here). Extend with:
```go
// TestContextProposalPhase_ResolvesFilesFromUserResponse
//   UserResponse.Selections = ["named/euclo/agent.go"]
//   UserResponse.Text = "explain @framework/core/types.go too"
//   Assert: PipelineInput.CurrentTurnFiles contains both paths
//   Assert: state key "context.current_turn_files" NOT read (euclo owns resolution)

// TestContextProposalPhase_DeltaOnlyNewFilesEnterPipeline
//   HybridMemory["context.pinned_files"] = ["a.go", "b.go"]
//   UserResponse.Selections = ["a.go", "b.go", "c.go"]
//   Assert: PipelineInput.CurrentTurnFiles = ["c.go"] only (delta)
//   Assert: "a.go" and "b.go" still in confirmed_files (from prior pins)

// TestContextProposalPhase_InvalidPathDropped
//   UserResponse.Selections = ["../../etc/passwd"]
//   Assert: PipelineInput.CurrentTurnFiles = []
//   Assert: phase advances normally (bad path does not block)

// TestContextProposalPhase_ConfirmedFilesUpsertedAsPinned
//   Mock ContextManager captures UpsertFileItem calls
//   User confirms bundle with 2 anchored files
//   Assert: UpsertFileItem called twice with Pinned=true
```

`named/euclo/runtime/context/runtime_impl_test.go` — extend existing suite:
```go
// TestBuildContextRuntime_LoadsConfirmedFiles
//   state["context.confirmed_files"] = ["framework/core/types.go"]
//   Assert: ProgressiveLoader.DrillDown called for the path
//   Assert: file appears in context before InitialLoad runs
```

`named/euclo/runtime/work/work_test.go` — extend existing suite:
```go
// TestBuildUnitOfWork_InjectsKnowledgeItems
//   state["context.knowledge_items"] = []KnowledgeEvidenceItem{...}
//   Assert: SemanticInputBundle contains converted SemanticFindingSummary entries
//   Assert: Kind and Title fields map correctly
```

---

### Phase 5 — Session persistence + observability

**Steps:**
1. Session pin accumulation via `HybridMemory` (MemoryScopeSession) — `"context.pinned_files"` persists across turns, incremental delta computed each turn
2. `PipelineTrace` written to state for relurpish observability pane
3. `show_confirmation_frame: false` silent enrichment mode in manifest config
4. Relurpish: auto-confirm timeout (relurpish settings, not euclo manifest)

**Files modified:**
- `named/euclo/interaction/modes/context_proposal.go` — add `HybridMemory` load/store calls for pin accumulation, `PipelineTrace` state write, `show_confirmation_frame` skip path
- `relurpify_cfg/agent.manifest.yaml` — add `context_enrichment` block under `skill_config` with `show_confirmation_frame` toggle
- `app/relurpish/tui/euclo_renderer.go` — auto-confirm timeout UI (relurpish-owned setting)

**Packages read (no changes):**
- `framework/memory/memory.go` — `HybridMemory`, `MemoryScopeSession`, `Remember`, `Recall`
- `named/euclo/runtime/pretask/resolver.go` — `computeFileDelta` (already implemented in Phase 1)

**Unit tests:**

`named/euclo/interaction/modes/context_proposal_test.go` — extend with:
```go
// TestContextProposalPhase_PinsAccumulateAcrossTurns
//   Turn 1: user confirms ["a.go"] → HybridMemory stores ["a.go"]
//   Turn 2: UserResponse.Selections = ["b.go"]
//   Assert: PipelineInput.CurrentTurnFiles = ["b.go"] (delta only)
//   Assert: confirmed_files after turn 2 = ["a.go", "b.go"]
//   Assert: HybridMemory["context.pinned_files"] = ["a.go", "b.go"]

// TestContextProposalPhase_PipelineTraceWrittenToState
//   Mock pipeline returns bundle with populated PipelineTrace
//   User confirms
//   Assert: state["context.pipeline_trace"] matches PipelineTrace fields

// TestContextProposalPhase_SilentModeSkipsFrame
//   config.ShowConfirmationFrame = false
//   Assert: no FrameProposal emitted
//   Assert: all pipeline results loaded into context directly
//   Assert: HybridMemory pins still updated (accumulation continues silently)

// TestContextProposalPhase_RemovePinClearsFromMemory
//   HybridMemory["context.pinned_files"] = ["a.go", "b.go"]
//   User responds with remove action targeting "b.go"
//   Assert: HybridMemory["context.pinned_files"] = ["a.go"] after phase
//   Assert: "b.go" absent from confirmed_files
```

`ayenitd/workspace_test.go` — extend existing suite:
```go
// TestWorkspace_ManifestContextEnrichmentConfig
//   Load manifest with skill_config.context_enrichment block
//   Assert: show_confirmation_frame parsed correctly
//   Assert: max_code_files, token_budget fields populated
```

---

## Resolved Design Questions

1. **`EucloEnvironment` vs `AgentEnvironment`** — no separate type needed. `ayenitd.WorkspaceEnvironment` is used directly. It already carries all required fields (`Model`, `Registry`, `IndexManager`, `Embedder`, `WorkflowStore`, `PlanStore`, `PatternStore`, `RetrievalDB`). `KnowledgeStore` will be added.

2. **`CurrentTurnFiles` delivery timing** — euclo owns file selection. `ContextProposalPhase.Execute` resolves files directly from the incoming `UserResponse` (`.Selections` + @mention parse of `.Text`) via `FileResolver`. Relurpish migrates to populate `UserResponse.Selections` from its file picker; it does not pre-seed agent state.

3. **Async embedding** — synchronous and sequential: generate hypothetical text → embed immediately → Stage 3. With a fixed Ollama instance pool, embedding and generation share the same endpoint; parallelism is not achievable in practice. A text-based Stage 3 fallback was rejected as a quality regression. Embedding failure: `Grounded=false`, Stage 3 skipped.

4. **`TensionService` wiring** — `NewPipeline` accepts an optional `TensionQuerier` parameter alongside `WorkspaceEnvironment`. Euclo passes `a.tensionService()` at construction. Nil degrades archaeo topic retrieval to pattern-only (no tension data). No change to `WorkspaceEnvironment`.

5. **`KnowledgeStore`** — added to `ayenitd.WorkspaceEnvironment`; wired in `ayenitd.Open()`.

6. **Embedder** — `retrieval.Embedder` interface already exists (`framework/retrieval/embedder.go`). `WorkspaceEnvironment.Embedder` is already typed as this interface (OllamaEmbedder concrete impl). No new interface or factory needed.

7. **Service initialization** — `ayenitd.Open()` is the composition root. All stores and services are instantiated before `WorkspaceEnvironment` is returned. `BootstrapAgentRuntime` (`ayenitd/bootstrap_extract.go`) handles agent-level wiring. `InitializeEnvironment` does not construct platform services.

8. **Auto-confirm timeout** — relurpish concern, not euclo's. Euclo emits the frame and awaits a response. Relurpish decides how long to wait and how to indicate it. Configuration belongs in relurpish settings.

9. **Knowledge item rendering** — extend `euclo_renderer.go` in relurpish to render `ContextProposalContent`. Euclo remains UI-agnostic.

10. **platform/fs** — no changes needed. File path validation and @mention parsing belong in `pretask.FileResolver`, which wraps `contextmgr.ExtractFileReferences`. platform/fs remains a pure tool/capability layer.

11. **File selection caching** — handled by existing framework primitives: `ProgressiveLoader.loadedFiles` prevents re-loading unchanged files; `FileContextItem.Pinned=true` protects confirmed files from pruning; `HybridMemory` (MemoryScopeSession) accumulates the session pin set across turns. New code: `FileResolver`, `computeFileDelta` (~20 lines each).
