# Chat Context Enrichment Pipeline

**Status**: Planned — pre-beta euclo requirement  
**Priority**: High — closes a fundamental gap in chat mode pre-task augmentation  
**Depends on**: IndexManager wiring confirmed correct (see `docs/issues/recurring-indexmanager-wiring-gap.md`)  
**Target**: `named/euclo/` chat mode execution path  
**Authored**: 2026-04-03

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
- At Ollama speeds for qwen2.5-coder:14b: ~2-4 seconds
- Acceptable for chat mode; hidden behind interaction round-trip if async prefetch used

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

// NewPipeline constructs a pipeline from a pre-initialized EucloEnvironment.
// All service fields are optional — nil dependencies cause the relevant stage
// to be skipped gracefully. See Environment Pre-initialization section.
func NewPipeline(env EucloEnvironment, config PipelineConfig) *Pipeline
```

---

### New Interaction Phase: `named/euclo/interaction/modes/context_proposal.go`

Euclo emits a structured frame and awaits a response. It does not encode any presentation logic — that is the host UI's responsibility (relurpish for the TUI, or any other future surface).

```go
package modes

// ContextProposalPhase emits a ContextProposalFrame and awaits the user's
// response before the main execution phase runs.
//
// Slots into the chat mode interaction machine immediately after the query
// is received and before any LLM execution.
type ContextProposalPhase struct {
    Pipeline ContextEnrichmentPipeline
}

// ContextEnrichmentPipeline is the narrow interface the phase needs.
type ContextEnrichmentPipeline interface {
    Run(ctx context.Context, input pretask.PipelineInput) (pretask.EnrichedContextBundle, error)
}

// Execute runs the pipeline, emits the proposal frame, and collects the response.
//
// Input resolution:
//   - input.CurrentTurnFiles from state key "context.current_turn_files"
//     (set by the host before Execute is called — e.g. @mentions parsed by relurpish)
//   - input.SessionPins from state key "context.session_pins"
//   - input.WorkflowID from state key "euclo.workflow_id"
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
//   "context.session_pins"      []string               — updated session pins
//   "context.pipeline_trace"    PipelineTrace          — observability
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

All services the pipeline depends on must be instantiated **before any agent loads**, as part of bootstrap. The euclo agent receives a fully-initialized `EucloEnvironment` — it does not assemble services itself.

### `EucloEnvironment`

Extend `framework/agentenv/environment.go` (or define in `named/euclo/`) with a euclo-specific environment type that carries all pre-initialized services:

```go
// EucloEnvironment carries all services euclo needs, pre-initialized by bootstrap.
// All fields are optional — absent services cause the relevant pipeline stages
// to degrade gracefully.
type EucloEnvironment struct {
    // From AgentEnvironment
    Model        core.LanguageModel
    Registry     *capability.Registry
    IndexManager *ast.IndexManager   // AST index, already running StartIndexing
    SearchEngine *search.SearchEngine
    Memory       memory.MemoryStore
    Config       *core.Config

    // Embedder — generic interface, not tied to any provider.
    // Bootstrap selects the concrete implementation based on configuration.
    Embedder retrieval.Embedder

    // Archaeo services — all instantiated before agent loads.
    TensionService  TensionQuerier               // archaeo/tensions
    PatternStore    PatternQuerier               // framework/patterns
    PlanStore       frameworkplan.PlanStore
    WorkflowStore   memory.WorkflowStateStore
    RetrievalDB     *sql.DB                      // for archaeo semantic retrieval

    // Checkpoint and memory
    CheckpointStore memory.CheckpointStore
    KnowledgeStore  memory.KnowledgeStore        // for ListKnowledge fallback
}
```

### Bootstrap responsibility

`app/relurpish/runtime/bootstrap.go` — `BootstrapAgentRuntime` must instantiate all services listed in `EucloEnvironment` before constructing the agent. The embedder is selected via a generic factory:

```go
// embedder selection — generic, not Ollama-specific
embedder := retrieval.NewEmbedder(opts.EmbedderConfig)
// EmbedderConfig carries provider type + endpoint + model
// concrete impl selected at runtime (Ollama, local, stub for tests)
```

All archaeo services are instantiated from their respective stores in bootstrap, not deferred to `InitializeEnvironment`.

## Integration Points

### Where the pipeline is constructed

`named/euclo/agent.go` — `InitializeEnvironment` receives a fully-populated `EucloEnvironment` and passes it directly to `pretask.NewPipeline`:

```go
a.ContextPipeline = pretask.NewPipeline(a.Environment, pretask.DefaultPipelineConfig())
```

`NewPipeline` extracts what it needs from the environment. Nil fields degrade gracefully.

### Where current-turn files are passed to the pipeline

The host (relurpish) is responsible for parsing `@mentions` and file picker selections from the user's message **before** the pipeline runs. It writes these to state key `"context.current_turn_files"` prior to executing `ContextProposalPhase`. Euclo reads that key in `Execute`.

### Where the phase is inserted

`named/euclo/interaction/modes/chat.go` — `ContextProposalPhase` is the first phase before the main execution phase. It reads `"context.current_turn_files"` and `"context.session_pins"` from state and writes back confirmed results.

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

### Phase 1 — Environment pre-initialization + foundation
1. Define `EucloEnvironment` — resolve embedding vs extending `AgentEnvironment` (open question 1)
2. Define generic `retrieval.Embedder` interface and `EmbedderConfig` bootstrap factory
3. Move archaeo service instantiation into `BootstrapAgentRuntime` — all services pre-initialized
4. Create `named/euclo/runtime/pretask/` package: `types.go`, `anchor.go`, `merger.go`, `pipeline.go`
5. Implement `AnchorExtractor` — `CurrentTurnFiles` highest priority, session pins second, query symbols third
6. Implement `ResultMerger` with unit tests
7. Fix `ResolveRetrievalPolicy` code mode (one-line `WidenWhenNoLocal` fix)
8. All unit tests pass without any LLM or embedder

### Phase 2 — Retrieval stages
1. Implement `IndexRetriever` — structural expansion via AST dependency/call graph
2. Implement `ArchaeoRetriever` — `RetrieveTopic` and `RetrieveExpanded`
3. Implement `Pipeline.Run` stages 0, 1, 3 (hypothetical skipped — Grounded=false)
4. `NewPipeline(env EucloEnvironment, config)` wired to pre-initialized services
5. Integration tests against fixture workspace

### Phase 3 — Hypothetical generation
1. Implement `HypotheticalGenerator` using generic `Embedder`
2. Add Stage 2 to `Pipeline.Run`; wire skip threshold
3. Integration test demonstrating retrieval improvement vs anchor-only baseline

### Phase 4 — Interaction phase
1. Add `ContextProposalContent`, `ContextFileEntry`, `ContextKnowledgeEntry` to `interaction/content.go`
2. Implement `ContextProposalPhase` — reads `"context.current_turn_files"` from state
3. Wire into chat mode interaction machine as first phase
4. Wire confirmed files into `BuildContextRuntime` / `ProgressiveLoader`
5. Wire `KnowledgeEvidenceItems` into `SemanticInputBundle` in `work.go`
6. Relurpish: extend `euclo_renderer.go` to render `ContextProposalContent`
7. Relurpish: write `"context.current_turn_files"` to state from @mention + file picker parsing

### Phase 5 — Session persistence + observability
1. Session pin persistence across turns (`"context.session_pins"` state key)
2. `PipelineTrace` written to state for relurpish observability pane
3. `show_confirmation_frame: false` silent enrichment mode in manifest config
4. Relurpish: auto-confirm timeout (relurpish settings, not euclo manifest)

---

## Resolved Design Questions

1. **Embedder** — generic `retrieval.Embedder` interface, not tied to Ollama or any provider. Bootstrap selects the concrete implementation via `EmbedderConfig`. The pipeline does not reference any provider-specific type.

2. **Service initialization** — all services (IndexManager, archaeo, embedder, stores) are instantiated in `BootstrapAgentRuntime` before the agent loads. The euclo agent receives a fully-populated `EucloEnvironment`. `InitializeEnvironment` does not construct services.

3. **Auto-confirm timeout** — relurpish concern, not euclo's. Euclo emits the frame and awaits a response. Relurpish decides how long to wait before auto-proceeding and how to indicate that to the user. Configuration belongs in relurpish settings.

4. **Knowledge item rendering** — extend `euclo_renderer.go` in relurpish to render `ContextProposalContent`, which contains typed `ContextKnowledgeEntry` items. Euclo remains UI-agnostic; the content type carries all information needed for any renderer.

## Open Questions

1. **`EucloEnvironment` vs `AgentEnvironment`** — should `EucloEnvironment` extend `framework/agentenv.AgentEnvironment` (embedding it) or be a parallel type? Embedding is cleaner for the existing `InitializeEnvironment` interface; a parallel type avoids coupling euclo to the framework environment shape. Decide before Phase 4.

2. **`CurrentTurnFiles` delivery timing** — relurpish must write `"context.current_turn_files"` to state *before* the interaction machine executes `ContextProposalPhase`. Confirm this is possible given the current TUI message dispatch flow. If not, `ContextProposalPhase` may need to accept file selections as part of the frame response rather than pre-seeded state.

3. **Embedder for embedding the hypothetical sketch** — `HypotheticalGenerator` needs to embed its output text. If the embedder is async or slow, this adds latency. Consider whether embedding should be deferred: generate the text sketch synchronously, embed asynchronously, and use the text as a fallback retrieval query if the embedding isn't ready.
