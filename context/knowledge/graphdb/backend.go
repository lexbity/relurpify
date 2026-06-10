package graphdb

import (
	"context"
	"io"
)

// backend abstracts durable graph storage from the in‑memory graph engine.
// Implementations MUST be safe for concurrent commit calls but MAY assume
// that load is called at most once before any commit, snapshot, flush, or
// close call.
type backend interface {
	// load replays stored data into the adjacency store during engine
	// initialisation. It is called exactly once before any other method.
	load(ctx context.Context, store *adjacencyStore) error

	// commit durably persists a single mutation batch. The engine applies
	// the batch to memory only after commit succeeds.
	commit(ctx context.Context, batch mutationBatch) error

	// snapshot atomically replaces the durable snapshot with the given state
	// and truncates any incremental log. After a successful snapshot the
	// engine resets its dirty counter.
	snapshot(ctx context.Context, state snapshotState) error

	// flush forces any buffered durable state to stable storage.
	flush() error

	// close releases all backend resources. The engine must not call any
	// other method after close returns.
	close() error

	// maintenance runs the requested storage maintenance operations.
	maintenance(ctx context.Context, req MaintenanceRequest) (MaintenanceResult, error)

	// backup streams a portable snapshot of the graph data to w.
	backup(ctx context.Context, w io.Writer) error

	// getNodeHistory retrieves the revision history for a node from durable
	// storage. Returns nil, nil when no history exists for the node.
	getNodeHistory(id string) ([]NodeRecord, error)

	// getEdgeHistory retrieves the revision history for an edge from durable
	// storage. Returns nil, nil when no history exists for the edge.
	getEdgeHistory(sourceID, targetID string, kind EdgeKind) ([]EdgeRecord, error)

	// getMutationResult retrieves a stored mutation result by stable ID.
	// Returns nil, nil when not found.
	getMutationResult(stableID string) (*MutationResult, error)

	// listMutationResults returns all stored mutation results.
	listMutationResults() ([]MutationResult, error)

	// getNodeRecord retrieves a single canonical node record from durable
	// storage by ID. Returns nil, nil when the node does not exist.
	getNodeRecord(id string) (*NodeRecord, error)

	// listNodeIDs returns all active canonical node IDs from durable storage.
	listNodeIDs() ([]string, error)

	// edgesBySource returns all outgoing edges for a source node ID from
	// durable storage.
	edgesBySource(sourceID string) ([]EdgeRecord, error)

	// edgesByTarget returns all incoming edges for a target node ID from
	// durable storage.
	edgesByTarget(targetID string) ([]EdgeRecord, error)

	// allEdges returns all edges from durable storage, organized by source.
	allEdges() (map[string][]EdgeRecord, error)
}

// mutationBatch carries one or more logical mutations for a single atomic
// commit. All ops are applied in one Badger transaction and either all
// succeed or none do.
type mutationBatch struct {
	opName string
	op     any
	// ops holds additional ops when a batch carries multiple operations.
	// When non-nil, commit applies all of them atomically.
	ops []any
}

// singleOpBatch returns a mutationBatch for a single operation.
func singleOpBatch(name string, op any) mutationBatch {
	return mutationBatch{opName: name, op: op}
}


