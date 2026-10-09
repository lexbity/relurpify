package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ChunkKind is the closed vocabulary of chunk identities. Every persisted
// chunk carries exactly one kind, and the kind is part of its canonical ID.
type ChunkKind string

const (
	ChunkKindCapture     ChunkKind = "capture"
	ChunkKindTool        ChunkKind = "tool"
	ChunkKindObservation ChunkKind = "observation"
	ChunkKindLLM         ChunkKind = "llm"
	ChunkKindFile        ChunkKind = "file"
	ChunkKindDerivation  ChunkKind = "derivation"
)

// Valid reports whether the kind is a member of the canonical set.
func (k ChunkKind) Valid() bool {
	switch k {
	case ChunkKindCapture, ChunkKindTool, ChunkKindObservation,
		ChunkKindLLM, ChunkKindFile, ChunkKindDerivation:
		return true
	default:
		return false
	}
}

// CanonicalChunkID is the only chunk-ID constructor in the tree. The ID is
// content-addressed: chunk:<kind>:<hash16>, where hash16 is the first 16 hex
// characters of SHA-256 over the canonical content encoding. Provenance lives
// in edges, never in the ID, so identical content grounded by two runs
// converges on one chunk.
func CanonicalChunkID(kind ChunkKind, canonicalContent []byte) ChunkID {
	sum := sha256.Sum256(canonicalContent)
	return CanonicalChunkIDFromHash(kind, hex.EncodeToString(sum[:]))
}

// CanonicalChunkIDFromHash builds the canonical ID from an already-computed
// hex SHA-256 digest. It consumes the first 16 hex characters, so a full
// 64-character digest and a pre-truncated 16-character digest yield the same
// ID.
func CanonicalChunkIDFromHash(kind ChunkKind, digestHex string) ChunkID {
	if len(digestHex) > 16 {
		digestHex = digestHex[:16]
	}
	return ChunkID(fmt.Sprintf("chunk:%s:%s", kind, digestHex))
}

// CanonicalJSON encodes a value into a deterministic byte representation:
// object keys are always sorted, numbers keep their literal spelling, and no
// map-iteration order or struct-field order leaks into the result. It is the
// shared content encoder behind content-addressed IDs, so identical values
// hash equal across runs and processes.
func CanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}
