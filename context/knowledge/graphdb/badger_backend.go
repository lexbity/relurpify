package graphdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

// BadgerOptions controls the behaviour of the Badger-backed durable store.
type BadgerOptions struct {
	Dir      string
	InMemory bool
}

type badgerBackend struct {
	opts BadgerOptions
	db   *badger.DB
}

func newBadgerBackend(opts BadgerOptions) (*badgerBackend, error) {
	if opts.Dir == "" && !opts.InMemory {
		return nil, errors.New("graphdb: Badger dir is required unless InMemory is set")
	}
	var bopts badger.Options
	if opts.InMemory {
		bopts = badger.DefaultOptions("").WithInMemory(true)
	} else {
		if err := fs.MkdirAllSecure(opts.Dir); err != nil {
			return nil, err
		}
		bopts = badger.DefaultOptions(opts.Dir).WithValueDir(opts.Dir)
	}
	bopts = bopts.WithLogger(nil)

	db, err := badger.Open(bopts)
	if err != nil {
		return nil, err
	}
	return &badgerBackend{opts: opts, db: db}, nil
}

// ────────────────────────────────────────────────────────────────────
// backend implementation
// ────────────────────────────────────────────────────────────────────

func (b *badgerBackend) load(_ context.Context, store *adjacencyStore) error {
	// Under LRU we skip node/edge body hydration and build only the
	// label and source indexes (they're small ID lists, far smaller
	// than node bodies).  Reads lazy-load from Badger on cache miss.
	hydrateBodies := store.lruMaxCapacity <= 0

	return b.db.View(func(txn *badger.Txn) error {
		// ── Nodes ──
		nit := txn.NewIterator(badger.DefaultIteratorOptions)
		nodePrefix := keyPrefix(famNode)
		for nit.Seek(nodePrefix); nit.ValidForPrefix(nodePrefix); nit.Next() {
			item := nit.Item()
			if err := item.Value(func(val []byte) error {
				var node NodeRecord
				if err := json.Unmarshal(val, &node); err != nil {
					return err
				}
				if hydrateBodies {
					n := node
					store.nodes[node.ID] = &n
				}
				store.addNodeSourceIndex(node)
				store.addNodeLabels(node)
				return nil
			}); err != nil {
				nit.Close()
				return err
			}
		}
		nit.Close()

		// ── Edges (outgoing) — hydrate only when not under LRU.
		if hydrateBodies {
			eit := txn.NewIterator(badger.DefaultIteratorOptions)
			edgePrefix := keyPrefix(famEdgeOut)
			for eit.Seek(edgePrefix); eit.ValidForPrefix(edgePrefix); eit.Next() {
				item := eit.Item()
				if err := item.Value(func(val []byte) error {
					var edge EdgeRecord
					if err := json.Unmarshal(val, &edge); err != nil {
						return err
					}
					store.forward[edge.SourceID] = append(store.forward[edge.SourceID], cloneEdge(edge))
					store.reverse[edge.TargetID] = append(store.reverse[edge.TargetID], cloneEdge(edge))
					return nil
				}); err != nil {
					eit.Close()
					return err
				}
			}
			eit.Close()
		}

		return nil
	})
}

func (b *badgerBackend) commit(_ context.Context, batch mutationBatch) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return b.commitInTxn(txn, batch)
	})
}

func (b *badgerBackend) commitInTxn(txn *badger.Txn, batch mutationBatch) error {
	switch batch.opName {
	case "upsert_node":
		op, ok := batch.op.(nodeOp)
		if !ok {
			return errors.New("graphdb: invalid upsert_node payload")
		}
		if old, _ := b.getNodeRecordInTxn(txn, op.Node.ID); old != nil {
			deleteNodeIndexes(txn, *old)
			// Persist old version to history before overwriting.
			if err := b.putNodeHistory(txn, *old); err != nil {
				return err
			}
		}
		if err := b.putNodeRecord(txn, op.Node); err != nil {
			return err
		}
		putNodeIndexes(txn, op.Node)
		return nil

	case "upsert_nodes":
		op, ok := batch.op.(nodeBatchOp)
		if !ok {
			return errors.New("graphdb: invalid upsert_nodes payload")
		}
		for _, node := range op.Nodes {
			if old, _ := b.getNodeRecordInTxn(txn, node.ID); old != nil {
				deleteNodeIndexes(txn, *old)
				if err := b.putNodeHistory(txn, *old); err != nil {
					return err
				}
			}
			if err := b.putNodeRecord(txn, node); err != nil {
				return err
			}
			putNodeIndexes(txn, node)
		}
		return nil

	case "delete_node":
		op, ok := batch.op.(deleteNodeOp)
		if !ok {
			return errors.New("graphdb: invalid delete_node payload")
		}
		old, err := b.getNodeRecordInTxn(txn, op.ID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if old != nil {
			deleteNodeIndexes(txn, *old)
			old.DeletedAt = time.Now().UnixNano()
			if err := b.putNodeRecord(txn, *old); err != nil {
				return err
			}
			if err := b.markEdgesForNode(txn, op.ID, old.DeletedAt); err != nil {
				return err
			}
		}
		return nil

	case "delete_nodes":
		op, ok := batch.op.(deleteNodesOp)
		if !ok {
			return errors.New("graphdb: invalid delete_nodes payload")
		}
		now := time.Now().UnixNano()
		for _, id := range op.IDs {
			old, err := b.getNodeRecordInTxn(txn, id)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
			if old != nil {
				deleteNodeIndexes(txn, *old)
				old.DeletedAt = now
				if err := b.putNodeRecord(txn, *old); err != nil {
					return err
				}
				if err := b.markEdgesForNode(txn, id, now); err != nil {
					return err
				}
			}
		}
		return nil

	case "link_edge":
		op, ok := batch.op.(edgeOp)
		if !ok {
			return errors.New("graphdb: invalid link_edge payload")
		}
		if old, _ := b.getEdgeRecord(txn, op.Edge.SourceID, op.Edge.TargetID, op.Edge.Kind); old != nil {
			if err := b.putEdgeHistory(txn, *old); err != nil {
				return err
			}
		}
		if err := b.putEdgeRecord(txn, op.Edge); err != nil {
			return err
		}
		putEdgeIndexes(txn, op.Edge)
		return nil

	case "link_edges":
		op, ok := batch.op.(edgeBatchOp)
		if !ok {
			return errors.New("graphdb: invalid link_edges payload")
		}
		for _, edge := range op.Edges {
			if old, _ := b.getEdgeRecord(txn, edge.SourceID, edge.TargetID, edge.Kind); old != nil {
				if err := b.putEdgeHistory(txn, *old); err != nil {
					return err
				}
			}
			if err := b.putEdgeRecord(txn, edge); err != nil {
				return err
			}
			putEdgeIndexes(txn, edge)
		}
		return nil

	case "unlink_edge":
		op, ok := batch.op.(unlinkOp)
		if !ok {
			return errors.New("graphdb: invalid unlink_edge payload")
		}
		if op.Hard {
			// Read the existing edge to obtain its StableID for index cleanup.
			if existing, _ := b.getEdgeRecord(txn, op.SourceID, op.TargetID, op.Kind); existing != nil {
				deleteEdgeIndexes(txn, *existing)
			}
			return txn.Delete(keyEdgeOut(op.SourceID, op.Kind, op.TargetID))
		}
		return nil

	case "annotate_node":
		op, ok := batch.op.(annotateNodeOp)
		if !ok {
			return errors.New("graphdb: invalid annotate_node payload")
		}
		existing, err := b.getNodeRecordInTxn(txn, op.ID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if existing == nil {
			return nil
		}
		// Persist old version to history before modifying.
		if err := b.putNodeHistory(txn, *existing); err != nil {
			return err
		}
		deleteNodeIndexes(txn, *existing)
		merged, err := mergeJSONProps(existing.Props, op.Props)
		if err != nil {
			return err
		}
		existing.Props = merged
		if err := b.putNodeRecord(txn, *existing); err != nil {
			return err
		}
		putNodeIndexes(txn, *existing)
		return nil

	case "annotate_edge":
		op, ok := batch.op.(annotateEdgeOp)
		if !ok {
			return errors.New("graphdb: invalid annotate_edge payload")
		}
		existing, err := b.getEdgeRecord(txn, op.SourceID, op.TargetID, op.Kind)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if existing == nil {
			return nil
		}
		if err := b.putEdgeHistory(txn, *existing); err != nil {
			return err
		}
		merged, err := mergeJSONProps(existing.Props, op.Props)
		if err != nil {
			return err
		}
		existing.Props = merged
		return b.putEdgeRecord(txn, *existing)

	case "record_mutation_result":
		op, ok := batch.op.(mutationResultOp)
		if !ok {
			return errors.New("graphdb: invalid record_mutation_result payload")
		}
		return b.putMutationResult(txn, op.Result)

	default:
		return nil
	}
}

func (b *badgerBackend) snapshot(_ context.Context, _ snapshotState) error {
	return nil
}

func (b *badgerBackend) flush() error {
	return nil
}

func (b *badgerBackend) close() error {
	return b.db.Close()
}

func (b *badgerBackend) maintenance(_ context.Context, req MaintenanceRequest) (MaintenanceResult, error) {
	result := MaintenanceResult{Backend: "badger"}
	if req.ValueLogGC {
		// Run GC repeatedly until no files can be rewritten.
		for {
			if err := b.db.RunValueLogGC(0.5); err != nil {
				if errors.Is(err, badger.ErrNoRewrite) {
					break
				}
				return result, err
			}
			result.Reclaimed = true
		}
	}
	if req.IntegrityCheck {
		// Count canonical records as a lightweight integrity indicator.
		_ = b.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			for it.Seek([]byte{0}); it.Valid(); it.Next() {
				result.CheckedRecords++
			}
			return nil
		})
	}
	return result, nil
}

func (b *badgerBackend) backup(ctx context.Context, w io.Writer) error {
	_, err := b.db.Backup(w, 0)
	return err
}

// ────────────────────────────────────────────────────────────────────
// Index maintenance
// ────────────────────────────────────────────────────────────────────

// putNodeIndexes writes secondary index keys for the given node.  The
// keys hold an empty value (a single zero byte) so they consume minimal
// space while still being non‑empty for Badger.
func putNodeIndexes(txn *badger.Txn, node NodeRecord) {
	if node.ID == "" {
		return
	}
	idxVal := []byte{0}

	if node.Kind != "" {
		_ = txn.Set(keyNodeByKind(node.Kind, node.ID), idxVal)
	}
	if node.SourceID != "" {
		_ = txn.Set(keyNodeBySource(node.SourceID, node.ID), idxVal)
	}
	if node.StableID != "" {
		_ = txn.Set(keyNodeByStable(node.StableID, node.ID), idxVal)
	}
	for _, label := range uniqueLabels(node.Labels) {
		_ = txn.Set(keyNodeByLabel(label, node.ID), idxVal)
	}
	if path, hash, media := extractIndexedProps(node.Props); path != "" {
		_ = txn.Set(keyNodeByPath(path, node.ID), idxVal)
		if hash != "" {
			_ = txn.Set(keyNodeByHash(hash, node.ID), idxVal)
		}
		if media != "" {
			_ = txn.Set(keyNodeByMedia(media, node.ID), idxVal)
		}
	}
}

// deleteNodeIndexes removes all secondary index keys for the given node.
func deleteNodeIndexes(txn *badger.Txn, node NodeRecord) {
	if node.ID == "" {
		return
	}
	if node.Kind != "" {
		_ = txn.Delete(keyNodeByKind(node.Kind, node.ID))
	}
	if node.SourceID != "" {
		_ = txn.Delete(keyNodeBySource(node.SourceID, node.ID))
	}
	if node.StableID != "" {
		_ = txn.Delete(keyNodeByStable(node.StableID, node.ID))
	}
	for _, label := range uniqueLabels(node.Labels) {
		_ = txn.Delete(keyNodeByLabel(label, node.ID))
	}
	if path, hash, media := extractIndexedProps(node.Props); path != "" {
		_ = txn.Delete(keyNodeByPath(path, node.ID))
		if hash != "" {
			_ = txn.Delete(keyNodeByHash(hash, node.ID))
		}
		if media != "" {
			_ = txn.Delete(keyNodeByMedia(media, node.ID))
		}
	}
}

// putEdgeIndexes writes secondary index keys for the given edge.
func putEdgeIndexes(txn *badger.Txn, edge EdgeRecord) {
	if edge.StableID == "" {
		return
	}
	_ = txn.Set(keyEdgeByStable(edge.StableID, edge.SourceID, edge.TargetID, edge.Kind), []byte{0})
}

// deleteEdgeIndexes removes secondary index keys for the given edge.
func deleteEdgeIndexes(txn *badger.Txn, edge EdgeRecord) {
	if edge.StableID == "" {
		return
	}
	_ = txn.Delete(keyEdgeByStable(edge.StableID, edge.SourceID, edge.TargetID, edge.Kind))
}

// ────────────────────────────────────────────────────────────────────
// Props metadata extraction
// ────────────────────────────────────────────────────────────────────

// extractIndexedProps reads path, content_hash, and media_type from a
// node's Props JSON.  It silently tolerates missing or malformed JSON.
func extractIndexedProps(props json.RawMessage) (path, hash, media string) {
	if len(props) == 0 {
		return "", "", ""
	}
	var fields struct {
		Path        string `json:"path"`
		ContentHash string `json:"content_hash"`
		MediaType   string `json:"media_type"`
	}
	if err := json.Unmarshal(props, &fields); err != nil {
		return "", "", ""
	}
	return fields.Path, fields.ContentHash, fields.MediaType
}

// ────────────────────────────────────────────────────────────────────
// Index rebuild
// ────────────────────────────────────────────────────────────────────

// rebuildIndexes scans all canonical node and edge records and
// reconstructs secondary index keys.  Old index keys are not explicitly
// removed because rebuild replaces every indexable key.
func (b *badgerBackend) rebuildIndexes() error {
	return b.db.Update(func(txn *badger.Txn) error {
		// Nodes
		nit := txn.NewIterator(badger.DefaultIteratorOptions)
		nodePrefix := keyPrefix(famNode)
		for nit.Seek(nodePrefix); nit.ValidForPrefix(nodePrefix); nit.Next() {
			item := nit.Item()
			if err := item.Value(func(val []byte) error {
				var node NodeRecord
				if err := json.Unmarshal(val, &node); err != nil {
					return err
				}
				putNodeIndexes(txn, node)
				return nil
			}); err != nil {
				nit.Close()
				return err
			}
		}
		nit.Close()

		// Edges
		eit := txn.NewIterator(badger.DefaultIteratorOptions)
		edgePrefix := keyPrefix(famEdgeOut)
		for eit.Seek(edgePrefix); eit.ValidForPrefix(edgePrefix); eit.Next() {
			item := eit.Item()
			if err := item.Value(func(val []byte) error {
				var edge EdgeRecord
				if err := json.Unmarshal(val, &edge); err != nil {
					return err
				}
				putEdgeIndexes(txn, edge)
				return nil
			}); err != nil {
				eit.Close()
				return err
			}
		}
		eit.Close()

		return nil
	})
}

// ────────────────────────────────────────────────────────────────────
// internal helpers – read / write single records
// ────────────────────────────────────────────────────────────────────

func (b *badgerBackend) putNodeRecord(txn *badger.Txn, node NodeRecord) error {
	key := keyNodeByID(node.ID)
	val, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return txn.Set(key, val)
}

func (b *badgerBackend) getNodeRecordInTxn(txn *badger.Txn, id string) (*NodeRecord, error) {
	key := keyNodeByID(id)
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	var node NodeRecord
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &node)
	}); err != nil {
		return nil, err
	}
	return &node, nil
}

func (b *badgerBackend) putEdgeRecord(txn *badger.Txn, edge EdgeRecord) error {
	key := keyEdgeOut(edge.SourceID, edge.Kind, edge.TargetID)
	val, err := json.Marshal(edge)
	if err != nil {
		return err
	}
	return txn.Set(key, val)
}

func (b *badgerBackend) getEdgeRecord(txn *badger.Txn, sourceID, targetID string, kind EdgeKind) (*EdgeRecord, error) {
	key := keyEdgeOut(sourceID, kind, targetID)
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	var edge EdgeRecord
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &edge)
	}); err != nil {
		return nil, err
	}
	return &edge, nil
}

// markEdgesForNode sets DeletedAt on all edges (outgoing and incoming)
// that involve the given node ID. It scans the edge_out prefix for
// outgoing edges and then iterates all edges to find incoming ones.
func (b *badgerBackend) markEdgesForNode(txn *badger.Txn, nodeID string, deletedAt int64) error {
	// Outgoing edges: scan edge_out/{nodeID} prefix.
	outPrefix := keyPrefix(famEdgeOut, nodeID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(outPrefix); it.ValidForPrefix(outPrefix); it.Next() {
		item := it.Item()
		if err := item.Value(func(val []byte) error {
			var edge EdgeRecord
			if err := json.Unmarshal(val, &edge); err != nil {
				return err
			}
			edge.DeletedAt = deletedAt
			return b.putEdgeRecord(txn, edge)
		}); err != nil {
			return err
		}
	}
	it.Close()

	// Incoming edges: scan ALL edge_out keys looking for matches.
	// This is O(all_edges) but is only done during node deletion.
	allPrefix := keyPrefix(famEdgeOut)
	it2 := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it2.Close()
	for it2.Seek(allPrefix); it2.ValidForPrefix(allPrefix); it2.Next() {
		item := it2.Item()
		if err := item.Value(func(val []byte) error {
			var edge EdgeRecord
			if err := json.Unmarshal(val, &edge); err != nil {
				return err
			}
			if edge.TargetID == nodeID {
				edge.DeletedAt = deletedAt
				return b.putEdgeRecord(txn, edge)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (b *badgerBackend) putNodeHistory(txn *badger.Txn, node NodeRecord) error {
	// Use current time for the key to avoid collisions when multiple
	// history entries share the same UpdatedAt.
	key := keyNodeHistory(node.ID, time.Now().UnixNano(), node.StateVersion)
	val, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return txn.Set(key, val)
}

func (b *badgerBackend) putEdgeHistory(txn *badger.Txn, edge EdgeRecord) error {
	key := keyEdgeHistory(edge.SourceID, edge.Kind, edge.TargetID, time.Now().UnixNano(), uint64(edge.Weight))
	val, err := json.Marshal(edge)
	if err != nil {
		return err
	}
	return txn.Set(key, val)
}

// getNodeHistory reads all history entries for a node from Badger.
// History entries are keyed (entityID, timestamp, seq) and ordered by
// timestamp. Returns nil, nil when no history exists.
func (b *badgerBackend) getNodeHistory(id string) ([]NodeRecord, error) {
	if id == "" {
		return nil, nil
	}
	var history []NodeRecord
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := encodeKey(famHistoryNode, id)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var node NodeRecord
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &node)
			}); err != nil {
				return err
			}
			history = append(history, node)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return history, nil
}

// getEdgeHistory reads all history entries for an edge from Badger.
func (b *badgerBackend) getEdgeHistory(sourceID, targetID string, kind EdgeKind) ([]EdgeRecord, error) {
	if sourceID == "" || targetID == "" || kind == "" {
		return nil, nil
	}
	var history []EdgeRecord
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := encodeKey(famHistoryEdge, sourceID, string(kind), targetID)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var edge EdgeRecord
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &edge)
			}); err != nil {
				return err
			}
			history = append(history, edge)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return history, nil
}

// getMutationResult reads a single mutation result from Badger.
func (b *badgerBackend) getMutationResult(stableID string) (*MutationResult, error) {
	if stableID == "" {
		return nil, errors.New("stableID is empty")
	}
	key := keyMutationByStable(stableID)
	var result MutationResult
	if err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &result)
		})
	}); err != nil {
		return nil, err
	}
	if result.StableID == "" {
		return nil, errors.New("mutation result has empty StableID")
	}
	return &result, nil
}

// listMutationResults returns all stored mutation results from Badger.
func (b *badgerBackend) listMutationResults() ([]MutationResult, error) {
	var results []MutationResult
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := keyPrefix(famMutation)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var result MutationResult
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &result)
			}); err != nil {
				return err
			}
			results = append(results, result)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return results, nil
}

// getNodeRecord reads a single canonical node record from Badger.
func (b *badgerBackend) getNodeRecord(id string) (*NodeRecord, error) {
	if id == "" {
		return nil, errors.New("node id is empty")
	}
	key := keyNodeByID(id)
	var node NodeRecord
	if err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &node)
		})
	}); err != nil {
		return nil, err
	}
	if node.ID == "" {
		return nil, errors.New("node record has empty ID")
	}
	return &node, nil
}

// listNodeIDs returns all canonical node IDs from Badger.
func (b *badgerBackend) listNodeIDs() ([]string, error) {
	var ids []string
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := keyPrefix(famNode)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			segs, err := decodeKey(key)
			if err != nil || len(segs) < 2 {
				continue
			}
			ids = append(ids, segs[1])
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return ids, nil
}

// edgesBySource returns all outgoing edges for a source node from Badger.
func (b *badgerBackend) edgesBySource(sourceID string) ([]EdgeRecord, error) {
	if sourceID == "" {
		return nil, nil
	}
	var edges []EdgeRecord
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := encodeKey(famEdgeOut, sourceID)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var edge EdgeRecord
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &edge)
			}); err != nil {
				return err
			}
			edges = append(edges, edge)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return edges, nil
}

// edgesByTarget returns all incoming edges for a target node from Badger.
func (b *badgerBackend) edgesByTarget(targetID string) ([]EdgeRecord, error) {
	if targetID == "" {
		return nil, nil
	}
	// Incoming edges are found by scanning all edge_out records.
	var edges []EdgeRecord
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := keyPrefix(famEdgeOut)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var edge EdgeRecord
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &edge)
			}); err != nil {
				return err
			}
			if edge.TargetID == targetID {
				edges = append(edges, edge)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return edges, nil
}

// allEdges returns all edges from Badger organized by source ID.
func (b *badgerBackend) allEdges() (map[string][]EdgeRecord, error) {
	out := make(map[string][]EdgeRecord)
	if err := b.db.View(func(txn *badger.Txn) error {
		prefix := keyPrefix(famEdgeOut)
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var edge EdgeRecord
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &edge)
			}); err != nil {
				return err
			}
			out[edge.SourceID] = append(out[edge.SourceID], edge)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *badgerBackend) putMutationResult(txn *badger.Txn, result MutationResult) error {
	key := keyMutationByStable(result.StableID)
	val, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return txn.Set(key, val)
}
