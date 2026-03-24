# Retrieval

## Overview

`framework/retrieval` is Relurpify's durable retrieval subsystem.

It sits between mutable source material and runtime/agent execution:

1. source material is ingested into durable document and chunk records
2. active chunk versions are indexed for sparse and dense search
3. queries are prefiltered by metadata before scoring
4. sparse and dense candidates are fused algorithmically
5. ranked evidence is packed into bounded `[]core.ContentBlock`
6. retrieval and packing decisions are logged for audit and telemetry

This package is intentionally separate from `framework/memory`.

- `framework/memory` owns runtime-facing memory APIs, workflow persistence, and compatibility stores
- `framework/retrieval` owns document identity, chunk lineage, indexing, retrieval fusion, context packing, and retrieval audit state

The two are integrated through shared SQLite databases and store adapters.

## Why It Exists

Retrieval is treated as a control system, not just a database query.

The essential idea is:

- preserve stable identities as content changes over time
- search broadly but only over active, policy-eligible material
- combine lexical and dense retrieval without score calibration
- inject only the evidence that fits the caller's context budget
- keep an audit trail of what was retrieved, injected, or excluded

This makes retrieval useful for:

- workflow history and knowledge lookup
- runtime memory recall
- retrieval-backed planning and coding prompts
- future graph/runtime retrieval nodes

## Current Architecture

The implementation in the repository today includes:

- SQLite schema and migrations for retrieval state
- structure-aware ingestion for Go, Markdown, and text
- stable document IDs, version IDs, chunk IDs, and chunk lineage
- append-only document version history
- append-only chunk-version history with active-lineage pointers
- sparse search with SQLite FTS5, with a fallback search table when FTS5 is unavailable
- dense search with exact cosine similarity over persisted embeddings
- metadata-first prefiltering
- hybrid retrieval with Reciprocal Rank Fusion
- bounded context packing with citations
- retrieval and packing audit persistence
- L1 exact-query cache
- L2 hot-result optimization validated against L3 full-corpus results
- maintenance operations for compaction, search rebuild, and embedding rebuild
- runtime wiring through `RetrieverService`
- workflow/runtime memory integration
- agent integration with preserved retrieval citations and prompt formatting

## Main Concepts

### Document

A document is the durable identity of an ingested source. It points to a canonical URI and tracks metadata such as:

- corpus scope
- source type
- parser/chunker versions
- policy tags
- source freshness
- ingestion freshness

Important distinction:

- `source_updated_at` means when the source content last changed
- `last_ingested_at` means when retrieval last processed the source

This separation matters because unchanged re-ingestion should not make stale content appear fresh to retrieval filters.

### Document Version

Each content revision creates an append-only document version row.

Document versions are keyed by stable document lineage plus content hash. Older versions are marked superseded when a newer active version exists.

### Chunk Lineage

Retrieval does not store only the latest chunk text.

Instead it keeps:

- `retrieval_chunks`: the stable logical chunk lineage row
- `retrieval_chunk_versions`: append-only chunk bodies per document version

`retrieval_chunks.active_version_id` points at the current active chunk version.

This gives the system:

- stable chunk identities across re-ingestion
- versioned chunk history
- the ability to compact stale physical rows later without losing lineage semantics

### Embeddings

Dense retrieval uses persisted embeddings keyed by:

- `chunk_id`
- `version_id`
- `model_id`

Embeddings are generated during ingestion when an embedder is configured, or later through embedding backfill/rebuild flows.

### Scope

Retrieval is scoped by `corpus_scope`.

Common examples:

- `workspace`
- `project`
- `session`
- `workflow:<workflowID>`

Scope is a first-class metadata filter and is applied before search scoring.

### Policy Tags

Policy tags are stored in two forms:

- `policy_tags_json` on `retrieval_documents` for convenient export/debugging
- normalized rows in `retrieval_document_policy_tags` for exact indexed filtering

The normalized table is the authoritative retrieval filter surface.

## Package Surface

Important files in `framework/retrieval`:

- `document.go`: durable document/chunk entities and stable IDs
- `schema.go`: SQLite schema and migrations
- `ingestion.go`: parse, chunk, persist, tombstone, and embedding backfill
- `index.go`: sparse and dense retrieval primitives
- `retriever.go`: metadata prefiltering, sparse+dense retrieval, RRF fusion
- `packer.go`: bounded evidence packing into `[]core.ContentBlock`
- `service.go`: runtime-facing service, cache tiers, audit persistence, telemetry
- `events.go`: retrieval/pacing event types and persistence
- `compaction.go`: maintenance and cleanup flows
- `embedder.go`: embedder interface

## Runtime-Facing Interface

The main runtime entry point is:

```go
type RetrieverService interface {
    Retrieve(ctx context.Context, q RetrievalQuery) ([]core.ContentBlock, RetrievalEvent, error)
}
```

Key `RetrievalQuery` fields:

- `Text`: query text
- `Scope`: corpus scope to search
- `SourceTypes`: optional source-type filter
- `AllowChunkIDs`: optional explicit allowlist
- `MaxTokens`: packing token budget
- `UpdatedAfter`: freshness filter based on `source_updated_at`
- `PolicyTags`: required policy tags
- `Limit`: final result limit after fusion

Important behavior:

- `Limit` is a final output limit, not a metadata prefilter cap
- prefiltering happens before scoring
- sparse and dense retrieval search more broadly than the final limit
- final evidence remains bounded by packing options

## Storage Model

Retrieval state lives in the same SQLite databases used by workflow and runtime stores.

Primary tables:

- `retrieval_documents`
- `retrieval_document_versions`
- `retrieval_chunks`
- `retrieval_chunk_versions`
- `retrieval_embeddings`
- `retrieval_document_policy_tags`
- `retrieval_chunks_fts`
- `retrieval_events`
- `retrieval_packing_events`

High-level responsibilities:

- `retrieval_documents`: canonical document identity and metadata
- `retrieval_document_versions`: append-only document revision history
- `retrieval_chunks`: stable chunk lineage and active pointers
- `retrieval_chunk_versions`: append-only versioned chunk text and offsets
- `retrieval_embeddings`: persisted dense vectors
- `retrieval_document_policy_tags`: normalized tags for exact filtering
- `retrieval_chunks_fts`: sparse search substrate
- `retrieval_events`: retrieval-cycle audit records
- `retrieval_packing_events`: packing audit records

## Ingestion Model

`IngestionPipeline` is responsible for turning source material into retrieval state.

Supported ingestion modes:

- ingest direct content through `Ingest`
- ingest local files through `IngestFile`
- backfill embeddings later through `BackfillEmbeddings`
- tombstone sources through `TombstoneDocument`

### Chunking Strategy

Chunking is structure-aware where possible:

- Go: top-level declarations
- Markdown: heading/section chunks
- Text: paragraph groups
- Fallback: line/token windows

Each chunk records:

- stable chunk ID
- document ID
- version ID
- source offsets
- structural key
- optional parent chunk

### Stable IDs

The ID rules are designed for lineage stability:

- `DocID` derives from canonical URI
- `VersionID` derives from document lineage plus content hash
- `ChunkID` derives from document lineage plus structural position

If structure stays stable across revisions, the logical chunk ID remains stable even though the chunk version changes.

### Idempotent Re-Ingestion

If a document is re-ingested with unchanged content:

- no new document version is created
- no new chunk-version rows are created
- metadata can still be updated
- `last_ingested_at` is refreshed
- `source_updated_at` is preserved

This is an intentional semantic guarantee.

## Retrieval Pipeline

The retrieval pipeline has five logical stages.

### 1. Metadata Prefilter

Prefiltering narrows candidate space before any scoring.

Current filters include:

- active chunk/version only
- scope
- source type
- explicit chunk allowlist
- `UpdatedAfter` against `source_updated_at`
- policy tags via normalized tag rows

This stage produces an allowlist of eligible chunk IDs.

### 2. Sparse Retrieval

Sparse retrieval searches active chunk text.

Current implementation:

- preferred backend: SQLite FTS5 with BM25 ranking
- fallback backend: ordinary table scan with `LIKE` matching and a simple term score

The FTS fallback keeps retrieval portable when SQLite is compiled without FTS5.

### 3. Dense Retrieval

Dense retrieval searches persisted embeddings.

Current implementation:

- exact cosine similarity over stored vectors
- constrained to active chunk versions
- constrained to the metadata prefilter allowlist

ANN is not currently used. The system prioritizes correctness and predictable behavior over approximate search complexity.

### 4. Hybrid Fusion

Sparse and dense candidates are combined with Reciprocal Rank Fusion.

Why RRF:

- it does not require calibrated sparse/dense scores
- it is deterministic
- it handles overlap naturally
- it is fully algorithmic

The final query `Limit` is applied after fusion, not before prefiltering or index search.

### 5. Context Packing

Ranked candidates are packed into bounded evidence blocks.

Current packing rules:

- dedupe repeated chunk IDs
- optionally stitch adjacent chunks from the same document/version
- enforce per-document caps
- respect explicit token budgets
- attach stable citations

Important current behavior:

- packed block order preserves fused retrieval rank
- adjacency merge does not reorder higher-ranked evidence behind lower-ranked documents

## Output Format

Packed retrieval output is emitted as `core.StructuredContentBlock`.

Current shape:

```go
core.StructuredContentBlock{
    Data: map[string]any{
        "type":      "retrieval_evidence",
        "text":      "...",
        "citations": []PackedCitation{...},
    },
}
```

`PackedCitation` includes:

- `doc_id`
- `chunk_id`
- `version_id`
- `canonical_uri`
- `source_type`
- `structural_key`
- `start_offset`
- `end_offset`

This citation payload is now the shared provenance contract across retrieval, runtime adapters, and agent prompt state.

## Cache and Tier Behavior

The retrieval service currently uses three conceptual tiers.

### L1: Exact Query Cache

An in-process exact cache stores:

- query hash
- corpus stamp
- packed blocks
- retrieval metadata

This is used for short-lived exact cache hits.

### L2: Hot Result Optimization

Recent packed chunk IDs from prior retrievals can be reused as a hot subset.

Important current behavior:

- hot results are an optimization, not a hard corpus restriction
- the service validates hot results against L3 full-corpus retrieval
- if the hot subset would hide a better full-corpus answer, the service falls back to L3

This avoids the correctness bug where recent evidence could suppress older but more relevant content.

### L3: Main Corpus

The full active retrieval corpus in SQLite.

This remains the authoritative retrieval source.

## Audit and Telemetry

Every non-cache-hit retrieval can persist:

- `RetrievalEvent`
- `PackingEvent`

These record:

- query ID and query text
- filter summary
- sparse, dense, and fused candidate counts
- cache tier
- excluded reasons
- injected chunks
- dropped chunks
- token budget and token usage

Current exclusion tracking includes both:

- retrieval-stage exclusions such as `retrieval:no_index_match` or `fusion:rank_cutoff`
- packing-stage exclusions such as `packing:budget_overflow`

This makes it possible to explain not just what was injected, but also what was filtered out and where.

Telemetry emission is optional and flows through `framework/core` telemetry events.

## Maintenance

`Maintenance` exposes physical cleanup and rebuild operations:

- `Compact`
- `RebuildSearchIndex`
- `RebuildEmbeddings`

Compaction removes physically stale rows while preserving active retrieval state.

Typical tasks:

- delete obsolete embeddings
- delete stale chunk versions
- delete tombstoned chunk lineages
- delete superseded document versions no longer referenced by active chunks
- rebuild sparse search rows
- rebuild embeddings for the active corpus

## Integration With Stores

### Runtime Memory Store

`SQLiteRuntimeMemoryStore` mirrors declarative records into retrieval.

Current behavior:

- retrieval schema is provisioned in the same SQLite database
- mirrored records become retrieval-searchable
- deletes tombstone the mirrored retrieval rows
- retrieval service is exposed through `RetrievalService()`

Dense retrieval can now be enabled explicitly by constructing the store with:

- `NewSQLiteRuntimeMemoryStoreWithRetrieval`

and supplying a configured `retrieval.Embedder`.

### Workflow State Store

`SQLiteWorkflowStateStore` mirrors:

- workflow artifacts
- step artifacts
- workflow knowledge

These are stored under workflow scopes like:

- `workflow:<workflowID>`

Dense retrieval can now be enabled explicitly through:

- `NewSQLiteWorkflowStateStoreWithRetrieval`

## Integration With Agents

Workflow-backed agent flows use retrieval today.

Current consumers include:

- planner workflow hydration
- architect workflow hydration
- pipeline workflow hydration

Workflow retrieval payloads now preserve provenance, not just text.

Current payload fields include:

- `query`
- `scope`
- `cache_tier`
- `query_id`
- `texts`
- `results`
- `summary`
- `result_size`
- `citation_count`

Each `results` entry contains:

- `text`
- `citations`

This means agent state and prompts can preserve:

- which document a snippet came from
- which chunk/version produced it
- where the source lives

## Prompt Presentation

Agent prompt renderers no longer dump workflow retrieval as raw JSON only.

Current prompt formatting surfaces:

- retrieval query
- scope
- cache tier
- short evidence snippets
- source references derived from citations

This keeps retrieval context readable for model consumers while preserving the structured state in memory/context.

## Dense Retrieval Configuration

The retrieval package itself is embedder-agnostic.

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    ModelID() string
    Dims() int
}
```

This allows:

- sparse-only retrieval when no embedder is configured
- hybrid retrieval when an embedder is available
- embedding rebuild when the model changes

The important operational detail is that stores must use the same embedder both for:

- indexing mirrored content
- retrieval service construction

Otherwise dense retrieval will be configured but embeddings will never be written.

## What Readers Should Remember

If you are new to this subsystem, the most important points are:

- retrieval is separate from memory, but tightly integrated with memory-backed stores
- the canonical retrieval unit is a versioned chunk lineage, not a single mutable row
- freshness means source freshness, not just ingestion freshness
- metadata filtering happens before search scoring
- hybrid retrieval is sparse + dense + RRF
- final limits are applied after fusion, not before prefiltering
- packed output is evidence with citations, not just text
- runtime and workflow stores can opt into dense retrieval by providing an embedder
- agent workflow retrieval now preserves provenance all the way into prompts

## Related Code

Useful entry points:

- `framework/retrieval/document.go`
- `framework/retrieval/schema.go`
- `framework/retrieval/ingestion.go`
- `framework/retrieval/retriever.go`
- `framework/retrieval/packer.go`
- `framework/retrieval/service.go`
- `framework/memory/db/sqlite_runtime_memory_store.go`
- `framework/memory/db/sqlite_workflow_state_store.go`
- `agents/workflow_retrieval.go`
