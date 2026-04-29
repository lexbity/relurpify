// Package contextdata defines the shared execution envelope contract for the framework.
//
// The context-streaming paradigm uses three tiers:
//
//  1. Streamed Context (read-only): Assembled by the compiler from knowledge chunks,
//     summaries, and checkpoint state. Represented by references to compiled chunks.
//
//  2. Working Memory (mutable): Per-turn, per-session in-memory state scoped by task ID.
//     Graph nodes may read and write working memory. Evicted at checkpoint boundaries.
//
//  3. Retrieval State (controlled read): Results from scatter-gather retrieval operations.
//     Graph nodes may trigger retrieval, but results are stored as references in the envelope.
//
// The Envelope is the execution context passed to graph nodes. It carries references to
// all three tiers without duplicating data. Branch clones copy working memory and
// references together. Branch merges union them without duplication.
//
// Handoff helpers support two common transfer modes:
//   - HandoffClone for the default cloned-envelope behavior
//   - HandoffSnapshot for paradigm-specific filtered envelopes
//
// Key invariants:
//   - Streamed context is read-only outside the compiler
//   - Working memory is the only mutable tier for graph nodes
//   - Checkpoints may be requested by nodes but are owned by the compiler
//   - References are used to avoid data duplication across branches
//   - Handoff policies should be derived from the source paradigm's semantics
package contextdata
