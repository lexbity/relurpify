# Memory Retrieval and Ingestion Architecture

This document defines the architecture for `framework/retrieval` — Relurpify's native retrieval-and-packing control system.

It is intentionally separate from the framework runtime refactor (graph contracts, checkpoint semantics, workflow-storage consolidation) so those concerns stay focused. This document covers the companion subsystem that runtime interfaces need:

- ingestion, parsing, and chunking
- document and chunk identity and versioning
- hybrid sparse and dense indexing
- candidate fusion and scoring
- bounded context packing
- cache tiers
- retrieval observability

## Goal

Build a platform-agnostic retrieval-and-packing control system in `framework/retrieval` that sits between a mutable corpus and runtime graph execution.

The system should:

- support structure-aware ingestion
- maintain stable document and chunk identities
- preserve append-only version history
- serve hybrid sparse and dense retrieval with algorithmic score fusion
- enforce metadata-first filtering
- pack retrieved evidence into bounded `[]ContentBlock` payloads
- expose retrieval and packing telemetry
- integrate cleanly with framework runtime nodes and workflow storage

## Non-Goals

This document does not redefine:

- graph node contracts
- graph checkpoint semantics
- workflow-state storage ownership
- capability-policy evaluation

Those belong in the runtime/storage refactor plan.

`framework/memory` is not replaced. It retains its simple K/V MemoryStore (session/project/global scopes) and existing workflow/checkpoint/message stores. `framework/retrieval` is a new, separate package.

## Mental Model

Retrieval is a control problem, not a database query.

Core idea:

1. ingest source material into durable records and indexes
2. prefilter candidates by metadata before any scoring
3. retrieve broadly and cheaply via hybrid sparse and dense search
4. fuse scores algorithmically into a ranked candidate set
5. pack tightly into a bounded evidence context
6. log what was retrieved versus what was actually injected

## Package Layout

```
framework/retrieval/
  ingestion.go     -- IngestionPipeline
  document.go      -- DocumentRecord, ChunkRecord, stable IDs, versioning
  embedder.go      -- Embedder interface + Go-native implementation
  index.go         -- SparseIndex (SQLite FTS5), DenseIndex interface
  retriever.go     -- Retriever: metadata prefilter, hybrid retrieval, RRF fusion
  packer.go        -- ContextPacker: budget, dedup, adjacency, ContentBlock output
  schema.go        -- SQLite migrations for retrieval tables
  events.go        -- RetrievalEvent, PackingEvent
  compaction.go    -- background compaction goroutine
```

## Core Subsystems

Four named subsystems with clear responsibilities:

- **`IngestionPipeline`** — parse, chunk, embed, and persist source material
- **`Retriever`** — metadata prefilter, hybrid sparse+dense retrieval, RRF score fusion
- **`ContextPacker`** — budget-bounded packing of candidates into `[]ContentBlock`
- **`Compactor`** — background goroutine for logical-delete cleanup and index compaction

`SparseIndex` and `DenseIndex` are implementation concerns internal to `Retriever`, not top-level subsystems. `CacheManager` is a struct inside `Retriever`, not a separate package.

## Canonical Entities

```go
// DocumentRecord is the durable identity of an ingested source.
// The filesystem is the authoritative content store for local sources;
// this record carries identity, provenance, and hash — not a blob copy.
type DocumentRecord struct {
    DocID       string    // stable logical identifier
    CanonicalURI string   // file path or URI
    ContentHash  string   // SHA-256 of source content
    SourceType   string   // "go", "markdown", "text", etc.
    ParserVersion string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// DocumentVersionRecord records each ingested revision.
type DocumentVersionRecord struct {
    DocID       string
    VersionID   string
    ContentHash string
    IngestedAt  time.Time
    Superseded  bool
}

// ChunkRecord is a stable logical subdivision of a document.
type ChunkRecord struct {
    ChunkID     string    // stable within a document lineage
    DocID       string
    VersionID   string
    Text        string
    StartOffset int
    EndOffset   int
    ParentChunk string    // optional, for hierarchical chunks
    Tombstoned  bool
    CreatedAt   time.Time
}

// EmbeddingRecord associates an embedding vector with a chunk and model version.
type EmbeddingRecord struct {
    ChunkID      string
    ModelID      string   // e.g. "relurpify-minilm-v1"
    Vector       []float32
    GeneratedAt  time.Time
}

// RetrievalEvent records what the retriever found and why candidates were excluded.
type RetrievalEvent struct {
    QueryID        string
    Query          string
    FilterSummary  string
    SparseCandidates  int
    DenseCandidates   int
    FusedCandidates   int
    ExcludedReasons   map[string]string // chunkID -> reason
    Timestamp      time.Time
}

// PackingEvent records what was actually injected into context.
type PackingEvent struct {
    QueryID        string
    InjectedChunks []string  // chunk IDs
    DroppedChunks  []string  // chunk IDs and why
    TokenBudget    int
    TokensUsed     int
    Timestamp      time.Time
}
```

## Storage

All retrieval state is stored in the existing SQLite database alongside workflow stores. No separate storage backend is required.

New tables:

| Table | Purpose |
|---|---|
| `retrieval_documents` | DocumentRecord rows |
| `retrieval_document_versions` | DocumentVersionRecord rows |
| `retrieval_chunks` | ChunkRecord rows |
| `retrieval_embeddings` | EmbeddingRecord rows (vectors as BLOB) |
| `retrieval_chunks_fts` | SQLite FTS5 virtual table over chunk text |
| `retrieval_events` | RetrievalEvent rows |
| `retrieval_packing_events` | PackingEvent rows |

The filesystem remains the authoritative content store for local code and documents. `DocumentRecord` points to it via `CanonicalURI`; ingestion does not copy blobs.

## Embedder Interface

The `Embedder` interface is platform-agnostic. No LLM or inference server dependency.

```go
// Embedder produces dense vector representations of text.
// Implementations may be Go-native (e.g. compiled model weights)
// or delegate to an inference engine — the Retriever does not care.
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    ModelID() string
    Dims() int
}
```

The default implementation uses a Go-native embedding model (no external process, no Ollama dependency for ingestion). This keeps embedding available regardless of what inference backend is configured at runtime.

When a higher-quality or domain-specific embedding model is needed, a different `Embedder` implementation can be registered without changing the pipeline.

## Ingestion Design

### Structure-Aware Chunking

Chunking is structure-aware where the source type allows it:

- **Go source**: chunk by top-level declaration (func, type, const block)
- **Markdown**: chunk by heading and section
- **Plain text**: chunk by paragraph group with fixed token ceiling
- **Generic fallback**: fixed token windows with overlap

Chunks preserve:

- source offsets (`StartOffset`, `EndOffset`) for navigation
- parent-chunk reference for hierarchical sources
- adjacency relationships (implicit via offset ordering) for later stitching

### Stable Identifiers

`DocID` is derived from the canonical URI (e.g. deterministic hash of path). `ChunkID` is derived from `DocID` + structural position (e.g. symbol name for Go, heading path for Markdown, offset range for text). These must remain stable across re-ingestion of unchanged content.

### Append-Only Versioning

Updates are modeled as new `DocumentVersionRecord` and `ChunkRecord` rows, not destructive overwrites. The previous version is marked `Superseded = true`. Tombstoning is logical; physical removal happens during compaction.

### Batched Embedding Generation

Embedding is a pipeline stage, not a per-record side effect:

1. Ingest document → write `DocumentRecord`, `DocumentVersionRecord`, `ChunkRecord` rows
2. Enqueue chunk IDs for embedding
3. Batch embed (configurable batch size, async backfill supported)
4. Write `EmbeddingRecord` rows with `ModelID` and timestamp

This makes it straightforward to re-embed the corpus when the embedding model changes.

### Logical Delete and Compaction

Deletes set `Tombstoned = true` on `ChunkRecord` and exclude those chunks from FTS5 and vector queries. Physical removal is deferred to the `Compactor` background goroutine, which:

- removes stale FTS5 entries
- removes superseded `EmbeddingRecord` rows
- reclaims storage from tombstoned chunks
- runs on a configurable schedule (default: on idle)

## Retrieval Flow

Retrieval is three stages. There is no separate reranking stage — scoring is fully algorithmic.

### Stage 1: Metadata Prefilter

Before any index query:

- filter to active (non-tombstoned, non-superseded) chunks only
- apply scope filter (workspace, session, global)
- apply source-type filter if specified
- apply freshness filter if specified (e.g. `updated_after`)
- apply ACL/policy filter if set on the query

This is a cheap SQL query against `retrieval_chunks` and `retrieval_document_versions` metadata. It produces a candidate chunk ID allowlist that constrains the subsequent index queries.

### Stage 2: Hybrid Retrieval with RRF Fusion

Run both retrievers against the prefiltered allowlist:

- **Sparse**: FTS5 BM25 query returning top-N sparse candidates with scores
- **Dense**: ANN query against `retrieval_embeddings` returning top-N dense candidates with cosine scores

Union the two candidate sets and apply **Reciprocal Rank Fusion (RRF)**:

```
rrf_score(d) = Σ_r 1 / (k + rank_r(d))
```

RRF is parameter-free (default `k=60`), does not require calibrated scores across retrievers, and consistently outperforms naïve score averaging. It is fully algorithmic — no LLM call, no user-defined ranking function.

Baseline candidate targets:

- sparse top-50, dense top-50 → fused top-100

### Stage 3: Context Packing

Pack the top-ranked fused candidates that fit within the token budget into `[]ContentBlock`.

Packing rules:

- deduplicate by `ChunkID` (can appear in both sparse and dense results)
- stitch adjacent chunks from the same document when both are selected (merge into one block)
- enforce per-source diversity cap (default: max 3 chunks per document)
- respect explicit token budget (caller sets `MaxTokens`)
- format each block with citation metadata (`DocID`, `ChunkID`, URI, version)

Baseline packing target:

- top 6 to 10 chunks, subject to token budget

Output is `[]core.ContentBlock` — the first-class content type already used by agents and capability envelopes throughout the framework.

## Memory Tiers

Three tiers. Each affects retrieval order, latency, and eviction.

| Tier | Backend | Latency | Eviction |
|---|---|---|---|
| **L1 — exact cache** | In-process LRU (query hash → `[]ContentBlock`) | Sub-millisecond | LRU + TTL |
| **L2 — hot store** | SQLite (recent session chunks, last N hours) | < 10ms | TTL-based, configurable window |
| **L3 — main corpus** | SQLite (full ingested workspace material) | < 100ms | Logical delete + compaction |

The `Retriever` checks tiers in order: L1 hit returns immediately. L2 narrows the prefilter allowlist before querying L3. Rolling summaries (e.g. conversation digests) are just chunks with a `source_type = "summary"` tag — they live in L3 and are retrieved the same way.

No cold archive tier. If content is no longer needed it is tombstoned and compacted.

## Observability

`RetrievalEvent` and `PackingEvent` are written to SQLite on every non-cache-hit retrieval. They feed `framework/telemetry` as spans.

Minimum observable fields per retrieval cycle:

- query string and query ID
- filter summary (which filters were applied, how many candidates passed)
- sparse candidate count, dense candidate count, fused candidate count
- per-excluded-chunk reason (tombstoned, filtered, budget overflow, deduped)
- injected chunk IDs and token count
- L1/L2 cache hit/miss

The key audit questions are: what was retrieved, what was injected, and why were candidates excluded.

## Integration With Runtime

`framework/retrieval` exposes a clean interface that runtime nodes call without coupling to the SQLite implementation.

```go
// RetrieverService is the runtime-facing interface for retrieval.
type RetrieverService interface {
    Retrieve(ctx context.Context, q RetrievalQuery) ([]core.ContentBlock, RetrievalEvent, error)
}

// RetrievalQuery is the caller-facing query struct.
type RetrievalQuery struct {
    Text       string
    Scope      string        // "workspace", "session", "global"
    SourceTypes []string     // optional filter
    MaxTokens  int
    UpdatedAfter *time.Time  // optional freshness filter
}
```

Integration points:

- retrieval graph nodes call `RetrieverService.Retrieve` and receive `[]core.ContentBlock` directly
- `PackingEvent` and `RetrievalEvent` can be attached to workflow-state records when full auditability is needed
- declarative retrieval queries remain capability-policy constrained (the capability envelope wraps the result as with any tool output)
- `IngestionPipeline` can be triggered by graph nodes or by file-watch events outside the graph

## Early Implementation Slice

Do not build the whole system at once. Validate the architecture with:

1. SQLite schema migrations (`schema.go`)
2. `DocumentRecord` + `ChunkRecord` with stable ID derivation (`document.go`)
3. `IngestionPipeline`: parse → chunk → write records → batch embed → write embeddings (`ingestion.go`)
4. `SparseIndex` via FTS5 (`index.go`)
5. `DenseIndex` via in-process ANN over `EmbeddingRecord` rows (`index.go`)
6. `Retriever` with metadata prefilter and RRF fusion (`retriever.go`)
7. `ContextPacker` with budget + dedup + `[]ContentBlock` output (`packer.go`)
8. `RetrievalEvent` + `PackingEvent` logging (`events.go`)

L1/L2 cache tiers, append-only versioning, and background compaction follow once the core flow is validated end-to-end.

## Review Notes

This architecture is directionally correct and aligns well with the current framework split:

- `framework/retrieval` should remain separate from `framework/memory`
- SQLite is the correct initial storage backend because retrieval state needs to live beside workflow state
- `[]core.ContentBlock` is the right runtime-facing output type because it already integrates with capability envelopes and graph execution
- the staged rollout is correct; the system should be validated end-to-end before cache tiers and compaction are added

Several implementation details should be clarified before continuing:

### 1. Append-only chunk history needs an explicit lineage model

The current entity description says chunk updates are append-only, but a single `retrieval_chunks` row keyed only by stable `ChunkID` is not sufficient to preserve historical chunk versions. To satisfy the append-only requirement, one of these must become canonical:

- make chunk storage versioned by composite key (`ChunkID`, `VersionID`)
- introduce a separate chunk-version table and keep `ChunkID` as the logical lineage key

Until that is done, document versions can be append-only while chunk state remains latest-wins.

### 2. Prefilter metadata needs to be modeled, not just queried

The retrieval flow requires scope, freshness, source type, and optional policy filtering. That means the schema needs explicit metadata to support those filters cheaply and correctly. At minimum:

- scope or corpus namespace
- document update timestamp
- source type on documents or chunks
- optional policy/ACL tags or references

Without those fields, “metadata-first filtering” will collapse into post-filtering.

### 3. FTS5 should remain preferred, but not assumed universally

The design currently assumes SQLite FTS5 availability. In practice, runtime environments may not ship SQLite with FTS5 enabled. The architecture should define:

- FTS5 as the preferred sparse backend
- a supported fallback for environments where FTS5 is unavailable

This keeps retrieval portable without changing the retriever surface.

### 4. Dense retrieval should begin with correctness, not ANN complexity

The early slice mentions ANN. For initial validation, an in-process exact scorer over stored embeddings is enough. ANN should be introduced only after:

- embedding persistence is stable
- retrieval quality can be measured
- corpus size justifies the extra complexity

### 5. Packing needs a concrete citation payload contract

The document states that packed blocks include citation metadata, but it does not define the payload shape. That should be fixed early so runtime nodes, telemetry, and UI consumers do not invent conflicting formats.

Recommended citation fields:

- `doc_id`
- `chunk_id`
- `version_id`
- `canonical_uri`
- `source_type`
- `start_offset`
- `end_offset`

## Continuation Plan

The next implementation work should proceed in eight phases. Each phase has a narrow objective, explicit deliverables, and acceptance criteria.

### Phase 1: Stabilize Storage and Lineage

Objective:
Make the retrieval schema faithfully represent the architecture before more retrieval logic is built on top of it.

Deliverables:

- finalize canonical document, version, and chunk lineage tables
- add the metadata fields required for prefiltering
- define whether chunk history is stored via composite primary keys or a dedicated chunk-version table
- document the supported sparse-index fallback when FTS5 is unavailable
- add migration tests for fresh and upgraded databases

Acceptance criteria:

- re-ingesting changed content preserves document history without destructive overwrite
- chunk history is queryable for prior versions
- all fields required by metadata prefiltering exist in schema and are indexed
- schema tests pass against both a clean database and an already-initialized database

### Phase 2: Complete Ingestion Semantics

Objective:
Turn ingestion into a durable, idempotent pipeline rather than a basic write path.

Deliverables:

- make `IngestionPipeline` support idempotent re-ingestion for unchanged content
- separate record persistence from embedding backfill so ingestion can succeed when embedding is deferred
- add tombstone handling for removals
- support file-backed and direct-content ingestion entry points
- define parser/chunker version tracking rules

Acceptance criteria:

- unchanged content does not create redundant versions
- changed content creates a new document version and correct chunk lineage records
- embedding generation can be skipped and backfilled later without corrupting state
- deleting or removing a source marks its chunks inactive without physical deletion

### Phase 3: Implement Index Layer

Objective:
Provide the retriever with concrete sparse and dense search primitives.

Deliverables:

- build `index.go` with a sparse search interface over `retrieval_chunks_fts`
- add a dense search implementation over `retrieval_embeddings`
- start with exact vector scoring; defer ANN-specific optimization
- define candidate result structs shared by sparse and dense retrieval
- add index-layer tests for ranking behavior and inactive-row exclusion

Acceptance criteria:

- sparse search returns ranked chunk candidates for active rows only
- dense search returns ranked chunk candidates for active rows only
- index implementations can be constrained to a provided chunk allowlist
- all index tests pass without requiring runtime graph integration

### Phase 4: Add Metadata Prefiltering

Objective:
Make the retrieval flow begin with cheap SQL filtering before any index scoring.

Deliverables:

- implement prefilter query generation in `retriever.go` or a dedicated helper
- support filters for active rows, scope, source type, freshness, and policy metadata
- return a chunk allowlist plus diagnostic counts
- define the retrieval query contract that callers will use

Acceptance criteria:

- prefiltering narrows candidate space before sparse or dense search runs
- excluded rows are excluded for the correct reason
- the prefilter stage is independently testable from index scoring
- query inputs map cleanly onto indexed metadata columns

### Phase 5: Build Hybrid Retrieval and Score Fusion

Objective:
Implement the actual ranked candidate retrieval path.

Deliverables:

- add `Retriever` with sparse top-N and dense top-N retrieval
- implement Reciprocal Rank Fusion with deterministic ordering
- expose a ranked candidate type before packing
- define configurable top-N defaults and limits
- add tests covering overlap, sparse-only, dense-only, and fused cases

Acceptance criteria:

- the retriever produces a stable fused ranking for the same query and corpus
- duplicate chunk candidates from sparse and dense search are merged correctly
- fused results preserve provenance about which retrievers contributed
- retrieval remains fully algorithmic with no model call in ranking

### Phase 6: Build Context Packing

Objective:
Convert ranked retrieval candidates into bounded, citation-bearing runtime context.

Deliverables:

- add `packer.go` with budget-based packing
- deduplicate by `ChunkID`
- stitch adjacent chunks from the same document when beneficial
- enforce a configurable per-document diversity cap
- emit `[]core.ContentBlock` with a concrete citation payload format

Acceptance criteria:

- packed output respects `MaxTokens`
- citation metadata is stable and complete
- adjacency stitching reduces fragmentation without exceeding budget
- packing tests cover deduplication, diversity caps, and overflow behavior

### Phase 7: Add Observability and Runtime Integration

Objective:
Make retrieval auditable and callable from runtime nodes through a stable service interface.

Deliverables:

- implement `events.go` with persisted `RetrievalEvent` and `PackingEvent`
- define and expose `RetrieverService`
- attach retrieval diagnostics to workflow state where needed
- add telemetry emission hooks compatible with `framework/telemetry`
- add integration tests for retrieval-to-packing service calls

Acceptance criteria:

- every non-cache retrieval emits durable retrieval and packing records
- audit data answers: what was retrieved, what was injected, and what was excluded
- runtime callers can consume retrieval through the service interface without depending on SQLite internals
- telemetry output remains consistent with persisted event records

### Phase 8: Productionize with Maintenance and Performance Paths

Objective:
Add operational features only after the core retrieval path is correct.

Deliverables:

- implement `compaction.go`
- add embedding rebuild and reindex flows
- introduce L1 exact-cache behavior
- introduce L2 hot-store narrowing behavior
- add operational controls for retention, rebuild, and compaction cadence

Acceptance criteria:

- compaction removes stale physical data without changing active retrieval results
- cache hits do not change correctness, only latency
- rebuild flows can regenerate sparse and dense indexes from persisted canonical records
- operational tasks are testable or at least verifiable via deterministic integration checks

## Recommended Immediate Next Steps

The next coding cycle should focus on these items in order:

1. revise the schema to fully support append-only chunk lineage
2. add explicit prefilter metadata columns and indexes
3. implement `index.go` with sparse and exact dense retrieval
4. add metadata prefiltering and fused retrieval before attempting packer or cache work
