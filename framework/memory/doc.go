// Package memory provides hybrid in-memory and durable runtime storage for
// agent state, organised into session, project, and global scopes.
//
// The runtime distinguishes:
//   - working memory for short-lived graph coordination
//   - declarative memory for durable facts and decisions
//   - procedural memory for reusable routines and execution strategies
//
// # Runtime storage
//
// CheckpointSnapshotStore is the definitive resumable graph-checkpoint
// abstraction. The primary implementation is db.SQLiteCheckpointStore, which
// stores checkpoints alongside durable workflow state. CheckpointStore remains
// available as a file-based compatibility fallback for legacy checkpoints.
//
// WorkflowRuntimeStore and CompositeRuntimeStore expose the unified runtime
// surface that combines workflow state, runtime memory, and resumable
// checkpoints.
//
// WorkflowStore and FileWorkflowStore remain only as migration-layer adapters
// for older snapshot formats. New code should not depend on them.
//
// # MemoryStore
//
// MemoryStore is the primary interface: Remember, Recall, Search, Forget, and
// Summarise. Implementations back storage with SQLite for durability across
// restarts.
//
// # Stores
//
//   - CheckpointSnapshotStore: captures resumable graph execution checkpoints.
//   - CheckpointStore: file-based checkpoint compatibility implementation.
//   - MessageStore: persists the agent's conversation history.
//   - VectorStore: stores embeddings for semantic recall.
//   - WorkflowStateStore: persists pipeline stage results keyed by workflow ID.
//   - WorkflowStore: migration-only legacy workflow snapshot surface.
//
// # Code index
//
// code_index.go and index_store.go maintain a searchable index of workspace
// symbols, used by agents for fast cross-file navigation without loading every
// file into context.
package memory
