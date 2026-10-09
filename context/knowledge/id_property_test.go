package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// randomValue builds a nested value whose maps and slices vary in size and
// shape. Depth bounds recursion so the generator terminates.
func randomValue(rng *rand.Rand, depth int) any {
	switch rng.Intn(7) {
	case 0:
		return nil
	case 1:
		return rng.Intn(1000)
	case 2:
		return rng.NormFloat64()
	case 3:
		return rng.Intn(2) == 0
	case 4:
		return fmt.Sprintf("s-%d", rng.Intn(1000))
	case 5:
		if depth <= 0 {
			return []any{rng.Intn(10)}
		}
		n := rng.Intn(4)
		arr := make([]any, n)
		for i := range arr {
			arr[i] = randomValue(rng, depth-1)
		}
		return arr
	default:
		if depth <= 0 {
			return map[string]any{"leaf": rng.Intn(10)}
		}
		n := rng.Intn(4)
		m := make(map[string]any, n)
		for i := 0; i < n; i++ {
			m[fmt.Sprintf("k%d_%d", i, rng.Intn(5))] = randomValue(rng, depth-1)
		}
		return m
	}
}

// shuffleRebuild reconstructs a value, re-inserting every map's keys in a
// shuffled order. Deterministic encoders must be blind to map iteration order.
func shuffleRebuild(rng *rand.Rand, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		out := make(map[string]any, len(typed))
		for _, key := range keys {
			out[key] = shuffleRebuild(rng, typed[key])
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = shuffleRebuild(rng, typed[i])
		}
		return out
	default:
		return value
	}
}

// TestCanonicalJSONAndIdentityDeterminism is the NFR-5 property test: for ten
// thousand random nested values, canonical encoding and the derived chunk ID
// are stable under map-iteration-order changes.
func TestCanonicalJSONAndIdentityDeterminism(t *testing.T) {
	rng := rand.New(rand.NewSource(20261008))
	for i := 0; i < 10000; i++ {
		value := randomValue(rng, 3)
		rebuilt := shuffleRebuild(rng, value)

		first, err := CanonicalJSON(value)
		require.NoError(t, err)
		second, err := CanonicalJSON(rebuilt)
		require.NoError(t, err)
		require.Equal(t, first, second, "canonical encoding drifted on iteration %d", i)

		idFirst := CanonicalChunkID(ChunkKindCapture, first)
		idSecond := CanonicalChunkID(ChunkKindCapture, second)
		require.Equal(t, idFirst, idSecond, "chunk ID drifted on iteration %d", i)
	}
}

// TestCanonicalJSONNormalizesStructAndMap proves the encoder normalizes
// representations: a struct and an equal map hash to the same bytes.
func TestCanonicalJSONNormalizesStructAndMap(t *testing.T) {
	type payload struct {
		Alpha int      `json:"alpha"`
		Beta  []string `json:"beta"`
	}
	fromStruct, err := CanonicalJSON(payload{Alpha: 7, Beta: []string{"x", "y"}})
	require.NoError(t, err)
	fromMap, err := CanonicalJSON(map[string]any{"beta": []any{"x", "y"}, "alpha": 7})
	require.NoError(t, err)
	require.Equal(t, fromStruct, fromMap)
}

// TestCanonicalChunkIDStableAcrossRuns pins one hundred IDs so a future change
// to the encoding is caught as an explicit, reviewable break.
func TestCanonicalChunkIDStableAcrossRuns(t *testing.T) {
	expected := []ChunkID{
		"chunk:capture:3a856ed058e9955c",
		"chunk:capture:61834701c25ceac9",
		"chunk:capture:e05618aa333f1d68",
		"chunk:capture:a5132d4bbc10fa90",
		"chunk:capture:f9f4419f38e3f39d",
		"chunk:capture:8ba6302364b6fa8d",
		"chunk:capture:625f89cca9c6ba19",
		"chunk:capture:cb9da68c2a41c549",
		"chunk:capture:dd05c9820096a96d",
		"chunk:capture:863e65a01a8132d3",
		"chunk:capture:fac4021334970e6b",
		"chunk:capture:27b9927230ae893c",
		"chunk:capture:ec9311615466ff47",
		"chunk:capture:c2c6eedf1a74a8b3",
		"chunk:capture:0ea619f43cde5fa0",
		"chunk:capture:6aff0322a0d6d0f4",
		"chunk:capture:a4f44985adae7e31",
		"chunk:capture:b6bfd4338f22f5f2",
		"chunk:capture:6afdb0a4f87b1868",
		"chunk:capture:e09dfe913a7a0e0c",
		"chunk:capture:1e591a029ac95253",
		"chunk:capture:bd2e3d3c8c85e257",
		"chunk:capture:4f49c4b109fbfc10",
		"chunk:capture:68e7d9efa1038e77",
		"chunk:capture:03309108436aefc3",
		"chunk:capture:75bb6d23d8acd9bf",
		"chunk:capture:d9bc246570223b43",
		"chunk:capture:eaf1d641f5e0d384",
		"chunk:capture:4d820601332a27f2",
		"chunk:capture:a0caa7a3a5e718b1",
		"chunk:capture:b184d9988b7f5f64",
		"chunk:capture:29909cd7145ae9d6",
		"chunk:capture:c7682c61138d7161",
		"chunk:capture:0a173cb79af9b3f5",
		"chunk:capture:19805c9632f2d210",
		"chunk:capture:e3eea8d0dfcebf1f",
		"chunk:capture:e4b32df5f83e8966",
		"chunk:capture:8e01fab1ea19f2a7",
		"chunk:capture:317703fcaef4ec78",
		"chunk:capture:756853ef753a880f",
		"chunk:capture:2598b2dc8818240c",
		"chunk:capture:23a560f29b86484d",
		"chunk:capture:9d98dfb8a60e24cc",
		"chunk:capture:178d0d4e6eda733f",
		"chunk:capture:ca50a05db1858ce3",
		"chunk:capture:23b9551a7fa53790",
		"chunk:capture:509e25e2bdbcdb9a",
		"chunk:capture:4df8841702b9e3b5",
		"chunk:capture:c069f29c91dc11c0",
		"chunk:capture:e000a4daf26e5fb0",
		"chunk:capture:7bec3fe566629bf0",
		"chunk:capture:60185f737187dc29",
		"chunk:capture:6b1734a492d8867a",
		"chunk:capture:d945a6f0180d87cf",
		"chunk:capture:9dd1b6f2a64e8711",
		"chunk:capture:ef7d12fc359d2883",
		"chunk:capture:de064b95c171fe7b",
		"chunk:capture:001445508b5a2c47",
		"chunk:capture:c6be862d88878d3a",
		"chunk:capture:f16e7750c2f5c463",
		"chunk:capture:94c88da0ac2554e1",
		"chunk:capture:4a19506f34af736f",
		"chunk:capture:3519e9c459a814be",
		"chunk:capture:c4b1b73fe0acc6ed",
		"chunk:capture:a19ee574a70bafca",
		"chunk:capture:07c84291a96ab973",
		"chunk:capture:841389d7c655bce5",
		"chunk:capture:d223dedb8b20bba9",
		"chunk:capture:2ca09698aadd3a35",
		"chunk:capture:ca3e618100d8260f",
		"chunk:capture:9f7d3e9f8441ebb2",
		"chunk:capture:112c398f35610e75",
		"chunk:capture:60a523160cb4e5da",
		"chunk:capture:0bfeefc77ef41119",
		"chunk:capture:59728cc62cffeb8c",
		"chunk:capture:8d2b6b343b6efcdd",
		"chunk:capture:291c7f9adef88d5a",
		"chunk:capture:4a31902a78ec959a",
		"chunk:capture:89102552c2a5fbcb",
		"chunk:capture:34b3cb3d56bc1d76",
		"chunk:capture:d4fa28be56a7d47a",
		"chunk:capture:5d9874f92594ef99",
		"chunk:capture:72ba9f5d77eda5a2",
		"chunk:capture:22146c12c691f549",
		"chunk:capture:6499757dfac3e90c",
		"chunk:capture:760eaf5e900568ff",
		"chunk:capture:c02ed02d253fe47f",
		"chunk:capture:fac01c791256aa33",
		"chunk:capture:19387c3240102eca",
		"chunk:capture:076eb287586b0307",
		"chunk:capture:b577974a3d891634",
		"chunk:capture:ca82cc4434ae415f",
		"chunk:capture:4603ca01519d2d35",
		"chunk:capture:0ba867d1aa276422",
		"chunk:capture:f90a9e2917d0a56f",
		"chunk:capture:4b7a509560e98a3d",
		"chunk:capture:d3b74e699997732a",
		"chunk:capture:aaa14097a6f35804",
		"chunk:capture:12388d4e9f3bf144",
		"chunk:capture:ba0eca0b95995934",
	}
	require.Len(t, expected, 100)
	for i, want := range expected {
		got := CanonicalChunkID(ChunkKindCapture, []byte(fmt.Sprintf("capture-value-%03d", i)))
		require.Equal(t, want, got, "iteration %d", i)
	}
}

func TestChunkKindValid(t *testing.T) {
	for _, kind := range []ChunkKind{
		ChunkKindCapture, ChunkKindTool, ChunkKindObservation,
		ChunkKindLLM, ChunkKindFile, ChunkKindDerivation,
	} {
		require.True(t, kind.Valid(), "%q must be valid", kind)
	}
	require.False(t, ChunkKind("llm_response").Valid())
	require.False(t, ChunkKind("").Valid())
}

func TestCanonicalChunkIDFromHashTruncatesConsistently(t *testing.T) {
	content := []byte("canonical-content")
	sum := sha256.Sum256(content)
	direct := CanonicalChunkID(ChunkKindTool, content)
	fromFull := CanonicalChunkIDFromHash(ChunkKindTool, hex.EncodeToString(sum[:]))
	require.Equal(t, direct, fromFull)
}
