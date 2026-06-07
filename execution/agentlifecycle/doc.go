// Package agentlifecycle manages runtime agent execution lifecycle state.
//
// This package owns workflow, run, delegation, lineage, and runtime event
// lifecycle management. It is domain-first: the record types and business
// logic live here, while persistence is delegated through an adapter interface.
//
// Ownership boundaries:
// - Workflow, run, and delegation records belong to agentlifecycle
// - Runtime event logging belongs to agentlifecycle
// - Lineage bindings used by bridges belong to agentlifecycle
// - Compiler state (compilation records, cache, replay metadata) belongs to framework/compiler
// - Knowledge/retrieval internals belong to their respective packages
//
// The package depends on framework/persistence for the adapter interface
// and framework/graphdb only through the adapter, not directly in domain logic.
//
// This is NOT a general-purpose workflow database. It is specifically for
// agent runtime lifecycle state related to agentgraph and runtime orchestration.
package agentlifecycle
