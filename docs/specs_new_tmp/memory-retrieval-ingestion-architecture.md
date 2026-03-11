# Memory Retrieval and Ingestion Architecture

This document is a follow-on architecture plan for Relurpify's native memory retrieval, ingestion, and serving stack.

It is intentionally separate from `framework-graph-context-capability-storage-refactor-plan.md` so the framework runtime refactor can stay focused on:

- graph contracts
- checkpoint correctness
- memory-class boundaries
- workflow-storage consolidation
- runtime-facing retrieval interfaces

This document covers the companion subsystem that those runtime interfaces need in order to provide a high-quality memory system:

- ingestion
- parsing and chunking
- document and chunk identity
- versioning
- dense and sparse indexing
- reranking
- context packing
- cache tiers
- retrieval observability

## Goal

Build a native Relurpify retrieval-and-packing control system that sits between a mutable corpus and runtime graph execution.

This subsystem should:

- support structure-aware ingestion
- maintain stable document and chunk identities
- preserve append-only version history
- serve hybrid sparse+dense retrieval
- enforce metadata-first filtering
- support reranking and bounded context packing
- expose retrieval and packing telemetry
- integrate cleanly with framework runtime nodes and workflow storage

## Non-Goals

This document does not redefine:

- graph node contracts
- graph checkpoint semantics
- workflow-state storage ownership
- capability-policy evaluation

Those belong in the runtime/storage refactor plan.

## Mental Model

Memory here should be treated as a retrieval-and-packing control system, not just a vector store.

Core idea:

1. ingest source material into durable records and indexes
2. retrieve broadly but cheaply
3. rerank more narrowly
4. pack tightly into bounded evidence context
5. log what was retrieved versus what was actually injected

## Core Subsystems

Suggested decomposition:

- `IngestionPipeline`
- `DocumentRegistry`
- `MetadataStore`
- `SparseIndex`
- `DenseIndex`
- `Retriever`
- `Reranker`
- `ContextPacker`
- `CacheManager`
- `Compactor`

## Canonical Entities

The subsystem should define explicit durable entities for:

- `DocumentRecord`
- `DocumentVersionRecord`
- `ChunkRecord`
- `ChunkVersionRecord`
- `EmbeddingRecord`
- `RetrievalEvent`
- `PackingEvent`
- `CacheEntry`

## Storage Roles

The architecture should separate storage roles even if multiple roles share one backend in an early implementation.

### 1. Document Store

Purpose:

- raw content authority
- provenance
- source references

For Relurpify, this may be filesystem-backed rather than copied blob storage.

Expected contents:

- canonical path or URI
- content hash
- source type
- parser version
- provenance references

### 2. Metadata Store

Purpose:

- source-of-truth for retrievable identities and filterable fields

Expected contents:

- `doc_id`
- `chunk_id`
- version lineage
- timestamps
- ACL or policy metadata
- tags
- language
- source scope
- tombstones
- compaction markers

### 3. Sparse Index

Purpose:

- lexical retrieval
- exact term and identifier matching

Expected behavior:

- inverted postings
- BM25-style scoring
- efficient prefiltered candidate generation

### 4. Dense Index

Purpose:

- semantic retrieval

Expected behavior:

- embedding lookup by `chunk_id`
- nearest-neighbor candidate generation
- support for bounded broad-recall candidate retrieval

## Ingestion Design

### Structure-Aware Parsing and Chunking

Chunking should be structure-aware where possible.

Examples:

- markdown headings and sections
- code symbols and blocks
- document sections
- paragraph groups

Chunking should preserve:

- source offsets or spans
- parent-child relationships when useful
- adjacency relationships for later stitching

### Stable Document and Chunk Identifiers

The system should use durable logical identifiers, not transient positional IDs.

Required properties:

- stable `doc_id`
- stable `chunk_id` within a logical document lineage
- clear relationship between logical identity and versioned content

### Append-Only Versioning

Updates should be modeled as append-only version events rather than destructive overwrite.

The system should support:

- new document version records
- new chunk version records
- superseded markers
- logical deletion

### Batched Embedding Generation

Embedding generation should be treated as a pipeline stage, not a per-record incidental side effect.

Expected behavior:

- batch by source or queue
- support async backfill
- record embedding model/version metadata

### Logical Delete and Offline Compaction

Deletes and superseded data should first be logical.

Then background compaction can:

- remove stale index entries
- collapse duplicate versions
- reclaim storage from tombstoned chunks

## Retrieval Flow

Default retrieval should be staged.

### Stage 1: Metadata Prefilter

Before expensive retrieval:

- apply scope filters
- apply ACL and policy filters
- apply source/type filters
- apply freshness or version filters

### Stage 2: Hybrid Broad Retrieval

Run both:

- sparse retrieval
- dense retrieval

Then union the candidate set.

Baseline target:

- top 100 candidate union

### Stage 3: Reranking

Run a stronger ranking stage over the bounded candidate set.

Baseline target:

- rerank top 25

### Stage 4: Context Packing

Pack only the best evidence that fits budget.

Baseline target:

- pack top 6 to 10

Packing should support:

- chunk dedupe
- adjacency stitching
- per-source diversity caps
- explicit evidence token budget
- citation-first formatting

## Memory Tiers

The retrieval subsystem should expose serving tiers that runtime can target.

Recommended tiers:

- exact cache
- semantic cache
- hot recent store
- warm main corpus
- cold archive
- rolling conversation and entity summaries

These tiers should affect:

- retrieval order
- latency expectations
- storage cost
- eviction policy
- whether content is injected directly or referenced indirectly

## Observability

The subsystem should make retrieval and packing inspectable.

Minimum events:

- retrieved chunk IDs
- filter reasons
- sparse and dense candidate counts
- rerank scores
- dropped chunks during packing
- injected chunk IDs
- cache hits and misses

The key audit question is:

- what did the system retrieve
- what did it actually inject
- why were some candidates excluded

## Integration With Runtime

This subsystem should satisfy runtime-facing contracts without forcing one implementation into the graph layer.

Expected integration points:

- retrieval nodes query this subsystem through interfaces
- packed context can be returned as artifact refs or compact structured payloads
- retrieval and packing events can be persisted into workflow-state records when needed
- declarative and procedural retrieval remain capability-policy constrained

## Suggested Early Slice

Do not build the whole engine at once.

Start with:

1. canonical document and chunk records
2. metadata store
3. simple sparse retrieval
4. simple dense retrieval
5. candidate union
6. lightweight rerank
7. bounded context packer
8. retrieval event logging

That is enough to validate the architecture before building more advanced cache and compaction behavior.
