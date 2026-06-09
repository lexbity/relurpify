package graphdb

import (
	"bytes"
	"math"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

// ────────────────────────────────────────────────────────────────────
// encodeKey / decodeKey round‑trip
// ────────────────────────────────────────────────────────────────────

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		segs []string
	}{
		{"single", []string{"node"}},
		{"two_parts", []string{"node", "abc123"}},
		{"three_parts", []string{"node_label", "tag:coverage", "n1"}},
		{"unicode", []string{"node", "日本語"}},
		{"spaces", []string{"node", "hello world"}},
		{"special_chars", []string{"node_path", "/src/main.go"}},
		{"with_colon", []string{"node_label", "media:image/png"}},
		{"with_dot", []string{"node", "file:sha256:abc.def"}},
		{"long_string", []string{
			"node", "a" + string(make([]byte, 4096)) + "z",
		}},
		{"minimal", []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := encodeKey(tc.segs...)
			decoded, err := decodeKey(encoded)
			require.NoError(t, err)
			require.Equal(t, tc.segs, decoded)
		})
	}
}

func TestEncodeDecodeRoundTrip_Quick(t *testing.T) {
	f := func(segs []string) bool {
		encoded := encodeKey(segs...)
		decoded, err := decodeKey(encoded)
		if err != nil {
			return false
		}
		// Empty segments are silently dropped by encodeKey.
		filtered := make([]string, 0, len(segs))
		for _, s := range segs {
			if s != "" {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return len(decoded) == 0
		}
		if len(decoded) != len(filtered) {
			return false
		}
		for i := range filtered {
			if decoded[i] != filtered[i] {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestEncodeDecodeEmpty(t *testing.T) {
	encoded := encodeKey()
	require.Empty(t, encoded)

	decoded, err := decodeKey(encoded)
	require.NoError(t, err)
	require.Empty(t, decoded)

	// All‑empty segments produce an empty key.
	encoded = encodeKey("", "", "")
	require.Empty(t, encoded)

	decoded, err = decodeKey(encoded)
	require.NoError(t, err)
	require.Empty(t, decoded)
}

func TestDecodeMalformed(t *testing.T) {
	// Truncated length header
	_, err := decodeKey([]byte{0x01})
	require.Error(t, err)
	require.ErrorIs(t, err, errMalformedKey)

	// Length says 5 but only 3 bytes remain
	_, err = decodeKey([]byte{0x05, 0x00, 0x00, 0x00, 'h', 'e', 'l'})
	require.Error(t, err)
	require.ErrorIs(t, err, errMalformedKey)

	// Negative‐looking length (very large uint32) – decode won't fail
	// on the header but will fail when trying to read that many bytes.
	_, err = decodeKey([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	require.Error(t, err)
}

func TestEncodeDeterministic(t *testing.T) {
	a := encodeKey("node", "abc")
	b := encodeKey("node", "abc")
	require.True(t, bytes.Equal(a, b), "encoding must be deterministic")
}

// ────────────────────────────────────────────────────────────────────
// Prefix ordering
// ────────────────────────────────────────────────────────────────────

func TestPrefixOrdering_SeparateFamilies(t *testing.T) {
	// "node" (len=4) must NOT match "node_kind" (len=9) as a prefix.
	nodeKey := encodeKey("node", "n1")
	nodePrefix := keyPrefix("node")

	require.True(t, bytes.HasPrefix(nodeKey, nodePrefix),
		"node/… should have prefix node")

	nodeKindKey := encodeKey("node_kind", "function", "n1")
	require.False(t, bytes.HasPrefix(nodeKindKey, nodePrefix),
		"node_kind/… should NOT have prefix node")
}

func TestPrefixOrdering_EdgeOutIn(t *testing.T) {
	outKey := encodeKey("edge_out", "src", "calls", "tgt")
	inKey := encodeKey("edge_in", "tgt", "calls", "src")

	outPrefix := keyPrefix("edge_out")
	inPrefix := keyPrefix("edge_in")

	require.True(t, bytes.HasPrefix(outKey, outPrefix))
	require.False(t, bytes.HasPrefix(outKey, inPrefix))
	require.True(t, bytes.HasPrefix(inKey, inPrefix))
	require.False(t, bytes.HasPrefix(inKey, outPrefix))
}

func TestPrefixOrdering_ScanBySource(t *testing.T) {
	srcA := encodeKey("node_source", "src-a", "n1")
	srcB := encodeKey("node_source", "src-b", "n1")
	srcAPrefix := keyPrefix("node_source", "src-a")

	require.True(t, bytes.HasPrefix(srcA, srcAPrefix))
	require.False(t, bytes.HasPrefix(srcB, srcAPrefix))
}

func TestPrefixOrdering_ScanByLabel(t *testing.T) {
	labelFoo := encodeKey("node_label", "tag:foo", "n1")
	labelFoobar := encodeKey("node_label", "tag:foobar", "n2")
	labelFooPrefix := keyPrefix("node_label", "tag:foo")

	require.True(t, bytes.HasPrefix(labelFoo, labelFooPrefix))
	require.False(t, bytes.HasPrefix(labelFoobar, labelFooPrefix),
		"tag:foobar should not match tag:foo prefix because lengths differ")
}

// ────────────────────────────────────────────────────────────────────
// Key builder functions
// ────────────────────────────────────────────────────────────────────

func TestKeyNodeByID(t *testing.T) {
	k := keyNodeByID("n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node", "n1"}, segs)
}

func TestKeyNodeByKind(t *testing.T) {
	k := keyNodeByKind("function", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_kind", "function", "n1"}, segs)
}

func TestKeyNodeBySource(t *testing.T) {
	k := keyNodeBySource("src/main.go", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_source", "src/main.go", "n1"}, segs)
}

func TestKeyNodeByLabel(t *testing.T) {
	k := keyNodeByLabel("tag:a", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_label", "tag:a", "n1"}, segs)
}

func TestKeyNodeByPath(t *testing.T) {
	k := keyNodeByPath("/src/main.go", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_path", "/src/main.go", "n1"}, segs)
}

func TestKeyNodeByHash(t *testing.T) {
	k := keyNodeByHash("sha256:abcdef", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_hash", "sha256:abcdef", "n1"}, segs)
}

func TestKeyNodeByMedia(t *testing.T) {
	k := keyNodeByMedia("image/png", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_media", "image/png", "n1"}, segs)
}

func TestKeyNodeByStable(t *testing.T) {
	k := keyNodeByStable("stable-1", "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_stable", "stable-1", "n1"}, segs)
}

func TestKeyEdgeOut(t *testing.T) {
	k := keyEdgeOut("src", "calls", "tgt")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"edge_out", "src", "calls", "tgt"}, segs)
}

func TestKeyEdgeIn(t *testing.T) {
	k := keyEdgeIn("tgt", "calls", "src")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"edge_in", "tgt", "calls", "src"}, segs)
}

func TestKeyEdgeByStable(t *testing.T) {
	k := keyEdgeByStable("edge-stable", "src", "tgt", "calls")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"edge_stable", "edge-stable", "src", "calls", "tgt"}, segs)
}

func TestKeySchemaVersion(t *testing.T) {
	k := keySchemaVersion()
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"meta", "schema_version"}, segs)
}

func TestKeyBackendID(t *testing.T) {
	k := keyBackendID()
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"meta", "backend_id"}, segs)
}

func TestKeyMigration(t *testing.T) {
	k := keyMigration("aof_to_badger")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"meta", "migration", "aof_to_badger"}, segs)
}

func TestKeyMutationByStable(t *testing.T) {
	k := keyMutationByStable("mut-1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"mutation", "mut-1"}, segs)
}

func TestKeyBatchByID(t *testing.T) {
	k := keyBatchByID("batch-1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"batch", "batch-1"}, segs)
}

// ────────────────────────────────────────────────────────────────────
// History keys (suffixed with timestamp + sequence)
// ────────────────────────────────────────────────────────────────────

func TestKeyNodeHistory(t *testing.T) {
	k := keyNodeHistory("n1", 1000, 1)

	// The key should start with history_node/n1 and have binary suffix.
	prefix := keyPrefix("history_node", "n1")
	require.True(t, bytes.HasPrefix(k, prefix),
		"history key should start with history_node/n1")
	require.Greater(t, len(k), len(prefix),
		"history key should have timestamp/seq suffix")

	// Verify the suffix is deterministic.
	k2 := keyNodeHistory("n1", 1000, 1)
	require.True(t, bytes.Equal(k, k2), "history key must be deterministic")

	// Different seq produces different key.
	k3 := keyNodeHistory("n1", 1000, 2)
	require.False(t, bytes.Equal(k, k3), "different seq must differ")
}

func TestKeyEdgeHistory(t *testing.T) {
	k := keyEdgeHistory("src", "calls", "tgt", 2000, 5)

	prefix := keyPrefix("history_edge", "src", "calls", "tgt")
	require.True(t, bytes.HasPrefix(k, prefix))
	require.Greater(t, len(k), len(prefix))

	// Deterministic
	k2 := keyEdgeHistory("src", "calls", "tgt", 2000, 5)
	require.True(t, bytes.Equal(k, k2))

	// Different timestamp
	k3 := keyEdgeHistory("src", "calls", "tgt", 2001, 5)
	require.False(t, bytes.Equal(k, k3))
}

// ────────────────────────────────────────────────────────────────────
// Unusual / adversarial inputs
// ────────────────────────────────────────────────────────────────────

func TestEncodeLabelWithSlash(t *testing.T) {
	label := "media:image/png"
	k := keyNodeByLabel(label, "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_label", label, "n1"}, segs)
}

func TestEncodeLabelWithUnicode(t *testing.T) {
	label := "標籤:測試"
	k := keyNodeByLabel(label, "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_label", label, "n1"}, segs)
}

func TestEncodeNodeIDWithSpecialChars(t *testing.T) {
	id := "file:sha256:abc123==?query"
	k := keyNodeByID(id)
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node", id}, segs)
}

func TestEncodeVeryLongPath(t *testing.T) {
	path := "/" + string(bytes.Repeat([]byte("a"), 2048)) + "/main.go"
	k := keyNodeByPath(path, "n1")
	segs, err := decodeKey(k)
	require.NoError(t, err)
	require.Equal(t, []string{"node_path", path, "n1"}, segs)
}

func TestEncodeMaxIntTimestamp(t *testing.T) {
	k := keyNodeHistory("n1", math.MaxInt64, math.MaxUint64)
	prefix := keyPrefix("history_node", "n1")
	require.True(t, bytes.HasPrefix(k, prefix))
}

func TestKeyPrefix_MatchesNodeKeyNotNodeKind(t *testing.T) {
	p := keyPrefix("node")
	require.True(t, bytes.HasPrefix(keyNodeByID("x"), p))
	require.False(t, bytes.HasPrefix(keyNodeByKind("f", "x"), p))
}

func TestKeyPrefix_EdgeOutScansOnlyOutgoing(t *testing.T) {
	source := "my-node"
	target := "tgt"
	kind := EdgeKind("calls")

	outPrefix := keyPrefix("edge_out", source)
	inPrefix := keyPrefix("edge_in", target)

	outKey := keyEdgeOut(source, kind, target)
	inKey := keyEdgeIn(target, kind, source)

	require.True(t, bytes.HasPrefix(outKey, outPrefix),
		"outKey should have edge_out/source prefix")
	require.False(t, bytes.HasPrefix(inKey, outPrefix),
		"inKey should NOT have edge_out/source prefix")
	require.False(t, bytes.HasPrefix(outKey, inPrefix),
		"outKey should NOT have edge_in/target prefix")
	require.True(t, bytes.HasPrefix(inKey, inPrefix),
		"inKey should have edge_in/target prefix")
}
