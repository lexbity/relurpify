# Bi-directional Knowledge Compiler (BKC) — Engineering Specification

## Status

Engineering specification. 

---

## 1. Problem Statement

The current system treats context as a linear, bounded tape. Knowledge accumulates
within a session — through archaeology findings, pattern confirmations, intent
grounding, and plan formation — but when the context window fills, the only
available recovery is a hard reset. The reset discards all inferred structure.

The living plan was designed to provide continuity across sessions, but has an
implementation gap: `VersionedLivingPlan` stores what was planned, not the
compiled knowledge that grounded the plan. A restored session receives the plan
skeleton but starts cold — it cannot reconstruct the reasoning, intent evidence,
or pattern relationships that made the plan meaningful. The session must rebuild
that context from scratch, which is expensive, incomplete, and often impossible
within a single context window.

The deeper problem is algorithmic: how do you stream knowledge to a
bounded-context system in an unlimited way, without hard resets?

The answer is a graph-based knowledge store with a budget-aware compilation
layer. The graph is unbounded. Any individual context window is bounded. The
missing piece is a principled mechanism to compile the right bounded slice from
the unbounded graph on demand — and to grow that graph continuously without
requiring full recomputation.

---

## 2. Core Concept

**Knowledge is a graph. Context is a compiled slice of that graph.**

A `KnowledgeChunk` is the atomic unit — a bounded, addressable, versioned,
provenance-preserving packet of inferred or confirmed knowledge. Chunks carry
multiple typed views and are never forced into a single ontology. A chunk about
a module's design intent is simultaneously interpretable as a pattern instance,
a design decision, a constraint on future changes, and a plan precondition. None
of those views is authoritative; all are derived projections.

The **forward pass** (compilation) takes findings — from archaeology exploration,
pattern confirmation, living plan formation, user interaction, and background
workspace scans — and compiles them into chunks with typed relationship edges.

The **backward pass** (streaming) takes a mode-specific seed and a context
budget, traverses the chunk graph in dependency order, and emits an ordered
chunk sequence for injection into the context manager. The session starts warm.

The graph grows continuously. Chunks become stale when code drifts; staleness
propagates through the edge graph; stale chunks surface as tensions for the
user to resolve. No hard reset is required at any point.

---

## 3. KnowledgeChunk Type

A `KnowledgeChunk` has two distinct layers.

### 3.1 Identity and Lifecycle Layer

```go
// ChunkID is a stable, content-addressed or UUID identifier.
type ChunkID string

// EdgeID is a stable identifier for a relationship edge.
type EdgeID string

// KnowledgeChunk is the atomic knowledge unit.
type KnowledgeChunk struct {
    ID            ChunkID         `json:"id"`
    Version       int             `json:"version"`
    WorkspaceID   string          `json:"workspace_id"`
    ContentHash   string          `json:"content_hash"`   // for drift detection
    TokenEstimate int             `json:"token_estimate"` // bounded cost for streaming
    Provenance    ChunkProvenance `json:"provenance"`
    Freshness     FreshnessState  `json:"freshness"`
    Body          ChunkBody       `json:"body"`
    Views         []ChunkView     `json:"views,omitempty"`
    CreatedAt     time.Time       `json:"created_at"`
    UpdatedAt     time.Time       `json:"updated_at"`
}

type FreshnessState string

const (
    FreshnessValid      FreshnessState = "valid"
    FreshnessStale      FreshnessState = "stale"
    FreshnessInvalid    FreshnessState = "invalid"    // hard contradiction detected
    FreshnessUnverified FreshnessState = "unverified" // produced without user confirmation
)

type ChunkProvenance struct {
    Sources      []ProvenanceSource `json:"sources"`
    SessionID    string             `json:"session_id,omitempty"`
    WorkflowID   string             `json:"workflow_id,omitempty"`
    CodeStateRef string             `json:"code_state_ref,omitempty"` // git commit SHA
    CompiledBy   CompilerPath       `json:"compiled_by"`
    Timestamp    time.Time          `json:"timestamp"`
}

// CompilerPath distinguishes how a chunk was produced. Deterministic chunks
// are auto-committed; LLM-assisted chunks require user confirmation.
type CompilerPath string

const (
    CompilerDeterministic CompilerPath = "deterministic" // no LLM — auto-committed
    CompilerLLMAssisted   CompilerPath = "llm_assisted"  // requires user confirmation
    CompilerUserDirect    CompilerPath = "user_direct"   // user stated this explicitly
)

type ProvenanceSource struct {
    Kind string `json:"kind"` // "exploration_finding" | "pattern_confirmation" |
                              // "learning_interaction" | "ast_index" | "plan_formation" |
                              // "user_statement" | "composition"
    Ref  string `json:"ref"`  // ID of the source artifact
}
```

### 3.2 Interpretation Layer

```go
// ChunkBody holds the raw knowledge payload. Raw is always present and
// readable without a schema. Fields is an optional structured overlay.
type ChunkBody struct {
    Raw    string         `json:"raw"`
    Fields map[string]any `json:"fields,omitempty"`
}

// ChunkView is a typed projection of the chunk body. Multiple views can
// coexist on the same chunk. None is authoritative.
type ChunkView struct {
    Kind ViewKind `json:"kind"`
    Data any      `json:"data"`
}

type ViewKind string

const (
    ViewKindPattern    ViewKind = "pattern"
    ViewKindDecision   ViewKind = "decision"
    ViewKindConstraint ViewKind = "constraint"
    ViewKindPlanStep   ViewKind = "plan_step"
    ViewKindAnchor     ViewKind = "anchor"
    ViewKindTension    ViewKind = "tension"
    ViewKindIntent     ViewKind = "intent"
)
```

Views are computed lazily on read by registered view renderers. New view kinds
can be registered without changing the chunk type. This is the "never forced
into one final ontology" property at the interpretation layer.

---

## 4. Edge Taxonomy

Relationship edges between chunks are typed but open-ended. The edge store
accepts any `EdgeKind` string; known kinds are defined as constants and have
documented semantics. Unknown kinds are stored and traversable but not
interpreted by the streaming or invalidation passes.

```go
type ChunkEdge struct {
    ID        EdgeID          `json:"id"`
    FromChunk ChunkID         `json:"from_chunk"`
    ToChunk   ChunkID         `json:"to_chunk,omitempty"` // empty for unary edges
    Kind      EdgeKind        `json:"kind"`
    Weight    float64         `json:"weight,omitempty"`   // 0.0–1.0, for relevance scoring
    Meta      map[string]any  `json:"meta,omitempty"`
    Provenance ChunkProvenance `json:"provenance"`
    CreatedAt time.Time       `json:"created_at"`
}

type EdgeKind string
```

### 4.1 Epistemic Edges

Describe how knowledge pieces relate semantically. Formed during compilation
when the LLM or deterministic compiler derives one chunk from another.

| Kind | Meaning |
|---|---|
| `grounds` | From provides evidence or basis for To |
| `contradicts` | From and To cannot both be valid (bidirectional in effect) |
| `refines` | To is a more precise version of From (same concept, more detail) |
| `generalizes` | From abstracts over To |
| `exemplifies` | From is a concrete instance of the concept in To |

### 4.2 Derivation Edges

Describe how a chunk was produced. Formed at compile time.

| Kind | Meaning |
|---|---|
| `derives_from` | From was compiled from To |
| `composed_of` | From is an aggregation; multiple To edges per From |
| `supersedes` | From replaces To after recompilation (versioning) |

### 4.3 Streaming Edges

Drive the backward pass. Determine load order and budget allocation. Formed
during compilation based on which chunks were required to derive the new chunk.

| Kind | Meaning |
|---|---|
| `requires_context` | To must be in context to correctly interpret From |
| `amplifies` | To adds useful precision to From but is not required |

`requires_context` edges form the DAG that the backward pass traverses
topologically. `amplifies` edges are optional enrichment loaded only when
budget remains after required dependencies are satisfied.

### 4.4 Validity Edges

Drive the invalidation pass. Connected to blast radius analysis and anchor
drift detection.

| Kind | Meaning |
|---|---|
| `invalidates` | If From becomes stale, To becomes stale (propagation) |
| `depends_on_code_state` | From is only valid at a specific code snapshot |

### 4.5 User-Grounded Edges

Produced by learning interactions and explicit user decisions. These carry the
highest confidence in the graph.

| Kind | Meaning |
|---|---|
| `confirmed` | Human confirmed From is valid (unary) |
| `rejected` | Human rejected From (unary) |
| `refined_by` | To is the human-refined successor of From |
| `deferred` | Human has not yet resolved From's validity (unary) |

---

## 5. Package Structure

BKC has a clean ownership split across two packages:

```
archaeo/chunks/                    # archaeo-owned: durable chunk state
    chunk.go                       # KnowledgeChunk, ChunkEdge, all core types
    graph.go                       # ChunkGraph: DAG traversal, subgraph extraction
    store.go                       # Persistence: framework/graphdb adapter
    view.go                        # ViewRenderer registry, lazy view computation
    staleness.go                   # FreshnessState transitions, propagation

archaeo/bkc/                   # euclo-owned: compilation and streaming execution
    compiler.go                    # Forward pass: findings → chunks + edges
    compiler_deterministic.go      # Deterministic compilation paths (no LLM)
    compiler_llm.go                # LLM-assisted compilation paths
    stream.go                      # Backward pass: seed + budget → ordered chunk list
    stream_modes.go                # Mode-specific seed resolution
    invalidation.go                # Blast radius events → chunk invalidation + tensions                  
    events.go                      # BKC event types for archaeo event bus
```

euclo/relurpicabilities/bkc/relurpic.go # euclo:bkc.* relurpic capability registrations

`archaeo/chunks/` depends on `framework/graphdb` for storage and `archaeo/domain`
for workspace and workflow identifiers.

`archaeo/bkc/` depends on `archaeo/chunks/`, `framework/contextmgr`,
`archaeo/domain`, `archaeo/tensions`, `archaeo/learning`

Neither package imports the other's internal logic. `archaeo/bkc/` uses
`archaeo/chunks/` as a store and graph client only.

---

## 6. Forward Pass — Compilation

The forward pass converts findings into chunks and writes them to the chunk
graph. It has two paths distinguished by `CompilerPath`.

### 6.1 Deterministic Path

Inputs that produce fully deterministic output require no LLM and are
auto-committed to the store without user confirmation:

- AST index entries from `framework/ast` (IndexManager output)
- Confirmed patterns from `framework/patterns` (already user-confirmed)
- Confirmed anchors from `framework/retrieval`
- Explicit user statements captured in learning interactions

These chunks receive `CompilerPath = CompilerDeterministic` and
`Freshness = FreshnessValid`.

### 6.2 LLM-Assisted Path

Inputs that require inference use the LLM path:

- Archaeology exploration findings (pattern proposals, gap inferences)
- Living plan formation outputs
- Chat mode knowledge derivation (the LLM derives a chunk from a question-answer pair)
- Long-running archaeology mode autonomous generation during plan execution

These chunks receive `CompilerPath = CompilerLLMAssisted` and initially
`Freshness = FreshnessUnverified`. They are surfaced to the user for
confirmation via the existing learning interaction model in `archaeo/learning`.
On confirmation they transition to `FreshnessValid`. On rejection they enter
the deferral system via `archaeo/deferred`.

### 6.3 Compilation Triggers

| Trigger | Source | Path |
|---|---|---|
| User confirms finding in archaeology explore UI | euclo / archaeo GraphQL | LLM-assisted → user confirmation |
| Living plan compilation (`euclo:archaeology.compile-plan`) | euclo planning mode | LLM-assisted + deterministic mix |
| Chat mode knowledge derivation | euclo chat mode | LLM-assisted |
| Long-running plan execution in archaeology mode | euclo archaeology mode | LLM-assisted, autonomous |
| Workspace bootstrap | ayenitd service on startup | Deterministic only |
| Timed workspace scan | ayenitd service periodic | Deterministic only |

The compiler is event-driven and does not poll. Triggers arrive as events on
the `archaeo/events` bus. The compiler subscribes and dispatches by event kind.

### 6.4 Edge Production During Compilation

When the compiler produces chunk A from chunk B:

- A `derives_from(A→B)` edge is written automatically
- If B was in the LLM prompt context when A was inferred, a `requires_context(A→B)` edge is written
- If A and B share a concept at different granularity, a `refines` or `generalizes` edge is written when the compiler can determine the relationship

Composition chunks produced by aggregating multiple findings write
`composed_of(A→[B,C,D])` edges.

---

## 7. Backward Pass — Context Streaming

The backward pass takes a seed and a context budget and emits an ordered
sequence of `KnowledgeChunk` values for injection into the context manager.

### 7.1 Seed Resolution by Mode

| Euclo Mode | Seed | Mechanism |
|---|---|---|
| `chat` | User-selected files | Find chunks with `depends_on_code_state` edges to those files |
| `planning` | `VersionedLivingPlan.RootChunkIDs` | Graph traversal from plan's root chunks |
| `archaeology` | Current `ExplorationSession` produced chunk set | Session's accumulated chunk IDs |
| `debug` | Files in scope + active tension refs | Chunks anchored to relevant code + tension-linked chunks |

In chat mode, the user's file selection is already a first-class operation in
the existing `pane_chat` sidebar. File selection directly seeds the backward
pass — any file that has been indexed and chunked contributes its anchored
chunks to the stream.

### 7.2 Traversal Algorithm

```
Given: seed_chunks []ChunkID, budget int (tokens)

1. Load all seed chunks. Add to work queue.
2. While work queue non-empty and accumulated tokens < budget:
   a. Dequeue next chunk C (priority: user-confirmed > deterministic > llm-unverified)
   b. If C.Freshness == FreshnessStale: skip; add to StaleDuringStream collection
   c. Add C to output sequence. Accumulate C.TokenEstimate.
   d. Find all edges from C where Kind == "requires_context"
   e. Enqueue targets not yet visited, ordered by edge Weight descending
3. After required deps exhausted and budget remains:
   a. Repeat step 2 using Kind == "amplifies" edges until budget exhausted
4. Return output sequence (ordered: dependencies before dependents)
   and StaleDuringStream []ChunkID
```

### 7.3 contextmgr Integration

`framework/contextmgr` currently operates at the message level. BKC sits above
it as a semantic compilation layer. The `ContextRequest` type gains a new field:

```go
// Added to framework/contextmgr.ContextRequest
ChunkSequence []chunks.KnowledgeChunk
```

The `ContextStrategy` interface gains an optional extension point:

```go
// ChunkLoader is an optional extension to ContextStrategy.
// When implemented, the strategy delegates initial context population to
// the BKC backward pass before applying message-level pruning.
type ChunkLoader interface {
    LoadChunks(task *core.Task, budget *core.ContextBudget) ([]chunks.KnowledgeChunk, error)
}
```

When `ChunkSequence` is non-empty, the context manager injects chunk content
ahead of message history in the order returned by the backward pass. Existing
pruning strategies operate within the remaining budget after chunk injection.
Strategies that do not implement `ChunkLoader` behave exactly as before.

---

## 8. Living Plan Anchoring

`archaeo/domain.VersionedLivingPlan` is extended with two new fields:

```go
// Added to VersionedLivingPlan in archaeo/domain/types.go
RootChunkIDs  []string `json:"root_chunk_ids,omitempty"`  // entry points into chunk graph
ChunkStateRef string   `json:"chunk_state_ref,omitempty"` // chunk graph snapshot at plan formation
```

`RootChunkIDs` are the direct outputs of the plan formation pass. They seed the
backward pass when restoring a planning session from this plan version.

`ChunkStateRef` identifies which version of the chunk graph this plan was
grounded on. This enables plan version comparison against current chunk state
and detection of plan versions that were grounded on now-stale knowledge.

`archaeo/plans.Service` gains two new methods:

```go
// AnchorChunks binds root chunk IDs to a plan version. Called at plan
// compilation complete by euclo:bkc.checkpoint.
func (s Service) AnchorChunks(ctx context.Context, workflowID string, version int,
    rootChunkIDs []string, chunkStateRef string) error

// ChunkSeedForVersion returns the root chunk IDs for backward-pass seeding
// when restoring from a specific plan version.
func (s Service) ChunkSeedForVersion(ctx context.Context, workflowID string,
    version int) ([]string, error)
```

---

## 9. Blast Radius → Invalidation

The invalidation pass connects git commit events to chunk staleness propagation.

### 9.1 Event Flow

```
git commit detected by ayenitd GitWatcherService
  → archaeo event: CodeRevisionChanged{NewRevision, AffectedPaths}
  → named/euclo/bkc/invalidation.go subscribes to CodeRevisionChanged
  → for each AffectedPath:
      query framework/graphdb blast radius for that path
      collect all code regions within blast radius
  → query archaeo/chunks store: find chunks with depends_on_code_state
    edges to any affected region
  → for each affected chunk:
      mark FreshnessStale
      propagate through invalidates edges (depth-limited, default 3)
      emit ChunkStaled event
  → ChunkStaled events → archaeo/tensions.Service: surface as tensions
```

### 9.2 Blast Radius Wiring

Blast radius analysis spans `framework/graphdb` (structural relationships),
`archaeo` (semantic relationships), and `named/euclo` (execution policy). The
BKC invalidation pass consumes the blast radius output as a set of affected
code regions. It does not drive recomputation — recompilation of stale chunks
is deferred to the next compilation trigger event.

### 9.3 Tension Surfacing

Stale chunks surface as tensions via `archaeo/tensions.Service.Infer` with
`SourceRef` pointing to the stale `ChunkID`. The initial tension status is
`TensionInferred`. The user can:

- **Resolve**: trigger recompilation (new `euclo:bkc.compile` event)
- **Accept**: mark the chunk as still valid despite code change (explicit waiver)
- **Defer**: leave as open tension for later

Chunks with no user-grounded edges (pure LLM inference, `FreshnessUnverified`)
that become stale are escalated to `TensionStatus = TensionConfirmed`
automatically. Unverified stale knowledge should not require user action to
clear from context — it is excluded from the backward pass until recompiled.

---

## 10. ayenitd Bootstrap and Scan Services

Two services register in `ayenitd.ServiceManager`:

### 10.1 WorkspaceBootstrapService

Registered as `"bkc.workspace_bootstrap"`. Runs once at workspace open.
Scans all files within the workspace's configured file scope
(`relurpify_cfg`) via `framework/ast` IndexManager, then runs the
deterministic compilation path to produce the initial chunk set.

```go
type WorkspaceBootstrapService struct {
    IndexManager *ast.IndexManager
    Compiler     *bkc.Compiler       // deterministic path only
    Config       workspacecfg.Config
    EventBus     archaeoevents.Bus
}

func (s *WorkspaceBootstrapService) Start(ctx context.Context) error
func (s *WorkspaceBootstrapService) Stop() error
```

Emits `BootstrapComplete` on the archaeo event bus when finished. Euclo's
context strategy subscribes to this event to know when chunk-enriched context
is available.

### 10.2 GitWatcherService

Registered as `"bkc.git_watcher"`. Runs continuously. Polls `git log` at a
short interval (configurable, default 30s) to detect new commits since the
last seen revision. On detection, emits `CodeRevisionChanged` to the archaeo
event bus.

```go
type GitWatcherService struct {
    WorkspaceRoot string
    EventBus      archaeoevents.Bus
    PollInterval  time.Duration
    LastRevision  string
}

func (s *GitWatcherService) Start(ctx context.Context) error
func (s *GitWatcherService) Stop() error
```

Git post-commit hook installation is explicitly out of scope for the initial
implementation. Polling is sufficient and requires no workspace-level
configuration by the end user.

---

## 11. Relurpic Capabilities

BKC registers four new relurpic capabilities in `named/euclo/relurpicabilities`.

```go
const (
    CapabilityBKCCompile    = "euclo:bkc.compile"
    CapabilityBKCStream     = "euclo:bkc.stream"
    CapabilityBKCInvalidate = "euclo:bkc.invalidate"
    CapabilityBKCCheckpoint = "euclo:bkc.checkpoint"
)
```

| Capability | Trigger | Effect | Mutability |
|---|---|---|---|
| `euclo:bkc.compile` | Finding or confirmation event | Forward pass; produces chunks and edges; routes to deterministic or LLM path | `policy_constrained_mutation` |
| `euclo:bkc.stream` | Session start or mode transition | Backward pass; returns ordered chunk sequence for contextmgr injection | `non_mutating` |
| `euclo:bkc.invalidate` | `CodeRevisionChanged` event | Invalidation pass; marks stale chunks; surfaces tensions | `non_mutating` |
| `euclo:bkc.checkpoint` | Plan compilation complete | Anchors root chunk IDs to the current `VersionedLivingPlan` version | `policy_constrained_mutation` |

All four are `ArchaeoAssociated: true` and `LLMDependent: false` except
`bkc.compile` which is `LLMDependent: true` on the LLM-assisted path.

---

## 12. Runtime State Keys

BKC introduces new euclo runtime state keys following existing `euclo.*`
conventions:

```
euclo.bkc.stream_result      # last backward pass output (chunk IDs, token total)
euclo.bkc.pending_chunks     # LLM-derived chunks awaiting user confirmation
euclo.bkc.stale_chunks       # chunks identified as stale in current session
euclo.bkc.checkpoint_ref     # current session's chunk graph snapshot reference
```

These are surfaced through euclo's runtime reporting infrastructure alongside
existing surfaces such as `euclo.archaeology_capability_runtime`.

---

## 13. Implementation Plan

Each phase delivers a complete, production-quality slice of the system. Phases
are ordered by dependency. No phase is a minimum viable product cut.

---

### Phase 1: `archaeo/chunks` — Chunk Store Foundation

**Goal**: Establish the durable chunk graph store backed by `framework/graphdb`.
All subsequent phases depend on this package. No LLM dependency. No euclo
dependency. No ayenitd dependency.

**Deliverables**:
- `archaeo/chunks/chunk.go` — all core types: `KnowledgeChunk`, `ChunkEdge`,
  `ChunkBody`, `ChunkView`, `ChunkProvenance`, `FreshnessState`, `EdgeKind`,
  `ViewKind`, `CompilerPath`, `ProvenanceSource`
- `archaeo/chunks/store.go` — `ChunkStore` backed by `framework/graphdb`:
  `Save`, `Load`, `LoadMany`, `Delete`, `SaveEdge`, `LoadEdge`,
  `LoadEdgesFrom`, `FindByCodeStateRef`, `FindByWorkspace`
- `archaeo/chunks/graph.go` — `ChunkGraph` client: subgraph extraction,
  topological sort of `requires_context` DAG, `amplifies` traversal,
  cycle detection and safe handling
- `archaeo/chunks/staleness.go` — `MarkStale`, `MarkInvalid`, `BulkMarkStale`,
  staleness propagation through `invalidates` edges with configurable depth limit
- `archaeo/chunks/view.go` — `ViewRendererRegistry`: `Register`, `RenderViews`
  (lazy, computed on read)

**Dependencies**: `framework/graphdb`, `archaeo/domain`

**Unit Tests** (`archaeo/chunks/store_test.go`, `archaeo/chunks/graph_test.go`,
`archaeo/chunks/staleness_test.go`):
- Chunk round-trip: `Save` → `Load` → all fields match including provenance
- Version increment: re-saving same logical chunk increments `Version`
- `SaveEdge` → `LoadEdgesFrom` → correct kind, weight, and provenance returned
- `FindByCodeStateRef`: returns only chunks matching the given code ref
- `FindByWorkspace`: scoped correctly; does not return chunks from other workspaces
- Subgraph extraction: given seed chunks and `requires_context` edges, correct complete set returned
- Topological sort: 5-node DAG with mixed depths — dependency order respected in output
- Cycle detection: malformed graph with cycle handled without panic or infinite loop
- `MarkStale`: single chunk transitions from `FreshnessValid` to `FreshnessStale`
- Staleness propagation: A stale → B stale via `A→B invalidates` edge
- Propagation depth limit: does not propagate beyond configured depth (default 3)
- `BulkMarkStale`: batch of chunk IDs all marked in one operation
- View renderer registry: registered renderer called on `RenderViews`; unknown kind returns empty list
- `Delete`: chunk no longer returned by `Load` after deletion; edges also removed

---

### Phase 2: ayenitd Bootstrap and Git Watcher Services

**Goal**: Cold start is handled. Before any LLM session, the workspace is
scanned and an initial deterministic chunk set is produced. Git commits trigger
invalidation events on the archaeo event bus.

**Deliverables**:
- `ayenitd/bkc_bootstrap.go` — `WorkspaceBootstrapService`: registered as
  `"bkc.workspace_bootstrap"`; scans workspace using `framework/ast`
  IndexManager; feeds output to deterministic compiler stub (compilation logic
  completed in Phase 3); emits `BootstrapComplete` archaeo event
- `ayenitd/bkc_git_watcher.go` — `GitWatcherService`: registered as
  `"bkc.git_watcher"`; polls `git log` at configured interval; emits
  `CodeRevisionChanged{NewRevision, AffectedPaths}` archaeo event

**Dependencies**: Phase 1 (`archaeo/chunks`), `framework/ast` (IndexManager),
`ayenitd.ServiceManager`, `archaeo/events`, `framework/workspacecfg`

**Unit Tests** (`ayenitd/bkc_bootstrap_test.go`, `ayenitd/bkc_git_watcher_test.go`):
- Bootstrap: `Start` completes → `BootstrapComplete` event emitted on bus
- Bootstrap: respects workspace file scope config; files outside scope not indexed
- Bootstrap: idempotent — running twice on same workspace state does not duplicate chunks
- Bootstrap: `Stop` cleanly cancels in-progress scan
- Git watcher: detects new commit → emits `CodeRevisionChanged` with correct
  new revision SHA and non-empty `AffectedPaths`
- Git watcher: does not emit event if no new commits since last poll
- Git watcher: `Stop` cleanly cancels the poll loop; no goroutine leak
- Service registration: both services register with `ServiceManager` and respond
  to `Start` and `Stop` lifecycle calls without error

---

### Phase 3: BKC Compiler — Deterministic Forward Pass

**Goal**: Confirmed findings from existing stores (patterns, anchors, AST index)
compile automatically into chunks without LLM involvement. Establishes the
compiler infrastructure and event subscription wiring that the LLM path
(Phase 7) builds on.

**Deliverables**:
- `named/euclo/bkc/compiler.go` — `Compiler` type with
  `Compile(ctx context.Context, input CompilerInput) (*CompileResult, error)`;
  dispatches to deterministic or LLM path based on input kind; returns produced
  chunk IDs and edge IDs
- `named/euclo/bkc/compiler_deterministic.go` — handlers:
  `fromPatternConfirmation`, `fromAnchorConfirmation`, `fromASTIndexEntry`,
  `fromUserStatement`; each produces a chunk, writes `derives_from` and
  `depends_on_code_state` edges; writes `requires_context` when source
  references another chunk already in store
- `named/euclo/bkc/events.go` — `CompilerInput` union type; `CompileResult` type;
  event kinds: `PatternConfirmed`, `AnchorConfirmed`, `IndexEntryProduced`
- Event subscription: compiler subscribes to those events on `archaeo/events`
  bus and invokes `Compile` for each

**Dependencies**: Phase 1 (`archaeo/chunks`), Phase 2 (event bus wiring),
`framework/patterns`, `framework/retrieval`, `framework/ast`, `archaeo/events`,
`archaeo/domain`

**Unit Tests** (`named/euclo/bkc/compiler_test.go`):
- Confirmed pattern input → chunk with body, `CompilerDeterministic`,
  `FreshnessValid`, provenance source kind `"pattern_confirmation"`
- Confirmed anchor input → chunk with `depends_on_code_state` edge to correct
  git commit SHA
- AST index entry input → chunk body contains expected structural content
- Two related confirmed patterns where one references the other →
  `requires_context` edge written between their chunks
- `CompileResult` contains all produced chunk IDs and edge IDs
- Re-compiling same source → new chunk version written; `supersedes` edge to
  prior chunk version
- Event subscription: `PatternConfirmed` event on bus → compiler invoked →
  chunk readable from store
- Unknown input kind → returns descriptive error, no partial state written

---

### Phase 4: BKC Backward Pass — Context Streaming

**Goal**: Mode-specific seed resolution and budget-aware chunk streaming.
Sessions start warm. Chat mode file selection seeds the stream immediately.
`framework/contextmgr` receives the `ChunkSequence` extension.

**Deliverables**:
- `named/euclo/bkc/stream.go` — `Streamer` type with
  `Stream(ctx context.Context, seed StreamSeed, budget int) (*StreamResult, error)`;
  implements the traversal algorithm from §7.2; returns `Chunks []KnowledgeChunk`
  and `StaleDuringStream []ChunkID`
- `named/euclo/bkc/stream_modes.go` — seed builders:
  `ChatSeed(files []string)`, `PlanningSeed(plan *domain.VersionedLivingPlan)`,
  `ArchaeologySeed(session *domain.ExplorationSession)`,
  `DebugSeed(files []string, tensionRefs []string)`
- `framework/contextmgr/context_policy_types.go` — add
  `ChunkSequence []chunks.KnowledgeChunk` to `ContextRequest`;
  add `ChunkLoader` interface
- `framework/contextmgr` strategy update — strategies check for `ChunkLoader`
  and prepend chunk content; token accounting subtracts chunk estimates before
  message-level pruning

**Dependencies**: Phase 1 (`archaeo/chunks`), Phase 3 (compiler producing
chunks to stream), `archaeo/domain`, `framework/contextmgr`, `framework/core`

**Unit Tests** (`named/euclo/bkc/stream_test.go`,
`framework/contextmgr/` additions):
- Chat seed: file A with indexed chunks → correct chunks returned in
  dependency order (dependencies before dependents)
- Planning seed: `VersionedLivingPlan` with `RootChunkIDs` → full subgraph
  within budget returned
- Budget respected: `TokenEstimate` sum does not exceed budget; no partial chunks
- Priority order: user-confirmed chunk loaded before deterministic, deterministic
  before unverified, when all in scope
- Stale chunk encountered: excluded from `Chunks`; added to `StaleDuringStream`
- `amplifies` edges: optional enrichment loaded only after all `requires_context`
  deps loaded and budget remains
- Empty seed: returns empty `Chunks` and empty `StaleDuringStream` without error
- `requires_context` cycle in graph: traversal completes without infinite loop
- contextmgr: strategy implementing `ChunkLoader` → chunk sequence prepended;
  pruning applies to remainder within reduced budget
- contextmgr: strategy not implementing `ChunkLoader` → existing behaviour unchanged
- contextmgr: chunks not reordered by pruning pass

---

### Phase 5: Living Plan Anchoring

**Goal**: `VersionedLivingPlan` becomes a first-class restoration point. Plan
versions carry their root chunk IDs. Restoring from any plan version seeds
the backward pass correctly, closing the session continuity gap.

**Deliverables**:
- `archaeo/domain/types.go` — add `RootChunkIDs []string` and
  `ChunkStateRef string` to `VersionedLivingPlan`
- `archaeo/plans/versioning.go` — extend `DraftVersionInput` with
  `RootChunkIDs` and `ChunkStateRef`; persist these fields through
  `DraftVersion`
- `archaeo/plans/service.go` — add `AnchorChunks` and `ChunkSeedForVersion`
  methods
- Wiring: `euclo:bkc.checkpoint` capability (stub in this phase, completed in
  Phase 7) calls `AnchorChunks` at plan compilation complete;
  `PlanningSeed` in `stream_modes.go` calls `ChunkSeedForVersion`

**Dependencies**: Phase 1 (`archaeo/chunks`), Phase 4 (backward pass seed
resolution), `archaeo/plans`, `archaeo/domain`

**Unit Tests** (`archaeo/plans/versioning_test.go` additions,
`archaeo/plans/service_test.go` additions):
- `DraftVersion` with `RootChunkIDs` set → fields persisted; `Load` returns them
- `AnchorChunks` on existing plan version → `RootChunkIDs` and `ChunkStateRef`
  updated correctly
- `AnchorChunks` on non-existent version → returns error; no state written
- `ChunkSeedForVersion` → returns correct `RootChunkIDs` for that version number
- `ChunkSeedForVersion` for a superseded version → still returns that version's
  chunk IDs (historical anchoring preserved)
- `ChunkSeedForVersion` for a version with no chunks anchored → returns empty
  slice, no error
- Planning mode backward pass: stream seeded from plan version `RootChunkIDs`
  → chunk set matches what was anchored at compilation time

---

### Phase 6: Blast Radius Invalidation Loop

**Goal**: Git commits trigger targeted chunk invalidation through the blast
radius mechanism. Stale chunks surface as tensions. The hard reset is replaced
by a principled staleness-and-recompile loop.

**Deliverables**:
- `named/euclo/bkc/invalidation.go` — `InvalidationPass`:
  subscribes to `CodeRevisionChanged`; calls `framework/graphdb` blast radius
  for each affected path; finds chunks via `depends_on_code_state` edges;
  calls `archaeo/chunks.BulkMarkStale`; propagates through `invalidates` edges;
  emits `ChunkStaled` events
- Tension surfacing: `ChunkStaled` handler calls `archaeo/tensions.Service`
  `Infer` with `SourceRef = ChunkID`; unverified stale chunks escalated to
  `TensionStatus = TensionConfirmed` automatically
- Stream integration: `StaleDuringStream` from Phase 4's `StreamResult`
  forwarded to tension service at stream time

**Dependencies**: Phase 1 (`archaeo/chunks`), Phase 2 (git watcher events),
Phase 4 (`StaleDuringStream` collection), `framework/graphdb` (blast radius
API), `archaeo/tensions`, `archaeo/events`

**Unit Tests** (`named/euclo/bkc/invalidation_test.go`):
- `CodeRevisionChanged` → blast radius queried for each affected path
- Blast radius output → chunks with `depends_on_code_state` to affected regions
  marked `FreshnessStale`
- Staleness propagation: chunk A stale → B stale via `invalidates` edge → tension
  inferred for B
- Propagation depth limit: staleness does not propagate beyond depth 3 by default;
  configurable
- Unverified stale chunk → tension status `TensionConfirmed` (no user action needed
  to exclude from context)
- Verified stale chunk → tension status `TensionInferred` (user action required)
- Tension `SourceRef` correctly points to stale `ChunkID`
- `StaleDuringStream`: stale chunks from stream result forwarded to tension
  service → tensions created with same rules as invalidation pass
- No blast radius result for a path → no chunks invalidated for that path

---

### Phase 7: LLM-Assisted Compilation and Relurpic Capabilities

**Goal**: The full LLM compilation path is operational. BKC relurpic
capabilities are registered. Archaeology explore UI and chat mode produce
LLM-derived chunks. Long-running archaeology mode autonomously generates
chunks during plan execution.

**Deliverables**:
- `named/euclo/bkc/compiler_llm.go` — LLM compilation path: constructs
  prompt from finding and supporting context; invokes LLM via euclo execution
  infrastructure; parses structured output into chunk body and view proposals;
  routes to `archaeo/learning` as a `LearningInteraction` for user confirmation
- Confirmation flow: `Confirmed` → `FreshnessValid`, committed to store;
  `Rejected` → `archaeo/deferred` deferral created; `Deferred` →
  `FreshnessUnverified` chunk persisted but excluded from backward pass
- Autonomous mode: during living plan execution (`euclo:archaeology.implement-plan`),
  `euclo:bkc.compile` called after each plan step completion; chunks enter the
  learning queue for asynchronous user review
- `named/euclo/bkc/relurpic.go` — registers all four BKC capabilities in
  `named/euclo/relurpicabilities` with correct `Descriptor` fields;
  `euclo:bkc.checkpoint` now calls `AnchorChunks` (completing Phase 5 wiring)

**Dependencies**: Phases 1–6, `named/euclo/relurpicabilities`,
`archaeo/learning`, `archaeo/deferred`, euclo execution infrastructure

**Unit Tests** (`named/euclo/bkc/compiler_llm_test.go`):
- LLM output parsed → chunk body, views, and provenance populated correctly
- LLM chunk routed to learning interaction; not in store until confirmed
- `Confirmed` event → chunk in store with `FreshnessValid`
- `Rejected` event → deferral record created; chunk not in store
- `Deferred` event → chunk in store with `FreshnessUnverified`; backward pass
  excludes it (lower priority; treated as stale for confirmation purposes)
- Capability descriptor: `euclo:bkc.compile` has correct mutability,
  `ArchaeoAssociated: true`, `LLMDependent: true`
- Capability descriptor: `euclo:bkc.stream` has `MutabilityNonMutating`
- Capability descriptor: `euclo:bkc.checkpoint` calls `AnchorChunks` and
  updates `VersionedLivingPlan.RootChunkIDs`
- Autonomous mode: plan step completion event → `euclo:bkc.compile` invoked →
  chunk proposal in learning queue without blocking step execution

---

### Phase 8: contextmgr Semantic Layer — Full Integration

**Goal**: Euclo's context strategies fully delegate initial context population
to the BKC backward pass. Chunk-enriched context is the default for all euclo
modes that have a seed available.

**Deliverables**:
- Euclo `ContextStrategy` implementations implement `ChunkLoader` by calling
  `Streamer.Stream` with the mode-appropriate seed from `stream_modes.go`
- Token accounting: chunk `TokenEstimate` values subtracted from context budget
  before message-level pruning; pruning does not touch chunk segments
- `BootstrapComplete` subscription: euclo's context strategy waits for
  `BootstrapComplete` before first `LoadChunks` call; falls back gracefully
  if bootstrap is not yet complete
- Chunk segments in context are labeled with their `ChunkID` for traceability
  in context introspection tools

**Dependencies**: Phases 1–7 (all), `framework/contextmgr`, `framework/core`,
euclo `ContextStrategy` implementations

**Unit Tests** (`framework/contextmgr/` additions, euclo strategy tests):
- Mode strategy with `ChunkLoader`: chunk sequence prepended to context output
- Mode strategy: tokens consumed by chunks reduce available budget for messages
- Stale chunk from stream: not injected into context; tension emission confirmed
- Pre-bootstrap state: `LoadChunks` before `BootstrapComplete` returns empty
  sequence gracefully; context falls back to message-level selection
- All four euclo modes (`chat`, `planning`, `archaeology`, `debug`): correct
  seed type resolved and chunks injected with correct ordering
- Chunk segment labels preserved through context injection; introspectable

---

### Phase 9: Explore UI Confirmation Flow and End-to-End Autonomous Archaeology

**Goal**: The full bidirectional loop is operational end-to-end. Users confirm
or reject LLM-derived chunks from the explore UI and chat mode. Long-running
archaeology mode continuously and autonomously populates the chunk graph.
Session continuity across hard boundaries is validated.

**Deliverables**:
- `app/relurpish/tui/pane_archaeo.go` additions — expose pending learning
  interactions for LLM-derived chunks as a confirmable queue in the archaeology
  pane; confirm/reject/defer keybindings consistent with existing HITL patterns
- Euclo `archaeology` mode: on each exploration finding surfaced to the user,
  `euclo:bkc.compile` called; chunk proposal queued in learning interaction
- Euclo `chat` mode: when a question-answer pair grounds a new chunk
  (determined by system prompt instruction to the LLM), `euclo:bkc.compile`
  called with the derived knowledge; chunk enters confirmation queue
- Autonomous plan execution full wiring: `euclo:archaeology.implement-plan`
  calls `euclo:bkc.compile` after each step; `euclo:bkc.checkpoint` called
  at plan activation boundary; `VersionedLivingPlan.RootChunkIDs` frozen

**Dependencies**: Phases 1–8, `app/relurpish/tui`, `archaeo/learning`,
euclo `archaeology` and `chat` mode execution

**Unit Tests** and **Integration Scenarios**
(`app/relurpish/tui/pane_archaeo_test.go` additions,
`named/euclo/bkc/` end-to-end scenario tests):
- Archaeology finding surfaced → LLM chunk proposed → appears in confirmation queue in pane
- User confirms → chunk in store with `FreshnessValid`; removed from queue
- User rejects → deferral created; chunk removed from queue; excluded from backward pass
- Chat Q&A → chunk proposed → enters confirmation queue
- Autonomous plan step completion → chunk compiled without blocking step execution
- End-to-end session continuity: session 1 start → bootstrap chunks loaded →
  archaeology mode exploration → findings compiled → plan formed and anchored →
  session 1 end; session 2 start → planning seed from `VersionedLivingPlan.RootChunkIDs`
  → backward pass → warm context matching session 1's grounding knowledge
- Git commit during session → invalidation pass → stale tension visible in
  archaeology pane → user resolves → recompilation event → chunk refreshed →
  tension cleared
- Long-running plan execution: 10 plan steps → 10+ chunks produced →
  chunk graph grown → session 3 backward pass includes autonomously-produced chunks
