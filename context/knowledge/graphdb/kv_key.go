package graphdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ────────────────────────────────────────────────────────────────────
// Key families – each constant is the first segment of every key in
// that family.  This keeps prefix comparisons simple and deterministic.
// ────────────────────────────────────────────────────────────────────

const (
	famMeta        = "meta"
	famNode        = "node"
	famNodeKind    = "node_kind"
	famNodeSource  = "node_source"
	famNodeLabel   = "node_label"
	famNodePath    = "node_path"
	famNodeHash    = "node_hash"
	famNodeMedia   = "node_media"
	famNodeStable  = "node_stable"
	famEdgeOut     = "edge_out"
	famEdgeIn      = "edge_in"
	famEdgeStable  = "edge_stable"
	famHistoryNode = "history_node"
	famHistoryEdge = "history_edge"
	famMutation    = "mutation"
	famBatch       = "batch"
)

// ────────────────────────────────────────────────────────────────────
// Low‑level encoding
// ────────────────────────────────────────────────────────────────────

var errMalformedKey = errors.New("graphdb: malformed key")

// encodeKey joins zero or more string segments into a single binary‑
// safe key.  Each segment is length‑prefixed (4 byte little‑endian
// uint32) so that keys sort first by family, then by each segment in
// declaration order.  Empty segments are silently dropped because they
// cannot be round‑tripped unambiguously.
func encodeKey(segments ...string) []byte {
	var buf []byte
	for _, s := range segments {
		if s == "" {
			continue
		}
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(s)))
		buf = append(buf, s...)
	}
	return buf
}

// decodeKey reverses encodeKey.  It returns an error if the data is
// truncated or has trailing garbage.
func decodeKey(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	r := bytes.NewReader(data)
	var segs []string
	for r.Len() > 0 {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, fmt.Errorf("%w: %w", errMalformedKey, err)
		}
		n := binary.LittleEndian.Uint32(hdr[:])
		raw := make([]byte, n)
		if _, err := io.ReadFull(r, raw); err != nil {
			return nil, fmt.Errorf("%w: %w", errMalformedKey, err)
		}
		segs = append(segs, string(raw))
	}
	return segs, nil
}

// keyPrefix returns the encoded prefix for the given family.  It is a
// shortcut for encodeKey that documents the intent.
func keyPrefix(segments ...string) []byte {
	return encodeKey(segments...)
}

// ────────────────────────────────────────────────────────────────────
// Meta keys
// ────────────────────────────────────────────────────────────────────

func keySchemaVersion() []byte { return encodeKey(famMeta, "schema_version") }
func keyBackendID() []byte     { return encodeKey(famMeta, "backend_id") }

func keyMigration(name string) []byte {
	return encodeKey(famMeta, "migration", name)
}

// ────────────────────────────────────────────────────────────────────
// Node canonical records
// ────────────────────────────────────────────────────────────────────

func keyNodeByID(id string) []byte { return encodeKey(famNode, id) }

// ────────────────────────────────────────────────────────────────────
// Node secondary indexes
// ────────────────────────────────────────────────────────────────────

func keyNodeByKind(kind NodeKind, id string) []byte {
	return encodeKey(famNodeKind, string(kind), id)
}

func keyNodeBySource(sourceID, id string) []byte {
	return encodeKey(famNodeSource, sourceID, id)
}

func keyNodeByLabel(label, id string) []byte {
	return encodeKey(famNodeLabel, label, id)
}

func keyNodeByPath(path, id string) []byte {
	return encodeKey(famNodePath, path, id)
}

func keyNodeByHash(hash, id string) []byte {
	return encodeKey(famNodeHash, hash, id)
}

func keyNodeByMedia(mediaType, id string) []byte {
	return encodeKey(famNodeMedia, mediaType, id)
}

func keyNodeByStable(stableID, id string) []byte {
	return encodeKey(famNodeStable, stableID, id)
}

// ────────────────────────────────────────────────────────────────────
// Edge canonical records and indexes
// ────────────────────────────────────────────────────────────────────

func keyEdgeOut(sourceID string, kind EdgeKind, targetID string) []byte {
	return encodeKey(famEdgeOut, sourceID, string(kind), targetID)
}

func keyEdgeIn(targetID string, kind EdgeKind, sourceID string) []byte {
	return encodeKey(famEdgeIn, targetID, string(kind), sourceID)
}

func keyEdgeByStable(stableID, sourceID, targetID string, kind EdgeKind) []byte {
	return encodeKey(famEdgeStable, stableID, sourceID, string(kind), targetID)
}

// ────────────────────────────────────────────────────────────────────
// History
// ────────────────────────────────────────────────────────────────────

func keyNodeHistory(id string, timestamp int64, seq uint64) []byte {
	var buf []byte
	buf = append(buf, encodeKey(famHistoryNode, id)...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(timestamp))
	buf = binary.BigEndian.AppendUint64(buf, seq)
	return buf
}

func keyEdgeHistory(sourceID string, kind EdgeKind, targetID string, timestamp int64, seq uint64) []byte {
	var buf []byte
	buf = append(buf, encodeKey(famHistoryEdge, sourceID, string(kind), targetID)...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(timestamp))
	buf = binary.BigEndian.AppendUint64(buf, seq)
	return buf
}

// ────────────────────────────────────────────────────────────────────
// Mutation / batch
// ────────────────────────────────────────────────────────────────────

func keyMutationByStable(stableID string) []byte {
	return encodeKey(famMutation, stableID)
}

func keyBatchByID(batchID string) []byte {
	return encodeKey(famBatch, batchID)
}
