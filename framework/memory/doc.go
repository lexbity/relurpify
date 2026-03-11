// Package memory provides hybrid in-memory and disk-backed storage for agent
// state, organised into session, project, and global scopes.
//
// # MemoryStore
//
// MemoryStore is the primary interface: Remember, Recall, Search, Forget, and
// Summarise. Implementations back storage with SQLite for durability across
// restarts.
//
// # Stores
//
//   - CheckpointStore: saves and restores graph execution checkpoints.
//   - MessageStore: persists the agent's conversation history.
//   - VectorStore: stores embeddings for semantic recall.
//   - WorkflowStateStore: persists pipeline stage results keyed by workflow ID.
//   - WorkflowStore: tracks top-level workflow metadata and status.
//
// # Code index
//
// code_index.go and index_store.go maintain a searchable index of workspace
// symbols, used by agents for fast cross-file navigation without loading every
// file into context.
package memory
