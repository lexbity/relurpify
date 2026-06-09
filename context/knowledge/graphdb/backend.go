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
}

// mutationBatch carries a single logical mutation and its AOF op name. Future
// slices may batch multiple ops in a single commit.
type mutationBatch struct {
	opName string
	op     any
}
