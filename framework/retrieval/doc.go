// Package retrieval provides the durable ingestion and retrieval foundation
// used to turn mutable workspace content into bounded evidence blocks for
// runtime graph execution.
//
// The package is intentionally separate from framework/memory. Memory remains
// the runtime-facing K/V and workflow persistence surface; retrieval owns
// document identity, chunk lineage, indexing, and evidence packing.
package retrieval
